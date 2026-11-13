// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func TestNewListenerValidation(t *testing.T) {
	pki := newTestPKI(t)
	base := ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: pki.caFile,
		CRLFiles:     []string{pki.crlFile},
	}
	handler := http.NotFoundHandler()
	listener, err := NewListener(base, handler)
	if err != nil {
		t.Fatalf("valid configuration: %v", err)
	}
	if listener.config.ReloadInterval != DefaultDownstreamReloadInterval ||
		listener.config.CRLRefreshInterval != DefaultCRLRefreshInterval ||
		listener.config.CRLMaxAge != DefaultCRLMaxAge {
		t.Errorf("defaults = %s/%s/%s", listener.config.ReloadInterval,
			listener.config.CRLRefreshInterval, listener.config.CRLMaxAge)
	}
	_ = listener.Close()

	directory := t.TempDir()
	badCA := writeFile(t, directory, "non-ca.pem", pki.clientCertPEM)
	emptyCRL := writeFile(t, directory, "empty.crl", nil)
	garbageCRL := writeFile(t, directory, "garbage.crl", []byte("not a CRL"))
	emptyCA := writeFile(t, directory, "empty-ca.pem", nil)

	cases := map[string]struct {
		handler http.Handler
		mutate  func(*ListenerConfig)
		want    error
	}{
		"nil handler": {handler: nil, mutate: func(*ListenerConfig) {}, want: ErrConfig},
		"empty endpoint": {handler: handler, mutate: func(config *ListenerConfig) {
			config.Endpoint = ""
		}, want: ErrConfig},
		"missing certificate": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CertPath = ""
		}, want: ErrTLSMaterial},
		"missing key": {handler: handler, mutate: func(config *ListenerConfig) {
			config.KeyPath = ""
		}, want: ErrTLSMaterial},
		"missing client CA": {handler: handler, mutate: func(config *ListenerConfig) {
			config.ClientCAPath = ""
		}, want: ErrTLSMaterial},
		"negative reload interval": {handler: handler, mutate: func(config *ListenerConfig) {
			config.ReloadInterval = -time.Second
		}, want: ErrConfig},
		"negative CRL refresh": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLRefreshInterval = -time.Second
		}, want: ErrConfig},
		"negative CRL max age": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLMaxAge = -time.Second
		}, want: ErrConfig},
		"no CRL source": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLFiles = nil
			config.CRLURLs = nil
		}, want: ErrCRL},
		"empty CRL file path": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLFiles = []string{""}
		}, want: ErrCRL},
		"duplicate CRL file": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLFiles = []string{pki.crlFile, pki.crlFile}
		}, want: ErrCRL},
		"plain HTTP CRL URL": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLURLs = []string{"http://example.test/clients.crl"}
		}, want: ErrCRL},
		"CRL URL with credentials": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLURLs = []string{"https://user:secret@example.test/clients.crl"}
		}, want: ErrCRL},
		"CRL URL with query": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLURLs = []string{"https://example.test/clients.crl?x=1"}
		}, want: ErrCRL},
		"CRL URL with fragment": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLURLs = []string{"https://example.test/clients.crl#x"}
		}, want: ErrCRL},
		"duplicate CRL URL": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLFiles = []string{pki.crlFile}
			config.CRLURLs = []string{"https://example.test/clients.crl", "https://example.test/clients.crl"}
		}, want: ErrCRL},
		"missing certificate file": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CertPath = directory + "/missing.pem"
		}, want: ErrTLSMaterial},
		"missing client CA file": {handler: handler, mutate: func(config *ListenerConfig) {
			config.ClientCAPath = directory + "/missing.pem"
		}, want: ErrTLSMaterial},
		"empty client CA file": {handler: handler, mutate: func(config *ListenerConfig) {
			config.ClientCAPath = emptyCA
		}, want: ErrTLSMaterial},
		"client CA with a non-CA certificate": {handler: handler, mutate: func(config *ListenerConfig) {
			config.ClientCAPath = badCA
		}, want: ErrTLSMaterial},
		"missing CRL file": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLFiles = []string{directory + "/missing.crl"}
		}, want: ErrCRL},
		"empty CRL file": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLFiles = []string{emptyCRL}
		}, want: ErrCRL},
		"garbage CRL file": {handler: handler, mutate: func(config *ListenerConfig) {
			config.CRLFiles = []string{garbageCRL}
		}, want: ErrCRL},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			config := base
			testCase.mutate(&config)
			_, err := NewListener(config, testCase.handler)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestNewListenerRequiresFreshCRLForEveryIssuer(t *testing.T) {
	pki := newTestPKI(t)
	otherCA, otherKey := newTestCA(t, "second client CA", time.Now())
	otherPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: otherCA.Raw})
	_ = otherKey
	bundle := append(append([]byte(nil), mustRead(t, pki.caFile)...), otherPEM...)
	bundleFile := writeFile(t, t.TempDir(), "bundle.pem", bundle)

	listener, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: bundleFile,
		CRLFiles:     []string{pki.crlFile},
	}, http.NotFoundHandler())
	if err == nil {
		listener.Close()
		t.Fatal("a client CA without a fresh CRL was accepted")
	}
	if !errors.Is(err, ErrCRL) || !strings.Contains(err.Error(), "second client CA") {
		t.Fatalf("error %v does not name the uncovered issuer", err)
	}
}

