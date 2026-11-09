// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

// This file holds the in-process Speedle Cloud fixture used by the end-to-end
// distribution suite. It implements the /policy-source/v1 data and channel
// contract and the /authz-check/v1 fallback endpoint, signs with the fixed
// test-only Ed25519 key from pkg/adsync/testdata/golden-vectors.json, and can
// inject the failures the suite has to survive: a disconnected source, a
// revoked credential, a slow service and a trimmed change log.

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"

	// Register the file store used for the fixture's own evaluator.
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

const (
	e2eInstanceA = "01HF7YAT000000000000000A01"
	e2eInstanceB = "01HF7YAT000000000000000B01"
	e2eInstanceC = "01HF7YAT000000000000000C01"

	e2eServiceBooks  = "01HF7YAT000000000000000201"
	e2eServiceOrders = "01HF7YAT000000000000000202"

	e2eEpochBooks     = "01HF7YAT000000000000000301"
	e2eEpochOrders    = "01HF7YAT000000000000000302"
	e2eEpochBooksNext = "01HF7YAT000000000000000303"

	// Channel thresholds advertised by the fixture. They match the local
	// thresholds the suite configures, so neither side can make the other
	// slower than the design's floor.
	e2eHeartbeat = "15s"
	e2eStale     = "30s"
	e2eStop      = "60s"

	e2eChangeUpsert = "upsert"
	e2eChangeDelete = "delete"
	e2eKindAuthz    = "authz"
	e2eKindRole     = "role"
	e2eKindService  = "service"

	e2eAgentVersion = "e2e-suite"
)

// e2eCredential builds a well-formed instance credential for an instance id.
func e2eCredential(instanceID string, number int) string {
	return "adsi_" + instanceID + "_" + fmt.Sprintf("%043d", number)
}

// e2eSource is where an agent connects for policy data and the channel.
type e2eSource struct {
	URL        string
	Credential string
	CAFile     string
	Keys       *adsync.PublicKeySet
}

// e2eSigner signs fixture snapshots and commits with the golden test key.
type e2eSigner struct {
	keyID   string
	private ed25519.PrivateKey
}

func (s *e2eSigner) sign(message []byte) adsync.Signature {
	return adsync.Signature{
		KeyID: s.keyID,
		Alg:   "Ed25519",
		Value: base64.StdEncoding.EncodeToString(ed25519.Sign(s.private, message)),
	}
}

