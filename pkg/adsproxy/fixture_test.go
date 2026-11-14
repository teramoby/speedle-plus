// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

const (
	testProxyID          = "0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b00"
	testUpstreamID       = "0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b01"
	testClientInstanceID = "0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b21"
	testClientInstanceB  = "0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b22"
	testServiceID1       = "01HF7YAT000000000000000101"
	testServiceID2       = "01HF7YAT000000000000000102"
	testUnknownID        = "01HF7YAT0000000000000001FF"

	testRevokedSerial = 3003
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

// testPKI is a generated certificate authority with server and client
// certificates and one CRL. Tests rewrite the CRL and material files through
// its paths.
type testPKI struct {
	root    *x509.Certificate
	rootKey *ecdsa.PrivateKey
	roots   *x509.CertPool

	serverCertFile string
	serverKeyFile  string
	serverCertPEM  []byte

	caFile string

	clientCertFile string
	clientKeyFile  string
	clientCertPEM  []byte
	clientKeyPEM   []byte

	clientBCertFile string
	clientBKeyFile  string
	clientBInstance string

	expiredCertFile string
	expiredKeyFile  string
	expiredPEM      []byte

	revokedCertFile string
	revokedKeyFile  string

	untrustedCertFile string
	untrustedKeyFile  string

	serverOnlyCertFile string
	serverOnlyKeyFile  string

	crlFile   string
	crlNumber int64
}

type certificateTemplate struct {
	serial    int64
	common    string
	notBefore time.Time
	notAfter  time.Time
	client    bool
	server    bool
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()
	now := time.Now()
	directory := t.TempDir()
	root, rootKey := newTestCA(t, "adsproxy test CA", now)
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})

	serverCert, serverKey := issueTestCertificate(t, root, rootKey, certificateTemplate{
		serial: 2001, common: "relay.test", notBefore: now.Add(-time.Hour),
		notAfter: now.Add(24 * time.Hour), server: true,
	})
	clientCert, clientKey := issueTestCertificate(t, root, rootKey, certificateTemplate{
		serial: 3001, common: "downstream-a", notBefore: now.Add(-time.Hour),
		notAfter: now.Add(24 * time.Hour), client: true,
	})
	clientBCert, clientBKey := issueTestCertificate(t, root, rootKey, certificateTemplate{
		serial: 3002, common: "downstream-b", notBefore: now.Add(-time.Hour),
		notAfter: now.Add(24 * time.Hour), client: true,
	})
	expiredCert, expiredKey := issueTestCertificate(t, root, rootKey, certificateTemplate{
		serial: 3004, common: "downstream-expired", notBefore: now.Add(-2 * time.Hour),
		notAfter: now.Add(-time.Hour), client: true,
	})
	revokedCert, revokedKey := issueTestCertificate(t, root, rootKey, certificateTemplate{
		serial: testRevokedSerial, common: "downstream-revoked", notBefore: now.Add(-time.Hour),
		notAfter: now.Add(24 * time.Hour), client: true,
	})
	otherCA, otherKey := newTestCA(t, "untrusted test CA", now)
	untrustedCert, untrustedKey := issueTestCertificate(t, otherCA, otherKey, certificateTemplate{
		serial: 3005, common: "downstream-untrusted", notBefore: now.Add(-time.Hour),
		notAfter: now.Add(24 * time.Hour), client: true,
	})
	serverOnlyCert, serverOnlyKey := issueTestCertificate(t, root, rootKey, certificateTemplate{
		serial: 3006, common: "downstream-server-only", notBefore: now.Add(-time.Hour),
		notAfter: now.Add(24 * time.Hour), server: true,
	})

	pki := &testPKI{
		root: root, rootKey: rootKey, roots: poolFor(rootPEM),
		caFile:             writeFile(t, directory, "clients-ca.pem", rootPEM),
		serverCertFile:     writeFile(t, directory, "server.pem", serverCert),
		serverKeyFile:      writeFile(t, directory, "server-key.pem", serverKey),
		serverCertPEM:      serverCert,
		clientCertFile:     writeFile(t, directory, "client.pem", clientCert),
		clientKeyFile:      writeFile(t, directory, "client-key.pem", clientKey),
		clientCertPEM:      clientCert,
		clientKeyPEM:       clientKey,
		clientBCertFile:    writeFile(t, directory, "client-b.pem", clientBCert),
		clientBKeyFile:     writeFile(t, directory, "client-b-key.pem", clientBKey),
		clientBInstance:    testClientInstanceB,
		expiredCertFile:    writeFile(t, directory, "expired.pem", expiredCert),
		expiredKeyFile:     writeFile(t, directory, "expired-key.pem", expiredKey),
		expiredPEM:         expiredCert,
		revokedCertFile:    writeFile(t, directory, "revoked.pem", revokedCert),
		revokedKeyFile:     writeFile(t, directory, "revoked-key.pem", revokedKey),
		untrustedCertFile:  writeFile(t, directory, "untrusted.pem", untrustedCert),
		untrustedKeyFile:   writeFile(t, directory, "untrusted-key.pem", untrustedKey),
		serverOnlyCertFile: writeFile(t, directory, "server-only.pem", serverOnlyCert),
		serverOnlyKeyFile:  writeFile(t, directory, "server-only-key.pem", serverOnlyKey),
		crlFile:            filepath.Join(directory, "clients.crl.pem"),
	}
	pki.writeCRL(t, 1, now.Add(-time.Hour), now.Add(24*time.Hour), big.NewInt(testRevokedSerial))
	return pki
}

