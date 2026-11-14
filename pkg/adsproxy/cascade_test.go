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

func TestRuntimeChainEvictsExistingAncestorSessions(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	readChannelFrame(t, conn)
	done := make(chan struct{})
	var status int
	var body []byte
	go func() {
		defer close(done)
		response, payload := server.post(server.defaultClient(), adsync.APIPrefix+"/channel/poll?wait=25", pollBody(t, testClientInstanceID))
		status = response.StatusCode
		body = payload
	}()
	// Observe the poll as well as the channel under the same session identity.
	waitFor(t, time.Second, "held poll", func() bool {
		server.server.mu.Lock()
		defer server.server.mu.Unlock()
		for _, count := range server.server.sessions {
			if count == 2 {
				return true
			}
		}
		return false
	})
	if err := server.server.SetChain([]string{testClientInstanceID, testProxyID}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ancestor poll was not released")
	}
	if status != 403 || !strings.Contains(string(body), "chain_loop") {
		t.Fatalf("ancestor poll: %d %s", status, body)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("existing ancestor channel stayed open")
	}
	if err := server.server.SetChain([]string{testProxyID, testProxyID}); err == nil {
		t.Fatal("invalid runtime chain accepted")
	}
	if !server.server.chainLoop(testClientInstanceID) {
		t.Fatal("invalid replacement changed the guard")
	}
}

func TestCascadeChainHasNoDepthOrFrameLimit(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	chain := make([]string, 30001)
	for i := 0; i < len(chain)-1; i++ {
		chain[i] = fmt.Sprintf("0194a5c2-7d1e-7b3a-9f10-%012x", i)
	}
	chain[len(chain)-1] = testProxyID
	if err := server.server.SetChain(chain); err != nil {
		t.Fatal(err)
	}
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	kind, payload := readChannelFrame(t, conn)
	ack := decodeBody[ackMessage](t, payload)
	if kind != "ack" || len(payload) <= 1<<20 || len(ack.Chain) != len(chain) {
		t.Fatalf("large chain ack: kind=%s bytes=%d entries=%d", kind, len(payload), len(ack.Chain))
	}
}

func TestRelayFreshnessGuardsDataAndPropagatesOnIdleChannels(t *testing.T) {
	var state atomic.Pointer[adsync.UpstreamStatus]
	state.Store(&adsync.UpstreamStatus{State: "live", FreshFor: 20 * time.Second})
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, &testServerOptions{upstream: func() adsync.UpstreamStatus { return *state.Load() }})
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	readChannelFrame(t, conn)
	for _, tc := range []struct {
		state  string
		budget time.Duration
		code   string
	}{{"syncing", 10 * time.Second, "upstream_syncing"}, {"stale", 0, "upstream_stale"}, {"live", 0, "upstream_stale"}} {
		state.Store(&adsync.UpstreamStatus{State: tc.state, FreshFor: tc.budget})
		// Force the same watcher path synchronously, avoiding a clock-based test.
		server.server.refreshAvailability()
		if tc.state != "live" {
			kind, payload := readChannelFrame(t, conn)
			ack := decodeBody[ackMessage](t, payload)
			if kind != "ack" || ack.UpstreamState != tc.state {
				t.Fatalf("idle channel did not propagate %s: %s", tc.state, payload)
			}
		}
		response, body := server.get(server.defaultClient(), adsync.APIPrefix+"/services", nil)
		if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), tc.code) {
			t.Fatalf("%s: %d %s", tc.state, response.StatusCode, body)
		}
		for _, operation := range []string{"is-allowed", "all-granted-roles", "all-granted-permissions"} {
			response, body := server.post(server.defaultClient(), adsync.AuthzCheckAPIPrefix+operation, []byte(`{}`))
			if response.StatusCode != 503 || !strings.Contains(string(body), tc.code) {
				t.Fatalf("%s %s: %d %s", tc.state, operation, response.StatusCode, body)
			}
		}
	}
	state.Store(&adsync.UpstreamStatus{State: "live", FreshFor: 20 * time.Second})
	response, _ := server.get(server.defaultClient(), adsync.APIPrefix+"/services", nil)
	if response.StatusCode != 200 {
		t.Fatalf("recovered data: %d", response.StatusCode)
	}
}

func TestRuntimeChainDeadlineExpiresWhileWaitingForCommit(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	readChannelFrame(t, conn)
	server.server.availabilityMu.Lock()
	deadline := time.Now().Add(30 * time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- server.server.SetChainUntil([]string{testClientInstanceID, testProxyID}, deadline) }()
	// Hold the actual confirming-response lock until the candidate expires.
	timer := time.NewTimer(time.Until(deadline) + time.Millisecond)
	<-timer.C
	server.server.availabilityMu.Unlock()
	if err := <-done; !errors.Is(err, adsync.ErrUnavailable) {
		t.Fatalf("expired topology commit: %v", err)
	}
	if server.server.chainLoop(testClientInstanceID) {
		t.Fatal("expired candidate changed ancestry")
	}
	writeJSONFrame(t, conn, heartbeatFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, conn); kind != "ack" {
		t.Fatal("expired candidate evicted an existing child")
	}
}

func TestRelayWithoutFreshnessProviderFailsClosed(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	recorder := httptest.NewRecorder()
	if server.dataAvailable(recorder) {
		t.Fatal("unwired server admitted data")
	}
	if recorder.Code != 503 || !strings.Contains(recorder.Body.String(), "upstream_syncing") {
		t.Fatalf("unwired data response: %d %s", recorder.Code, recorder.Body.String())
	}
	ack := server.ack()
	if ack.UpstreamState != "syncing" || ack.UpstreamFreshForMs != 0 {
		t.Fatalf("unwired ack advertised freshness: %+v", ack)
	}
}
