// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

const (
	testProxyID      = "01HF7YAT0000000000000002A1"
	testUpstreamID   = "01HF7YAT0000000000000002A2"
	testDownstreamID = "01HF7YAT0000000000000002B1"
	testDownstreamB  = "01HF7YAT0000000000000002B2"
	testServiceID1   = "01HF7YAT000000000000000101"
	testServiceID2   = "01HF7YAT000000000000000102"
	testUnknownID    = "01HF7YAT0000000000000001FF"
	// testClientInstanceID is the instance ID a downstream adsync client
	// sends; the proxy identifies downstreams by their tokens.
	testClientInstanceID = "0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b21"
)

// fixtureService is a source of consistent snapshots and change pages.
// It is the Cloud stand-in for these tests.
type fixtureService struct {
	ID         string
	Name       string
	Epoch      string
	Revision   uint64
	State      adsync.Snapshot
	StateHash  adsync.Hash
	CommitHash adsync.Hash
	commits    []adsync.Commit
}

func newFixtureService(id, name string) *fixtureService {
	return newFixtureServiceWithEpoch(id, id, name)
}

func newFixtureServiceWithEpoch(id, epoch, name string) *fixtureService {
	state := adsync.Snapshot{Name: name, Policies: []adsync.Policy{}, RolePolicies: []adsync.Policy{}}
	stateHash := adsync.StateHash(state)
	return &fixtureService{
		ID:         id,
		Name:       name,
		Epoch:      epoch,
		Revision:   1,
		State:      state,
		StateHash:  stateHash,
		CommitHash: stateHash,
	}
}

func (s *fixtureService) publish(t *testing.T, changes ...adsync.Change) adsync.Commit {
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
	state := cloneSnapshot(s.State)
	if err := adsync.ApplyChanges(&state, changes); err != nil && !errors.Is(err, adsync.ErrSourceGone) {
		t.Fatalf("fixture apply changes: %v", err)
	} else if err == nil {
		s.State = state
	}
	s.Name = state.Name
	s.StateHash = adsync.StateHash(state)
	s.CommitHash = hash
	s.Revision = commit.Revision
	s.commits = append(s.commits, commit)
	return commit
}

func (s *fixtureService) snapshotResponse(t *testing.T) *adsync.SnapshotResponse {
	t.Helper()
	return &adsync.SnapshotResponse{
		ServiceID:  s.ID,
		SourceID:   s.ID,
		Epoch:      s.Epoch,
		Revision:   s.Revision,
		StateHash:  s.StateHash,
		CommitHash: s.CommitHash,
		Snapshot:   cloneSnapshot(s.State),
	}
}

func (s *fixtureService) changesPage(since uint64, limit int) adsync.ChangesResponse {
	commits := make([]adsync.Commit, 0, limit)
	for _, commit := range s.commits {
		if commit.Revision <= since {
			continue
		}
		commits = append(commits, cloneCommit(commit))
		if len(commits) == limit {
			break
		}
	}
	to := since
	if len(commits) > 0 {
		to = commits[len(commits)-1].Revision
	}
	return adsync.ChangesResponse{
		ServiceID:     s.ID,
		SourceID:      s.ID,
		Epoch:         s.Epoch,
		From:          since + 1,
		To:            to,
		Head:          s.Revision,
		HeadStateHash: s.StateHash,
		Commits:       commits,
		NextSince:     to + 1,
	}
}

func (s *fixtureService) service() adsync.Service {
	return adsync.Service{
		ID:            s.ID,
		Name:          s.Name,
		Revision:      s.Revision,
		Epoch:         s.Epoch,
		HeadStateHash: s.StateHash,
	}
}

func upsertAuthz(id, name, spdl string) adsync.Change {
	return adsync.Change{Op: "upsert", Kind: "authz", ID: id, Name: &name, SPDL: &spdl}
}

func upsertRole(id, name, spdl string) adsync.Change {
	return adsync.Change{Op: "upsert", Kind: "role", ID: id, Name: &name, SPDL: &spdl}
}

func deleteChange(kind, id string) adsync.Change {
	return adsync.Change{Op: "delete", Kind: kind, ID: id}
}

func renameService(id, name string) adsync.Change {
	return adsync.Change{Op: "upsert", Kind: "service", ID: id, Name: &name}
}

// testCredential is one downstream's configuration.
type testCredential struct {
	downstreamID string
	token        string
	services     []string
}