func TestNewListenerRejectsUnusableCRLs(t *testing.T) {
	pki := newTestPKI(t)
	now := time.Now()
	directory := t.TempDir()
	otherCA, otherKey := newTestCA(t, "untrusted CRL CA", now)

	cases := map[string]func() string{
		"CRL from an untrusted CA": func() string {
			der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
				Number: big.NewInt(1), ThisUpdate: now.Add(-time.Hour), NextUpdate: now.Add(time.Hour),
			}, otherCA, otherKey)
			if err != nil {
				t.Fatalf("create CRL: %v", err)
			}
			return writeFile(t, directory, "untrusted.crl",
				pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}))
		},
		"expired CRL": func() string {
			pki.writeCRL(t, 10, now.Add(-2*time.Hour), now.Add(-time.Hour))
			return pki.crlFile
		},
		"delta CRL": func() string {
			der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
				Number: big.NewInt(11), ThisUpdate: now.Add(-time.Hour), NextUpdate: now.Add(time.Hour),
				ExtraExtensions: []pkix.Extension{deltaIndicator()},
			}, pki.root, pki.rootKey)
			if err != nil {
				t.Fatalf("create delta CRL: %v", err)
			}
			return writeFile(t, directory, "delta.crl",
				pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}))
		},
		"CRL scoped to CA certificates": func() string {
			// A CA-only CRL says nothing about client leaf certificates, so it
			// must not count as fresh revocation coverage for their issuer.
			der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
				Number: big.NewInt(13), ThisUpdate: now.Add(-time.Hour), NextUpdate: now.Add(time.Hour),
				ExtraExtensions: []pkix.Extension{issuingDistributionPoint()},
			}, pki.root, pki.rootKey)
			if err != nil {
				t.Fatalf("create scoped CRL: %v", err)
			}
			return writeFile(t, directory, "scoped.crl",
				pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}))
		},
		"CRL with an unsupported critical extension": func() string {
			der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
				Number: big.NewInt(14), ThisUpdate: now.Add(-time.Hour), NextUpdate: now.Add(time.Hour),
				ExtraExtensions: []pkix.Extension{{
					Id: asn1.ObjectIdentifier{1, 2, 3, 4, 5}, Critical: true, Value: []byte{0x05, 0x00},
				}},
			}, pki.root, pki.rootKey)
			if err != nil {
				t.Fatalf("create annotated CRL: %v", err)
			}
			return writeFile(t, directory, "annotated.crl",
				pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}))
		},
		"CRL from the future": func() string {
			der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
				Number: big.NewInt(12), ThisUpdate: now.Add(time.Hour), NextUpdate: now.Add(2 * time.Hour),
			}, pki.root, pki.rootKey)
			if err != nil {
				t.Fatalf("create CRL: %v", err)
			}
			return writeFile(t, directory, "future.crl",
				pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}))
		},
		"PEM without a CRL": func() string {
			return writeFile(t, directory, "certificate-only.pem", pki.clientCertPEM)
		},
	}
	for name, makeCRL := range cases {
		t.Run(name, func(t *testing.T) {
			crlFile := makeCRL()
			listener, err := NewListener(ListenerConfig{
				Endpoint:     "127.0.0.1:0",
				CertPath:     pki.serverCertFile,
				KeyPath:      pki.serverKeyFile,
				ClientCAPath: pki.caFile,
				CRLFiles:     []string{crlFile},
			}, http.NotFoundHandler())
			if err == nil {
				listener.Close()
				t.Fatal("unusable CRL was accepted")
			}
			if !errors.Is(err, ErrCRL) {
				t.Fatalf("error %v, want ErrCRL", err)
			}
		})
	}
}

func TestListenerRevocationClosesConnectionsAndRejectsReconnects(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	clientB := server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	clientA := server.defaultClient()
	path := adsync.APIPrefix + "/services"

	response, _, err := tracedGet(clientB, server.address+path)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("initial request: %v %v", response, err)
	}

	// Revoke client A. The CRL set changes, so every established connection
	// is closed, even one whose certificate stays valid.
	now := time.Now()
	server.pki.writeCRL(t, 2, now.Add(-time.Hour), now.Add(time.Hour),
		big.NewInt(3001), big.NewInt(testRevokedSerial))
	if err := server.listener.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	response, reused, err := tracedGet(clientB, server.address+path)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request after CRL change: %v %v", response, err)
	}
	if reused {
		t.Error("an established connection survived a CRL change")
	}
	if _, err := clientA.Get(server.address + path); err == nil {
		t.Fatal("a revoked certificate was accepted")
	}

	// A channel established before the reload is closed too.
	conn, _, err := server.dial(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, conn); kind != "ack" {
		t.Fatalf("first frame = %q", kind)
	}
	server.pki.writeCRL(t, 3, now.Add(-time.Hour), now.Add(time.Hour), big.NewInt(testRevokedSerial))
	if err := server.listener.Reload(); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("a channel survived a CRL change")
	}
	conn.Close()

	// A fresh replacement without the revocation recovers without restart.
	server.pki.writeCRL(t, 4, now.Add(-time.Hour), now.Add(time.Hour))
	if err := server.listener.Reload(); err != nil {
		t.Fatalf("recovery reload: %v", err)
	}
	clientA = server.defaultClient()
	if response, _, err := tracedGet(clientA, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("recovered request: %v %v", response, err)
	}
}

