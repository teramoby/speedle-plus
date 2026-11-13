// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func newPopulatedMirror(t *testing.T) (*Mirror, *fixtureService, *fixtureService) {
	t.Helper()
	mirror := newTestMirror(t, nil)
	books := newFixtureService(testServiceID1, "books")
	orders := newFixtureService(testServiceID2, "orders")
	if err := mirror.ApplySnapshot(books.snapshotResponse(t)); err != nil {
		t.Fatalf("apply books snapshot: %v", err)
	}
	if err := mirror.ApplySnapshot(orders.snapshotResponse(t)); err != nil {
		t.Fatalf("apply orders snapshot: %v", err)
	}
	return mirror, books, orders
}

func decodeBody[T any](t *testing.T, body []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode response %s: %v", body, err)
	}
	return value
}

func TestDownstreamRouteTable(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.defaultClient()

	// The policy-source protocol is mounted.
	response, body := server.get(client, adsync.APIPrefix+"/services", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("services: status %d body %s", response.StatusCode, body)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	services := decodeBody[adsync.ServicesResponse](t, body)
	if services.ServicesVersion <= 0 || len(services.Services) != 2 {
		t.Errorf("services = %+v", services)
	}

	// The three authorization operations are mounted for POST only.
	for _, operation := range []string{"is-allowed", "all-granted-roles", "all-granted-permissions"} {
		response, body = server.post(client, "/authz-check/v1/"+operation, []byte(`{}`))
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d body %s", operation, response.StatusCode, body)
		}
		response, _ = server.get(client, "/authz-check/v1/"+operation, nil)
		if response.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: status %d", operation, response.StatusCode)
		}
	}
	if server.authz.calls != 3 {
		t.Errorf("authorization operations reached the handler %d times, want 3", server.authz.calls)
	}

	// Policy management, diagnostics and unrelated paths are not mounted.
	for _, path := range []string{
		"/authz-check/v1/diagnose",
		"/authz-check/v1/discover",
		"/policy-mgmt/v1/services",
		"/healthz",
		adsync.APIPrefix + "/nope",
		"/",
	} {
		response, _ = server.get(client, path, nil)
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, response.StatusCode)
		}
	}
}

