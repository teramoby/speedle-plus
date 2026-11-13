// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
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
)

const (
	testInstanceID = "01HF7YAT000000000000000001"
	testServiceID1 = "01HF7YAT000000000000000101"
	testServiceID2 = "01HF7YAT000000000000000102"
	// 43 base62 characters.
	testCredential = "adsi_" + testInstanceID + "_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
)

type fixtureSigner struct {
	keyID   string
	private ed25519.PrivateKey
}

func (s *fixtureSigner) sign(message []byte) Signature {
	return Signature{
		KeyID: s.keyID,
		Alg:   "Ed25519",
		Value: base64.StdEncoding.EncodeToString(ed25519.Sign(s.private, message)),
	}
}

type fixtureService struct {
	ID         string
	Name       string
	Epoch      string
	Revision   uint64
	State      Snapshot
	StateHash  Hash
	CommitHash Hash
	commits    []Commit
}

func newFixtureService(id, name string) *fixtureService {
	state := Snapshot{Name: name, Policies: []Policy{}, RolePolicies: []Policy{}}
	stateHash := StateHash(state)
	return &fixtureService{
		ID:         id,
		Name:       name,
		Epoch:      id,
		Revision:   1,
		State:      state,
		StateHash:  stateHash,
		CommitHash: stateHash,
	}
}

// publish appends one signed revision. The fixture applies changes with its
// own independent implementation so the client's application path is tested
// against a second one.
func (s *fixtureService) publish(t *testing.T, signer *fixtureSigner, changes ...Change) Commit {
	t.Helper()
	prev := s.CommitHash
	hash, err := CommitHash(prev, changes)
	if err != nil {
		t.Fatalf("fixture commit hash: %v", err)
	}
	commit := Commit{
		Revision: s.Revision + 1,
		PrevHash: prev,
		Hash:     hash,
		At:       1762500000000 + int64(s.Revision)*1000,
		Changes:  changes,
	}
	commit.Signature = signer.sign(CommitSignedBytes(s.ID, s.Epoch, commit.Revision, hash))
	state := cloneSnapshot(s.State)
	if err := fixtureApplyChanges(&state, changes); err != nil {
		t.Fatalf("fixture apply changes: %v", err)
	}
	for _, change := range changes {
		if change.Kind == changeKindServiceName && change.Op == changeOpUpsertName && change.Name != nil {
			s.Name = *change.Name
		}
	}
	s.State = state
	s.StateHash = StateHash(state)
	s.CommitHash = hash
	s.Revision = commit.Revision
	s.commits = append(s.commits, commit)
	return commit
}

func fixtureApplyChanges(snapshot *Snapshot, changes []Change) error {
	for _, change := range changes {
		var target *[]Policy
		switch change.Kind {
		case changeKindAuthzName:
			target = &snapshot.Policies
		case changeKindRoleName:
			target = &snapshot.RolePolicies
		case changeKindServiceName:
			if change.Op == changeOpDeleteName {
				return fmt.Errorf("%w: fixture service delete", ErrSourceGone)
			}
			if change.Name == nil {
				return fmt.Errorf("fixture service change without a name")
			}
			snapshot.Name = *change.Name
			continue
		default:
			return fmt.Errorf("fixture unknown change kind %q", change.Kind)
		}
		switch change.Op {
		case changeOpDeleteName:
			kept := (*target)[:0]
			for _, policy := range *target {
				if policy.ID != change.ID {
					kept = append(kept, policy)
				}
			}
			*target = kept
		case changeOpUpsertName:
			if change.Name == nil || change.SPDL == nil {
				return fmt.Errorf("fixture upsert without name or SPDL")
			}
			replaced := false
			for index := range *target {
				if (*target)[index].ID == change.ID {
					(*target)[index] = Policy{ID: change.ID, Name: *change.Name, SPDL: *change.SPDL}
					replaced = true
					break
				}
			}
			if !replaced {
				*target = append(*target, Policy{ID: change.ID, Name: *change.Name, SPDL: *change.SPDL})
			}
		default:
			return fmt.Errorf("fixture unknown change op %q", change.Op)
		}
	}
	return nil
}

type fixtureRequest struct {
	Method        string
	Path          string
	Query         string
	Authorization string
	Instance      string
}

type fixtureSource struct {
	mu     sync.Mutex
	mux    *http.ServeMux
	signer *fixtureSigner

	credential      string
	services        map[string]*fixtureService
	order           []string
	servicesVersion int64
	requests        []fixtureRequest

	failRemaining  int
	failStatus     int
	failRetryAfter string
	delay          time.Duration
	pageSize       int

	changesReason            string
	dropCommitRevision       uint64
	snapshotOverride         *SnapshotResponse
	snapshotStarted          chan struct{}
	snapshotRelease          chan struct{}
	corruptSnapshotHash      bool
	corruptSnapshotSignature bool
	corruptCommitSignature   bool
	corruptHeadStateHash     bool
	corruptDiscoveryHash     bool
}