func testToken(instanceID string, number int) string {
	// The wire form is adsi_<ULID>_<43 base62 characters>. Digits are base62.
	return "adsi_" + instanceID + "_" + fmt.Sprintf("%043d", number)
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

type testServer struct {
	t              *testing.T
	mirror         *Mirror
	server         *Server
	httpServer     *httptest.Server
	caFile         string
	credentialFile string
	roots          *x509.CertPool
	credentials    map[string]string // downstream id -> token
}

// writeCredentialFile replaces the downstream credential file. Callers reload
// the credentials afterwards.
func (s *testServer) writeCredentialFile(lines ...string) {
	s.t.Helper()
	if err := os.WriteFile(s.credentialFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		s.t.Fatalf("rewrite credential file: %v", err)
	}
}

type testServerOptions struct {
	chain          []string
	coalesceWindow time.Duration
	hub            *Hub
	maxDownstream  int
	maxConnections int
	maxChainDepth  int
	paused         func() bool
}

func newTestServer(t *testing.T, mirror *Mirror, options *testServerOptions, credentials ...testCredential) *testServer {
	t.Helper()
	if options == nil {
		options = &testServerOptions{}
	}
	directory := t.TempDir()
	credentialFile := filepath.Join(directory, "downstream-credentials")
	lines := make([]string, 0, len(credentials))
	tokens := make(map[string]string, len(credentials))
	for _, credential := range credentials {
		subset := "*"
		if len(credential.services) > 0 {
			subset = strings.Join(credential.services, ",")
		}
		lines = append(lines, credential.downstreamID+":"+hashToken(credential.token)+":"+subset)
		tokens[credential.downstreamID] = credential.token
	}
	if err := os.WriteFile(credentialFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}

	server, err := NewServer(Config{
		InstanceID:                  testProxyID,
		Chain:                       options.chain,
		Mirror:                      mirror,
		CredentialFile:              credentialFile,
		MaxDownstream:               options.maxDownstream,
		MaxConnectionsPerDownstream: options.maxConnections,
		MaxChainDepth:               options.maxChainDepth,
		CoalesceWindow:              options.coalesceWindow,
		Hub:                         options.hub,
		Paused:                      options.paused,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	httpServer := httptest.NewUnstartedServer(server)
	httpServer.StartTLS()
	t.Cleanup(func() {
		httpServer.Close()
		server.Close()
	})

	caFile := filepath.Join(directory, "ca.pem")
	certificate := httpServer.Certificate()
	if certificate == nil {
		t.Fatal("test server has no certificate")
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(caFile, pemBytes, 0o644); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		t.Fatal("append CA certificate")
	}
	return &testServer{
		t:              t,
		mirror:         mirror,
		server:         server,
		httpServer:     httpServer,
		caFile:         caFile,
		credentialFile: credentialFile,
		roots:          roots,
		credentials:    tokens,
	}
}

func (s *testServer) token(downstreamID string) string {
	s.t.Helper()
	token, found := s.credentials[downstreamID]
	if !found {
		s.t.Fatalf("no token for downstream %q", downstreamID)
	}
	return token
}

func (s *testServer) client() *http.Client {
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: s.roots}},
	}
}

func (s *testServer) get(path, token string, header http.Header) (*http.Response, []byte) {
	s.t.Helper()
	request, err := http.NewRequest(http.MethodGet, s.httpServer.URL+path, nil)
	if err != nil {
		s.t.Fatalf("build request: %v", err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := s.client().Do(request)
	if err != nil {
		s.t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		s.t.Fatalf("read response: %v", err)
	}
	return response, body
}

func (s *testServer) post(path, token string, body []byte) (*http.Response, []byte) {
	s.t.Helper()
	request, err := http.NewRequest(http.MethodPost, s.httpServer.URL+path, strings.NewReader(string(body)))
	if err != nil {
		s.t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := s.client().Do(request)
	if err != nil {
		s.t.Fatalf("POST %s: %v", path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		s.t.Fatalf("read response: %v", err)
	}
	return response, payload
}

func (s *testServer) dial(token string) (*websocket.Conn, *http.Response, error) {
	dialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{RootCAs: s.roots},
		HandshakeTimeout: 5 * time.Second,
	}
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	target := "wss" + strings.TrimPrefix(s.httpServer.URL, "https") + adsync.APIPrefix + "/channel"
	return dialer.Dial(target, header)
}

func (s *testServer) newClient(token string) *adsync.Client {
	s.t.Helper()
	client, err := adsync.NewClient(adsync.Config{
		BaseURL:         s.httpServer.URL,
		APIKey:          token,
		InstanceID:      testClientInstanceID,
		CAFile:          s.caFile,
		RequestTimeout:  2 * time.Second,
		SnapshotTimeout: 2 * time.Second,
		Retry: adsync.RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
		},
	})
	if err != nil {
		s.t.Fatalf("new downstream client: %v", err)
	}
	return client
}

func syncToHead(t *testing.T, syncer *adsync.Syncer, service adsync.Service) *adsync.ServiceState {
	t.Helper()
	var (
		state *adsync.ServiceState
		err   error
	)
	for attempt := 0; attempt < 10; attempt++ {
		state, err = syncer.Sync(context.Background(), service)
		if err != nil {
			t.Fatalf("sync %q: %v", service.Name, err)
		}
		if state.Revision == service.Revision {
			return state
		}
	}
	t.Fatalf("sync %q stalled at revision %d, head is %d", service.Name, state.Revision, service.Revision)
	return nil
}

func waitFor(t *testing.T, timeout time.Duration, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func readChannelFrame(t *testing.T, conn *websocket.Conn) (string, []byte) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read channel message: %v", err)
	}
	if messageType != websocket.TextMessage {
		t.Fatalf("channel message type = %d", messageType)
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode channel message: %v", err)
	}
	return envelope.Type, payload
}

func writeJSONFrame(t *testing.T, conn *websocket.Conn, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

func readClose(t *testing.T, conn *websocket.Conn) (int, string) {
	t.Helper()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			closeErr, ok := err.(*websocket.CloseError)
			if !ok {
				t.Fatalf("read until close: %v", err)
			}
			return closeErr.Code, closeErr.Text
		}
		if messageType == websocket.TextMessage && len(payload) > 0 {
			// Ignore an ack that raced with the close.
			continue
		}
	}
}
