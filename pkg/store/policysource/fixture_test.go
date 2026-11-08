// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	speedleerrors "github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
)

const (
	testInstanceID = "01HF7YAT000000000000000001"
	// 43 base62 characters.
	testCredential = "adsi_" + testInstanceID + "_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	testServiceID1 = "01HF7YAT000000000000000101"
	testServiceID2 = "01HF7YAT000000000000000102"
	testServiceID3 = "01HF7YAT000000000000000103"
)

// testSigner signs snapshots with a key generated for one test.
type testSigner struct {
	keyID   string
	private ed25519.PrivateKey
	public  ed25519.PublicKey
}

func newTestSigner(t *testing.T) *testSigner {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &testSigner{keyID: "test-key-1", private: private, public: public}
}

func (s *testSigner) sign(message []byte) adsync.Signature {
	return adsync.Signature{
		KeyID: s.keyID,
		Alg:   "Ed25519",
		Value: base64.StdEncoding.EncodeToString(ed25519.Sign(s.private, message)),
	}
}

func (s *testSigner) keySet(t *testing.T) *adsync.PublicKeySet {
	t.Helper()
	keys, err := adsync.NewPublicKeySet(map[string]ed25519.PublicKey{s.keyID: s.public})
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func (s *testSigner) writeKeysFile(t *testing.T) string {
	t.Helper()
	document := adsync.PublicKeyFile{Keys: []adsync.PublicKeyEntry{
		{KeyID: s.keyID, PublicKey: base64.StdEncoding.EncodeToString(s.public)},
	}}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "policy-keys.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// testService is one service held by the fixture source.
type testService struct {
	ID         string
	Name       string
	Epoch      string
	Revision   uint64
	Snapshot   adsync.Snapshot
	StateHash  adsync.Hash
	CommitHash adsync.Hash
}

func newTestService(id, name string) *testService {
	snapshot := adsync.Snapshot{Name: name, Policies: []adsync.Policy{}, RolePolicies: []adsync.Policy{}}
	hash := adsync.StateHash(snapshot)
	return &testService{
		ID:         id,
		Name:       name,
		Epoch:      id,
		Revision:   1,
		Snapshot:   snapshot,
		StateHash:  hash,
		CommitHash: hash,
	}
}

// publish replaces the service state and advances the revision.
func (s *testService) publish(mutate func(*adsync.Snapshot)) {
	snapshot := adsync.Snapshot{
		Name:         s.Snapshot.Name,
		Policies:     append([]adsync.Policy(nil), s.Snapshot.Policies...),
		RolePolicies: append([]adsync.Policy(nil), s.Snapshot.RolePolicies...),
	}
	mutate(&snapshot)
	s.Snapshot = snapshot
	s.Revision++
	s.StateHash = adsync.StateHash(snapshot)
	s.CommitHash = s.StateHash
}

func policy(id, name, spdl string) adsync.Policy {
	return adsync.Policy{ID: id, Name: name, SPDL: spdl}
}

// testSource is a test policy source. It serves discovery and signed
// snapshots. Incremental history is always reported unavailable, so every
// update is fetched as a fresh signed snapshot, which is a valid source
// behaviour when the change log has been trimmed.
type testSource struct {
	mu         sync.Mutex
	signer     *testSigner
	services   []*testService
	credential string

	requests int
	paths    []string
	failAll  bool
}

func newTestSource(signer *testSigner, services ...*testService) *testSource {
	return &testSource{signer: signer, services: services, credential: testCredential}
}

func (f *testSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests++
	f.paths = append(f.paths, r.URL.Path)
	failing := f.failAll
	f.mu.Unlock()
	if failing {
		writeJSONError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+f.credential {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == adsync.APIPrefix+"/services":
		f.serveServices(w)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, adsync.APIPrefix+"/services/"):
		rest := strings.TrimPrefix(r.URL.Path, adsync.APIPrefix+"/services/")
		switch {
		case strings.HasSuffix(rest, "/snapshot"):
			f.serveSnapshot(w, r, strings.TrimSuffix(rest, "/snapshot"))
		case strings.HasSuffix(rest, "/changes"):
			writeJSONError(w, http.StatusGone, "snapshot_required", "history_unavailable")
		default:
			writeJSONError(w, http.StatusNotFound, "service_not_found", "")
		}
	default:
		writeJSONError(w, http.StatusNotFound, "not_found", "")
	}
}

func (f *testSource) serveServices(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	response := adsync.ServicesResponse{ServicesVersion: 1}
	for _, service := range f.services {
		response.Services = append(response.Services, adsync.Service{
			ID:            service.ID,
			Name:          service.Name,
			Revision:      service.Revision,
			Epoch:         service.Epoch,
			HeadStateHash: service.StateHash,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (f *testSource) serveSnapshot(w http.ResponseWriter, r *http.Request, serviceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var service *testService
	for _, candidate := range f.services {
		if candidate.ID == serviceID {
			service = candidate
			break
		}
	}
	if service == nil {
		writeJSONError(w, http.StatusNotFound, "service_not_found", "")
		return
	}
	etag := `"` + service.ID + "-" + service.Epoch + "-" +
		strconv.FormatUint(service.Revision, 10) + "-" + service.StateHash.String() + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	signature := f.signer.sign(adsync.SnapshotSignedBytes(
		service.ID, service.Epoch, service.Revision, service.StateHash, service.CommitHash,
	))
	writeJSON(w, http.StatusOK, adsync.SnapshotResponse{
		ServiceID:  service.ID,
		SourceID:   service.ID,
		Epoch:      service.Epoch,
		Revision:   service.Revision,
		StateHash:  service.StateHash,
		CommitHash: service.CommitHash,
		Signature:  signature,
		Snapshot:   service.Snapshot,
	})
}

// mutate applies a change to the source under its lock.
func (f *testSource) mutate(update func(*testSource)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	update(f)
}

// publishService applies a new revision to one service.
func (f *testSource) publishService(name string, mutate func(*adsync.Snapshot)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, service := range f.services {
		if service.Name == name {
			service.publish(mutate)
			return
		}
	}
}

// removeService drops one service from discovery and from the source.
func (f *testSource) removeService(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.services[:0]
	for _, service := range f.services {
		if service.Name != name {
			kept = append(kept, service)
		}
	}
	f.services = kept
}

func (f *testSource) service(name string) (*testService, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, service := range f.services {
		if service.Name == name {
			return service, true
		}
	}
	return nil, false
}

func (f *testSource) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

func (f *testSource) requestsForSuffix(suffix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, path := range f.paths {
		if strings.HasSuffix(path, suffix) {
			count++
		}
	}
	return count
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, code, reason string) {
	body := `{"error":"` + code + `"`
	if reason != "" {
		body += `,"reason":"` + reason + `"`
	}
	body += `}`
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

type testFixture struct {
	t       *testing.T
	signer  *testSigner
	source  *testSource
	server  *httptest.Server
	caFile  string
	options Options
}

func newTestFixture(t *testing.T, services ...*testService) *testFixture {
	t.Helper()
	signer := newTestSigner(t)
	source := newTestSource(signer, services...)
	server := httptest.NewUnstartedServer(source)
	server.StartTLS()
	t.Cleanup(server.Close)

	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("test server has no certificate")
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(caFile, pemBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	return &testFixture{
		t:      t,
		signer: signer,
		source: source,
		server: server,
		caFile: caFile,
		options: Options{
			PolicySourceURL: server.URL,
			Credential:      testCredential,
			PublicKeys:      signer.keySet(t),
			HTTPClient:      server.Client(),
			RequestTimeout:  2 * time.Second,
			Retry: adsync.RetryPolicy{
				MaxAttempts:    1,
				InitialBackoff: time.Millisecond,
				MaxBackoff:     10 * time.Millisecond,
			},
		},
	}
}

func (f *testFixture) open(mutate func(*Options)) *Store {
	f.t.Helper()
	options := f.options
	if mutate != nil {
		mutate(&options)
	}
	opened, err := Open(nil, options)
	if err != nil {
		f.t.Fatalf("open policy-source store: %v", err)
	}
	return opened
}

// storeProperties returns store properties that use the real transport, for
// the registered-store builder path.
func (f *testFixture) storeProperties() map[string]interface{} {
	f.t.Helper()
	credentialFile := filepath.Join(f.t.TempDir(), "credential")
	if err := os.WriteFile(credentialFile, []byte(testCredential+"\n"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return map[string]interface{}{
		PolicySourceURLKey: f.server.URL,
		CredentialFileKey:  credentialFile,
		PublicKeysKey:      f.signer.writeKeysFile(f.t),
		CAFileKey:          f.caFile,
	}
}

func (f *testFixture) newStore() *Store {
	f.t.Helper()
	manager, err := store.NewStore(StoreType, f.storeProperties())
	if err != nil {
		f.t.Fatalf("create registered store: %v", err)
	}
	store, ok := manager.(*Store)
	if !ok {
		f.t.Fatalf("registered store has type %T", manager)
	}
	return store
}

func wantErrorCode(t *testing.T, what string, err error, code speedleerrors.ErrorCode) {
	t.Helper()
	if speedleerrors.Code(err) != code {
		t.Fatalf("%s: error %v, code %s, want %s", what, err, speedleerrors.Code(err), code)
	}
}
