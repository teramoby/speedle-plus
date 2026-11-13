// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
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
)

const (
	testInstanceID = "0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b20"
	testAPIKey     = "spdl_01HF7YAT000000000000000K01_storeSecretValue0123456789abcdef"
	testServiceID1 = "01HF7YAT000000000000000101"
	testServiceID2 = "01HF7YAT000000000000000102"
	testServiceID3 = "01HF7YAT000000000000000103"
)

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
	services   []*testService
	credential string

	requests int
	paths    []string
	failAll  bool

	corruptSnapshotHash bool
	snapshotHolds       map[string]chan struct{}

	// Fallback endpoint behaviour.
	fallbackCalls  int
	fallbackPaths  []string
	fallbackBodies [][]byte
	fallbackStatus int
	fallbackBody   string
}

func newTestSource(services ...*testService) *testSource {
	return &testSource{services: services, credential: testAPIKey}
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
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, adsync.AuthzCheckAPIPrefix):
		f.serveFallback(w, r)
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

// serveFallback answers a forwarded authorization request with the configured
// status and body.
func (f *testSource) serveFallback(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.fallbackCalls++
	f.fallbackPaths = append(f.fallbackPaths, r.URL.Path)
	f.fallbackBodies = append(f.fallbackBodies, body)
	status := f.fallbackStatus
	responseBody := f.fallbackBody
	f.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	if responseBody == "" {
		responseBody = `{"allowed":true,"reason":0}`
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(responseBody))
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
	var service *testService
	for _, candidate := range f.services {
		if candidate.ID == serviceID {
			service = candidate
			break
		}
	}
	if service == nil {
		f.mu.Unlock()
		writeJSONError(w, http.StatusNotFound, "service_not_found", "")
		return
	}
	etag := `"` + service.ID + "-" + service.Epoch + "-" +
		strconv.FormatUint(service.Revision, 10) + "-" + service.StateHash.String() + `"`
	if r.Header.Get("If-None-Match") == etag {
		f.mu.Unlock()
		w.WriteHeader(http.StatusNotModified)
		return
	}
	stateHash := service.StateHash
	if f.corruptSnapshotHash {
		stateHash[0] ^= 0xff
	}
	response := adsync.SnapshotResponse{
		ServiceID:  service.ID,
		SourceID:   service.ID,
		Epoch:      service.Epoch,
		Revision:   service.Revision,
		StateHash:  stateHash,
		CommitHash: service.CommitHash,
		Snapshot: adsync.Snapshot{
			Name:         service.Snapshot.Name,
			Policies:     append([]adsync.Policy(nil), service.Snapshot.Policies...),
			RolePolicies: append([]adsync.Policy(nil), service.Snapshot.RolePolicies...),
		},
	}
	hold := f.snapshotHolds[serviceID]
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-r.Context().Done():
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
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

// renameService publishes a new revision under a new service name, so
// discovery, the snapshot and later acks all carry the change together. The
// optional mutate runs before the revision is hashed.
func (f *testSource) renameService(name, newName string, mutate func(*adsync.Snapshot)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, service := range f.services {
		if service.Name != name {
			continue
		}
		service.Name = newName
		service.publish(func(snapshot *adsync.Snapshot) {
			snapshot.Name = newName
			if mutate != nil {
				mutate(snapshot)
			}
		})
		return
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

func (f *testSource) holdSnapshots(serviceID string) {
	f.mutate(func(source *testSource) {
		if source.snapshotHolds == nil {
			source.snapshotHolds = make(map[string]chan struct{})
		}
		source.snapshotHolds[serviceID] = make(chan struct{})
	})
}

func (f *testSource) releaseSnapshots(serviceID string) {
	f.mutate(func(source *testSource) {
		if hold := source.snapshotHolds[serviceID]; hold != nil {
			close(hold)
			delete(source.snapshotHolds, serviceID)
		}
	})
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

func (f *testSource) fallbackRequest() (string, []byte, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fallbackCalls == 0 {
		return "", nil, 0
	}
	index := f.fallbackCalls - 1
	return f.fallbackPaths[index], f.fallbackBodies[index], f.fallbackCalls
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
	source  *testSource
	server  *httptest.Server
	caFile  string
	options Options
}

func newTestFixture(t *testing.T, services ...*testService) *testFixture {
	t.Helper()
	source := newTestSource(services...)
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
		source: source,
		server: server,
		caFile: caFile,
		options: Options{
			UpstreamURL:    server.URL,
			APIKey:         testAPIKey,
			InstanceID:     testInstanceID,
			HTTPClient:     server.Client(),
			RequestTimeout: 2 * time.Second,
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
	// Most store tests start from an established, evaluator-installed copy.
	// Simulate the channel ack and reload acknowledgement explicitly; focused
	// lifecycle tests use Open directly to observe the unconfirmed state.
	if _, err := opened.client.Services(context.Background()); err == nil {
		confirmStoreGeneration(f.t, opened)
	}
	return opened
}

// confirmStoreGeneration simulates the two external events that make a synced
// revision eligible for local evaluation: a confirming channel ack and the
// evaluator's acknowledgement that it installed the published assembly.
func confirmStoreGeneration(t *testing.T, store *Store) {
	t.Helper()
	if err := store.Confirm(); err != nil {
		t.Fatalf("confirm channel ack: %v", err)
	}
	_, acknowledge, err := store.ReadPolicyStoreForReload()
	if err != nil {
		t.Fatalf("read published generation: %v", err)
	}
	acknowledge()
}

// storeProperties exercises the new configuration surface, without registration.
func (f *testFixture) storeProperties() map[string]interface{} {
	return map[string]interface{}{UpstreamURLKey: f.server.URL, CAFileKey: f.caFile}
}

func (f *testFixture) newStore() *Store {
	f.t.Helper()
	return f.open(func(options *Options) { options.StartRuntime = true })
}

func wantErrorCode(t *testing.T, what string, err error, code speedleerrors.ErrorCode) {
	t.Helper()
	if speedleerrors.Code(err) != code {
		t.Fatalf("%s: error %v, code %s, want %s", what, err, speedleerrors.Code(err), code)
	}
}
