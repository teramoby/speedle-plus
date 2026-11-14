// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsproxy"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"sync/atomic"
)

type cascadeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *cascadeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *cascadeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func chainCredentials(t *testing.T) (Options, tls.Certificate, *x509.CertPool) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certPEM)
	return Options{ClientCertFile: certFile, ClientKeyFile: keyFile, CAFile: certFile, StartRuntime: true, ServeDownstream: true, Transport: adsync.TransportWebSocket, RequestTimeout: time.Second, ReconcilePeriod: 50 * time.Millisecond, Retry: adsync.RetryPolicy{MaxAttempts: 1}}, pair, roots
}

func chainListener(t *testing.T, store *Store, cert tls.Certificate, roots *x509.CertPool) *httptest.Server {
	t.Helper()
	return chainProtocolListener(t, store.DownstreamHandler(), cert, roots)
}

func chainProtocolListener(t *testing.T, handler http.Handler, cert tls.Certificate, roots *x509.CertPool) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func chainAck(t *testing.T, client *http.Client, address, instance string) map[string]json.RawMessage {
	t.Helper()
	response, err := client.Post(address+adsync.APIPrefix+"/channel/poll?wait=0", "application/json", strings.NewReader(`{"type":"heartbeat","instanceId":"`+instance+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("ack: %d %s", response.StatusCode, body)
	}
	var ack map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&ack); err != nil {
		t.Fatal(err)
	}
	return ack
}