func TestListenerCRLExpiryClosesConnectionsAndRecovers(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	path := adsync.APIPrefix + "/services"

	now := time.Now()
	server.pki.writeCRL(t, 2, now.Add(-time.Minute), now.Add(2*time.Second))
	if err := server.listener.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if response, _, err := tracedGet(client, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request before expiry: %v %v", response, err)
	}

	time.Sleep(2500 * time.Millisecond)

	if _, err := client.Get(server.address + path); err == nil {
		t.Fatal("an expired CRL still admitted a new connection")
	}

	server.pki.writeCRL(t, 3, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	if err := server.listener.Reload(); err != nil {
		t.Fatalf("recovery reload: %v", err)
	}
	// The client's old keep-alive connection was closed at expiry; a fresh
	// handshake succeeds.
	client = server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	if response, _, err := tracedGet(client, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request after recovery: %v %v", response, err)
	}
}

func TestListenerCRLURLFetchesConditionally(t *testing.T) {
	pki := newTestPKI(t)
	state := &crlURLState{body: mustRead(t, pki.crlFile), etag: `"v1"`}
	address := startCRLServer(t, pki, state)
	listener := newURLListener(t, pki, address)
	defer listener.Close()

	state.mu.Lock()
	firstFetches := state.requests
	state.mu.Unlock()
	if firstFetches != 1 {
		t.Fatalf("startup fetches = %d", firstFetches)
	}

	// The second fetch is conditional and gets a 304; the cached CRL stays.
	if err := listener.Reload(); err != nil {
		t.Fatalf("conditional reload: %v", err)
	}
	state.mu.Lock()
	if state.lastIfNoneMatch != `"v1"` {
		t.Errorf("If-None-Match = %q", state.lastIfNoneMatch)
	}
	state.mu.Unlock()

	// A changed ETag and body is accepted.
	state.mu.Lock()
	state.body = crlWithNumber(t, pki, 2, time.Now().Add(time.Hour))
	state.etag = `"v2"`
	state.mu.Unlock()
	if err := listener.Reload(); err != nil {
		t.Fatalf("changed reload: %v", err)
	}
}

func TestListenerCRLURLFailureKeepsTheLastGoodCRL(t *testing.T) {
	pki := newTestPKI(t)
	state := &crlURLState{body: mustRead(t, pki.crlFile), etag: `"v1"`}
	address := startCRLServer(t, pki, state)
	listener := newURLListener(t, pki, address)
	defer listener.Close()

	// The source starts failing; the cached fresh CRL keeps working.
	state.mu.Lock()
	state.status = http.StatusInternalServerError
	state.mu.Unlock()
	if err := listener.Reload(); err != nil {
		t.Fatalf("reload with a failing source: %v", err)
	}
	if len(listener.generation.crls) != 1 {
		t.Fatalf("cached CRLs = %d", len(listener.generation.crls))
	}

	// A redirect is refused; the fetch fails but the cached CRL stays.
	state.mu.Lock()
	state.status = http.StatusFound
	state.redirect = true
	state.mu.Unlock()
	if err := listener.Reload(); err != nil {
		t.Fatalf("reload with a redirect: %v", err)
	}

	// A malformed response is rejected without displacing the last good
	// bytes: the next failed fetch still leaves the cached CRL usable.
	state.mu.Lock()
	state.status = http.StatusOK
	state.redirect = false
	state.body = []byte("garbage")
	state.etag = `"bad"`
	state.mu.Unlock()
	if err := listener.Reload(); !errors.Is(err, ErrCRL) {
		t.Fatalf("malformed reload: %v, want ErrCRL", err)
	}
	state.mu.Lock()
	state.status = http.StatusInternalServerError
	state.mu.Unlock()
	if err := listener.Reload(); err != nil {
		t.Fatalf("reload after a malformed response: %v", err)
	}

	// With no cached copy, the same failures fail startup.
	empty := &crlURLState{status: http.StatusInternalServerError}
	emptyAddress := startCRLServer(t, pki, empty)
	if _, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: pki.caFile,
		CRLURLs:      []string{emptyAddress},
	}, http.NotFoundHandler()); !errors.Is(err, ErrCRL) {
		t.Fatalf("first fetch failure: %v, want ErrCRL", err)
	}

	redirect := &crlURLState{status: http.StatusFound, redirect: true}
	redirectAddress := startCRLServer(t, pki, redirect)
	if _, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: pki.caFile,
		CRLURLs:      []string{redirectAddress},
	}, http.NotFoundHandler()); !errors.Is(err, ErrCRL) {
		t.Fatalf("redirected first fetch: %v, want ErrCRL", err)
	}
}

func TestListenerCRLURLStallTimesOut(t *testing.T) {
	pki := newTestPKI(t)
	stalled := &crlURLState{body: mustRead(t, pki.crlFile), stall: true}
	address := startCRLServer(t, pki, stalled)

	previous := crlStallTimeout
	crlStallTimeout = 100 * time.Millisecond
	defer func() { crlStallTimeout = previous }()

	start := time.Now()
	_, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: pki.caFile,
		CRLURLs:      []string{address},
	}, http.NotFoundHandler())
	if !errors.Is(err, ErrCRL) {
		t.Fatalf("stalled fetch: %v, want ErrCRL", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("stalled fetch took %s", elapsed)
	}
}

