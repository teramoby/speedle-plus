// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

// End-to-end tests for policy distribution (Engine step E7). An in-process
// Speedle Cloud fixture feeds ADS A, which can proxy to ADS B and C. The tests
// drive the real store, evaluator, REST entry point, notification channel and
// proxy server together, and use an injected clock whenever a design threshold
// would otherwise make the test wait.
//
// The suite is kept inside package policysource because one instance owns one
// state machine: the notification channel feeds exactly the machine the store
// and request gate consult, and a resync command invalidates exactly the copy
// the store holds. Building that composition out of exported pieces would need
// production accessors used nowhere else.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/adsproxy"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
)

const e2eWaitTimeout = 5 * time.Second

func e2eWaitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	e2eWaitForWithin(t, e2eWaitTimeout, what, condition)
}

func e2eWaitForWithin(t *testing.T, timeout time.Duration, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func e2eWaitLiveness(t *testing.T, agent *e2eAgent, liveness adsync.Liveness) {
	t.Helper()
	e2eWaitFor(t, fmt.Sprintf("instance %s to be %s", agent.instanceID, liveness), func() bool {
		return agent.liveness() == liveness
	})
}

func e2eWaitState(t *testing.T, agent *e2eAgent, serviceName string, state adsync.SyncState, reason adsync.SyncReason) {
	t.Helper()
	what := fmt.Sprintf("instance %s service %s to be %s", agent.instanceID, serviceName, state)
	if reason != "" {
		what += " (" + string(reason) + ")"
	}
	e2eWaitFor(t, what, func() bool {
		status, known := agent.status(serviceName)
		return known && status.Service.State == state && status.Service.Reason == reason
	})
}

func e2eWaitAllInSync(t *testing.T, agent *e2eAgent, services ...string) {
	t.Helper()
	e2eWaitLiveness(t, agent, adsync.LivenessLive)
	for _, service := range services {
		e2eWaitState(t, agent, service, adsync.SyncStateInSync, "")
	}
}

func e2eWaitServiceRevision(t *testing.T, agent *e2eAgent, serviceName string, revision uint64) {
	t.Helper()
	e2eWaitFor(t, fmt.Sprintf("instance %s service %s to reach revision %d", agent.instanceID, serviceName, revision), func() bool {
		status, known := agent.status(serviceName)
		return known && status.Service.State == adsync.SyncStateInSync && status.Service.Revision == revision
	})
}

// e2eWaitDecision waits until the instance answers the check with the wanted
// decision, which is a stronger signal than the sync state alone when a source
// change keeps the revision number.
func e2eWaitDecision(t *testing.T, agent *e2eAgent, check e2eCheck, allowed bool) {
	t.Helper()
	e2eWaitFor(t, fmt.Sprintf("instance %s to answer %+v with allowed=%v", agent.instanceID, check, allowed), func() bool {
		status, _, body := agent.isAllowed(check.body(t))
		if status != 200 {
			return false
		}
		return e2eDecodeResponse(t, body).Allowed == allowed
	})
}

// e2eWaitServiceNotFound waits until the evaluator has dropped the service's
// copy, which follows the state machine's removed state through the store's
// watch events.
func e2eWaitServiceNotFound(t *testing.T, agent *e2eAgent, check e2eCheck) adsrest.IsAllowedResponse {
	t.Helper()
	var found adsrest.IsAllowedResponse
	e2eWaitFor(t, fmt.Sprintf("instance %s to drop %s", agent.instanceID, check.Service), func() bool {
		status, _, body := agent.isAllowed(check.body(t))
		if status != 200 {
			return false
		}
		response := e2eDecodeResponse(t, body)
		if response.Allowed || response.Reason != int32(adsapi.SERVICE_NOT_FOUND) {
			return false
		}
		found = response
		return true
	})
	return found
}

func e2eDecodeResponse(t *testing.T, body []byte) adsrest.IsAllowedResponse {
	t.Helper()
	var response adsrest.IsAllowedResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode authorization response %q: %v", body, err)
	}
	return response
}

func e2eRequireAllowed(t *testing.T, agent *e2eAgent, check e2eCheck) adsrest.IsAllowedResponse {
	t.Helper()
	status, header, body := agent.isAllowed(check.body(t))
	if status != 200 {
		t.Fatalf("%s: status %d body %s", agent.instanceID, status, body)
	}
	if source := header.Get(svcs.DecisionSourceHeader); source != "" {
		t.Fatalf("%s: local decision carried decision source %q", agent.instanceID, source)
	}
	response := e2eDecodeResponse(t, body)
	if !response.Allowed {
		t.Fatalf("%s: request %+v was denied: %+v", agent.instanceID, check, response)
	}
	return response
}