func newFixtureSource(credential string, signer *fixtureSigner, services ...*fixtureService) *fixtureSource {
	source := &fixtureSource{
		signer:          signer,
		credential:      credential,
		services:        make(map[string]*fixtureService, len(services)),
		servicesVersion: 1,
		failStatus:      http.StatusServiceUnavailable,
	}
	for _, service := range services {
		source.services[service.ID] = service
		source.order = append(source.order, service.ID)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+APIPrefix+"/services", source.serveServices)
	mux.HandleFunc("GET "+APIPrefix+"/services/{id}/snapshot", source.serveSnapshot)
	mux.HandleFunc("GET "+APIPrefix+"/services/{id}/changes", source.serveChanges)
	source.mux = mux
	return source
}

func (f *fixtureSource) configure(update func(*fixtureSource)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	update(f)
}

func (f *fixtureSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}

	f.mu.Lock()
	f.requests = append(f.requests, fixtureRequest{
		Method:        r.Method,
		Path:          r.URL.Path,
		Query:         r.URL.RawQuery,
		Authorization: r.Header.Get("Authorization"),
		Instance:      r.Header.Get(InstanceHeader),
	})
	fail := f.failRemaining > 0
	if fail {
		f.failRemaining--
	}
	failStatus := f.failStatus
	failRetryAfter := f.failRetryAfter
	credential := f.credential
	f.mu.Unlock()

	if fail {
		if failRetryAfter != "" {
			w.Header().Set("Retry-After", failRetryAfter)
		}
		writeFixtureError(w, failStatus, "source_unavailable", "")
		return
	}
	if credential != "" && r.Header.Get("Authorization") != "Bearer "+credential {
		writeFixtureError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	f.mux.ServeHTTP(w, r)
}

func (f *fixtureSource) serveServices(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	response := ServicesResponse{ServicesVersion: f.servicesVersion}
	for _, id := range f.order {
		service := f.services[id]
		headStateHash := service.StateHash
		if f.corruptDiscoveryHash {
			headStateHash = flipHash(headStateHash)
		}
		response.Services = append(response.Services, Service{
			ID:            service.ID,
			Name:          service.Name,
			Revision:      service.Revision,
			Epoch:         service.Epoch,
			HeadStateHash: headStateHash,
		})
	}
	writeFixtureJSON(w, http.StatusOK, response)
}