func TestCascadeThreeLevelRuntimeChainAndFreshness(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	clock := &cascadeClock{now: time.Now()}
	root := fixture.open(func(o *Options) { o.ServeDownstream = true; o.Clock = clock })
	t.Cleanup(func() { root.Close() })
	// Only five seconds of the root freshness window remain when descendants
	// connect. Healthy child channels must not each grant another minute.
	clock.advance(55 * time.Second)
	options, cert, roots := chainCredentials(t)
	rootServer := chainListener(t, root, cert, roots)
	openChild := func(address string) *Store {
		o := options
		o.UpstreamURL = address
		o.CacheDir = filepath.Join(t.TempDir(), "cache")
		child, err := Open(context.Background(), o)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf("child instance=%+v services=%+v upstream=%+v root upstream=%+v root services=%+v", child.Instance(), child.Services(), child.machine.Upstream(), root.machine.Upstream(), root.Services())
			}
			child.Close()
		})
		engine, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, child)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { engine.(*eval.PolicyEvalImpl).Close() })
		e2eWaitFor(t, "child to synchronize", func() bool {
			status, ok := child.machine.Status("books")
			return ok && status.Service.State == adsync.SyncStateInSync && child.machine.Upstream().State == "live"
		})
		return child
	}
	relay := openChild(rootServer.URL)
	relayServer := chainListener(t, relay, cert, roots)
	leaf := openChild(relayServer.URL)
	leafServer := chainListener(t, leaf, cert, roots)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{cert}}}}
	t.Cleanup(client.CloseIdleConnections)
	observer := "0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b99"
	ack := chainAck(t, client, leafServer.URL, observer)
	var chain []string
	if err := json.Unmarshal(ack["chain"], &chain); err != nil {
		t.Fatal(err)
	}
	want := []string{mustInstanceID(t, root), mustInstanceID(t, relay), mustInstanceID(t, leaf)}
	if strings.Join(chain, ",") != strings.Join(want, ",") {
		t.Fatalf("runtime chain = %v, want %v", chain, want)
	}
	var budget int64
	if err := json.Unmarshal(ack["upstreamFreshForMs"], &budget); err != nil {
		t.Fatal(err)
	}
	if budget <= 0 || budget > 5000 {
		t.Fatalf("freshness was renewed at a hop: %dms", budget)
	}
	if root.DownstreamCount() != 1 || relay.DownstreamCount() != 1 {
		t.Fatalf("direct-child counts root=%d relay=%d", root.DownstreamCount(), relay.DownstreamCount())
	}
	for _, ancestor := range want {
		response, err := client.Post(leafServer.URL+adsync.APIPrefix+"/channel/poll?wait=0", "application/json", strings.NewReader(`{"type":"heartbeat","instanceId":"`+ancestor+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 403 || !strings.Contains(string(body), "chain_loop") {
			t.Fatalf("ancestor %s admitted: %d %s", ancestor, response.StatusCode, body)
		}
	}
	clock.advance(6 * time.Second)
	e2eWaitFor(t, "transitive stale state", func() bool {
		return relay.machine.Upstream().State == "stale" && leaf.machine.Upstream().State == "stale"
	})
	for _, address := range []string{rootServer.URL, relayServer.URL, leafServer.URL} {
		response, err := client.Get(address + adsync.APIPrefix + "/services")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 503 || !strings.Contains(string(body), "upstream_stale") {
			t.Fatalf("stale data served: %d %s", response.StatusCode, body)
		}
	}
	if len(leaf.machine.Services()) != 1 || leaf.machine.Services()[0].State == adsync.SyncStateRemoved {
		t.Fatal("stale parent erased retained service")
	}
	if err := root.Confirm(); err != nil {
		t.Fatal(err)
	}
	e2eWaitForWithin(t, 10*time.Second, "transitive freshness recovery", func() bool {
		return relay.machine.Upstream().State == "live" && leaf.machine.Upstream().State == "live"
	})
	// A syncing ancestor returns its state without turning its withheld mirror
	// into an authoritative empty service set at either descendant.
	root.machine.Resync(testServiceID1)
	root.downstream.server.DistributionChange(func() {})
	ack = chainAck(t, client, rootServer.URL, observer)
	if string(ack["upstreamState"]) != `"syncing"` {
		t.Fatalf("syncing ancestor ack: %s", ack["upstreamState"])
	}
	response, err := client.Get(rootServer.URL + adsync.APIPrefix + "/services")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 503 || !strings.Contains(string(body), "upstream_syncing") {
		t.Fatalf("syncing data served: %d %s", response.StatusCode, body)
	}
}

func TestCascadeRestartRejectsHeadBelowDiskCacheBeforePulling(t *testing.T) {
	options, cert, roots := chainCredentials(t)
	options.CacheDir = filepath.Join(t.TempDir(), "cache")
	cache, err := adsync.NewCache(options.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := adsync.Snapshot{Name: "books", Policies: []adsync.Policy{}, RolePolicies: []adsync.Policy{}}
	hash := adsync.StateHash(snapshot)
	retained := &adsync.ServiceState{ServiceID: testServiceID1, SourceID: testServiceID1, Epoch: testServiceID1, Revision: 2, StateHash: hash, CommitHash: hash, Snapshot: snapshot}
	if err := cache.Save(retained); err != nil {
		t.Fatal(err)
	}
	var olderPulls, newerPulls atomic.Int32
	makeRelay := func(id string, revision uint64, pulls *atomic.Int32) *httptest.Server {
		mirror, err := adsproxy.NewMirror(adsproxy.MirrorOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err := mirror.ApplySnapshot(&adsync.SnapshotResponse{ServiceID: testServiceID1, SourceID: testServiceID1, Epoch: testServiceID1, Revision: revision, StateHash: hash, CommitHash: hash, Snapshot: snapshot}); err != nil {
			t.Fatal(err)
		}
		relay, err := adsproxy.NewServer(adsproxy.Config{InstanceID: id, Mirror: mirror})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(relay.Close)
		return chainProtocolListener(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/snapshot") {
				pulls.Add(1)
			}
			relay.ServeHTTP(w, r)
		}), cert, roots)
	}
	primary := makeRelay("0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b51", 1, &olderPulls)
	backup := makeRelay("0193a5c2-7d1e-7b3a-9f10-4c2d8e6a1b52", 2, &newerPulls)
	options.UpstreamURL = primary.URL
	options.UpstreamURLs = []string{primary.URL, backup.URL}
	child, err := Open(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	engine, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, child)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.(*eval.PolicyEvalImpl).Close()
	e2eWaitFor(t, "cached floor to reject the first relay and confirm the backup", func() bool {
		status, known := child.machine.Status("books")
		return known && status.Service.State == adsync.SyncStateInSync && status.Service.Revision == 2
	})
	if olderPulls.Load() != 0 {
		t.Fatal("lower-revision relay reached snapshot loading")
	}
	held, err := cache.Load(testServiceID1)
	if err != nil || held.Revision != 2 {
		t.Fatalf("retained cache changed: %+v %v", held, err)
	}
}