func e2eDownstreamCredential(downstreamID, instanceID string, number int) (adsproxy.Credential, string) {
	token := e2eCredential(instanceID, number)
	return adsproxy.Credential{DownstreamID: downstreamID, Token: token}, token
}

func e2eAgentA(t *testing.T, cloud *e2eCloud, clock *testClock) *e2eAgent {
	t.Helper()
	credentialB, _ := e2eDownstreamCredential("B", e2eInstanceB, 2)
	source := cloud.source()
	return newE2EAgent(t, e2eAgentOptions{
		InstanceID: e2eInstanceA,
		Source:     source,
		Fallback:   &source,
		Clock:      clock,
		Chain:      []string{e2eInstanceA},
		Downstream: []adsproxy.Credential{credentialB},
	})
}

func e2eAgentB(t *testing.T, cloud *e2eCloud, upstream *e2eAgent, tokenB string, clock *testClock, cacheDir string) *e2eAgent {
	t.Helper()
	return newE2EAgent(t, e2eAgentOptions{
		InstanceID: e2eInstanceB,
		Source: e2eSource{
			URL:        upstream.server.URL,
			Credential: tokenB,
			CAFile:     upstream.caFile,
			Keys:       cloud.keys,
		},
		Clock:    clock,
		CacheDir: cacheDir,
		Chain:    []string{e2eInstanceA, e2eInstanceB},
	})
}

func e2eCacheDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create private cache directory: %v", err)
	}
	return dir
}

// TestE2EChainFirstSync covers the first synchronization of a three-instance
// chain Cloud -> A -> B -> C. Every instance must reach in_sync, answer the
// same decision locally, and only talk to its own upstream.
func TestE2EChainFirstSync(t *testing.T) {
	cloud := newE2ECloud(t)
	clockA := newTestClock()
	credentialB, tokenB := e2eDownstreamCredential("B", e2eInstanceB, 2)
	a := newE2EAgent(t, e2eAgentOptions{
		InstanceID: e2eInstanceA,
		Source:     cloud.source(),
		Clock:      clockA,
		Chain:      []string{e2eInstanceA},
		Downstream: []adsproxy.Credential{credentialB},
	})
	credentialC, tokenC := e2eDownstreamCredential("C", e2eInstanceC, 3)
	clockB := newTestClock()
	b := newE2EAgent(t, e2eAgentOptions{
		InstanceID: e2eInstanceB,
		Source: e2eSource{
			URL:        a.server.URL,
			Credential: tokenB,
			CAFile:     a.caFile,
			Keys:       cloud.keys,
		},
		Clock:      clockB,
		Chain:      []string{e2eInstanceA, e2eInstanceB},
		Downstream: []adsproxy.Credential{credentialC},
	})
	clockC := newTestClock()
	c := newE2EAgent(t, e2eAgentOptions{
		InstanceID: e2eInstanceC,
		Source: e2eSource{
			URL:        b.server.URL,
			Credential: tokenC,
			CAFile:     b.caFile,
			Keys:       cloud.keys,
		},
		Clock: clockC,
		Chain: []string{e2eInstanceA, e2eInstanceB, e2eInstanceC},
	})

	for _, agent := range []*e2eAgent{a, b, c} {
		e2eWaitAllInSync(t, agent, "books", "orders")
	}

	books := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}
	orders := e2eCheck{Service: "orders", Resource: "/orders/1", Action: "approve", Principals: e2eUser("carol")}
	for _, agent := range []*e2eAgent{a, b, c} {
		e2eRequireAllowed(t, agent, books)
		e2eRequireAllowed(t, agent, orders)
	}

	// B and C never contact Cloud directly: their data comes through the
	// proxy chain.
	if count := cloud.countRequestsWithCredential(tokenB); count != 0 {
		t.Errorf("Cloud saw %d requests with B's credential", count)
	}
	if count := cloud.countRequestsWithCredential(tokenC); count != 0 {
		t.Errorf("Cloud saw %d requests with C's credential", count)
	}
	if count := a.proxyDataRequestsWithToken(tokenB); count == 0 {
		t.Error("A never served a data request to B")
	}
	if count := a.proxyDataRequestsWithToken(tokenC); count != 0 {
		t.Errorf("A served %d data requests to C", count)
	}
	if count := b.proxyDataRequestsWithToken(tokenC); count == 0 {
		t.Error("B never served a data request to C")
	}
}

