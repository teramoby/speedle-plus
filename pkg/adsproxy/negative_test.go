// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func TestServerAccessors(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	if got := server.server.InstanceID(); got != testProxyID {
		t.Errorf("InstanceID = %q", got)
	}
	if got := server.server.DownstreamCount(); got != 0 {
		t.Errorf("DownstreamCount = %d", got)
	}
}

func TestWriteJSONMarshalError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusOK, map[string]any{"bad": make(chan int)})
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "internal_error") {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestTrimChangesPage(t *testing.T) {
	large := strings.Repeat("x", 600<<10)
	huge := strings.Repeat("x", 1200<<10)
	page := adsync.ChangesResponse{
		ServiceID: testServiceID1,
		SourceID:  testServiceID1,
		Epoch:     testServiceID1,
		From:      2,
		To:        3,
		Head:      3,
		Commits: []adsync.Commit{
			{Revision: 2, Changes: []adsync.Change{{Op: "upsert", Kind: "authz", ID: "A", SPDL: &large}}},
			{Revision: 3, Changes: []adsync.Change{{Op: "upsert", Kind: "authz", ID: "B", SPDL: &large}}},
		},
		NextSince: 4,
	}
	encoded, err := trimChangesPage(&page)
	if err != nil {
		t.Fatalf("trim page: %v", err)
	}
	if len(encoded) > maxChangesPageBytes {
		t.Errorf("trimmed page is %d bytes", len(encoded))
	}
	if len(page.Commits) != 1 || page.To != 2 || page.NextSince != 3 {
		t.Errorf("trimmed page = commits %d to %d next %d", len(page.Commits), page.To, page.NextSince)
	}

	single := adsync.ChangesResponse{
		Commits: []adsync.Commit{
			{Revision: 2, Changes: []adsync.Change{{Op: "upsert", Kind: "authz", ID: "A", SPDL: &huge}}},
		},
	}
	if _, err := trimChangesPage(&single); !errors.Is(err, ErrHistoryUnavailable) {
		t.Errorf("single oversized commit: %v, want ErrHistoryUnavailable", err)
	}
}

func TestWriteMirrorErrorMapping(t *testing.T) {
	for _, testCase := range []struct {
		err    error
		status int
		body   string
	}{
		{fmt.Errorf("%w: x", ErrUnknownService), http.StatusNotFound, `{"error":"service_not_found"}`},
		{fmt.Errorf("%w: x", ErrServiceRemoved), http.StatusGone, `{"error":"snapshot_required","reason":"source_gone"}`},
		{fmt.Errorf("%w: x", ErrHistoryUnavailable), http.StatusGone, `{"error":"snapshot_required","reason":"history_unavailable"}`},
		{fmt.Errorf("%w: x", ErrIdentityChanged), http.StatusGone, `{"error":"snapshot_required","reason":"epoch_mismatch"}`},
		{fmt.Errorf("%w: x", adsync.ErrIntegrity), http.StatusGone, `{"error":"snapshot_required","reason":"epoch_mismatch"}`},
		{fmt.Errorf("%w: x", ErrSnapshotRequired), http.StatusServiceUnavailable, `{"error":"source_unavailable"}`},
		{errors.New("boom"), http.StatusInternalServerError, `{"error":"internal_error"}`},
	} {
		recorder := httptest.NewRecorder()
		(&Server{}).writeMirrorError(recorder, testCase.err)
		if recorder.Code != testCase.status || recorder.Body.String() != testCase.body {
			t.Errorf("%v: status %d body %s", testCase.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestResolveServiceSourceUnavailable(t *testing.T) {
	mirror := newTestMirror(t, func(options *MirrorOptions) {
		options.MaxLogCommits = 1
	})
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	for index := 0; index < 2; index++ {
		service.publish(t, upsertAuthz(testServiceID1+string(rune('A'+index)), "policy", "grant read"))
		if err := mirror.ApplyChanges(pageFor(t, service, service.Revision-1, 100)); err != nil {
			t.Fatalf("apply changes: %v", err)
		}
	}
	server := newTestServer(t, mirror, nil)
	for _, action := range []string{"state", "snapshot", "changes?since=1"} {
		response, body := server.get(server.defaultClient(),
			adsync.APIPrefix+"/services/"+testServiceID1+"/"+action, nil)
		if response.StatusCode != http.StatusServiceUnavailable || string(body) != `{"error":"source_unavailable"}` {
			t.Errorf("%s: status %d body %s", action, response.StatusCode, body)
		}
	}
}

func TestNormalizeThresholdsErrors(t *testing.T) {
	mirror := newTestMirror(t, nil)
	for _, testCase := range []struct {
		name      string
		heartbeat time.Duration
		stale     time.Duration
		stop      time.Duration
	}{
		{name: "heartbeat below minimum", heartbeat: time.Second},
		{name: "stale not above heartbeat", heartbeat: 30 * time.Second, stale: 30 * time.Second},
		{name: "stop below minimum", stop: time.Second},
		{name: "stop not above stale", stale: 60 * time.Second, stop: 60 * time.Second},
	} {
		_, err := NewServer(Config{
			InstanceID:        testProxyID,
			Mirror:            mirror,
			HeartbeatInterval: testCase.heartbeat,
			StaleAfter:        testCase.stale,
			StopAfter:         testCase.stop,
		})
		if !errors.Is(err, ErrConfig) {
			t.Errorf("%s: %v, want ErrConfig", testCase.name, err)
		}
	}
}

func TestNewServerRejectsInvalidChain(t *testing.T) {
	mirror := newTestMirror(t, nil)
	for _, testCase := range []struct {
		name  string
		chain []string
	}{
		{name: "does not end with the instance", chain: []string{testProxyID, testUpstreamID}},
		{name: "invalid entry", chain: []string{"not-a-uuid", testProxyID}},
		{name: "repeated entry", chain: []string{testProxyID, testProxyID}},
	} {
		_, err := NewServer(Config{InstanceID: testProxyID, Chain: testCase.chain, Mirror: mirror})
		if !errors.Is(err, ErrConfig) {
			t.Errorf("%s: %v, want ErrConfig", testCase.name, err)
		}
	}
	server, err := NewServer(Config{
		InstanceID: testProxyID,
		Chain:      []string{testUpstreamID, testProxyID},
		Mirror:     mirror,
	})
	if err != nil {
		t.Fatalf("valid chain: %v", err)
	}
	server.Close()
}

func TestFormatChannelDuration(t *testing.T) {
	if got := formatChannelDuration(30 * time.Second); got != "30s" {
		t.Errorf("30s = %q", got)
	}
	if got := formatChannelDuration(1500 * time.Millisecond); got != "1500ms" {
		t.Errorf("1500ms = %q", got)
	}
}

func TestHubEdgeCases(t *testing.T) {
	if _, err := NewHub(-time.Second); !errors.Is(err, ErrConfig) {
		t.Errorf("negative window: %v, want ErrConfig", err)
	}
	hub, err := NewHub(time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	subscriber, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if hub.Subscribers() != 1 {
		t.Errorf("Subscribers = %d", hub.Subscribers())
	}
	var absent *Subscriber
	if absent.C() != nil {
		t.Error("nil subscriber returns a channel")
	}
	hub.Unsubscribe(nil)
	hub.Publish("", 1)
	hub.Publish(testServiceID1, 0)
	hub.Close()
	if _, err := hub.Subscribe(); !errors.Is(err, ErrConfig) {
		t.Errorf("closed hub: %v, want ErrConfig", err)
	}
	hub.Publish(testServiceID1, 2)
	hub.Unsubscribe(subscriber)
}

func TestWriteChannelJSONBounds(t *testing.T) {
	if err := writeChannelJSON(nil, make(chan int)); err == nil {
		t.Error("a non-JSON value was accepted")
	}
	if err := writeChannelJSON(nil, strings.Repeat("x", maxChannelFrameBytes+1)); !errors.Is(err, ErrConfig) {
		t.Errorf("oversized frame: %v, want ErrConfig", err)
	}
}

func TestPollOnClosedHub(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	hub, err := NewHub(0)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	server := newTestServer(t, mirror, &testServerOptions{hub: hub})
	hub.Close()
	response, payload := server.post(server.defaultClient(), adsync.APIPrefix+"/channel/poll?wait=0",
		pollBody(t, testClientInstanceID))
	if response.StatusCode != http.StatusServiceUnavailable || string(payload) != `{"error":"source_unavailable"}` {
		t.Errorf("status %d body %s", response.StatusCode, payload)
	}
}

func TestChannelFirstFrameIsAckEvenWithPendingNotification(t *testing.T) {
	mirror, books, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, &testServerOptions{coalesceWindow: time.Millisecond})
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	// A head advance lands while the channel starts. Its coalesced
	// notification must not overtake the initial ack.
	books.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	kind, payload := readChannelFrame(t, conn)
	if kind != "ack" {
		t.Fatalf("first frame = %q, payload %s", kind, payload)
	}
	ack := decodeBody[ackMessage](t, payload)
	if len(ack.Services) != 2 {
		t.Errorf("ack services = %+v", ack.Services)
	}
}

func TestServersShareAHubWithoutStealingItsLifetime(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	hub, err := NewHub(time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	first := newTestServer(t, mirror, &testServerOptions{hub: hub})
	second := newTestServer(t, mirror, &testServerOptions{hub: hub})

	first.server.Close()
	response, body := second.post(second.defaultClient(), adsync.APIPrefix+"/channel/poll?wait=0",
		pollBody(t, testClientInstanceID))
	if response.StatusCode != http.StatusOK {
		t.Errorf("second server after the first closed: status %d body %s", response.StatusCode, body)
	}
	subscriber, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("hub after the first server closed: %v", err)
	}
	hub.Unsubscribe(subscriber)
}

func TestCloseEndsHeldPolls(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	type result struct {
		status int
		body   []byte
	}
	done := make(chan result, 1)
	go func() {
		response, body := server.post(server.defaultClient(), adsync.APIPrefix+"/channel/poll?wait=30",
			pollBody(t, testClientInstanceID))
		done <- result{status: response.StatusCode, body: body}
	}()
	waitFor(t, 2*time.Second, "the poll to be held", func() bool {
		return server.server.DownstreamCount() == 1
	})
	server.server.Close()
	select {
	case got := <-done:
		if got.status != http.StatusServiceUnavailable {
			t.Errorf("held poll after Close: status %d body %s", got.status, got.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a held poll stayed blocked after Close")
	}
}

func TestHeldPollAnswersDistributionPaused(t *testing.T) {
	mirror, books, _ := newPopulatedMirror(t)
	var paused atomic.Bool
	server := newTestServer(t, mirror, &testServerOptions{paused: paused.Load})
	type result struct {
		status int
		body   []byte
	}
	done := make(chan result, 1)
	go func() {
		response, body := server.post(server.defaultClient(), adsync.APIPrefix+"/channel/poll?wait=30",
			pollBody(t, testClientInstanceID))
		done <- result{status: response.StatusCode, body: body}
	}()
	waitFor(t, 2*time.Second, "the poll to be held", func() bool {
		return server.server.DownstreamCount() == 1
	})
	paused.Store(true)
	// A head advance releases the held wait, so the paused state is rechecked.
	books.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	select {
	case got := <-done:
		if got.status != http.StatusServiceUnavailable || string(got.body) != `{"error":"distribution_paused"}` {
			t.Errorf("held poll while paused: status %d body %s", got.status, got.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("held poll did not answer")
	}
}
