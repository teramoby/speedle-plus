// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func newPopulatedMirror(t *testing.T, signer *testSigner) (*Mirror, *fixtureService, *fixtureService) {
	t.Helper()
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	books := newFixtureService(testServiceID1, "books")
	orders := newFixtureService(testServiceID2, "orders")
	if err := mirror.ApplySnapshot(books.snapshotResponse(t, signer)); err != nil {
		t.Fatalf("apply books snapshot: %v", err)
	}
	if err := mirror.ApplySnapshot(orders.snapshotResponse(t, signer)); err != nil {
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

func TestServerAuthentication(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})

	response, body := server.get(adsync.APIPrefix+"/services", "", nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no credential: status %d", response.StatusCode)
	}
	if string(body) != `{"error":"unauthorized"}` {
		t.Errorf("no credential body = %s", body)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}

	response, body = server.get(adsync.APIPrefix+"/services", testToken(testDownstreamB, 9), nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown credential: status %d", response.StatusCode)
	}
	if string(body) != `{"error":"unauthorized"}` {
		t.Errorf("unknown credential body = %s", body)
	}

	response, body = server.get(adsync.APIPrefix+"/services", token, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid credential: status %d body %s", response.StatusCode, body)
	}
	services := decodeBody[adsync.ServicesResponse](t, body)
	if services.ServicesVersion <= 0 || len(services.Services) != 2 {
		t.Errorf("services = %+v", services)
	}
}

func TestServiceSubsetRejectsLikeUnknown(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
		services:     []string{testServiceID1},
	})

	response, body := server.get(adsync.APIPrefix+"/services", token, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("services: status %d", response.StatusCode)
	}
	services := decodeBody[adsync.ServicesResponse](t, body)
	if len(services.Services) != 1 || services.Services[0].ID != testServiceID1 {
		t.Errorf("subset services = %+v", services.Services)
	}

	endpoints := []string{"state", "snapshot", "changes?since=1"}
	for _, endpoint := range endpoints {
		response, subsetBody := server.get(adsync.APIPrefix+"/services/"+testServiceID2+"/"+endpoint, token, nil)
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("%s outside subset: status %d body %s", endpoint, response.StatusCode, subsetBody)
		}
		response, unknownBody := server.get(adsync.APIPrefix+"/services/"+testUnknownID+"/"+endpoint, token, nil)
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("%s unknown service: status %d body %s", endpoint, response.StatusCode, unknownBody)
		}
		if string(subsetBody) != string(unknownBody) {
			t.Errorf("%s: subset body %s differs from unknown body %s", endpoint, subsetBody, unknownBody)
		}
		if string(subsetBody) != `{"error":"service_not_found"}` {
			t.Errorf("%s: body = %s", endpoint, subsetBody)
		}
	}

	// A credential with * sees the second service.
	allToken := testToken(testDownstreamB, 2)
	allServer := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamB,
		token:        allToken,
	})
	response, body = allServer.get(adsync.APIPrefix+"/services/"+testServiceID2+"/snapshot", allToken, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("all-services snapshot: status %d body %s", response.StatusCode, body)
	}
	snapshot := decodeBody[adsync.SnapshotResponse](t, body)
	if snapshot.ServiceID != testServiceID2 || snapshot.Snapshot.Name != "orders" {
		t.Errorf("snapshot = %+v", snapshot)
	}
}

func TestSnapshotETag(t *testing.T) {
	signer := newTestSigner(t)
	mirror, books, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	path := adsync.APIPrefix + "/services/" + testServiceID1 + "/snapshot"
	response, body := server.get(path, token, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("snapshot: status %d", response.StatusCode)
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		t.Fatal("snapshot has no ETag")
	}
	snapshot := decodeBody[adsync.SnapshotResponse](t, body)
	if snapshot.Signature != books.snapshotResponse(t, signer).Signature {
		t.Error("snapshot signature was not passed through unchanged")
	}

	header := http.Header{}
	header.Set("If-None-Match", etag)
	response, body = server.get(path, token, header)
	if response.StatusCode != http.StatusNotModified {
		t.Fatalf("matching If-None-Match: status %d", response.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("304 body = %s", body)
	}

	header.Set("If-None-Match", `"stale-tag"`)
	response, _ = server.get(path, token, header)
	if response.StatusCode != http.StatusOK {
		t.Errorf("stale If-None-Match: status %d", response.StatusCode)
	}

	// A wildcard is not an exact identity tuple and must not suppress a new
	// snapshot, even though generic HTTP caching often gives it that meaning.
	header.Set("If-None-Match", "*")
	response, _ = server.get(path, token, header)
	if response.StatusCode != http.StatusOK {
		t.Errorf("wildcard If-None-Match: status %d, want 200", response.StatusCode)
	}
}

func TestStateEndpoint(t *testing.T) {
	signer := newTestSigner(t)
	mirror, books, _ := newPopulatedMirror(t, signer)
	books.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	response, body := server.get(adsync.APIPrefix+"/services/"+testServiceID1+"/state", token, nil)
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
	signer := newTestSigner(t)
	mirror, books, _ := newPopulatedMirror(t, signer)
	for index := 0; index < 3; index++ {
		books.publish(t, signer, upsertAuthz(
			testServiceID1+"A"+string(rune('1'+index)), "policy", "grant read"))
		if err := mirror.ApplyChanges(pageFor(t, books, books.Revision-1, 100)); err != nil {
			t.Fatalf("apply commit %d: %v", index, err)
		}
	}
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	path := adsync.APIPrefix + "/services/" + testServiceID1 + "/changes"

	response, body := server.get(path+"?since=1&limit=2", token, nil)
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

	response, body = server.get(path+"?since=3&limit=2", token, nil)
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
		response, body := server.get(path+query, token, nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("%q: status %d body %s", query, response.StatusCode, body)
		}
	}

	response, body = server.get(path+"?since=0", token, nil)
	if response.StatusCode != http.StatusGone || string(body) != `{"error":"snapshot_required","reason":"history_unavailable"}` {
		t.Errorf("history unavailable: status %d body %s", response.StatusCode, body)
	}
	response, body = server.get(path+"?since=99", token, nil)
	if response.StatusCode != http.StatusGone {
		t.Errorf("since after head: status %d body %s", response.StatusCode, body)
	}
}