func TestListenerReloadKeepsLastGoodMaterial(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	path := adsync.APIPrefix + "/services"
	if response, _, err := tracedGet(client, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("initial request: %v %v", response, err)
	}

	serverCert, err := os.ReadFile(server.pki.serverCertFile)
	if err != nil {
		t.Fatalf("read server certificate: %v", err)
	}
	ca, err := os.ReadFile(server.pki.caFile)
	if err != nil {
		t.Fatalf("read client CA: %v", err)
	}
	crl, err := os.ReadFile(server.pki.crlFile)
	if err != nil {
		t.Fatalf("read CRL: %v", err)
	}
	writeFile(t, filepath.Dir(server.pki.serverCertFile), "server.pem", []byte("garbage"))
	if err := server.listener.Reload(); !errors.Is(err, ErrTLSMaterial) {
		t.Fatalf("invalid server certificate: %v", err)
	}
	if response, _, err := tracedGet(client, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request with invalid replacement: %v %v", response, err)
	}

	if err := os.WriteFile(server.pki.serverCertFile, serverCert, 0o600); err != nil {
		t.Fatalf("restore server certificate: %v", err)
	}
	if err := os.WriteFile(server.pki.caFile, []byte("garbage"), 0o600); err != nil {
		t.Fatalf("break client CA: %v", err)
	}
	if err := server.listener.Reload(); !errors.Is(err, ErrTLSMaterial) {
		t.Fatalf("invalid client CA: %v", err)
	}
	if err := os.WriteFile(server.pki.caFile, ca, 0o600); err != nil {
		t.Fatalf("restore client CA: %v", err)
	}

	if err := os.WriteFile(server.pki.crlFile, []byte("garbage"), 0o600); err != nil {
		t.Fatalf("break CRL: %v", err)
	}
	if err := server.listener.Reload(); !errors.Is(err, ErrCRL) {
		t.Fatalf("invalid CRL: %v", err)
	}
	if err := os.WriteFile(server.pki.crlFile, crl, 0o600); err != nil {
		t.Fatalf("restore CRL: %v", err)
	}
	if response, _, err := tracedGet(client, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request after restoring material: %v %v", response, err)
	}
}

func TestListenerCAChangeRotatesTrust(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	oldClient := server.defaultClient()
	path := adsync.APIPrefix + "/services"
	if response, _, err := tracedGet(oldClient, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("old client: %v %v", response, err)
	}

	now := time.Now()
	newCA, newCAKey := newTestCA(t, "rotated client CA", now)
	newClientCert, newClientKey := issueTestCertificate(t, newCA, newCAKey, certificateTemplate{
		serial: 4001, common: "downstream-rotated", notBefore: now.Add(-time.Hour),
		notAfter: now.Add(time.Hour), client: true,
	})
	directory := filepath.Dir(server.pki.caFile)
	newClientFile := writeFile(t, directory, "new-client.pem", newClientCert)
	newClientKeyFile := writeFile(t, directory, "new-client-key.pem", newClientKey)
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(1), ThisUpdate: now.Add(-time.Hour), NextUpdate: now.Add(time.Hour),
	}, newCA, newCAKey)
	if err != nil {
		t.Fatalf("create new CRL: %v", err)
	}
	// The listener reads its configured paths; rotating the CA means
	// replacing those files.
	if err := os.WriteFile(server.pki.caFile,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: newCA.Raw}), 0o600); err != nil {
		t.Fatalf("write new client CA: %v", err)
	}
	if err := os.WriteFile(server.pki.crlFile,
		pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write new CRL: %v", err)
	}
	if err := server.listener.Reload(); err != nil {
		t.Fatalf("CA reload: %v", err)
	}
	if _, err := oldClient.Get(server.address + path); err == nil {
		t.Fatal("a certificate from the replaced CA was accepted")
	}
	newClient := server.client(newClientFile, newClientKeyFile)
	if response, _, err := tracedGet(newClient, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("rotated client: %v %v", response, err)
	}
}

func TestListenerServerCertificateReloadKeepsConnections(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	path := adsync.APIPrefix + "/services"
	if response, _, err := tracedGet(client, server.address+path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("initial request: %v %v", response, err)
	}

	now := time.Now()
	serverCert, serverKey := issueTestCertificate(t, server.pki.root, server.pki.rootKey, certificateTemplate{
		serial: 2002, common: "relay-v2.test", notBefore: now.Add(-time.Minute),
		notAfter: now.Add(24 * time.Hour), server: true,
	})
	directory := filepath.Dir(server.pki.serverCertFile)
	writeFile(t, directory, "server.pem", serverCert)
	writeFile(t, directory, "server-key.pem", serverKey)
	if err := server.listener.Reload(); err != nil {
		t.Fatalf("server certificate reload: %v", err)
	}

	response, reused, err := tracedGet(client, server.address+path)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request after server certificate reload: %v %v", response, err)
	}
	if !reused {
		t.Error("a server certificate reload closed the established connection")
	}

	fresh := server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	response, _, err = tracedGet(fresh, server.address+path)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("fresh handshake: %v %v", response, err)
	}
	if len(response.TLS.PeerCertificates) == 0 || response.TLS.PeerCertificates[0].SerialNumber.Cmp(big.NewInt(2002)) != 0 {
		t.Error("a fresh handshake did not use the reloaded certificate")
	}
}

func TestListenerDisablesSessionResumption(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:            server.pki.roots,
			ClientSessionCache: tls.NewLRUClientSessionCache(4),
		},
	}
	certificate, err := tls.LoadX509KeyPair(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("load client certificate: %v", err)
	}
	transport.TLSClientConfig.Certificates = []tls.Certificate{certificate}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	path := server.address + adsync.APIPrefix + "/services"
	for attempt := 0; attempt < 2; attempt++ {
		response, err := client.Get(path)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		_ = response.Body.Close()
		if response.TLS.DidResume {
			t.Fatalf("attempt %d resumed a session", attempt)
		}
		transport.CloseIdleConnections()
	}
}