func (f *fixtureSource) serveSnapshot(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	started := f.snapshotStarted
	release := f.snapshotRelease
	f.snapshotStarted = nil
	f.mu.Unlock()
	if started != nil {
		close(started)
	}
	if release != nil {
		<-release
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.snapshotOverride != nil {
		writeFixtureJSON(w, http.StatusOK, f.snapshotOverride)
		return
	}
	service, found := f.services[r.PathValue("id")]
	if !found {
		writeFixtureError(w, http.StatusNotFound, "service_not_found", "")
		return
	}
	etag := `"` + service.ID + "-" + service.Epoch + "-" +
		strconv.FormatUint(service.Revision, 10) + "-" + service.StateHash.String() + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	stateHash := service.StateHash
	if f.corruptSnapshotHash {
		stateHash = flipHash(stateHash)
	}
	signature := f.signer.sign(SnapshotSignedBytes(
		service.ID, service.Epoch, service.Revision, stateHash, service.CommitHash,
	))
	if f.corruptSnapshotSignature {
		signature = corruptSignature(signature)
	}
	writeFixtureJSON(w, http.StatusOK, SnapshotResponse{
		ServiceID:  service.ID,
		SourceID:   service.ID,
		Epoch:      service.Epoch,
		Revision:   service.Revision,
		StateHash:  stateHash,
		CommitHash: service.CommitHash,
		Signature:  signature,
		Snapshot:   cloneSnapshot(service.State),
	})
}

func (f *fixtureSource) serveChanges(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	service, found := f.services[r.PathValue("id")]
	if !found {
		writeFixtureError(w, http.StatusNotFound, "service_not_found", "")
		return
	}
	rawSince := r.URL.Query().Get("since")
	since, err := strconv.ParseUint(rawSince, 10, 64)
	if err != nil || strconv.FormatUint(since, 10) != rawSince {
		writeFixtureError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	limit := changePageLimit
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 {
			writeFixtureError(w, http.StatusBadRequest, "invalid_request", "")
			return
		}
		limit = parsed
	}
	if f.pageSize > 0 && limit > f.pageSize {
		limit = f.pageSize
	}
	if f.changesReason != "" {
		writeFixtureError(w, http.StatusGone, "snapshot_required", f.changesReason)
		return
	}
	if since > service.Revision {
		writeFixtureError(w, http.StatusGone, "snapshot_required", snapshotReasonEpochMismatch)
		return
	}

	commits := make([]Commit, 0, limit)
	for _, commit := range service.commits {
		if commit.Revision <= since || commit.Revision == f.dropCommitRevision {
			continue
		}
		copied := commit
		if f.corruptCommitSignature {
			copied.Signature = corruptSignature(copied.Signature)
		}
		commits = append(commits, copied)
		if len(commits) == limit {
			break
		}
	}
	to := since
	if len(commits) > 0 {
		to = commits[len(commits)-1].Revision
	}
	headStateHash := service.StateHash
	if f.corruptHeadStateHash {
		headStateHash = flipHash(headStateHash)
	}
	writeFixtureJSON(w, http.StatusOK, ChangesResponse{
		ServiceID:     service.ID,
		SourceID:      service.ID,
		Epoch:         service.Epoch,
		From:          since + 1,
		To:            to,
		Head:          service.Revision,
		HeadStateHash: headStateHash,
		Commits:       commits,
		NextSince:     to + 1,
	})
}

func flipHash(hash Hash) Hash {
	hash[0] ^= 0xff
	return hash
}

func corruptSignature(signature Signature) Signature {
	raw, err := base64.StdEncoding.DecodeString(signature.Value)
	if err != nil || len(raw) == 0 {
		return signature
	}
	raw[0] ^= 0xff
	signature.Value = base64.StdEncoding.EncodeToString(raw)
	return signature
}

func writeFixtureJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeFixtureError(w http.ResponseWriter, status int, code, reason string) {
	body := `{"error":"` + code + `"`
	if reason != "" {
		body += `,"reason":"` + reason + `"`
	}
	body += `}`
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

type fixture struct {
	t          *testing.T
	source     *fixtureSource
	server     *httptest.Server
	caFile     string
	keys       *PublicKeySet
	signer     *fixtureSigner
	credential string
}

func newFixture(t *testing.T, services ...*fixtureService) *fixture {
	t.Helper()
	vectors := loadGolden(t)
	signer := &fixtureSigner{
		keyID:   vectors.TestKey.KeyID,
		private: goldenPrivateKey(t, vectors),
	}
	source := newFixtureSource(testCredential, signer, services...)
	server := httptest.NewUnstartedServer(source)
	server.StartTLS()
	t.Cleanup(server.Close)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("test server has no certificate")
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(caFile, pemBytes, 0o644); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	return &fixture{
		t:          t,
		source:     source,
		server:     server,
		caFile:     caFile,
		keys:       goldenPublicKeys(t, vectors),
		signer:     signer,
		credential: testCredential,
	}
}

func (f *fixture) config(mutate func(*Config)) Config {
	config := Config{
		BaseURL:         f.server.URL,
		Credential:      f.credential,
		PublicKeys:      f.keys,
		CAFile:          f.caFile,
		RequestTimeout:  2 * time.Second,
		SnapshotTimeout: 2 * time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
		},
	}
	if mutate != nil {
		mutate(&config)
	}
	return config
}

func (f *fixture) newClient(mutate func(*Config)) *Client {
	f.t.Helper()
	client, err := NewClient(f.config(mutate))
	if err != nil {
		f.t.Fatalf("create client: %v", err)
	}
	return client
}

func (f *fixture) newClientAt(baseURL string, mutate func(*Config)) *Client {
	f.t.Helper()
	return f.newClient(func(config *Config) {
		config.BaseURL = baseURL
		if mutate != nil {
			mutate(config)
		}
	})
}

func (f *fixture) newSyncer(cacheDir string, mutate func(*Config)) *Syncer {
	f.t.Helper()
	if cacheDir != "" {
		if err := os.Chmod(cacheDir, 0o700); err != nil {
			f.t.Fatalf("secure cache directory: %v", err)
		}
	}
	client := f.newClient(mutate)
	syncer, err := NewSyncer(client, cacheDir)
	if err != nil {
		f.t.Fatalf("create syncer: %v", err)
	}
	return syncer
}

func (f *fixture) service(name string) *fixtureService {
	f.t.Helper()
	f.source.mu.Lock()
	defer f.source.mu.Unlock()
	for _, id := range f.source.order {
		if service := f.source.services[id]; service.Name == name {
			return service
		}
	}
	f.t.Fatalf("fixture has no service named %q", name)
	return nil
}

func (f *fixture) requestCount(pathSuffix string) int {
	f.source.mu.Lock()
	defer f.source.mu.Unlock()
	count := 0
	for _, request := range f.source.requests {
		if strings.HasSuffix(request.Path, pathSuffix) {
			count++
		}
	}
	return count
}

func (f *fixture) requestCounts() map[string]int {
	f.source.mu.Lock()
	defer f.source.mu.Unlock()
	counts := make(map[string]int)
	for _, request := range f.source.requests {
		counts[request.Path]++
	}
	return counts
}

func syncFixtureService(t *testing.T, syncer *Syncer, name string) *ServiceState {
	t.Helper()
	state, err := syncer.SyncByName(context.Background(), name)
	if err != nil {
		t.Fatalf("sync %q: %v", name, err)
	}
	return state
}