// TestE2ESyncingServiceUsesFallbackWhileAnotherServesLocally covers the
// per-service routing rule: while one service is catching up, its requests go
// to Cloud, the other service keeps evaluating locally, and the switch back to
// local happens without a restart.
func TestE2ESyncingServiceUsesFallbackWhileAnotherServesLocally(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	_, tokenB := e2eDownstreamCredential("B", e2eInstanceB, 2)
	clockB := newTestClock()
	b := e2eAgentB(t, cloud, a, tokenB, clockB, "")

	e2eWaitAllInSync(t, a, "books", "orders")
	e2eWaitAllInSync(t, b, "books", "orders")

	cloud.hold(e2eServiceOrders)
	cloud.setHistoryUnavailable(e2eServiceOrders, true)
	head := cloud.publish(e2eServiceOrders, e2eUpsertAuthz("p-orders-dave-approve", "grant user dave approve /orders/1"))
	// A head notification alone is enough to start the catch-up; the ack that
	// confirms the head follows on the next heartbeat.
	cloud.notify(e2eServiceOrders, head)

	e2eWaitState(t, a, "orders", adsync.SyncStateSyncing, adsync.SyncReasonCatchUp)
	e2eWaitState(t, a, "books", adsync.SyncStateInSync, "")

	// The service that is still in sync answers locally.
	e2eRequireAllowed(t, a, e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")})

	// The syncing service is forwarded to Cloud unchanged.
	dave := e2eCheck{Service: "orders", Resource: "/orders/1", Action: "approve", Principals: e2eUser("dave")}
	fallbacksBefore := cloud.fallbackCount()
	status, header, body := a.isAllowed(dave.body(t))
	if status != 200 {
		t.Fatalf("fallback response: status %d body %s", status, body)
	}
	if source := header.Get(svcs.DecisionSourceHeader); source != svcs.DecisionSourceCloudFallback {
		t.Fatalf("fallback response decision source = %q, want %q", source, svcs.DecisionSourceCloudFallback)
	}
	fallbackResponse := e2eDecodeResponse(t, body)
	if !fallbackResponse.Allowed {
		t.Fatalf("fallback denied a granted request: %+v", fallbackResponse)
	}
	if count := cloud.fallbackCount(); count != fallbacksBefore+1 {
		t.Fatalf("Cloud saw %d fallback calls after one forwarded request, want %d", count, fallbacksBefore+1)
	}

	// Cloud evaluated the same body with the same policies and reached the same
	// decision.
	direct := cloud.isAllowed(t, dave.body(t))
	if direct.Allowed != fallbackResponse.Allowed || direct.Reason != fallbackResponse.Reason {
		t.Fatalf("fallback %+v does not match Cloud's own evaluation %+v", fallbackResponse, direct)
	}

	cloud.release(e2eServiceOrders)
	e2eWaitState(t, a, "orders", adsync.SyncStateInSync, "")
	// The evaluator reloads the published assembly through the store's watch
	// events, so wait for the decision itself before asserting it is local.
	e2eWaitDecision(t, a, dave, true)

	status, header, body = a.isAllowed(dave.body(t))
	if status != 200 {
		t.Fatalf("local response after catch-up: status %d body %s", status, body)
	}
	if source := header.Get(svcs.DecisionSourceHeader); source != "" {
		t.Fatalf("decision after catch-up still came from %q", source)
	}
	if response := e2eDecodeResponse(t, body); !response.Allowed {
		t.Fatalf("local decision after catch-up denied a granted request: %+v", response)
	}
}

// TestE2ESourceDisconnectStaleAndStopped covers the heartbeat loss ladder: a
// stale instance keeps evaluating complete local copies but reports not ready,
// a stopped instance answers 503 and never a decision, and recovery needs a
// fresh verified synchronization rather than a heartbeat alone.
func TestE2ESourceDisconnectStaleAndStopped(t *testing.T) {
	cloud := newE2ECloud(t)
	clock := newTestClock()
	a := e2eAgentA(t, cloud, clock)
	e2eWaitAllInSync(t, a, "books", "orders")

	books := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}

	cloud.disconnect()

	// Warning threshold: stale, still local, not ready.
	clock.Advance(31 * time.Second)
	e2eWaitLiveness(t, a, adsync.LivenessStale)
	e2eRequireAllowed(t, a, books)
	if status, body := a.readyz(); status != 503 {
		t.Fatalf("stale readiness = %d body %s, want 503", status, body)
	} else if !strings.Contains(string(body), `"liveness":"stale"`) {
		t.Fatalf("stale readiness body = %s", body)
	}

	// Stop threshold: no decision at all.
	clock.Advance(31 * time.Second)
	e2eWaitLiveness(t, a, adsync.LivenessStopped)
	status, _, body := a.isAllowed(books.body(t))
	if status != 503 {
		t.Fatalf("stopped authorization = %d body %s, want 503", status, body)
	}
	var unavailable struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &unavailable); err != nil {
		t.Fatalf("decode stopped response: %v", err)
	}
	if unavailable.Error != "authorization_unavailable" || unavailable.Reason != string(adsync.ReasonHeartbeatLost) {
		t.Fatalf("stopped response = %+v", unavailable)
	}
	for _, operation := range []string{"all-granted-roles", "all-granted-permissions"} {
		if status, _, body := a.post(svcs.PolicyAtzPath+operation, books.body(t)); status != 503 {
			t.Fatalf("stopped %s = %d body %s, want 503", operation, status, body)
		}
	}
	if status, _ := a.readyz(); status != 503 {
		t.Fatalf("stopped readiness = %d, want 503", status)
	}
	if status := a.healthOK(); status != 200 {
		t.Fatalf("liveness probe = %d, want 200", status)
	}

	// Reconnect: the instance recovers only after a verified synchronization.
	cloud.reconnect()
	e2eWaitAllInSync(t, a, "books", "orders")
	e2eRequireAllowed(t, a, books)
}