func TestListenerWatchReloadsOnSignal(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	go server.listener.Watch(ctx, signals)

	now := time.Now()
	server.pki.writeCRL(t, 2, now.Add(-time.Hour), now.Add(time.Hour), big.NewInt(3001))
	signals <- syscallSignal{}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := server.defaultClient().Get(server.address + adsync.APIPrefix + "/services"); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a signal did not reload the revoked CRL")
}

// syscallSignal satisfies os.Signal for the watch test without depending on a
// platform signal number.
type syscallSignal struct{}

func (syscallSignal) String() string { return "sighup" }
func (syscallSignal) Signal()        {}

func TestListenerWatchReloadsOnThePollInterval(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	server.listener.config.ReloadInterval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.listener.Watch(ctx, nil)

	time.Sleep(20 * time.Millisecond)
	now := time.Now()
	server.pki.writeCRL(t, 2, now.Add(-time.Hour), now.Add(time.Hour), big.NewInt(3001))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := server.defaultClient().Get(server.address + adsync.APIPrefix + "/services"); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the poll interval did not reload the revoked CRL")
}

func TestListenerCloseEndsServing(t *testing.T) {
	pki := newTestPKI(t)
	listener, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: pki.caFile,
		CRLFiles:     []string{pki.crlFile},
	}, http.NotFoundHandler())
	if err != nil {
		t.Fatalf("new listener: %v", err)
	}
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- listener.Serve(raw) }()
	time.Sleep(20 * time.Millisecond)
	if err := listener.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve after close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop")
	}
	if err := listener.Reload(); !errors.Is(err, net.ErrClosed) {
		t.Errorf("reload after close: %v", err)
	}
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := listener.Serve(second); !errors.Is(err, net.ErrClosed) {
		t.Errorf("serve on a closed listener: %v", err)
	}
	if err := listener.ListenAndServe(); err == nil {
		t.Error("ListenAndServe on a closed listener succeeded")
	}
}

func TestListenerListenAndServeInvalidEndpoint(t *testing.T) {
	pki := newTestPKI(t)
	listener, err := NewListener(ListenerConfig{
		Endpoint:     "not-an-address",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: pki.caFile,
		CRLFiles:     []string{pki.crlFile},
	}, http.NotFoundHandler())
	if err != nil {
		t.Fatalf("new listener: %v", err)
	}
	defer listener.Close()
	if err := listener.ListenAndServe(); err == nil {
		t.Error("an invalid endpoint was accepted")
	}
}

func TestVerifyConnectionRejectsStaleMaterial(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	listener := server.listener
	listener.mu.Lock()
	old := listener.generation
	listener.mu.Unlock()

	now := time.Now()
	server.pki.writeCRL(t, 2, now.Add(-time.Hour), now.Add(time.Hour))
	if err := listener.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	listener.mu.Lock()
	current := listener.generation
	listener.mu.Unlock()
	if current == old {
		t.Fatal("reload kept the same generation after a CRL change")
	}
	if err := listener.verifyConnection(old, nil, tls.ConnectionState{}); !errors.Is(err, ErrStaleTLSGeneration) {
		t.Errorf("stale generation: %v", err)
	}
	if err := listener.verifyConnection(current, nil, tls.ConnectionState{}); !errors.Is(err, ErrTLSMaterial) {
		t.Errorf("empty chain: %v", err)
	}
}

func TestListenerNoOpReloadKeepsConnections(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	path := server.address + adsync.APIPrefix + "/services"
	if response, _, err := tracedGet(client, path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("initial request: %v %v", response, err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := server.listener.Reload(); err != nil {
			t.Fatalf("no-op reload %d: %v", attempt, err)
		}
	}
	response, reused, err := tracedGet(client, path)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request after no-op reloads: %v %v", response, err)
	}
	if !reused {
		t.Error("a no-op reload closed the established connection")
	}
}

func TestListenerHandshakesRacingReloads(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	path := server.address + adsync.APIPrefix + "/services"

	var wait sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for attempt := 0; attempt < 10; attempt++ {
				if response, err := client.Get(path); err == nil {
					_ = response.Body.Close()
				}
			}
		}()
	}
	now := time.Now()
	for attempt := int64(2); attempt < 12; attempt++ {
		server.pki.writeCRL(t, attempt, now.Add(-time.Hour), now.Add(time.Hour))
		if err := server.listener.Reload(); err != nil {
			t.Fatalf("reload %d: %v", attempt, err)
		}
	}
	wait.Wait()
	fresh := server.client(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	if response, _, err := tracedGet(fresh, path); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request after the race: %v %v", response, err)
	}
}