// e2eLoadSigner reads the fixed test-only key from the golden vectors. The
// vectors must never be edited; the same file is shared with Speedle Cloud.
func e2eLoadSigner(t *testing.T) (*e2eSigner, *adsync.PublicKeySet) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate the test source file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "adsync", "testdata", "golden-vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden vectors: %v", err)
	}
	var vectors struct {
		TestKey struct {
			KeyID                string `json:"keyId"`
			PrivateKeySeedBase64 string `json:"privateKeySeedBase64"`
			PublicKeyBase64      string `json:"publicKeyBase64"`
		} `json:"testKey"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("parse golden vectors: %v", err)
	}
	seed, err := base64.StdEncoding.DecodeString(vectors.TestKey.PrivateKeySeedBase64)
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatalf("golden private seed is invalid: %v", err)
	}
	private := ed25519.NewKeyFromSeed(seed)
	public, err := base64.StdEncoding.DecodeString(vectors.TestKey.PublicKeyBase64)
	if err != nil {
		t.Fatalf("decode golden public key: %v", err)
	}
	if !private.Public().(ed25519.PublicKey).Equal(ed25519.PublicKey(public)) {
		t.Fatal("golden private seed does not match its public key")
	}
	keys, err := adsync.NewPublicKeySet(map[string]ed25519.PublicKey{
		vectors.TestKey.KeyID: ed25519.PublicKey(public),
	})
	if err != nil {
		t.Fatalf("build golden public key set: %v", err)
	}
	return &e2eSigner{keyID: vectors.TestKey.KeyID, private: private}, keys
}

// e2eService is one service held by the fixture. Its fields are guarded by
// the owning fixture's mutex.
type e2eService struct {
	ID         string
	Name       string
	Epoch      string
	Revision   uint64
	State      adsync.Snapshot
	StateHash  adsync.Hash
	CommitHash adsync.Hash
	Commits    []adsync.Commit

	HistoryUnavailable bool
	// DataUnavailable makes snapshot and change requests fail with 503, so
	// synchronizing this service fails while every other service works.
	DataUnavailable bool
	// Hold makes snapshot and change requests wait until it is closed.
	Hold chan struct{}
}

func e2ePolicy(id, spdl string) adsync.Policy {
	return adsync.Policy{ID: id, Name: id, SPDL: spdl}
}

func e2eUpsertAuthz(id, spdl string) adsync.Change {
	name := id
	return adsync.Change{Op: e2eChangeUpsert, Kind: e2eKindAuthz, ID: id, Name: &name, SPDL: &spdl}
}

func e2eNewService(id, name, epoch string, policies, rolePolicies []adsync.Policy) *e2eService {
	if policies == nil {
		policies = []adsync.Policy{}
	}
	if rolePolicies == nil {
		rolePolicies = []adsync.Policy{}
	}
	state := adsync.Snapshot{Name: name, Policies: policies, RolePolicies: rolePolicies}
	hash := adsync.StateHash(state)
	return &e2eService{
		ID:         id,
		Name:       name,
		Epoch:      epoch,
		Revision:   1,
		State:      state,
		StateHash:  hash,
		CommitHash: hash,
	}
}

func e2eCloneSnapshot(snapshot adsync.Snapshot) adsync.Snapshot {
	cloned := snapshot
	cloned.Policies = append([]adsync.Policy(nil), snapshot.Policies...)
	cloned.RolePolicies = append([]adsync.Policy(nil), snapshot.RolePolicies...)
	return cloned
}

// e2eApplyChanges applies one commit independently of the engine's own
// implementation, so a shared bug cannot make both sides agree.
func e2eApplyChanges(state *adsync.Snapshot, changes []adsync.Change) error {
	for _, change := range changes {
		var target *[]adsync.Policy
		switch change.Kind {
		case e2eKindAuthz:
			target = &state.Policies
		case e2eKindRole:
			target = &state.RolePolicies
		case e2eKindService:
			if change.Op == e2eChangeDelete {
				return adsync.ErrSourceGone
			}
			if change.Name == nil {
				return fmt.Errorf("service change without a name")
			}
			state.Name = *change.Name
			continue
		default:
			return fmt.Errorf("unknown change kind %q", change.Kind)
		}
		switch change.Op {
		case e2eChangeDelete:
			kept := (*target)[:0]
			for _, policy := range *target {
				if policy.ID != change.ID {
					kept = append(kept, policy)
				}
			}
			*target = kept
		case e2eChangeUpsert:
			if change.Name == nil || change.SPDL == nil {
				return fmt.Errorf("upsert without a name or SPDL")
			}
			updated := adsync.Policy{ID: change.ID, Name: *change.Name, SPDL: *change.SPDL}
			replaced := false
			for index := range *target {
				if (*target)[index].ID == change.ID {
					(*target)[index] = updated
					replaced = true
					break
				}
			}
			if !replaced {
				*target = append(*target, updated)
			}
		default:
			return fmt.Errorf("unknown change op %q", change.Op)
		}
	}
	sort.Slice(state.Policies, func(left, right int) bool { return state.Policies[left].ID < state.Policies[right].ID })
	sort.Slice(state.RolePolicies, func(left, right int) bool { return state.RolePolicies[left].ID < state.RolePolicies[right].ID })
	return nil
}

// publish appends one signed revision. The caller must hold the fixture lock.
func (s *e2eService) publish(t *testing.T, signer *e2eSigner, changes ...adsync.Change) adsync.Commit {
	t.Helper()
	hash, err := adsync.CommitHash(s.CommitHash, changes)
	if err != nil {
		t.Fatalf("fixture commit hash: %v", err)
	}
	commit := adsync.Commit{
		Revision: s.Revision + 1,
		PrevHash: s.CommitHash,
		Hash:     hash,
		At:       1762500000000 + int64(s.Revision)*1000,
		Changes:  changes,
	}
	commit.Signature = signer.sign(adsync.CommitSignedBytes(s.ID, s.Epoch, commit.Revision, hash))
	next := e2eCloneSnapshot(s.State)
	if err := e2eApplyChanges(&next, changes); err != nil {
		t.Fatalf("fixture apply changes: %v", err)
	}
	s.State = next
	s.StateHash = adsync.StateHash(next)
	s.CommitHash = hash
	s.Revision = commit.Revision
	s.Commits = append(s.Commits, commit)
	return commit
}

// replaceEpoch replaces the dataset identity, as a new policy source would.
// The caller must hold the fixture lock.
func (s *e2eService) replaceEpoch(epoch string, policies, rolePolicies []adsync.Policy) {
	if policies == nil {
		policies = []adsync.Policy{}
	}
	if rolePolicies == nil {
		rolePolicies = []adsync.Policy{}
	}
	state := adsync.Snapshot{Name: s.Name, Policies: policies, RolePolicies: rolePolicies}
	hash := adsync.StateHash(state)
	s.Epoch = epoch
	s.Revision = 1
	s.State = state
	s.StateHash = hash
	s.CommitHash = hash
	s.Commits = nil
	s.HistoryUnavailable = false
}

// e2eWireCommand is one management command waiting to be confirmed.
type e2eWireCommand struct {
	ID        string          `json:"id"`
	Action    string          `json:"action"`
	ServiceID string          `json:"serviceId,omitempty"`
	Params    *e2eCommandArgs `json:"params,omitempty"`
	ExpiresAt int64           `json:"expiresAt"`
}

type e2eCommandArgs struct {
	URL               string `json:"url"`
	CredentialProfile string `json:"credentialProfile"`
}

type e2eRequest struct {
	Path          string
	Authorization string
}

// e2eChannelConn serializes writes to one downstream channel.
type e2eChannelConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *e2eChannelConn) write(payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = c.conn.WriteMessage(websocket.TextMessage, payload)
}

func (c *e2eChannelConn) close() {
	_ = c.conn.Close()
}

// e2eCloud is the in-process Speedle Cloud stand-in.
type e2eCloud struct {
	t          *testing.T
	signer     *e2eSigner
	keys       *adsync.PublicKeySet
	server     *httptest.Server
	caFile     string
	credential string
	client     *http.Client

	mu                  sync.Mutex
	fallbackCredentials map[string]struct{}
	services            map[string]*e2eService
	order               []string
	servicesVersion     int64
	disconnected        bool
	revoked             bool
	commands            []e2eWireCommand
	commandResults      map[string]string
	commandSeq          int
	conns               map[*e2eChannelConn]struct{}
	requests            []e2eRequest
	fallbackCalls       int
	generation          int64
	evaluator           *e2eEvaluatorEntry

	upgrader websocket.Upgrader
	closeOne sync.Once
}

// e2eEvaluatorEntry is the cached fallback evaluator for one generation of the
// fixture state. Requests borrow it for the whole evaluation; a rebuild retires
// the entry instead of closing it so an evaluator is never closed under a
// request that is still using it. Every field is guarded by e2eCloud.mu.
type e2eEvaluatorEntry struct {
	evaluator  adsapi.PolicyEvaluator
	generation int64
	users      int
	retired    bool
	closed     bool
}

func e2eBooksPolicies() []adsync.Policy {
	return []adsync.Policy{
		e2ePolicy("p-books-alice-read", "grant user alice read /books/1"),
		e2ePolicy("p-books-alice-delete", "grant user alice delete /books/1"),
		e2ePolicy("p-books-alice-deny", "deny user alice delete /books/1 if department == 'finance'"),
		e2ePolicy("p-books-reviewer-read", "grant role reviewer read /reviews/1"),
		e2ePolicy("p-books-hero-read", "grant user hero read /profile if request_user == 'mallory'"),
		e2ePolicy("p-books-archive-read", "grant user alice read /archive if request_time < '2000-01-01T00:00:00Z'"),
		e2ePolicy("p-books-bob-read", "grant user bob read /books/1 if level >= 3"),
		e2ePolicy("p-books-staff-read", "grant group staff read /books/1"),
	}
}

func e2eBooksRolePolicies() []adsync.Policy {
	return []adsync.Policy{
		e2ePolicy("r-books-alice-reviewer", "grant user alice role reviewer"),
	}
}

func e2eOrdersPolicies() []adsync.Policy {
	return []adsync.Policy{
		e2ePolicy("p-orders-carol-approve", "grant user carol approve /orders/1"),
		e2ePolicy("p-orders-ops-approve", "grant group ops approve /orders/1"),
	}
}

// newE2ECloud starts a fixture with the books and orders services.
func newE2ECloud(t *testing.T) *e2eCloud {
	t.Helper()
	signer, keys := e2eLoadSigner(t)
	cloud := &e2eCloud{
		t:              t,
		signer:         signer,
		keys:           keys,
		credential:     e2eCredential(e2eInstanceA, 1),
		services:       make(map[string]*e2eService),
		commandResults: make(map[string]string),
		conns:          make(map[*e2eChannelConn]struct{}),
		fallbackCredentials: map[string]struct{}{
			e2eCredential(e2eInstanceA, 1): {},
		},
		servicesVersion: 1,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(*http.Request) bool { return true },
		},
	}
	cloud.addServiceLocked(e2eNewService(e2eServiceBooks, "books", e2eEpochBooks, e2eBooksPolicies(), e2eBooksRolePolicies()))
	cloud.addServiceLocked(e2eNewService(e2eServiceOrders, "orders", e2eEpochOrders, e2eOrdersPolicies(), nil))

	cloud.server = httptest.NewUnstartedServer(cloud)
	cloud.server.Config.ErrorLog = e2eDiscardLogger()
	cloud.server.StartTLS()
	cloud.caFile = e2eWriteCAFile(t, cloud.server.Certificate())
	cloud.client = cloud.server.Client()
	cloud.client.Timeout = 5 * time.Second
	t.Cleanup(cloud.close)
	return cloud
}

func e2eDiscardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func e2eWriteCAFile(t *testing.T, certificate *x509.Certificate) string {
	t.Helper()
	if certificate == nil {
		t.Fatal("test server has no certificate")
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pemBytes, 0o644); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	return path
}

func (f *e2eCloud) source() e2eSource {
	return e2eSource{
		URL:        f.server.URL,
		Credential: f.credential,
		CAFile:     f.caFile,
		Keys:       f.keys,
	}
}

// fallbackSource returns a Cloud-only credential that is valid for fallback
// authorization, never for policy synchronization. A downstream proxy token is
// deliberately not accepted here.
func (f *e2eCloud) fallbackSource(instanceID string, number int) e2eSource {
	credential := e2eCredential(instanceID, number)
	f.mu.Lock()
	f.fallbackCredentials[credential] = struct{}{}
	f.mu.Unlock()
	return e2eSource{
		URL:        f.server.URL,
		Credential: credential,
		CAFile:     f.caFile,
		Keys:       f.keys,
	}
}

func (f *e2eCloud) addServiceLocked(service *e2eService) {
	if _, exists := f.services[service.ID]; !exists {
		f.order = append(f.order, service.ID)
	}
	f.services[service.ID] = service
	f.generation++
}

func (f *e2eCloud) close() {
	f.closeOne.Do(func() {
		f.mu.Lock()
		conns := make([]*e2eChannelConn, 0, len(f.conns))
		for conn := range f.conns {
			conns = append(conns, conn)
		}
		f.conns = make(map[*e2eChannelConn]struct{})
		f.mu.Unlock()
		for _, conn := range conns {
			conn.close()
		}
		// The server waits for outstanding requests, so every borrow is
		// returned before the cached evaluator is closed.
		f.server.Close()
		f.mu.Lock()
		entry := f.evaluator
		f.evaluator = nil
		if entry != nil {
			entry.retired = true
		}
		closeEntry := f.closeEvaluatorLocked(entry)
		f.mu.Unlock()
		if closeEntry {
			e2eCloseEvaluator(entry.evaluator)
		}
	})
}

// closeEvaluatorLocked marks a retired entry closed once its last borrower has
// released it and reports whether the caller must close the evaluator. The
// caller runs the close after unlocking.
func (f *e2eCloud) closeEvaluatorLocked(entry *e2eEvaluatorEntry) bool {
	if entry == nil || !entry.retired || entry.users > 0 || entry.closed {
		return false
	}
	entry.closed = true
	return true
}

func e2eCloseEvaluator(evaluator adsapi.PolicyEvaluator) {
	if evaluator == nil {
		return
	}
	if closer, ok := evaluator.(io.Closer); ok {
		_ = closer.Close()
	}
}

// --- Fixture controls used by the scenarios ---

func (f *e2eCloud) disconnect() {
	f.mu.Lock()
	f.disconnected = true
	conns := f.connListLocked()
	f.mu.Unlock()
	for _, conn := range conns {
		conn.close()
	}
}

func (f *e2eCloud) reconnect() {
	f.mu.Lock()
	f.disconnected = false
	f.mu.Unlock()
}

func (f *e2eCloud) revoke() {
	f.mu.Lock()
	f.revoked = true
	conns := f.connListLocked()
	f.mu.Unlock()
	for _, conn := range conns {
		conn.close()
	}
}

func (f *e2eCloud) unrevoke() {
	f.mu.Lock()
	f.revoked = false
	f.mu.Unlock()
}

func (f *e2eCloud) hold(serviceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if service := f.services[serviceID]; service != nil {
		service.Hold = make(chan struct{})
	}
}

func (f *e2eCloud) release(serviceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if service := f.services[serviceID]; service != nil && service.Hold != nil {
		close(service.Hold)
		service.Hold = nil
	}
}

func (f *e2eCloud) setHistoryUnavailable(serviceID string, unavailable bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if service := f.services[serviceID]; service != nil {
		service.HistoryUnavailable = unavailable
	}
}

func (f *e2eCloud) setDataUnavailable(serviceID string, unavailable bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if service := f.services[serviceID]; service != nil {
		service.DataUnavailable = unavailable
	}
}

// publish appends one revision and returns the new head.
func (f *e2eCloud) publish(serviceID string, changes ...adsync.Change) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	service := f.services[serviceID]
	if service == nil {
		f.t.Fatalf("fixture has no service %q", serviceID)
	}
	service.publish(f.t, f.signer, changes...)
	f.generation++
	return service.Revision
}

func (f *e2eCloud) replaceEpoch(serviceID, epoch string, policies, rolePolicies []adsync.Policy) {
	f.mu.Lock()
	defer f.mu.Unlock()
	service := f.services[serviceID]
	if service == nil {
		f.t.Fatalf("fixture has no service %q", serviceID)
	}
	service.replaceEpoch(epoch, policies, rolePolicies)
	f.generation++
}

func (f *e2eCloud) addService(service *e2eService) {
	f.mu.Lock()
	f.addServiceLocked(service)
	f.servicesVersion++
	f.mu.Unlock()
}

func (f *e2eCloud) removeService(serviceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.services[serviceID]; !exists {
		return
	}
	delete(f.services, serviceID)
	kept := f.order[:0]
	for _, id := range f.order {
		if id != serviceID {
			kept = append(kept, id)
		}
	}
	f.order = kept
	f.servicesVersion++
	f.generation++
}

// service returns the fixture's service object so a test can re-add it after
// removing it from the set. The object is owned by the fixture; callers only
// pass it back to addService.
func (f *e2eCloud) service(serviceID string) *e2eService {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.services[serviceID]
}

func (f *e2eCloud) serviceEpoch(serviceID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if service := f.services[serviceID]; service != nil {
		return service.Epoch
	}
	return ""
}

// --- Channel messages and acks ---

func (f *e2eCloud) connListLocked() []*e2eChannelConn {
	conns := make([]*e2eChannelConn, 0, len(f.conns))
	for conn := range f.conns {
		conns = append(conns, conn)
	}
	return conns
}

func (f *e2eCloud) ackLocked() []byte {
	type ackService struct {
		ID            string      `json:"id"`
		Name          string      `json:"name"`
		Head          uint64      `json:"head"`
		OriginHead    uint64      `json:"originHead"`
		Epoch         string      `json:"epoch"`
		HeadStateHash adsync.Hash `json:"headStateHash"`
	}
	type ack struct {
		Type              string           `json:"type"`
		ServicesVersion   int64            `json:"servicesVersion"`
		Services          []ackService     `json:"services"`
		ServerTime        int64            `json:"serverTime"`
		HeartbeatInterval string           `json:"heartbeatInterval"`
		StaleAfter        string           `json:"staleAfter"`
		StopAfter         string           `json:"stopAfter"`
		Commands          []e2eWireCommand `json:"commands"`
	}
	message := ack{
		Type:              "ack",
		ServicesVersion:   f.servicesVersion,
		Services:          []ackService{},
		ServerTime:        time.Now().UnixMilli(),
		HeartbeatInterval: e2eHeartbeat,
		StaleAfter:        e2eStale,
		StopAfter:         e2eStop,
		Commands:          append([]e2eWireCommand(nil), f.commands...),
	}
	for _, id := range f.order {
		service := f.services[id]
		if service == nil {
			continue
		}
		message.Services = append(message.Services, ackService{
			ID:            service.ID,
			Name:          service.Name,
			Head:          service.Revision,
			OriginHead:    service.Revision,
			Epoch:         service.Epoch,
			HeadStateHash: service.StateHash,
		})
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return nil
	}
	return encoded
}

func (f *e2eCloud) pushAck() {
	f.mu.Lock()
	payload := f.ackLocked()
	conns := f.connListLocked()
	f.mu.Unlock()
	for _, conn := range conns {
		conn.write(payload)
	}
}

func (f *e2eCloud) notify(serviceID string, head uint64) {
	payload, err := json.Marshal(struct {
		Type      string `json:"type"`
		ServiceID string `json:"serviceId"`
		Head      uint64 `json:"head"`
	}{Type: "notify", ServiceID: serviceID, Head: head})
	if err != nil {
		return
	}
	f.mu.Lock()
	conns := f.connListLocked()
	f.mu.Unlock()
	for _, conn := range conns {
		conn.write(payload)
	}
}

// sendCommand queues a command and pushes an ack so a connected instance sees
// it immediately. The id is a valid ULID built from a per-fixture counter.
func (f *e2eCloud) sendCommand(action, serviceID string, params *e2eCommandArgs) string {
	f.mu.Lock()
	f.commandSeq++
	id := fmt.Sprintf("01HF7YAT000000000000000%03d", f.commandSeq)
	f.commands = append(f.commands, e2eWireCommand{
		ID:        id,
		Action:    action,
		ServiceID: serviceID,
		Params:    params,
		ExpiresAt: time.Now().Add(5 * time.Minute).UnixMilli(),
	})
	f.mu.Unlock()
	f.pushAck()
	return id
}

func (f *e2eCloud) recordCommandResult(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commandResults[id] = status
	kept := f.commands[:0]
	for _, command := range f.commands {
		if command.ID != id {
			kept = append(kept, command)
		}
	}
	f.commands = kept
}

func (f *e2eCloud) commandResult(id string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status, found := f.commandResults[id]
	return status, found
}

func (f *e2eCloud) fallbackCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fallbackCalls
}

func (f *e2eCloud) countRequests(suffix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, request := range f.requests {
		if strings.HasSuffix(request.Path, suffix) {
			count++
		}
	}
	return count
}

// isAllowed asks the fixture's Cloud authorization endpoint directly, exactly
// as a forwarded fallback request would arrive.
func (f *e2eCloud) isAllowed(t *testing.T, body []byte) adsrest.IsAllowedResponse {
	t.Helper()
	status, _, payload := f.post(t, adsync.AuthzCheckAPIPrefix+"is-allowed", body)
	if status != http.StatusOK {
		t.Fatalf("Cloud authorization endpoint: status %d body %s", status, payload)
	}
	var response adsrest.IsAllowedResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode Cloud authorization response: %v", err)
	}
	return response
}

func (f *e2eCloud) post(t *testing.T, path string, body []byte) (int, http.Header, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, f.server.URL+path, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("build Cloud request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+f.credential)
	request.Header.Set("Content-Type", "application/json")
	response, err := f.client.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatalf("read Cloud response: %v", err)
	}
	return response.StatusCode, response.Header.Clone(), payload
}

func (f *e2eCloud) countRequestsWithCredential(credential string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	expected := "Bearer " + credential
	count := 0
	for _, request := range f.requests {
		if request.Authorization == expected {
			count++
		}
	}
	return count
}

// --- HTTP surface ---

func (f *e2eCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, e2eRequest{
		Path:          r.URL.Path,
		Authorization: r.Header.Get("Authorization"),
	})
	revoked := f.revoked
	disconnected := f.disconnected
	credential := f.credential
	f.mu.Unlock()

	if revoked {
		e2eWriteCloudError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	if disconnected {
		e2eWriteCloudError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	authorization := r.Header.Get("Authorization")
	isFallback := r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, adsync.AuthzCheckAPIPrefix)
	if isFallback {
		f.mu.Lock()
		_, allowed := f.fallbackCredentials[strings.TrimPrefix(authorization, "Bearer ")]
		f.mu.Unlock()
		if !allowed {
			e2eWriteCloudError(w, http.StatusUnauthorized, "unauthorized", "")
			return
		}
	} else if authorization != "Bearer "+credential {
		e2eWriteCloudError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	switch {
	case isFallback:
		f.serveFallback(w, r)
	case r.Method == http.MethodGet && r.URL.Path == adsync.APIPrefix+"/services":
		f.serveServices(w)
	case r.Method == http.MethodGet && r.URL.Path == adsync.APIPrefix+"/channel":
		f.serveChannel(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, adsync.APIPrefix+"/services/"):
		rest := strings.TrimPrefix(r.URL.Path, adsync.APIPrefix+"/services/")
		switch {
		case strings.HasSuffix(rest, "/snapshot"):
			f.serveSnapshot(w, r, strings.TrimSuffix(rest, "/snapshot"))
		case strings.HasSuffix(rest, "/changes"):
			f.serveChanges(w, r, strings.TrimSuffix(rest, "/changes"))
		default:
			e2eWriteCloudError(w, http.StatusNotFound, "not_found", "")
		}
	default:
		e2eWriteCloudError(w, http.StatusNotFound, "not_found", "")
	}
}

func (f *e2eCloud) serveServices(w http.ResponseWriter) {
	f.mu.Lock()
	response := adsync.ServicesResponse{ServicesVersion: f.servicesVersion}
	for _, id := range f.order {
		service := f.services[id]
		if service == nil {
			continue
		}
		response.Services = append(response.Services, adsync.Service{
			ID:            service.ID,
			Name:          service.Name,
			Revision:      service.Revision,
			Epoch:         service.Epoch,
			HeadStateHash: service.StateHash,
		})
	}
	f.mu.Unlock()
	e2eWriteCloudJSON(w, http.StatusOK, response)
}

func (f *e2eCloud) serveSnapshot(w http.ResponseWriter, r *http.Request, serviceID string) {
	f.mu.Lock()
	service := f.services[serviceID]
	if service == nil {
		f.mu.Unlock()
		e2eWriteCloudError(w, http.StatusNotFound, "service_not_found", "")
		return
	}
	if service.DataUnavailable {
		f.mu.Unlock()
		e2eWriteCloudError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	signature := f.signer.sign(adsync.SnapshotSignedBytes(
		service.ID, service.Epoch, service.Revision, service.StateHash, service.CommitHash,
	))
	response := adsync.SnapshotResponse{
		ServiceID:  service.ID,
		SourceID:   service.ID,
		Epoch:      service.Epoch,
		Revision:   service.Revision,
		StateHash:  service.StateHash,
		CommitHash: service.CommitHash,
		Signature:  signature,
		Snapshot:   e2eCloneSnapshot(service.State),
	}
	hold := service.Hold
	f.mu.Unlock()

	e2eWaitHold(r, hold)
	e2eWriteCloudJSON(w, http.StatusOK, response)
}

func (f *e2eCloud) serveChanges(w http.ResponseWriter, r *http.Request, serviceID string) {
	sinceText := r.URL.Query().Get("since")
	since, err := strconv.ParseUint(sinceText, 10, 64)
	if err != nil || strconv.FormatUint(since, 10) != sinceText {
		e2eWriteCloudError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	limit := 100
	if text := r.URL.Query().Get("limit"); text != "" {
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 1 || parsed > 100 {
			e2eWriteCloudError(w, http.StatusBadRequest, "invalid_request", "")
			return
		}
		limit = parsed
	}

	f.mu.Lock()
	service := f.services[serviceID]
	if service == nil {
		f.mu.Unlock()
		e2eWriteCloudError(w, http.StatusNotFound, "service_not_found", "")
		return
	}
	if service.DataUnavailable {
		f.mu.Unlock()
		e2eWriteCloudError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	if service.HistoryUnavailable {
		f.mu.Unlock()
		e2eWriteCloudError(w, http.StatusGone, "snapshot_required", "history_unavailable")
		return
	}
	if since > service.Revision {
		f.mu.Unlock()
		e2eWriteCloudError(w, http.StatusGone, "snapshot_required", "epoch_mismatch")
		return
	}
	commits := make([]adsync.Commit, 0, limit)
	for _, commit := range service.Commits {
		if commit.Revision <= since {
			continue
		}
		commits = append(commits, commit)
		if len(commits) == limit {
			break
		}
	}
	to := since
	if len(commits) > 0 {
		to = commits[len(commits)-1].Revision
	}
	response := adsync.ChangesResponse{
		ServiceID:     service.ID,
		SourceID:      service.ID,
		Epoch:         service.Epoch,
		From:          since + 1,
		To:            to,
		Head:          service.Revision,
		HeadStateHash: service.StateHash,
		Commits:       commits,
		NextSince:     to,
	}
	hold := service.Hold
	f.mu.Unlock()

	e2eWaitHold(r, hold)
	e2eWriteCloudJSON(w, http.StatusOK, response)
}

func e2eWaitHold(r *http.Request, hold chan struct{}) {
	if hold == nil {
		return
	}
	select {
	case <-hold:
	case <-r.Context().Done():
	}
}

// serveFallback evaluates a forwarded authorization request with the fixture's
// own evaluator, built from the current policy state through the file store.
func (f *e2eCloud) serveFallback(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		e2eWriteCloudError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	f.mu.Lock()
	f.fallbackCalls++
	f.mu.Unlock()

	evaluator, release, err := f.acquireEvaluator()
	if err != nil {
		e2eWriteCloudError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	defer release()
	var request adsrest.JsonContext
	if err := json.Unmarshal(body, &request); err != nil {
		e2eWriteCloudError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	context, err := adsrest.ConvertJSONRequestToContext(&request)
	if err != nil {
		e2eWriteCloudError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}

	operation := strings.TrimPrefix(r.URL.Path, adsync.AuthzCheckAPIPrefix)
	switch adsync.FallbackOperation(operation) {
	case adsync.FallbackIsAllowed:
		allowed, reason, evalErr := evaluator.IsAllowed(*context)
		response := adsrest.IsAllowedResponse{Allowed: allowed, Reason: int32(reason)}
		if evalErr != nil {
			response.ErrorMessage = evalErr.Error()
		}
		e2eWriteCloudJSON(w, http.StatusOK, &response)
	case adsync.FallbackAllGrantedRoles:
		roles, evalErr := evaluator.GetAllGrantedRoles(*context)
		if evalErr != nil {
			e2eWriteCloudError(w, http.StatusInternalServerError, "evaluation_error", "")
			return
		}
		if roles == nil {
			roles = []string{}
		}
		e2eWriteCloudJSON(w, http.StatusOK, roles)
	case adsync.FallbackAllGrantedPermissions:
		permissions, evalErr := evaluator.GetAllGrantedPermissions(*context)
		if evalErr != nil {
			e2eWriteCloudError(w, http.StatusInternalServerError, "evaluation_error", "")
			return
		}
		if permissions == nil {
			permissions = []pms.Permission{}
		}
		e2eWriteCloudJSON(w, http.StatusOK, permissions)
	default:
		e2eWriteCloudError(w, http.StatusNotFound, "not_found", "")
	}
}

// acquireEvaluator returns an evaluator over the current state together with a
// release function the caller must invoke when it is done evaluating. The
// evaluator is rebuilt when the state changed, but the replaced one stays open
// until every request that borrowed it has released it. A rebuild rechecks the
// generation before installing, so a build from an older state never replaces a
// newer evaluator.
func (f *e2eCloud) acquireEvaluator() (adsapi.PolicyEvaluator, func(), error) {
	for {
		f.mu.Lock()
		if entry := f.evaluator; entry != nil && entry.generation == f.generation {
			entry.users++
			f.mu.Unlock()
			return entry.evaluator, func() { f.releaseEvaluator(entry) }, nil
		}
		generation := f.generation
		text := f.spdlLocked()
		f.mu.Unlock()

		evaluator, err := f.buildEvaluator(text)
		if err != nil {
			return nil, nil, err
		}

		f.mu.Lock()
		if f.generation != generation {
			// The state moved while this evaluator was being built. Discard
			// it and build against the newer state instead of installing it.
			f.mu.Unlock()
			e2eCloseEvaluator(evaluator)
			continue
		}
		entry := &e2eEvaluatorEntry{evaluator: evaluator, generation: generation, users: 1}
		previous := f.evaluator
		f.evaluator = entry
		if previous != nil {
			previous.retired = true
		}
		closePrevious := f.closeEvaluatorLocked(previous)
		f.mu.Unlock()
		if closePrevious {
			e2eCloseEvaluator(previous.evaluator)
		}
		return evaluator, func() { f.releaseEvaluator(entry) }, nil
	}
}

// releaseEvaluator drops one borrow and closes the evaluator once it is retired
// and unused.
func (f *e2eCloud) releaseEvaluator(entry *e2eEvaluatorEntry) {
	f.mu.Lock()
	entry.users--
	closeEntry := f.closeEvaluatorLocked(entry)
	f.mu.Unlock()
	if closeEntry {
		e2eCloseEvaluator(entry.evaluator)
	}
}

// buildEvaluator parses the SPDL exactly as an ADS would, so the fixture does
// not reuse the engine's snapshot structures.
func (f *e2eCloud) buildEvaluator(text string) (adsapi.PolicyEvaluator, error) {
	path := filepath.Join(f.t.TempDir(), "cloud.spdl")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return nil, err
	}
	return eval.NewFromFile(path, false)
}

// e2eFallbackContext is the request the fallback evaluator answers in the
// lifetime tests: a granted read on the books service.
func e2eFallbackContext(t *testing.T) *adsapi.RequestContext {
	t.Helper()
	body := (e2eCheck{
		Service:    "books",
		Resource:   "/books/1",
		Action:     "read",
		Principals: e2eUser("alice"),
	}).body(t)
	var request adsrest.JsonContext
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("decode fallback context: %v", err)
	}
	context, err := adsrest.ConvertJSONRequestToContext(&request)
	if err != nil {
		t.Fatalf("convert fallback context: %v", err)
	}
	return context
}

func (f *e2eCloud) evaluatorEntry() *e2eEvaluatorEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.evaluator
}

// TestE2ECloudFallbackEvaluatorLifetime keeps a borrowed evaluator open across
// a state change and closes the replaced one only after its last borrow ends.
func TestE2ECloudFallbackEvaluatorLifetime(t *testing.T) {
	cloud := newE2ECloud(t)
	context := e2eFallbackContext(t)

	first, releaseFirst, err := cloud.acquireEvaluator()
	if err != nil {
		t.Fatalf("acquire the first evaluator: %v", err)
	}
	firstEntry := cloud.evaluatorEntry()
	if allowed, _, err := first.IsAllowed(*context); err != nil || !allowed {
		t.Fatalf("first evaluator before the change: allowed=%v err=%v", allowed, err)
	}

	cloud.publish(e2eServiceBooks, e2eUpsertAuthz("p-books-carol-read", "grant user carol read /books/2"))
	second, releaseSecond, err := cloud.acquireEvaluator()
	if err != nil {
		t.Fatalf("acquire the rebuilt evaluator: %v", err)
	}
	if secondEntry := cloud.evaluatorEntry(); secondEntry == firstEntry {
		t.Fatal("the state change did not rebuild the evaluator")
	}
	if allowed, _, err := second.IsAllowed(*context); err != nil || !allowed {
		t.Fatalf("rebuilt evaluator: allowed=%v err=%v", allowed, err)
	}
	cloud.mu.Lock()
	closedWhileBorrowed := firstEntry.closed
	cloud.mu.Unlock()
	if closedWhileBorrowed {
		t.Fatal("the replaced evaluator was closed while a request still borrowed it")
	}
	if allowed, _, err := first.IsAllowed(*context); err != nil || !allowed {
		t.Fatalf("borrowed evaluator after the rebuild: allowed=%v err=%v", allowed, err)
	}

	releaseFirst()
	cloud.mu.Lock()
	closedAfterRelease := firstEntry.closed
	cloud.mu.Unlock()
	if !closedAfterRelease {
		t.Fatal("the replaced evaluator stayed open after its last borrow was released")
	}
	releaseSecond()
}

// TestE2ECloudFallbackEvaluatorConcurrentRebuilds evaluates while the fixture
// state changes, so the race detector can see any evaluator that a rebuild
// closes or replaces under a request that is still evaluating with it.
func TestE2ECloudFallbackEvaluatorConcurrentRebuilds(t *testing.T) {
	cloud := newE2ECloud(t)

	// One request context per worker: evaluation may annotate the subject.
	contexts := make([]*adsapi.RequestContext, 4)
	for worker := range contexts {
		contexts[worker] = e2eFallbackContext(t)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range contexts {
		context := contexts[worker]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				evaluator, release, err := cloud.acquireEvaluator()
				if err != nil {
					t.Errorf("acquire evaluator: %v", err)
					return
				}
				allowed, _, err := evaluator.IsAllowed(*context)
				release()
				if err != nil {
					t.Errorf("evaluate with a borrowed evaluator: %v", err)
					return
				}
				if !allowed {
					t.Errorf("borrowed evaluator denied a granted request")
					return
				}
			}
		}()
	}
	for revision := 0; revision < 10; revision++ {
		cloud.publish(e2eServiceBooks, e2eUpsertAuthz(
			fmt.Sprintf("p-books-rebuild-%d", revision),
			"grant user alice read /books/1",
		))
	}
	close(stop)
	wg.Wait()
}

func (f *e2eCloud) spdlLocked() string {
	var builder strings.Builder
	for _, id := range f.order {
		service := f.services[id]
		if service == nil {
			continue
		}
		fmt.Fprintf(&builder, "[service.%s]\n[policy]\n", service.Name)
		for _, policy := range service.State.Policies {
			builder.WriteString(policy.SPDL)
			builder.WriteByte('\n')
		}
		builder.WriteString("[rolepolicy]\n")
		for _, policy := range service.State.RolePolicies {
			builder.WriteString(policy.SPDL)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

// --- Channel ---

func (f *e2eCloud) serveChannel(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	disconnected := f.disconnected
	revoked := f.revoked
	f.mu.Unlock()
	if revoked {
		e2eWriteCloudError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	if disconnected {
		e2eWriteCloudError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}

	conn, err := f.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	channelConn := &e2eChannelConn{conn: conn}
	f.mu.Lock()
	f.conns[channelConn] = struct{}{}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.conns, channelConn)
		f.mu.Unlock()
		conn.Close()
	}()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var hello struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&hello); err != nil || hello.Type != "hello" {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	f.mu.Lock()
	initial := f.ackLocked()
	f.mu.Unlock()
	channelConn.write(initial)

	for {
		var frame struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		switch frame.Type {
		case "heartbeat":
			f.mu.Lock()
			payload := f.ackLocked()
			f.mu.Unlock()
			channelConn.write(payload)
		case "command_result":
			f.recordCommandResult(frame.ID, frame.Status)
		case "hello":
			return
		default:
			return
		}
	}
}

// --- JSON helpers ---

func e2eWriteCloudJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func e2eWriteCloudError(w http.ResponseWriter, status int, code, reason string) {
	body := `{"error":"` + code + `"`
	if reason != "" {
		body += `,"reason":"` + reason + `"`
	}
	body += `}`
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