// TestE2ENetworkPartitionBetweenAAndB cuts a downstream instance off from its
// proxy. B loses its heartbeats and follows the stale then stopped ladder,
// while A keeps evaluating and distributing. Healing the partition recovers B
// without a restart.
func TestE2ENetworkPartitionBetweenAAndB(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	_, tokenB := e2eDownstreamCredential("B", e2eInstanceB, 2)
	b := e2eAgentB(t, cloud, a, tokenB, newTestClock(), "")
	e2eWaitAllInSync(t, a, "books", "orders")
	e2eWaitAllInSync(t, b, "books", "orders")

	books := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}

	b.partition()
	b.clock.Advance(31 * time.Second)
	e2eWaitLiveness(t, b, adsync.LivenessStale)
	e2eWaitLiveness(t, a, adsync.LivenessLive)
	e2eRequireAllowed(t, a, books)
	e2eRequireAllowed(t, b, books)
	if status, body := b.readyz(); status != 503 {
		t.Fatalf("partitioned readiness = %d body %s, want 503", status, body)
	}

	b.clock.Advance(31 * time.Second)
	e2eWaitLiveness(t, b, adsync.LivenessStopped)
	status, _, body := b.isAllowed(books.body(t))
	if status != 503 {
		t.Fatalf("partitioned authorization = %d body %s, want 503", status, body)
	}
	if unavailable := e2eDecodeUnavailable(t, body); unavailable.Reason != string(adsync.ReasonHeartbeatLost) {
		t.Fatalf("partitioned stop reason = %q", unavailable.Reason)
	}
	e2eRequireAllowed(t, a, books)

	b.heal()
	e2eWaitAllInSync(t, b, "books", "orders")
	e2eRequireAllowed(t, b, books)
}

// e2eDecodeUnavailable decodes the fail-closed body of a stopped instance.
func e2eDecodeUnavailable(t *testing.T, body []byte) struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
} {
	t.Helper()
	var unavailable struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &unavailable); err != nil {
		t.Fatalf("decode unavailable response %q: %v", body, err)
	}
	if unavailable.Error != "authorization_unavailable" {
		t.Fatalf("unavailable response = %+v", unavailable)
	}
	return unavailable
}