func TestDownstreamHandlerRejectsAnUnverifiedConnection(t *testing.T) {
	// The listener requires a client certificate. This checks the handler's
	// own defense-in-depth answer if it were ever mounted without mTLS.
	mirror, _, _ := newPopulatedMirror(t)
	server, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()
	authz := &authzStub{}
	handler, err := DownstreamHandler(server, AuthzHandlers{
		IsAllowed:             http.HandlerFunc(authz.isAllowed),
		AllGrantedRoles:       http.HandlerFunc(authz.allGrantedRoles),
		AllGrantedPermissions: http.HandlerFunc(authz.allGrantedPermissions),
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, adsync.APIPrefix+"/services", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unverified request: status %d body %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != `{"error":"unauthorized"}` {
		t.Errorf("unverified body = %s", recorder.Body.String())
	}
}

func TestSnapshotETag(t *testing.T) {
	mirror, books, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.defaultClient()
	path := adsync.APIPrefix + "/services/" + testServiceID1 + "/snapshot"
	response, body := server.get(client, path, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("snapshot: status %d", response.StatusCode)
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		t.Fatal("snapshot has no ETag")
	}
	snapshot := decodeBody[adsync.SnapshotResponse](t, body)
	if expected := books.snapshotResponse(t); snapshot.StateHash != expected.StateHash || snapshot.CommitHash != expected.CommitHash {
		t.Error("snapshot hashes were not passed through unchanged")
	}

	header := http.Header{}
	header.Set("If-None-Match", etag)
	response, body = server.get(client, path, header)
	if response.StatusCode != http.StatusNotModified {
		t.Fatalf("matching If-None-Match: status %d", response.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("304 body = %s", body)
	}

	header.Set("If-None-Match", `"stale-tag"`)
	response, _ = server.get(client, path, header)
	if response.StatusCode != http.StatusOK {
		t.Errorf("stale If-None-Match: status %d", response.StatusCode)
	}

	// A wildcard is not an exact identity tuple and must not suppress a new
	// snapshot, even though generic HTTP caching often gives it that meaning.
	header.Set("If-None-Match", "*")
	response, _ = server.get(client, path, header)
	if response.StatusCode != http.StatusOK {
		t.Errorf("wildcard If-None-Match: status %d, want 200", response.StatusCode)
	}
}

func TestStateEndpoint(t *testing.T) {
	mirror, books, _ := newPopulatedMirror(t)
	books.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	server := newTestServer(t, mirror, nil)
	response, body := server.get(server.defaultClient(),
		adsync.APIPrefix+"/services/"+testServiceID1+"/state", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("state: status %d", response.StatusCode)
	}
	state := decodeBody[stateResponse](t, body)
	if state.ServiceID != testServiceID1 || state.SourceID != testServiceID1 ||
		state.Epoch != testServiceID1 || state.Head != 2 ||
		state.HeadStateHash != books.StateHash || state.ServerTime == 0 {
		t.Errorf("state = %+v", state)
	}
}

func TestChangesPaging(t *testing.T) {
	mirror, books, _ := newPopulatedMirror(t)
	for index := 0; index < 3; index++ {
		books.publish(t, upsertAuthz(
			testServiceID1+"A"+string(rune('1'+index)), "policy", "grant read"))
		if err := mirror.ApplyChanges(pageFor(t, books, books.Revision-1, 100)); err != nil {
			t.Fatalf("apply commit %d: %v", index, err)
		}
	}
	server := newTestServer(t, mirror, nil)
	client := server.defaultClient()
	path := adsync.APIPrefix + "/services/" + testServiceID1 + "/changes"

	response, body := server.get(client, path+"?since=1&limit=2", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("page one: status %d body %s", response.StatusCode, body)
	}
	page := decodeBody[adsync.ChangesResponse](t, body)
	if page.From != 2 || page.To != 3 || page.Head != 4 || page.NextSince != 4 {
		t.Errorf("page one = from %d to %d head %d next %d", page.From, page.To, page.Head, page.NextSince)
	}
	if len(page.Commits) != 2 {
		t.Fatalf("page one commits = %d", len(page.Commits))
	}

	response, body = server.get(client, path+"?since=3&limit=2", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("page two: status %d body %s", response.StatusCode, body)
	}
	page = decodeBody[adsync.ChangesResponse](t, body)
	if page.From != 4 || page.To != 4 || page.Head != 4 || page.NextSince != 5 {
		t.Errorf("page two = from %d to %d head %d next %d", page.From, page.To, page.Head, page.NextSince)
	}
	if page.HeadStateHash != books.StateHash {
		t.Error("head state hash does not match the source")
	}

	for _, query := range []string{
		"", "?since=abc", "?since=+1", "?since=1&limit=0", "?since=1&limit=101",
		"?since=1&since=2", "?since=1&limit=2&limit=3", "?since=1&unknown=1",
		"?since=01", "?since=1&limit=01", "?since=1&limit=",
	} {
		response, body := server.get(client, path+query, nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("%q: status %d body %s", query, response.StatusCode, body)
		}
	}

	response, body = server.get(client, path+"?since=0", nil)
	if response.StatusCode != http.StatusGone || string(body) != `{"error":"snapshot_required","reason":"history_unavailable"}` {
		t.Errorf("history unavailable: status %d body %s", response.StatusCode, body)
	}
	response, body = server.get(client, path+"?since=99", nil)
	if response.StatusCode != http.StatusGone {
		t.Errorf("since after head: status %d body %s", response.StatusCode, body)
	}
}

func TestServerNeedsSnapshotAndRemovedMapping(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	mirror.RemoveService(testServiceID2)
	server := newTestServer(t, mirror, nil)
	client := server.defaultClient()

	response, body := server.get(client, adsync.APIPrefix+"/services/"+testServiceID2+"/snapshot", nil)
	if response.StatusCode != http.StatusGone || string(body) != `{"error":"snapshot_required","reason":"source_gone"}` {
		t.Errorf("removed service: status %d body %s", response.StatusCode, body)
	}
	response, body = server.get(client, adsync.APIPrefix+"/services", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("services: status %d", response.StatusCode)
	}
	services := decodeBody[adsync.ServicesResponse](t, body)
	if len(services.Services) != 1 || services.Services[0].ID != testServiceID1 {
		t.Errorf("services after removal = %+v", services.Services)
	}
}

func TestDistributionPaused(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	var paused atomic.Bool
	server := newTestServer(t, mirror, &testServerOptions{paused: paused.Load})
	client := server.defaultClient()

	response, _ := server.get(client, adsync.APIPrefix+"/services", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("services before pause: status %d", response.StatusCode)
	}

	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial before pause: %v", err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, conn); kind != "ack" {
		t.Fatalf("first frame = %q", kind)
	}

	paused.Store(true)
	response, body := server.get(client, adsync.APIPrefix+"/services", nil)
	if response.StatusCode != http.StatusServiceUnavailable || string(body) != `{"error":"distribution_paused"}` {
		t.Errorf("services while paused: status %d body %s", response.StatusCode, body)
	}
	lateConn, response, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err == nil {
		lateConn.Close()
		t.Fatal("paused server accepted a new channel")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("new channel while paused: status %v", response)
	}

	// An existing channel is closed when distribution stops, without waiting
	// for the downstream's next heartbeat.
	code, reason := readClose(t, conn)
	if code != websocket.ClosePolicyViolation || reason != "distribution_paused" {
		t.Errorf("paused channel close = %d %q", code, reason)
	}
}

func TestServerUnknownPathAndMethod(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.defaultClient()
	response, _ := server.get(client, adsync.APIPrefix+"/nope", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path: status %d", response.StatusCode)
	}
	response, _ = server.get(client, adsync.APIPrefix+"/channel/poll", nil)
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET poll: status %d", response.StatusCode)
	}
}

func TestServerConfigValidation(t *testing.T) {
	mirror := newTestMirror(t, nil)
	cases := []Config{
		{Mirror: mirror},
		{InstanceID: "not-a-uuid", Mirror: mirror},
		{InstanceID: testProxyID},
		{InstanceID: testProxyID, Mirror: mirror, HeartbeatInterval: 1},
		{InstanceID: testProxyID, Mirror: mirror, StaleAfter: 10, StopAfter: 20},
	}
	for index, config := range cases {
		if _, err := NewServer(config); err == nil {
			t.Errorf("case %d: configuration was accepted", index)
		}
	}
	server, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror})
	if err != nil {
		t.Fatalf("valid configuration: %v", err)
	}
	server.Close()
}

