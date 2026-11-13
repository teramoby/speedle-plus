// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// cascadePKI exposes an issuer so rotation uses the SAME trust anchor, and
// verification tests can independently vary SAN, EKU and validity.
type cascadePKI struct {
	ca  *x509.Certificate
	key ed25519.PrivateKey
	pem []byte
}

func newCascadePKI(t *testing.T) *cascadePKI {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "cascade CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &cascadePKI{ca: ca, key: private, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}
func (p *cascadePKI) issue(t *testing.T, serial int64, usage x509.ExtKeyUsage, host string, expiry time.Time) (tls.Certificate, []byte, []byte) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "not an instance ID"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: expiry, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else if host != "" {
		template.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, p.ca, public, p.key)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return pair, certPEM, keyPEM
}
func cascadeConfig(t *testing.T, pki *cascadePKI, address string) Config {
	t.Helper()
	dir := t.TempDir()
	_, cert, key := pki.issue(t, 3, x509.ExtKeyUsageClientAuth, "", time.Now().Add(time.Hour))
	return Config{BaseURL: address, InstanceIDDir: filepath.Join(t.TempDir(), "cache"), CAFile: writeTestFile(t, dir, "ca.pem", pki.pem, 0600), ClientCertFile: writeTestFile(t, dir, "client.pem", cert, 0600), ClientKeyFile: writeTestFile(t, dir, "client.key", key, 0600), Retry: RetryPolicy{MaxAttempts: 1}, RequestTimeout: time.Second}
}
func cascadeServer(t *testing.T, pki *cascadePKI, cert tls.Certificate, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	roots := x509.NewCertPool()
	roots.AddCert(pki.ca)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}
func mustCascadeClient(t *testing.T, config Config) *Client {
	t.Helper()
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}
func TestCascadeVerifiesRelay(t *testing.T) {
	for _, test := range []struct {
		name, host string
		usage      x509.ExtKeyUsage
		expiry     time.Time
		wrongCA    bool
	}{
		{"valid", "127.0.0.1", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour), false},
		{"wrong SAN", "elsewhere.example", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour), false},
		{"wrong EKU", "127.0.0.1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour), false},
		{"expired", "127.0.0.1", x509.ExtKeyUsageServerAuth, time.Now().Add(-time.Minute), false},
		{"untrusted", "127.0.0.1", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pki := newCascadePKI(t)
			serverCert, _, _ := pki.issue(t, 2, test.usage, test.host, test.expiry)
			var requests atomic.Int32
			server := cascadeServer(t, pki, serverCert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Header.Get("Authorization") != "" || !ValidInstanceID(r.Header.Get(InstanceHeader)) {
					t.Error("cascade sent token or missing UUIDv7")
				}
				io.WriteString(w, `{"servicesVersion":1,"services":[]}`)
			}))
			config := cascadeConfig(t, pki, server.URL)
			if test.wrongCA {
				config.CAFile = writeTestFile(t, t.TempDir(), "wrong.pem", newCascadePKI(t).pem, 0600)
			}
			client := mustCascadeClient(t, config)
			_, err := client.Services(context.Background())
			if test.name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if requests.Load() != 1 {
					t.Fatal("no request")
				}
			} else if err == nil || requests.Load() != 0 {
				t.Fatalf("invalid relay accepted: %v", err)
			}
		})
	}
}
func TestCascadeCredentialValidation(t *testing.T) {
	pki := newCascadePKI(t)
	valid := cascadeConfig(t, pki, "https://relay.example")
	for _, test := range []struct {
		name   string
		change func(*Config)
	}{
		{"empty host", func(c *Config) { c.BaseURL = "https://:443" }},
		{"missing cert", func(c *Config) { c.ClientCertFile = "" }}, {"missing key", func(c *Config) { c.ClientKeyFile = "" }}, {"missing CA", func(c *Config) { c.CAFile = "" }}, {"missing CacheDir", func(c *Config) { c.InstanceIDDir = "" }},
		{"configured ID", func(c *Config) { c.InstanceID = testInstanceID }}, {"custom transport", func(c *Config) { c.HTTPClient = &http.Client{} }}, {"custom dialer", func(c *Config) { c.Dialer = &websocket.Dialer{} }},
		{"negative reload", func(c *Config) { c.TLSReloadInterval = -time.Second }}, {"unreadable cert", func(c *Config) { c.ClientCertFile += ".absent" }}, {"unreadable CA", func(c *Config) { c.CAFile += ".absent" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := valid
			test.change(&c)
			if _, err := NewClient(c); err == nil {
				t.Fatal("invalid cascade configuration accepted")
			}
		})
	}
	for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
		end := time.Now().Add(time.Hour)
		if usage == x509.ExtKeyUsageClientAuth {
			end = time.Now().Add(-time.Minute)
		}
		_, cert, key := pki.issue(t, 5, usage, "", end)
		writeTestFile(t, filepath.Dir(valid.ClientCertFile), "client.pem", cert, 0600)
		writeTestFile(t, filepath.Dir(valid.ClientKeyFile), "client.key", key, 0600)
		if _, err := NewClient(valid); err == nil {
			t.Fatal("invalid client certificate accepted")
		}
	}
}