func TestCRLComparisonAndMerge(t *testing.T) {
	now := time.Now()
	older := &crlEntry{number: big.NewInt(1), thisUpdate: now.Add(-2 * time.Hour), raw: []byte("a")}
	newer := &crlEntry{number: big.NewInt(2), thisUpdate: now.Add(-time.Hour), raw: []byte("b")}
	sameTimeHigherNumber := &crlEntry{number: big.NewInt(3), thisUpdate: older.thisUpdate, raw: []byte("c")}
	if !crlNewer(newer, older) || crlNewer(older, newer) {
		t.Error("CRL number comparison")
	}
	if !crlNewer(sameTimeHigherNumber, older) {
		t.Error("a higher number with the same ThisUpdate is newer")
	}
	if !sameCRLs([]*crlEntry{older, newer}, []*crlEntry{newer, older}) {
		t.Error("CRL sets in a different order differ")
	}
	if sameCRLs([]*crlEntry{older}, []*crlEntry{older, newer}) {
		t.Error("different CRL set sizes compared equal")
	}

	key := issuerKey{subject: "s", publicKey: "k"}
	newer.issuer = key
	older.issuer = key
	previous := &tlsGeneration{crls: []*crlEntry{newer}}
	// The newest CRL replaces an older one for the same issuer, and a
	// previous CRL is dropped when its issuer is no longer trusted.
	merged := mergeCRLs(previous, []*crlEntry{older}, map[issuerKey]struct{}{key: {}})
	if len(merged) != 1 || string(merged[0].raw) != "b" {
		t.Errorf("older candidate was not dropped: %d", len(merged))
	}
	if len(mergeCRLs(previous, nil, map[issuerKey]struct{}{})) != 0 {
		t.Error("an untrusted issuer's previous CRL was kept")
	}
	// Equal CRLs from two sources are deduplicated.
	if len(mergeCRLs(previous, []*crlEntry{newer}, map[issuerKey]struct{}{key: {}})) != 1 {
		t.Error("an equal CRL was duplicated")
	}
}

func TestMergeCRLsUnionsSources(t *testing.T) {
	now := time.Now()
	issuer := issuerKey{subject: "s", publicKey: "k", name: "CN=test"}
	trusted := map[issuerKey]struct{}{issuer: {}}
	fromFile := &crlEntry{raw: []byte("file"), issuer: issuer, source: "file:/a.crl",
		number: big.NewInt(3), thisUpdate: now.Add(-time.Hour)}
	fromURL := &crlEntry{raw: []byte("url"), issuer: issuer, source: "url:https://crl/a",
		number: big.NewInt(7), thisUpdate: now.Add(-time.Hour)}
	merged := mergeCRLs(nil, []*crlEntry{fromFile, fromURL}, trusted)
	if len(merged) != 2 {
		t.Fatalf("current CRLs from two sources were not unioned: %d", len(merged))
	}

	// A source that serves an older CRL does not replace its current one.
	previous := &tlsGeneration{crls: []*crlEntry{fromURL}}
	older := &crlEntry{raw: []byte("older"), issuer: issuer, source: fromURL.source,
		number: big.NewInt(6), thisUpdate: now.Add(-2 * time.Hour)}
	merged = mergeCRLs(previous, []*crlEntry{older}, trusted)
	if len(merged) != 1 || string(merged[0].raw) != "url" {
		t.Fatalf("older same-source CRL replaced a newer one: %d", len(merged))
	}

	// A newer CRL from the same source supersedes it.
	newer := &crlEntry{raw: []byte("newer"), issuer: issuer, source: fromURL.source,
		number: big.NewInt(8), thisUpdate: now.Add(-time.Minute)}
	merged = mergeCRLs(previous, []*crlEntry{newer}, trusted)
	if len(merged) != 1 || string(merged[0].raw) != "newer" {
		t.Fatalf("newer same-source CRL did not supersede: %d", len(merged))
	}
}

func TestParseCRLLowLevel(t *testing.T) {
	pki := newTestPKI(t)
	now := time.Now()
	caCerts := []*x509.Certificate{pki.root}
	crlData := mustRead(t, pki.crlFile)

	entries, err := parseCRLs(crlData, "file", now, caCerts)
	if err != nil || len(entries) != 1 {
		t.Fatalf("PEM CRL: %v %d", err, len(entries))
	}
	block, _ := pem.Decode(crlData)
	if block == nil {
		t.Fatal("decode CRL PEM")
	}
	entries, err = parseCRLs(block.Bytes, "der", now, caCerts)
	if err != nil || len(entries) != 1 {
		t.Fatalf("DER CRL: %v %d", err, len(entries))
	}
	// A PEM file with two CRLs yields both.
	double := append(append([]byte(nil), crlData...), crlData...)
	entries, err = parseCRLs(double, "double", now, caCerts)
	if err != nil || len(entries) != 2 {
		t.Fatalf("double PEM CRL: %v %d", err, len(entries))
	}
	if _, err := parseCRLs([]byte("garbage"), "garbage", now, caCerts); !errors.Is(err, ErrCRL) {
		t.Errorf("garbage CRL: %v", err)
	}
	if _, err := parseCRLs(nil, "empty", now, caCerts); !errors.Is(err, ErrCRL) {
		t.Errorf("empty CRL: %v", err)
	}
	// An unknown non-critical extension does not restrict the CRL's scope, so
	// it stays usable.
	annotatedDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(21), ThisUpdate: now.Add(-time.Hour), NextUpdate: now.Add(time.Hour),
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 3, 4, 6}}},
	}, pki.root, pki.rootKey)
	if err != nil {
		t.Fatalf("create annotated CRL: %v", err)
	}
	entries, err = parseCRLs(annotatedDER, "annotated", now, caCerts)
	if err != nil || len(entries) != 1 {
		t.Fatalf("CRL with a non-critical extension: %v %d", err, len(entries))
	}
	if got := subjectName(pki.root); !strings.Contains(got, "adsproxy test CA") {
		t.Errorf("subjectName = %q", got)
	}
	emptySubject := &x509.Certificate{}
	if got := subjectName(emptySubject); !strings.Contains(got, "empty subject") {
		t.Errorf("empty subjectName = %q", got)
	}
	if clientBlock, _ := pem.Decode(pki.clientCertPEM); clientBlock != nil {
		clientCertificate, err := x509.ParseCertificate(clientBlock.Bytes)
		if err != nil {
			t.Fatalf("parse client certificate: %v", err)
		}
		if !hasUsage(clientCertificate, x509.ExtKeyUsageClientAuth) {
			t.Error("clientAuth usage was not detected")
		}
		if hasUsage(clientCertificate, x509.ExtKeyUsageServerAuth) {
			t.Error("a clientAuth-only certificate allows serverAuth")
		}
	}
	if !hasUsage(pki.root, x509.ExtKeyUsageTimeStamping) {
		t.Error("a certificate with no EKU does not allow every usage")
	}
	if got := joinComma([]string{"a", "b"}); got != "a, b" {
		t.Errorf("joinComma = %q", got)
	}
	if got := joinComma(nil); got != "" {
		t.Errorf("joinComma(nil) = %q", got)
	}
}