// TestE2ERestartReusesTheCache restarts an instance with its on-disk copy. The
// restart must reach in_sync without pulling a snapshot or change page, while a
// restart with an empty cache has to download both services.
func TestE2ERestartReusesTheCache(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	_, tokenB := e2eDownstreamCredential("B", e2eInstanceB, 2)
	cacheDir := e2eCacheDir(t)

	b := e2eAgentB(t, cloud, a, tokenB, newTestClock(), cacheDir)
	e2eWaitAllInSync(t, b, "books", "orders")

	cached, err := filepath.Glob(filepath.Join(cacheDir, "service-*.json"))
	if err != nil {
		t.Fatalf("list cache files: %v", err)
	}
	if len(cached) != 2 {
		t.Fatalf("cache holds %d service files, want 2", len(cached))
	}
	b.stop()

	// Restart with the same cache: the bootstrap confirms the cached revision
	// against the source without downloading policy content again.
	before := a.proxyDataRequests()
	restarted := e2eAgentB(t, cloud, a, tokenB, newTestClock(), cacheDir)
	e2eWaitAllInSync(t, restarted, "books", "orders")
	if pulled := a.proxyDataRequests() - before; pulled != 0 {
		t.Fatalf("cache restart pulled %d snapshot/change pages, want 0", pulled)
	}
	e2eRequireAllowed(t, restarted, e2eCheck{
		Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice"),
	})
	restarted.stop()

	// Control: an empty cache has to download one snapshot per service.
	before = a.proxyDataRequests()
	fresh := e2eAgentB(t, cloud, a, tokenB, newTestClock(), e2eCacheDir(t))
	e2eWaitAllInSync(t, fresh, "books", "orders")
	if pulled := a.proxyDataRequests() - before; pulled < 2 {
		t.Fatalf("cold start pulled %d snapshot/change pages, want at least 2", pulled)
	}
}

// TestE2ECredentialRevocationStopsTheInstance revokes the instance credential.
// The instance must stop, never return a decision again, stop reconnecting, and
// stay stopped even after the credential is restored: recovery needs an
// operator action, so a fresh process has to pick up the new configuration.
func TestE2ECredentialRevocationStopsTheInstance(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	e2eWaitAllInSync(t, a, "books", "orders")

	books := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}
	cloud.revoke()
	e2eWaitLiveness(t, a, adsync.LivenessStopped)

	status, _, body := a.isAllowed(books.body(t))
	if status != 503 {
		t.Fatalf("revoked authorization = %d body %s, want 503", status, body)
	}
	if unavailable := e2eDecodeUnavailable(t, body); unavailable.Reason != string(adsync.ReasonRevoked) {
		t.Fatalf("revoked stop reason = %q, want %q", unavailable.Reason, adsync.ReasonRevoked)
	}
	e2eWaitFor(t, "the notification channel to stop", a.channelFinished)

	// A terminal stop is not retried: no new channel attempts appear.
	before := cloud.countRequests(adsync.APIPrefix + "/channel")
	time.Sleep(200 * time.Millisecond)
	if after := cloud.countRequests(adsync.APIPrefix + "/channel"); after != before {
		t.Fatalf("revoked instance made %d new channel attempts", after-before)
	}

	// Restoring the credential alone must not resurrect the stopped instance.
	cloud.unrevoke()
	time.Sleep(200 * time.Millisecond)
	if liveness := a.liveness(); liveness != adsync.LivenessStopped {
		t.Fatalf("restored credential moved the instance to %s", liveness)
	}

	// A fresh process with the restored credential serves again.
	fresh := e2eAgentA(t, cloud, newTestClock())
	e2eWaitAllInSync(t, fresh, "books", "orders")
	e2eRequireAllowed(t, fresh, books)
}

// TestE2EPauseAndResume pauses downstream distribution. A keeps synchronizing
// and evaluating locally, its proxy refuses downstream data with
// distribution_paused, B eventually stops when its heartbeats lapse, and a
// resume reopens the fan-out and recovers B.
func TestE2EPauseAndResume(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	_, tokenB := e2eDownstreamCredential("B", e2eInstanceB, 2)
	b := e2eAgentB(t, cloud, a, tokenB, newTestClock(), "")
	e2eWaitAllInSync(t, a, "books", "orders")
	e2eWaitAllInSync(t, b, "books", "orders")

	books := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}

	pauseID := cloud.sendCommand("pause", "", nil)
	e2eWaitFor(t, "A to receive the pause command", func() bool {
		return a.commandReceived("pause", "")
	})
	e2eWaitFor(t, "Cloud to record the pause as acked", func() bool {
		status, found := cloud.commandResult(pauseID)
		return found && status == "acked"
	})

	status, body := a.proxyGet(tokenB, adsync.APIPrefix+"/services")
	if status != 503 {
		t.Fatalf("paused distribution = %d body %s, want 503", status, body)
	}
	var paused struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &paused); err != nil || paused.Error != "distribution_paused" {
		t.Fatalf("paused distribution body = %s", body)
	}

	// Pausing distribution leaves local synchronization and evaluation alone.
	e2eRequireAllowed(t, a, books)
	e2eWaitAllInSync(t, a, "books", "orders")
	// B still holds a complete copy and is inside its thresholds.
	e2eRequireAllowed(t, b, books)

	e2eWaitFor(t, "A to close B's paused channel", func() bool {
		return a.proxy.DownstreamCount() == 0
	})
	b.clock.Advance(61 * time.Second)
	e2eWaitLiveness(t, b, adsync.LivenessStopped)
	if status, _, _ := b.isAllowed(books.body(t)); status != 503 {
		t.Fatalf("B answered with %d while stopped", status)
	}

	resumeID := cloud.sendCommand("resume", "", nil)
	e2eWaitFor(t, "A to receive the resume command", func() bool {
		return a.commandReceived("resume", "")
	})
	e2eWaitFor(t, "Cloud to record the resume as acked", func() bool {
		status, found := cloud.commandResult(resumeID)
		return found && status == "acked"
	})
	status, body = a.proxyGet(tokenB, adsync.APIPrefix+"/services")
	if status != 200 {
		t.Fatalf("resumed distribution = %d body %s, want 200", status, body)
	}
	e2eWaitAllInSync(t, b, "books", "orders")
	e2eRequireAllowed(t, b, books)
}