func TestCascadeReloadReconnectsHTTPAndWebSocket(t *testing.T) {
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	var requests atomic.Int32
	upgrader := websocket.Upgrader{}
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("cascade bearer header")
		}
		if strings.HasSuffix(r.URL.Path, "/channel") {
			connection, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer connection.Close()
			connection.WriteMessage(websocket.TextMessage, []byte(r.TLS.PeerCertificates[0].SerialNumber.String()))
			for {
				if _, _, err := connection.ReadMessage(); err != nil {
					return
				}
			}
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"servicesVersion":1,"services":[]}`)
	}))
	config := cascadeConfig(t, pki, server.URL)
	client := mustCascadeClient(t, config)
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatal(err)
	}
	header := http.Header{}
	if err := client.authorize(header); err != nil {
		t.Fatal(err)
	}
	id := header.Get(InstanceHeader)
	socket, _, err := client.dialer.Dial("wss"+strings.TrimPrefix(server.URL, "https")+APIPrefix+"/channel", header)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	_, serial, err := socket.ReadMessage()
	if err != nil || string(serial) != "3" {
		t.Fatalf("old cert: %s, %v", serial, err)
	}
	// A malformed replacement is rejected without closing the healthy session.
	writeTestFile(t, filepath.Dir(config.ClientKeyFile), "client.key", []byte("invalid key"), 0600)
	if err := client.ReloadTLS(); err == nil {
		t.Fatal("bad replacement accepted")
	}
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatalf("last generation lost: %v", err)
	}
	_, nextCert, nextKey := pki.issue(t, 4, x509.ExtKeyUsageClientAuth, "", time.Now().Add(time.Hour))
	writeTestFile(t, filepath.Dir(config.ClientCertFile), "client.pem", nextCert, 0600)
	writeTestFile(t, filepath.Dir(config.ClientKeyFile), "client.key", nextKey, 0600)
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	socket.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := socket.ReadMessage(); err == nil {
		t.Fatal("old channel remained open")
	}
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatal(err)
	}
	nextSocket, _, err := client.dialer.Dial("wss"+strings.TrimPrefix(server.URL, "https")+APIPrefix+"/channel", header)
	if err != nil {
		t.Fatal(err)
	}
	defer nextSocket.Close()
	_, serial, err = nextSocket.ReadMessage()
	if err != nil || string(serial) != "4" {
		t.Fatalf("rotated cert: %s, %v", serial, err)
	}
	if again, err := client.InstanceID(); err != nil || again != id {
		t.Fatalf("rotation changed ID: %s, %v", again, err)
	}
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	} // no-op retains live channel
	if err := nextSocket.WriteMessage(websocket.TextMessage, []byte("still open")); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 3 {
		t.Fatalf("HTTP calls: %d", requests.Load())
	}
}

func TestCascadePollAndSignalReload(t *testing.T) {
	for _, useSignal := range []bool{false, true} {
		t.Run(fmt.Sprint(useSignal), func(t *testing.T) {
			pki := newCascadePKI(t)
			config := cascadeConfig(t, pki, "https://relay.example")
			config.TLSReloadInterval = 5 * time.Millisecond
			if useSignal {
				config.TLSReloadInterval = time.Hour
			}
			client := mustCascadeClient(t, config)
			signals := make(chan struct{}, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() { defer close(done); client.WatchTLS(ctx, signals) }()
			_, cert, key := pki.issue(t, 7, x509.ExtKeyUsageClientAuth, "", time.Now().Add(time.Hour))
			writeTestFile(t, filepath.Dir(config.ClientCertFile), "client.pem", cert, 0600)
			writeTestFile(t, filepath.Dir(config.ClientKeyFile), "client.key", key, 0600)
			if useSignal {
				signals <- struct{}{}
			}
			deadline := time.Now().Add(time.Second)
			for {
				pair, err := client.tlsMaterial.config().GetClientCertificate(nil)
				if err != nil {
					t.Fatal(err)
				}
				if pair.Leaf.SerialNumber.Int64() == 7 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("TLS watcher failed to reload")
				}
				time.Sleep(time.Millisecond)
			}
			close(signals)
			cancel()
			<-done
		})
	}
}

func TestDirectModeRejectsCascadeTLSSettings(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.TLSReloadInterval = time.Second },
		func(c *Config) { c.CRLFiles = []string{} },
		func(c *Config) { c.CRLURLs = []string{"http://invalid.example/crl"} },
		func(c *Config) { c.CRLRefreshInterval = time.Hour },
		func(c *Config) { c.CRLMaxAge = 24 * time.Hour },
	} {
		config := Config{BaseURL: "https://source.example", APIKey: testAPIKey}
		change(&config)
		if _, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "cascade") {
			t.Fatalf("ignored direct mode security setting: %v", err)
		}
	}
}

func TestCascadeDialTimeoutBoundsDNSWithoutCallerDeadline(t *testing.T) {
	pki := newCascadePKI(t)
	config := cascadeConfig(t, pki, "https://relay.example")
	config.RequestTimeout = 20 * time.Millisecond
	client := mustCascadeClient(t, config)
	sawDeadline := make(chan bool, 8)
	client.tlsMaterial.dialer.Resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		_, bounded := ctx.Deadline()
		select {
		case sawDeadline <- bounded:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	started := time.Now()
	if _, err := client.tlsMaterial.dialContext(context.Background(), "tcp", "cascade-dial-timeout.invalid:7443"); err == nil {
		t.Fatal("stalled DNS unexpectedly succeeded")
	}
	if time.Since(started) > time.Second {
		t.Fatal("dial exceeded its deadline")
	}
	select {
	case bounded := <-sawDeadline:
		if !bounded {
			t.Fatal("DNS lookup has no dial deadline")
		}
	default:
		t.Fatal("test resolver was not used")
	}
}

func TestMissingAuthenticationFailsBeforeTLSLoading(t *testing.T) {
	for _, cert := range []string{"", "client.pem"} {
		_, err := NewClient(Config{BaseURL: "https://source.example", CAFile: "ca.pem", ClientCertFile: cert, InstanceIDDir: t.TempDir()})
		if err == nil {
			t.Fatal("empty APIKey without complete mTLS credentials accepted")
		}
		for _, setting := range []string{"APIKey", "ClientCertFile", "ClientKeyFile", "CAFile"} {
			if !strings.Contains(err.Error(), setting) {
				t.Fatalf("missing authentication error lacks %s: %v", setting, err)
			}
		}
	}
}

func TestCascadeRejectsExplicitlyEmptyCRLSources(t *testing.T) {
	pki := newCascadePKI(t)
	for _, urls := range []bool{false, true} {
		config := cascadeConfig(t, pki, "https://relay.example")
		if urls {
			config.CRLURLs = []string{}
		} else {
			config.CRLFiles = []string{}
		}
		if _, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "CRL") {
			t.Fatalf("empty CRL settings accepted: %v", err)
		}
	}
}