func TestStallWatchStops(t *testing.T) {
	cancelled := make(chan struct{}, 1)
	watch := newStallWatch(100*time.Millisecond, func() { cancelled <- struct{}{} })
	for attempt := 0; attempt < 3; attempt++ {
		watch.progress()
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case <-cancelled:
		t.Fatal("progress did not reset the watchdog")
	case <-time.After(10 * time.Millisecond):
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the watchdog did not fire")
	}
	watch.stop()

	stopped := make(chan struct{}, 1)
	stoppedWatch := newStallWatch(10*time.Millisecond, func() { stopped <- struct{}{} })
	stoppedWatch.stop()
	time.Sleep(30 * time.Millisecond)
	select {
	case <-stopped:
		t.Fatal("a stopped watchdog fired")
	default:
	}
}

func TestProgressReader(t *testing.T) {
	watch := newStallWatch(time.Hour, func() { t.Error("watchdog fired") })
	defer watch.stop()
	reader := &progressReader{reader: strings.NewReader("payload"), watch: watch}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "payload" {
		t.Fatalf("read: %q %v", data, err)
	}
}

func TestEmptyClientCABundle(t *testing.T) {
	pki := newTestPKI(t)
	directory := t.TempDir()
	path := writeFile(t, directory, "empty.pem", nil)
	if _, _, err := loadClientCA(path); !errors.Is(err, ErrTLSMaterial) {
		t.Errorf("empty bundle: %v", err)
	}
	badCA := writeFile(t, directory, "non-ca.pem", pki.clientCertPEM)
	if _, _, err := loadClientCA(badCA); !errors.Is(err, ErrTLSMaterial) {
		t.Errorf("non-CA bundle: %v", err)
	}
	if _, err := loadServerCertificate(path, path, time.Now()); !errors.Is(err, ErrTLSMaterial) {
		t.Errorf("empty server certificate: %v", err)
	}
	if _, err := loadServerCertificate(pki.serverCertFile, pki.serverKeyFile, time.Now().Add(48*time.Hour)); !errors.Is(err, ErrTLSMaterial) {
		t.Errorf("expired server certificate: %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// deltaIndicator builds the delta CRL indicator extension.
func deltaIndicator() pkix.Extension {
	der, _ := asn1.Marshal(big.NewInt(1))
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 27}, Critical: true, Value: der}
}

// issuingDistributionPoint builds the CRL scope extension with
// onlyContainsCACerts set, so the CRL does not cover client leaf certificates.
func issuingDistributionPoint() pkix.Extension {
	der, _ := asn1.Marshal(struct {
		OnlyContainsCACerts bool `asn1:"optional,tag:1"`
	}{OnlyContainsCACerts: true})
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 28}, Critical: true, Value: der}
}

// crlWithNumber returns a CRL signed by the test CA.
func crlWithNumber(t *testing.T, pki *testPKI, number int64, nextUpdate time.Time) []byte {
	t.Helper()
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(number), ThisUpdate: time.Now().Add(-time.Hour), NextUpdate: nextUpdate,
	}, pki.root, pki.rootKey)
	if err != nil {
		t.Fatalf("create CRL: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der})
}

// crlURLState is one HTTPS CRL endpoint whose behavior a test can change.
type crlURLState struct {
	mu              sync.Mutex
	status          int
	body            []byte
	etag            string
	redirect        bool
	stall           bool
	requests        int
	lastIfNoneMatch string
}

func (s *crlURLState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests++
	s.lastIfNoneMatch = r.Header.Get("If-None-Match")
	status, body, etag := s.status, s.body, s.etag
	redirect, stall := s.redirect, s.stall
	s.mu.Unlock()
	if stall {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
		return
	}
	if redirect {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
		return
	}
	if status != 0 && status != http.StatusOK {
		w.WriteHeader(status)
		return
	}
	if etag != "" && r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	if status == http.StatusOK {
		w.WriteHeader(status)
	}
	_, _ = w.Write(body)
}

func startCRLServer(t *testing.T, pki *testPKI, handler http.Handler) string {
	t.Helper()
	certPEM, keyPEM := issueTestCertificate(t, pki.root, pki.rootKey, certificateTemplate{
		serial: 9001, common: "crl.test", notBefore: time.Now().Add(-time.Hour),
		notAfter: time.Now().Add(24 * time.Hour), server: true,
	})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("CRL server certificate: %v", err)
	}
	server := &http.Server{
		Handler:   handler,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{certificate}},
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.ServeTLS(listener, "", "") }()
	t.Cleanup(func() { _ = server.Close() })
	return "https://" + listener.Addr().String() + "/clients.crl"
}

