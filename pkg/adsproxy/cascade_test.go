// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"fmt"
	"net/http"
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
	}
	state.Store(&adsync.UpstreamStatus{State: "live", FreshFor: 20 * time.Second})
	response, _ := server.get(server.defaultClient(), adsync.APIPrefix+"/services", nil)
	if response.StatusCode != 200 {
		t.Fatalf("recovered data: %d", response.StatusCode)
	}
}