func newTestCA(t *testing.T, common string, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1001),
		Subject:               pkix.Name{CommonName: common},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(7 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	return certificate, key
}

// issueIntermediateCA creates a CA signed by parent. The returned PEM is the
// intermediate certificate itself.
func issueIntermediateCA(t *testing.T, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate intermediate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1500),
		Subject:               pkix.Name{CommonName: "adsproxy intermediate CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(6 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatalf("create intermediate certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse intermediate certificate: %v", err)
	}
	return certificate, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func issueTestCertificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, template certificateTemplate) (certificatePEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate certificate key: %v", err)
	}
	templateValue := &x509.Certificate{
		SerialNumber: big.NewInt(template.serial),
		Subject:      pkix.Name{CommonName: template.common},
		NotBefore:    template.notBefore,
		NotAfter:     template.notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if template.client {
		templateValue.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	if template.server {
		templateValue.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		templateValue.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		templateValue.DNSNames = []string{"localhost"}
	}
	der, err := x509.CreateCertificate(rand.Reader, templateValue, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal certificate key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func poolFor(pemBytes []byte) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pemBytes)
	return pool
}

func writeFile(t *testing.T, directory, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// writeCRL replaces the client CA's CRL file. Every call increments the CRL
// number unless the caller supplies one.
func (p *testPKI) writeCRL(t *testing.T, number int64, thisUpdate, nextUpdate time.Time, revoked ...*big.Int) {
	t.Helper()
	entries := make([]x509.RevocationListEntry, 0, len(revoked))
	for _, serial := range revoked {
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   serial,
			RevocationTime: thisUpdate,
		})
	}
	template := &x509.RevocationList{
		Number:                    big.NewInt(number),
		ThisUpdate:                thisUpdate,
		NextUpdate:                nextUpdate,
		RevokedCertificateEntries: entries,
	}
	der, err := x509.CreateRevocationList(rand.Reader, template, p.root, p.rootKey)
	if err != nil {
		t.Fatalf("create CRL: %v", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der})
	if err := os.WriteFile(p.crlFile, encoded, 0o600); err != nil {
		t.Fatalf("write CRL: %v", err)
	}
	p.crlNumber = number
}

// testServer is one running downstream listener with its generated PKI.
type testServer struct {
	t        *testing.T
	mirror   *Mirror
	server   *Server
	listener *Listener
	pki      *testPKI
	authz    *authzStub
	handler  http.Handler
	address  string
	done     chan struct{}
}

type testServerOptions struct {
	upstream       func() adsync.UpstreamStatus
	coalesceWindow time.Duration
	hub            *Hub
	paused         func() bool
	chain          []string
}

func newTestServer(t *testing.T, mirror *Mirror, options *testServerOptions) *testServer {
	t.Helper()
	if options == nil {
		options = &testServerOptions{}
	}
	pki := newTestPKI(t)
	server, err := NewServer(Config{
		InstanceID:     testProxyID,
		Chain:          options.chain,
		Upstream:       options.upstream,
		Mirror:         mirror,
		CoalesceWindow: options.coalesceWindow,
		Hub:            options.hub,
		Paused:         options.paused,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	authz := &authzStub{}
	handler, err := DownstreamHandler(server, AuthzHandlers{
		IsAllowed:             http.HandlerFunc(authz.isAllowed),
		AllGrantedRoles:       http.HandlerFunc(authz.allGrantedRoles),
		AllGrantedPermissions: http.HandlerFunc(authz.allGrantedPermissions),
	})
	if err != nil {
		t.Fatalf("downstream handler: %v", err)
	}
	listener, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: pki.caFile,
		CRLFiles:     []string{pki.crlFile},
	}, handler)
	if err != nil {
		t.Fatalf("new listener: %v", err)
	}
	rawListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	running := &testServer{
		t:        t,
		mirror:   mirror,
		server:   server,
		listener: listener,
		pki:      pki,
		authz:    authz,
		handler:  handler,
		address:  "https://" + rawListener.Addr().String(),
		done:     make(chan struct{}),
	}
	go func() {
		defer close(running.done)
		if err := listener.Serve(rawListener); err != nil {
			t.Logf("downstream listener stopped: %v", err)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-running.done:
		case <-time.After(5 * time.Second):
			t.Log("downstream listener did not stop")
		}
		server.Close()
	})
	running.waitReady()
	return running
}