func newURLListener(t *testing.T, pki *testPKI, address string) *Listener {
	t.Helper()
	listener, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: pki.caFile,
		CRLURLs:      []string{address},
	}, http.NotFoundHandler())
	if err != nil {
		t.Fatalf("new URL listener: %v", err)
	}
	return listener
}

// tracedGet issues a GET and reports whether it reused an established
// connection.
func tracedGet(client *http.Client, target string) (*http.Response, bool, error) {
	var reused atomic.Bool
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused.Store(info.Reused) },
	}
	request, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(context.Background(), trace), http.MethodGet, target, nil)
	if err != nil {
		return nil, false, err
	}
	response, err := client.Do(request)
	if response != nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	return response, reused.Load(), err
}

// TestListenerRequiresFreshCRLForEveryChainIssuer uses a client certificate
// issued by an intermediate CA: both the intermediate (for the leaf) and the
// root (for the intermediate, which is also a trust anchor here) need fresh
// revocation coverage.
func TestListenerRequiresFreshCRLForEveryChainIssuer(t *testing.T) {
	pki := newTestPKI(t)
	now := time.Now()
	intermediate, intermediateKey, intermediatePEM := issueIntermediateCA(t, pki.root, pki.rootKey, now)
	leafPEM, leafKeyPEM := issueTestCertificate(t, intermediate, intermediateKey, certificateTemplate{
		serial: 5001, common: "downstream-intermediate", notBefore: now.Add(-time.Hour),
		notAfter: now.Add(24 * time.Hour), client: true,
	})
	directory := t.TempDir()
	clientCertFile := writeFile(t, directory, "client-chain.pem",
		append(append([]byte(nil), leafPEM...), intermediatePEM...))
	clientKeyFile := writeFile(t, directory, "client-chain-key.pem", leafKeyPEM)
	bundleFile := writeFile(t, directory, "bundle.pem",
		append(mustRead(t, pki.caFile), intermediatePEM...))
	intermediateCRLFile := writeFile(t, directory, "intermediate.crl.pem",
		crlSignedBy(t, intermediate, intermediateKey, 1, now.Add(-time.Hour),
			now.Add(24*time.Hour), nil))

	// Startup fails when the intermediate has no fresh CRL of its own.
	if _, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: bundleFile,
		CRLFiles:     []string{pki.crlFile},
	}, http.NotFoundHandler()); !errors.Is(err, ErrCRL) || !strings.Contains(err.Error(), "intermediate") {
		t.Fatalf("missing intermediate CRL: %v", err)
	}

	listener, err := NewListener(ListenerConfig{
		Endpoint:     "127.0.0.1:0",
		CertPath:     pki.serverCertFile,
		KeyPath:      pki.serverKeyFile,
		ClientCAPath: bundleFile,
		CRLFiles:     []string{pki.crlFile, intermediateCRLFile},
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err != nil {
		t.Fatalf("new listener: %v", err)
	}
	defer listener.Close()
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- listener.Serve(raw) }()
	defer func() {
		listener.Close()
		<-done
	}()
	address := "https://" + raw.Addr().String() + "/"

	client := intermediateClient(t, pki, clientCertFile, clientKeyFile)
	if response, err := client.Get(address); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("client behind an intermediate: %v %v", response, err)
	}

	// Revoking the leaf in the intermediate's CRL rejects it.
	leaf, _ := parseCertificatePEM(t, leafPEM)
	crlSigned := crlSignedBy(t, intermediate, intermediateKey, 2, time.Now().Add(-time.Hour),
		time.Now().Add(24*time.Hour), []*big.Int{leaf.SerialNumber})
	if err := os.WriteFile(intermediateCRLFile, crlSigned, 0o600); err != nil {
		t.Fatalf("write intermediate CRL: %v", err)
	}
	if err := listener.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	client = intermediateClient(t, pki, clientCertFile, clientKeyFile)
	if _, err := client.Get(address); err == nil {
		t.Fatal("a client revoked by its intermediate CA was accepted")
	}
}

func intermediateClient(t *testing.T, pki *testPKI, certFile, keyFile string) *http.Client {
	t.Helper()
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("load client chain: %v", err)
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:      pki.roots,
			Certificates: []tls.Certificate{certificate},
		}},
		Timeout: 10 * time.Second,
	}
}

func parseCertificatePEM(t *testing.T, data []byte) (*x509.Certificate, []byte) {
	t.Helper()
	block, rest := pem.Decode(data)
	if block == nil {
		t.Fatal("no certificate in PEM")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return certificate, rest
}

func crlSignedBy(t *testing.T, ca *x509.Certificate, key *ecdsa.PrivateKey, number int64,
	thisUpdate, nextUpdate time.Time, revoked []*big.Int) []byte {
	t.Helper()
	entries := make([]x509.RevocationListEntry, 0, len(revoked))
	for _, serial := range revoked {
		entries = append(entries, x509.RevocationListEntry{SerialNumber: serial, RevocationTime: thisUpdate})
	}
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:                    big.NewInt(number),
		ThisUpdate:                thisUpdate,
		NextUpdate:                nextUpdate,
		RevokedCertificateEntries: entries,
	}, ca, key)
	if err != nil {
		t.Fatalf("create CRL: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der})
}