func TestDownstreamHandlerValidation(t *testing.T) {
	mirror := newTestMirror(t, nil)
	server, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()
	authz := &authzStub{}
	if _, err := DownstreamHandler(nil, AuthzHandlers{
		IsAllowed:             http.HandlerFunc(authz.isAllowed),
		AllGrantedRoles:       http.HandlerFunc(authz.allGrantedRoles),
		AllGrantedPermissions: http.HandlerFunc(authz.allGrantedPermissions),
	}); err == nil {
		t.Error("nil data handler was accepted")
	}
	if _, err := DownstreamHandler(server, AuthzHandlers{IsAllowed: http.HandlerFunc(authz.isAllowed)}); err == nil {
		t.Error("partial authorization handlers were accepted")
	}
}

func TestIfNoneMatch(t *testing.T) {
	etag := `"a-b-1-sha256:x"`
	if !ifNoneMatch(etag, etag) {
		t.Error("exact match not detected")
	}
	if !ifNoneMatch(`"other", `+etag, etag) {
		t.Error("list match not detected")
	}
	if ifNoneMatch("*", etag) {
		t.Error("wildcard matched without an exact identity tuple")
	}
	if ifNoneMatch(`"a-b-1-sha256:y"`, etag) {
		t.Error("different tag matched")
	}
}