func (s *testServer) waitReady() {
	s.t.Helper()
	client := s.client(s.pki.clientCertFile, s.pki.clientKeyFile)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(s.address + adsync.APIPrefix + "/services")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatal("downstream listener did not become ready")
}

func (s *testServer) client(certFile, keyFile string) *http.Client {
	s.t.Helper()
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		s.t.Fatalf("load client certificate: %v", err)
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:      s.pki.roots,
			Certificates: []tls.Certificate{certificate},
		}},
		Timeout: 10 * time.Second,
	}
}

func (s *testServer) defaultClient() *http.Client {
	return s.client(s.pki.clientCertFile, s.pki.clientKeyFile)
}

func (s *testServer) get(client *http.Client, path string, header http.Header) (*http.Response, []byte) {
	s.t.Helper()
	request, err := http.NewRequest(http.MethodGet, s.address+path, nil)
	if err != nil {
		s.t.Fatalf("build request: %v", err)
	}
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := client.Do(request)
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

func (s *testServer) post(client *http.Client, path string, body []byte) (*http.Response, []byte) {
	s.t.Helper()
	request, err := http.NewRequest(http.MethodPost, s.address+path, strings.NewReader(string(body)))
	if err != nil {
		s.t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
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

// dialAnonymous opens a WebSocket channel without a client certificate.
func (s *testServer) dialAnonymous() (*websocket.Conn, *http.Response, error) {
	s.t.Helper()
	dialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{RootCAs: s.pki.roots},
		HandshakeTimeout: 5 * time.Second,
	}
	target := "wss" + strings.TrimPrefix(s.address, "https") + adsync.APIPrefix + "/channel"
	return dialer.Dial(target, http.Header{})
}

// dial opens a WebSocket channel with one client certificate.
func (s *testServer) dial(certFile, keyFile string) (*websocket.Conn, *http.Response, error) {
	s.t.Helper()
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		s.t.Fatalf("load client certificate: %v", err)
	}
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			RootCAs:      s.pki.roots,
			Certificates: []tls.Certificate{certificate},
		},
		HandshakeTimeout: 5 * time.Second,
	}
	target := "wss" + strings.TrimPrefix(s.address, "https") + adsync.APIPrefix + "/channel"
	return dialer.Dial(target, http.Header{})
}

// newClient returns a downstream adsync client that authenticates with the
// generated client certificate.
func (s *testServer) newClient(instanceID string) *adsync.Client {
	s.t.Helper()
	client, err := adsync.NewClient(adsync.Config{
		BaseURL:         s.address,
		APIKey:          "test-downstream-key",
		InstanceID:      instanceID,
		CAFile:          s.pki.caFile,
		ClientCertFile:  s.pki.clientCertFile,
		ClientKeyFile:   s.pki.clientKeyFile,
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

// authzStub records the three authorization operations. Authz handlers run
// after the listener's TLS authentication, so a request that reaches them
// proves the route and the certificate.
type authzStub struct {
	calls int
}

func (a *authzStub) isAllowed(w http.ResponseWriter, r *http.Request) {
	a.calls++
	writeJSON(w, http.StatusOK, map[string]bool{"allowed": true})
}

func (a *authzStub) allGrantedRoles(w http.ResponseWriter, r *http.Request) {
	a.calls++
	writeJSON(w, http.StatusOK, []string{})
}

func (a *authzStub) allGrantedPermissions(w http.ResponseWriter, r *http.Request) {
	a.calls++
	writeJSON(w, http.StatusOK, []any{})
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

// helloFrame returns a valid hello frame for one instance.
func helloFrame(instanceID string, services ...string) map[string]any {
	entries := make([]map[string]any, 0, len(services))
	for _, service := range services {
		entries = append(entries, map[string]any{
			"id": service, "name": "service", "revision": 1, "state": "in_sync",
		})
	}
	return map[string]any{
		"type":         "hello",
		"instanceId":   instanceID,
		"services":     entries,
		"agentVersion": "test",
		"host":         nil,
	}
}

func heartbeatFrame(instanceID string) map[string]any {
	return map[string]any{
		"type":            "heartbeat",
		"instanceId":      instanceID,
		"seq":             1,
		"services":        []any{},
		"liveness":        "live",
		"fallback":        map[string]any{"active": false, "services": []any{}, "since": nil, "routedCalls": 0},
		"agentVersion":    "test",
		"transport":       "websocket",
		"downstreamCount": nil,
	}
}