// TestE2EForcedResync sends a resync command for one service while its change
// history is gone. The service has to fall back to Cloud while it reloads the
// snapshot, then return to local decisions with the new policy, and the new
// revision has to reach the downstream instance too.
func TestE2EForcedResync(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	_, tokenB := e2eDownstreamCredential("B", e2eInstanceB, 2)
	b := e2eAgentB(t, cloud, a, tokenB, newTestClock(), "")
	e2eWaitAllInSync(t, a, "books", "orders")
	e2eWaitAllInSync(t, b, "books", "orders")

	dave := e2eCheck{Service: "orders", Resource: "/orders/1", Action: "approve", Principals: e2eUser("dave")}
	if response := e2eDecodeResponseFromAgent(t, a, dave); response.Allowed {
		t.Fatalf("dave was allowed before the policy that grants the action: %+v", response)
	}

	// The new revision cannot be caught up from the change log, so reaching it
	// requires a fresh signed snapshot, which the fixture holds open until the
	// test releases it.
	cloud.setHistoryUnavailable(e2eServiceOrders, true)
	cloud.hold(e2eServiceOrders)
	cloud.publish(e2eServiceOrders, e2eUpsertAuthz("p-orders-dave-approve", "grant user dave approve /orders/1"))

	resyncID := cloud.sendCommand("resync", e2eServiceOrders, nil)
	e2eWaitFor(t, "A to receive the resync command", func() bool {
		return a.commandReceived("resync", e2eServiceOrders)
	})
	e2eWaitState(t, a, "orders", adsync.SyncStateSyncing, adsync.SyncReasonResync)

	// While the snapshot is in flight the service is served by Cloud.
	status, header, body := a.isAllowed(dave.body(t))
	if status != 200 {
		t.Fatalf("resync fallback: status %d body %s", status, body)
	}
	if source := header.Get(svcs.DecisionSourceHeader); source != svcs.DecisionSourceCloudFallback {
		t.Fatalf("resync fallback decision source = %q", source)
	}
	if response := e2eDecodeResponse(t, body); !response.Allowed {
		t.Fatalf("resync fallback denied a granted request: %+v", response)
	}

	cloud.release(e2eServiceOrders)
	e2eWaitState(t, a, "orders", adsync.SyncStateInSync, "")
	e2eWaitFor(t, "Cloud to record the resync as acked", func() bool {
		status, found := cloud.commandResult(resyncID)
		return found && status == "acked"
	})

	e2eWaitDecision(t, a, dave, true)
	status, header, body = a.isAllowed(dave.body(t))
	if status != 200 || header.Get(svcs.DecisionSourceHeader) != "" {
		t.Fatalf("post-resync decision: status %d source %q body %s", status, header.Get(svcs.DecisionSourceHeader), body)
	}
	if response := e2eDecodeResponse(t, body); !response.Allowed {
		t.Fatalf("post-resync decision denied a granted request: %+v", response)
	}

	aStatus, _ := a.status("orders")
	e2eWaitServiceRevision(t, b, "orders", aStatus.Service.Revision)
	e2eWaitDecision(t, b, dave, true)
}

func e2eDecodeResponseFromAgent(t *testing.T, agent *e2eAgent, check e2eCheck) adsrest.IsAllowedResponse {
	t.Helper()
	status, _, body := agent.isAllowed(check.body(t))
	if status != 200 {
		t.Fatalf("%s: status %d body %s", agent.instanceID, status, body)
	}
	return e2eDecodeResponse(t, body)
}