func TestServerNeedsSnapshotAndRemovedMapping(t *testing.T) {
	signer := newTestSigner(t)
	mirror, books, _ := newPopulatedMirror(t, signer)
	mirror.RemoveService(testServiceID2)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})

	response, body := server.get(adsync.APIPrefix+"/services/"+testServiceID2+"/snapshot", token, nil)
	if response.StatusCode != http.StatusGone || string(body) != `{"error":"snapshot_required","reason":"source_gone"}` {
		t.Errorf("removed service: status %d body %s", response.StatusCode, body)
	}
	response, body = server.get(adsync.APIPrefix+"/services", token, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("services: status %d", response.StatusCode)
	}
	services := decodeBody[adsync.ServicesResponse](t, body)
	if len(services.Services) != 1 || services.Services[0].ID != testServiceID1 {
		t.Errorf("services after removal = %+v", services.Services)
	}
	_ = books
}

func TestDistributionPaused(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	var paused atomic.Bool
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		paused: paused.Load,
	}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})

	response, _ := server.get(adsync.APIPrefix+"/services", token, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("services before pause: status %d", response.StatusCode)
	}

	conn, _, err := server.dial(token)
	if err != nil {
		t.Fatalf("dial before pause: %v", err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloMessage{
		Type:         "hello",
		InstanceID:   testDownstreamID,
		Services:     []helloService{},
		Chain:        []string{testProxyID, testDownstreamID},
		AgentVersion: "test",
	})
	if kind, _ := readChannelFrame(t, conn); kind != "ack" {
		t.Fatalf("first frame = %q", kind)
	}

	paused.Store(true)
	response, body := server.get(adsync.APIPrefix+"/services", token, nil)
	if response.StatusCode != http.StatusServiceUnavailable || string(body) != `{"error":"distribution_paused"}` {
		t.Errorf("services while paused: status %d body %s", response.StatusCode, body)
	}
	lateConn, response, err := server.dial(token)
	if err == nil {
		lateConn.Close()
		t.Fatal("paused server accepted a new channel")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("new channel while paused: status %v", response)
	}

	// An existing channel ends at its next heartbeat.
	writeJSONFrame(t, conn, heartbeatMessage{
		Type:     "heartbeat",
		Seq:      1,
		Services: []heartbeatService{},
		Liveness: "live",
		Chain:    []string{testProxyID, testDownstreamID},
		Fallback: fallbackMessage{Services: []string{}},
	})
	code, reason := readClose(t, conn)
	if code != websocket.ClosePolicyViolation || reason != "distribution_paused" {
		t.Errorf("paused channel close = %d %q", code, reason)
	}
}

func TestServerUnknownPathAndMethod(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	response, _ := server.get(adsync.APIPrefix+"/nope", token, nil)
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path: status %d", response.StatusCode)
	}
	response, _ = server.get(adsync.APIPrefix+"/channel/poll", token, nil)
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET poll: status %d", response.StatusCode)
	}
}

func TestServerConfigValidation(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	credentials, err := NewCredentialSet([]Credential{{DownstreamID: "edge", Token: "token", Services: []string{"*"}}})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	cases := []Config{
		{Mirror: mirror, Credentials: credentials},
		{InstanceID: "not-a-ulid", Mirror: mirror, Credentials: credentials},
		{InstanceID: testProxyID, Credentials: credentials},
		{InstanceID: testProxyID, Mirror: mirror},
		{InstanceID: testProxyID, Mirror: mirror, Credentials: credentials, MaxChainDepth: 4},
		{InstanceID: testProxyID, Mirror: mirror, Credentials: credentials, MaxDownstream: 101},
		{InstanceID: testProxyID, Mirror: mirror, Credentials: credentials,
			Chain: []string{testUpstreamID}},
		{InstanceID: testProxyID, Mirror: mirror, Credentials: credentials, MaxChainDepth: 1,
			Chain: []string{testUpstreamID, testDownstreamB, testProxyID}},
	}
	for index, config := range cases {
		if _, err := NewServer(config); err == nil {
			t.Errorf("case %d: configuration was accepted", index)
		}
	}
	if _, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror, Credentials: credentials}); err != nil {
		t.Errorf("valid configuration: %v", err)
	}
}

func TestBearerTokenParsing(t *testing.T) {
	cases := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"BEARER abc", "abc", true},
		{"Bearer", "", false},
		{"Bearer ", "", false},
		{"Basic abc", "", false},
		{"", "", false},
	}
	for _, testCase := range cases {
		token, ok := bearerToken(testCase.header)
		if token != testCase.want || ok != testCase.ok {
			t.Errorf("%q: token %q ok %v", testCase.header, token, ok)
		}
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
