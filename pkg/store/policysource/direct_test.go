// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	speedlestore "github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgate"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
)

const testAPIKey = "spdl_01HF7YAT000000000000000K01_directModeSecretValue0123456789"

// directCloud turns the e2e Cloud fixture into a direct-mode source: it accepts
// only the API key, for synchronization and forwarding alike, and signs
// nothing.
func directCloud(t *testing.T) *e2eCloud {
	t.Helper()
	cloud := newE2ECloud(t)
	cloud.mu.Lock()
	cloud.credential = testAPIKey
	cloud.fallbackCredentials = map[string]struct{}{testAPIKey: {}}
	cloud.unsigned = true
	cloud.mu.Unlock()
	return cloud
}

// startDirectADS builds the registered store from a direct-mode configuration
// (no credential file, no public keys) behind the production evaluator and
// REST router.
func startDirectADS(t *testing.T, cloud *e2eCloud, cacheDir string) (*Store, *httptest.Server) {
	t.Helper()
	properties := map[string]interface{}{
		UpstreamURLKey:       cloud.server.URL,
		APIKeyKey:            testAPIKey,
		CAFileKey:            cloud.caFile,
		CacheDirKey:          cacheDir,
		TransportKey:         string(adsync.TransportWebSocket),
		HeartbeatIntervalKey: "15s",
		StaleAfterKey:        "30s",
		StopAfterKey:         "60s",
	}
	manager, err := speedlestore.NewStore(StoreType, properties)
	if err != nil {
		t.Fatalf("build direct-mode store: %v", err)
	}
	ads := manager.(*Store)
	engine, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, ads)
	if err != nil {
		t.Fatalf("build evaluator: %v", err)
	}
	evaluator := engine.(*eval.PolicyEvalImpl)
	t.Cleanup(func() { _ = evaluator.Close() })
	router, err := adsrest.NewRouter(evaluator)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}
	server := httptest.NewTLSServer(router)
	t.Cleanup(server.Close)
	return ads, server
}