// TestE2ESourceChangeWithANewEpoch covers a source change that keeps the
// service id but presents a new dataset identity. The new epoch must reach A
// and, through the proxy, B, which reloads its snapshot even though the
// revision number did not grow.
func TestE2ESourceChangeWithANewEpoch(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	_, tokenB := e2eDownstreamCredential("B", e2eInstanceB, 2)
	b := e2eAgentB(t, cloud, a, tokenB, newTestClock(), "")
	e2eWaitAllInSync(t, a, "books", "orders")
	e2eWaitAllInSync(t, b, "books", "orders")

	alice := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}
	erin := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("erin")}
	e2eRequireAllowed(t, a, alice)
	e2eWaitDecision(t, a, erin, false)

	// A set_source command naming a local credential profile is accepted; one
	// naming an unknown profile is rejected locally.
	acceptedID := cloud.sendCommand("set_source", "", &e2eCommandArgs{
		URL:               cloud.server.URL,
		CredentialProfile: "local",
	})
	e2eWaitFor(t, "A to receive the set_source command", func() bool {
		return a.commandReceived("set_source", "")
	})
	e2eWaitFor(t, "Cloud to record set_source as acked", func() bool {
		status, found := cloud.commandResult(acceptedID)
		return found && status == "acked"
	})
	rejectedID := cloud.sendCommand("set_source", "", &e2eCommandArgs{
		URL:               "https://policy-source.invalid",
		CredentialProfile: "missing",
	})
	e2eWaitFor(t, "Cloud to record the unknown profile as rejected", func() bool {
		status, found := cloud.commandResult(rejectedID)
		return found && status == "rejected"
	})

	// The new source presents a new epoch with different policy content.
	proxyPullsBefore := a.proxyDataRequestsWithToken(tokenB)
	cloud.replaceEpoch(e2eServiceBooks, e2eEpochBooksNext, []adsync.Policy{
		e2ePolicy("p-books-erin-read", "grant user erin read /books/1"),
	}, nil)
	cloud.pushAck()

	e2eWaitDecision(t, a, erin, true)
	e2eWaitDecision(t, a, alice, false)
	e2eWaitDecision(t, b, erin, true)
	e2eWaitDecision(t, b, alice, false)
	if epoch := cloud.serviceEpoch(e2eServiceBooks); epoch != e2eEpochBooksNext {
		t.Fatalf("fixture epoch = %q, want %q", epoch, e2eEpochBooksNext)
	}
	if pulls := a.proxyDataRequestsWithToken(tokenB) - proxyPullsBefore; pulls == 0 {
		t.Fatal("B did not reload a snapshot after the epoch change")
	}
}

// TestE2EServiceSetChanges removes a service from the instance's set and adds
// it back. A removed service is not served and is never forwarded to Cloud; an
// added service bootstraps and returns to local decisions on both instances.
func TestE2EServiceSetChanges(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	_, tokenB := e2eDownstreamCredential("B", e2eInstanceB, 2)
	b := e2eAgentB(t, cloud, a, tokenB, newTestClock(), "")
	e2eWaitAllInSync(t, a, "books", "orders")
	e2eWaitAllInSync(t, b, "books", "orders")

	orders := cloud.service(e2eServiceOrders)
	carol := e2eCheck{Service: "orders", Resource: "/orders/1", Action: "approve", Principals: e2eUser("carol")}
	if !e2eDecodeResponseFromAgent(t, a, carol).Allowed {
		t.Fatal("orders was not served before removal")
	}

	fallbacksBefore := cloud.fallbackCount()
	cloud.removeService(e2eServiceOrders)
	cloud.pushAck()

	e2eWaitState(t, a, "orders", adsync.SyncStateRemoved, "")
	e2eWaitState(t, b, "orders", adsync.SyncStateRemoved, "")
	e2eWaitServiceNotFound(t, a, carol)
	e2eWaitServiceNotFound(t, b, carol)
	if count := cloud.fallbackCount(); count != fallbacksBefore {
		t.Fatalf("removed service caused %d fallback calls", count-fallbacksBefore)
	}

	status, body := a.proxyGet(tokenB, adsync.APIPrefix+"/services")
	if status != 200 {
		t.Fatalf("proxy service list = %d body %s", status, body)
	}
	var listed adsync.ServicesResponse
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("decode proxy service list: %v", err)
	}
	for _, service := range listed.Services {
		if service.ID == e2eServiceOrders {
			t.Fatal("the proxy still advertises the removed service")
		}
	}

	// Adding the service back bootstraps it again. The source publishes newer
	// state with its return: a proxy cannot revive a removed service from the
	// same revision its tombstone records.
	cloud.addService(orders)
	head := cloud.publish(e2eServiceOrders, e2eUpsertAuthz("p-orders-carol-approve", "grant user carol approve /orders/1"))
	cloud.pushAck()
	e2eWaitServiceRevision(t, a, "orders", head)
	e2eWaitServiceRevision(t, b, "orders", head)
	e2eWaitDecision(t, a, carol, true)
	e2eWaitDecision(t, b, carol, true)
	e2eRequireAllowed(t, a, carol)
	e2eRequireAllowed(t, b, carol)
}

// TestE2EDecisionParityBetweenLocalAndCloud evaluates the same request bodies
// through A's local path and Cloud's authorization endpoint. The decisions must
// be identical, including the two by-design attribute rules: a condition on an
// attribute missing from the request does not match, and PEP-supplied
// attributes override the built-ins.
func TestE2EDecisionParityBetweenLocalAndCloud(t *testing.T) {
	cloud := newE2ECloud(t)
	a := e2eAgentA(t, cloud, newTestClock())
	e2eWaitAllInSync(t, a, "books", "orders")

	yes, no := true, false
	cases := []struct {
		name        string
		check       e2eCheck
		wantAllowed *bool
	}{
		{"user grant", e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}, &yes},
		{"group grant", e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eGroup("staff")}, &yes},
		{"role grant through a role policy", e2eCheck{Service: "books", Resource: "/reviews/1", Action: "read", Principals: e2eUser("alice")}, &yes},
		{"conditional grant with a missing attribute", e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("bob")}, &no},
		{"conditional grant with the attribute", e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("bob"), Attributes: e2eAttr("level", "numeric", float64(3))}, &yes},
		{"conditional deny with a missing attribute does not apply", e2eCheck{Service: "books", Resource: "/books/1", Action: "delete", Principals: e2eUser("alice")}, &yes},
		{"conditional deny with the attribute applies", e2eCheck{Service: "books", Resource: "/books/1", Action: "delete", Principals: e2eUser("alice"), Attributes: e2eAttr("department", "string", "finance")}, &no},
		{"built-in request_user does not match", e2eCheck{Service: "books", Resource: "/profile", Action: "read", Principals: e2eUser("hero")}, &no},
		{"PEP request_user overrides the built-in", e2eCheck{Service: "books", Resource: "/profile", Action: "read", Principals: e2eUser("hero"), Attributes: e2eAttr("request_user", "string", "mallory")}, &yes},
		{"built-in request_time does not match", e2eCheck{Service: "books", Resource: "/archive", Action: "read", Principals: e2eUser("alice")}, &no},
		{"PEP request_time overrides the built-in", e2eCheck{Service: "books", Resource: "/archive", Action: "read", Principals: e2eUser("alice"), Attributes: e2eAttr("request_time", "numeric", float64(0))}, &yes},
		{"orders user grant", e2eCheck{Service: "orders", Resource: "/orders/1", Action: "approve", Principals: e2eUser("carol")}, &yes},
		{"orders group grant", e2eCheck{Service: "orders", Resource: "/orders/1", Action: "approve", Principals: e2eGroup("ops")}, &yes},
		{"no applicable policy", e2eCheck{Service: "books", Resource: "/missing", Action: "read", Principals: e2eUser("alice")}, &no},
		{"unmatched action", e2eCheck{Service: "books", Resource: "/books/1", Action: "write", Principals: e2eUser("alice")}, nil},
		{"unknown service", e2eCheck{Service: "unknown", Resource: "/x", Action: "read", Principals: e2eUser("alice")}, nil},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := testCase.check.body(t)
			localStatus, localHeader, localBody := a.isAllowed(body)
			cloudStatus, _, cloudBody := cloud.post(t, adsync.AuthzCheckAPIPrefix+"is-allowed", body)
			if localStatus != cloudStatus {
				t.Fatalf("status local %d, Cloud %d", localStatus, cloudStatus)
			}
			if localHeader.Get(svcs.DecisionSourceHeader) != "" {
				t.Fatalf("local decision carried a decision source")
			}
			local := e2eDecodeResponse(t, localBody)
			remote := e2eDecodeResponse(t, cloudBody)
			if local.Allowed != remote.Allowed || local.Reason != remote.Reason {
				t.Fatalf("local %+v, Cloud %+v", local, remote)
			}
			if testCase.wantAllowed != nil && local.Allowed != *testCase.wantAllowed {
				t.Fatalf("allowed = %v, want %v (%+v)", local.Allowed, *testCase.wantAllowed, local)
			}
		})
	}
}