func directIsAllowed(t *testing.T, server *httptest.Server, check e2eCheck) (int, http.Header, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+svcs.PolicyAtzPath+"is-allowed", bytes.NewReader(check.body(t)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("is-allowed: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, response.Header.Clone(), body
}

func directRequests(cloud *e2eCloud) []e2eRequest {
	cloud.mu.Lock()
	defer cloud.mu.Unlock()
	return append([]e2eRequest(nil), cloud.requests...)
}

func directHellos(cloud *e2eCloud) [][]byte {
	cloud.mu.Lock()
	defer cloud.mu.Unlock()
	return append([][]byte(nil), cloud.hellos...)
}

func readInstanceID(t *testing.T, cacheDir string) string {
	t.Helper()
	path := filepath.Join(cacheDir, adsync.InstanceIDFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("instance id file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("instance id file mode = %v, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(string(data))
	if !adsync.ValidInstanceID(id) {
		t.Fatalf("instance id %q is not a UUIDv7", id)
	}
	return id
}

var aliceReadsBooks = e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}

// TestDirectModeRegistersSyncsAndServes runs the documented direct-mode
// configuration end to end: it registers by instance ID in the channel hello,
// sends the key and instance header on every request, syncs unsigned data
// without public keys, never polls discovery, reconciles on acks and keeps its
// instance ID across a restart.
func TestDirectModeRegistersSyncsAndServes(t *testing.T) {
	cloud := directCloud(t)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	ads, server := startDirectADS(t, cloud, cacheDir)
	e2eWaitAllInSync(t, &e2eAgent{store: ads}, "books", "orders")

	instanceID := readInstanceID(t, cacheDir)
	for _, request := range directRequests(cloud) {
		if request.Authorization != "Bearer "+testAPIKey || request.Instance != instanceID {
			t.Fatalf("request %s carried the wrong authorization or instance %q", request.Path, request.Instance)
		}
		if request.Path == adsync.APIPrefix+"/services" {
			t.Fatal("direct mode polled the discovery endpoint")
		}
	}
	hellos := directHellos(cloud)
	if len(hellos) == 0 {
		t.Fatal("no hello received")
	}
	var hello map[string]interface{}
	if err := json.Unmarshal(hellos[0], &hello); err != nil {
		t.Fatal(err)
	}
	if hello["instanceId"] != instanceID || hello["agentVersion"] == "" {
		t.Fatalf("hello = %s", hellos[0])
	}
	if _, found := hello["credentialId"]; found {
		t.Fatalf("hello carries a credential id: %s", hellos[0])
	}
	if chain, ok := hello["chain"].([]interface{}); !ok || len(chain) != 0 {
		t.Fatalf("hello chain = %v", hello["chain"])
	}
	if host, _ := os.Hostname(); host != "" && hello["hostName"] != host {
		t.Fatalf("hello hostName = %v, want %q", hello["hostName"], host)
	}

	status, header, body := directIsAllowed(t, server, aliceReadsBooks)
	if status != http.StatusOK || !e2eDecodeResponse(t, body).Allowed || header.Get(svcs.DecisionSourceHeader) != "" {
		t.Fatalf("local decision: %d %s %v", status, body, header)
	}

	// A change confirmed by an ack alone (the notification was lost).
	cloud.publish(e2eServiceBooks, e2eUpsertAuthz("p-books-dave-read", "grant user dave read /books/1"))
	cloud.pushAck()
	e2eWaitServiceRevision(t, &e2eAgent{store: ads}, "books", 2)

	// A notification fetches the new head before the next ack confirms it.
	changes := cloud.countRequests("/changes")
	revision := cloud.publish(e2eServiceBooks, e2eUpsertAuthz("p-books-erin-read", "grant user erin read /books/1"))
	cloud.notify(e2eServiceBooks, revision)
	e2eWaitFor(t, "the notified head to be fetched", func() bool {
		return cloud.countRequests("/changes") > changes
	})
	cloud.pushAck()
	e2eWaitServiceRevision(t, &e2eAgent{store: ads}, "books", revision)

	// A service missing from the ack is deleted locally, cache file included.
	cloud.removeService(e2eServiceOrders)
	cloud.pushAck()
	e2eWaitFor(t, "orders to be removed", func() bool {
		status, known := ads.Status("orders")
		return known && status.Service.State == adsync.SyncStateRemoved
	})
	e2eWaitFor(t, "the orders cache file to be deleted", func() bool {
		_, err := os.Stat(filepath.Join(cacheDir, "service-"+e2eServiceOrders+".json"))
		return os.IsNotExist(err)
	})

	// A restart keeps the instance ID.
	_ = ads.Close()
	restarted, _ := startDirectADS(t, cloud, cacheDir)
	defer restarted.Close()
	e2eWaitAllInSync(t, &e2eAgent{store: restarted}, "books")
	if readInstanceID(t, cacheDir) != instanceID || restarted.client.InstanceID() != instanceID {
		t.Fatal("restart changed the instance id")
	}
}

// TestDirectModeFailsClosedPerService covers the two reconciliation outcomes
// that fail one service closed without stopping the instance: an ack head
// behind the verified copy, and a state hash that a snapshot reload cannot
// reproduce. Both clear once the ack agrees again.
func TestDirectModeFailsClosedPerService(t *testing.T) {
	cloud := directCloud(t)
	ads, server := startDirectADS(t, cloud, filepath.Join(t.TempDir(), "cache"))
	agent := &e2eAgent{store: ads}
	e2eWaitAllInSync(t, agent, "books", "orders")
	cloud.publish(e2eServiceBooks, e2eUpsertAuthz("p-books-dave-read", "grant user dave read /books/1"))
	cloud.pushAck()
	e2eWaitServiceRevision(t, agent, "books", 2)

	expectIntegrityDenial := func() {
		t.Helper()
		e2eWaitState(t, agent, "books", adsync.SyncStateSyncing, adsync.SyncReasonIntegrity)
		status, _, body := directIsAllowed(t, server, aliceReadsBooks)
		var unavailable adsrest.UnavailableResponse
		_ = json.Unmarshal(body, &unavailable)
		if status != http.StatusServiceUnavailable || unavailable.Error != "authorization_unavailable" ||
			unavailable.Reason != string(adsync.ReasonIntegrityFailure) {
			t.Fatalf("failed service answered %d %s", status, body)
		}
		if ads.Instance().Liveness != adsync.LivenessLive {
			t.Fatal("a per-service failure stopped the instance")
		}
		e2eWaitState(t, agent, "orders", adsync.SyncStateInSync, "")
	}

	// Ahead of the ack: never rolled back, denied until the ack catches up.
	cloud.mu.Lock()
	cloud.ackHead = map[string]uint64{e2eServiceBooks: 1}
	cloud.mu.Unlock()
	cloud.pushAck()
	expectIntegrityDenial()
	cloud.mu.Lock()
	cloud.ackHead = nil
	cloud.mu.Unlock()
	cloud.pushAck()
	e2eWaitServiceRevision(t, agent, "books", 2)

	// Same revision, different hash: the reloaded snapshot still disagrees.
	snapshots := cloud.countRequests("/snapshot")
	cloud.mu.Lock()
	cloud.ackHash = map[string]adsync.Hash{e2eServiceBooks: adsync.Hash{1}}
	cloud.mu.Unlock()
	cloud.pushAck()
	expectIntegrityDenial()
	if cloud.countRequests("/snapshot") == snapshots {
		t.Fatal("a hash mismatch did not reload the snapshot")
	}
	cloud.mu.Lock()
	cloud.ackHash = nil
	cloud.mu.Unlock()
	cloud.pushAck()
	e2eWaitServiceRevision(t, agent, "books", 2)
}

// TestDirectModeForwardsWhileSyncingWithoutLeakingTheKey holds one service's
// first sync. Its requests are forwarded to the upstream with the same key and
// marked as upstream decisions; after the key is revoked every request is
// denied. The key never appears in a log line or an error.
func TestDirectModeForwardsWhileSyncingWithoutLeakingTheKey(t *testing.T) {
	var logs lockedBuffer
	previous := log.StandardLogger().Out
	previousLevel := log.GetLevel()
	log.SetOutput(&logs)
	log.SetLevel(log.DebugLevel)
	defer func() {
		log.SetOutput(previous)
		log.SetLevel(previousLevel)
	}()

	cloud := directCloud(t)
	cloud.hold(e2eServiceBooks)
	ads, server := startDirectADS(t, cloud, "")
	agent := &e2eAgent{store: ads}
	e2eWaitState(t, agent, "books", adsync.SyncStateSyncing, adsync.SyncReasonBootstrap)

	status, header, body := directIsAllowed(t, server, aliceReadsBooks)
	if status != http.StatusOK || !e2eDecodeResponse(t, body).Allowed {
		t.Fatalf("forwarded decision: %d %s", status, body)
	}
	if source := header.Get(svcs.DecisionSourceHeader); source != svcs.DecisionSourceUpstream {
		t.Fatalf("decision source = %q, want upstream", source)
	}
	if cloud.fallbackCount() != 1 {
		t.Fatalf("fallback calls = %d", cloud.fallbackCount())
	}
	instanceID := ads.client.InstanceID()
	for _, request := range directRequests(cloud) {
		if strings.HasPrefix(request.Path, adsync.AuthzCheckAPIPrefix) &&
			(request.Authorization != "Bearer "+testAPIKey || request.Instance != instanceID) {
			t.Fatal("forwarded request did not carry the key and instance id")
		}
	}

	cloud.revoke()
	e2eWaitFor(t, "the revoked instance to stop", func() bool {
		return ads.Instance().Liveness == adsync.LivenessStopped
	})
	status, _, body = directIsAllowed(t, server, aliceReadsBooks)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("stopped instance answered %d %s", status, body)
	}
	cloud.release(e2eServiceBooks)

	if strings.Contains(logs.String(), testAPIKey) || strings.Contains(logs.String(), "directModeSecretValue") {
		t.Fatal("the API key appeared in a log line")
	}
	if !strings.Contains(logs.String(), "every restart registers a new instance") {
		t.Fatal("no warning about the unpersisted instance id")
	}
	if _, err := ads.ForwardFallback(context.Background(), adsync.FallbackIsAllowed, aliceReadsBooks.body(t)); err == nil ||
		strings.Contains(err.Error(), "directModeSecretValue") {
		t.Fatalf("forward error after revocation = %v", err)
	}
}

// TestDirectModeCachedCopyIsNotAuthoritativeBeforeTheFirstAck reopens a
// cache written by an earlier run. Before any ack the cached service is
// tracked as syncing and routed to the upstream, never evaluated locally.
func TestDirectModeCachedCopyIsNotAuthoritativeBeforeTheFirstAck(t *testing.T) {
	cloud := directCloud(t)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	first, _ := startDirectADS(t, cloud, cacheDir)
	e2eWaitAllInSync(t, &e2eAgent{store: first}, "books", "orders")
	_ = first.Close()

	options := Options{
		PolicySourceURL: cloud.server.URL,
		APIKey:          testAPIKey,
		CAFile:          cloud.caFile,
		CacheDir:        cacheDir,
	}
	reopened, err := Open(context.Background(), options)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if reopened.client.InstanceID() != readInstanceID(t, cacheDir) {
		t.Fatal("reopened store did not reuse the instance id")
	}
	status, known := reopened.Status("books")
	if !known || status.Service.State != adsync.SyncStateSyncing {
		t.Fatalf("cached service before the first ack = %+v, %v", status.Service, known)
	}
	if route := adsgate.New(reopened).Route("books"); route.Kind != adsgate.RouteFallback {
		t.Fatalf("cached service routed %+v, want forwarding", route)
	}
	if err := reopened.Sync(context.Background()); err != nil {
		t.Fatalf("sync before the first ack: %v", err)
	}
	if count := cloud.countRequests("/snapshot"); count != 2 {
		t.Fatalf("snapshot pulls = %d; nothing may be pulled before the first ack", count)
	}
}

// TestV17ConfigurationIsUnchanged runs an unchanged Engine v1.7.x store
// configuration (credential file, public keys, separate Cloud fallback)
// through the client and checks the wire behaviour of a credential instance:
// no instance header, the credential in every request, the credential
// instance's hello, and signatures still required.
func TestV17ConfigurationIsUnchanged(t *testing.T) {
	cloud := newE2ECloud(t)
	ads, _ := startProductionADS(t, cloud, nil)
	agent := &e2eAgent{store: ads}
	e2eWaitAllInSync(t, agent, "books", "orders")
	if ads.directMode() || ads.ForwardWhileStale() || !ads.client.SignaturesVerified() {
		t.Fatal("a v1.7.x configuration selected direct mode")
	}
	for _, request := range directRequests(cloud) {
		if request.Instance != "" {
			t.Fatalf("credential instance sent an instance header on %s", request.Path)
		}
	}
	if cloud.countRequests(adsync.APIPrefix+"/services") == 0 {
		t.Fatal("credential instance did not use discovery")
	}
	var hello map[string]interface{}
	hellos := directHellos(cloud)
	if len(hellos) == 0 || json.Unmarshal(hellos[0], &hello) != nil {
		t.Fatal("no hello")
	}
	if hello["instanceId"] != e2eInstanceA {
		t.Fatalf("hello instanceId = %v", hello["instanceId"])
	}
	if _, found := hello["hostName"]; found {
		t.Fatalf("credential hello carries direct-mode fields: %s", hellos[0])
	}
	if chain, _ := hello["chain"].([]interface{}); len(chain) != 1 || chain[0] != e2eInstanceA {
		t.Fatalf("hello chain = %v", hello["chain"])
	}

	// Unsigned data is still rejected.
	cloud.mu.Lock()
	cloud.unsigned = true
	cloud.mu.Unlock()
	cloud.publish(e2eServiceBooks, e2eUpsertAuthz("p-books-dave-read", "grant user dave read /books/1"))
	cloud.pushAck()
	time.Sleep(time.Second)
	if status, _ := ads.Status("books"); status.Service.Revision == 2 {
		t.Fatal("a credential instance installed unsigned data")
	}
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
