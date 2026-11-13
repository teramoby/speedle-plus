// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

// TestAckIsRefusedOnceDistributionStops proves a confirming ack is abandoned
// before any frame is written when distribution has already stopped. The nil
// connection documents that the guard returns before touching the socket.
func TestAckIsRefusedOnceDistributionStops(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	var paused atomic.Bool
	server := newTestServer(t, mirror, &testServerOptions{paused: paused.Load})
	paused.Store(true)
	if err := server.server.writeChannelAck(nil); !errors.Is(err, errDistributionStopped) {
		t.Fatalf("ack after the stop = %v, want errDistributionStopped", err)
	}
}

// TestAvailabilityFlipWaitsForAConfirmingWrite proves an availability flip
// cannot land between a confirming write's check and its write: the flip waits
// for the write's availability read lock.
func TestAvailabilityFlipWaitsForAConfirmingWrite(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	var paused atomic.Bool
	server := newTestServer(t, mirror, &testServerOptions{paused: paused.Load})
	server.server.availabilityMu.RLock()
	paused.Store(true)
	// A write that is already in flight delays the flip and the channel
	// close, but it must not keep the relay distributing: new data requests
	// read the authority directly and are refused immediately.
	response, body := server.get(server.defaultClient(), adsync.APIPrefix+"/services", nil)
	if response.StatusCode != http.StatusServiceUnavailable || string(body) != `{"error":"distribution_paused"}` {
		t.Fatalf("data request during an in-flight write: status %d body %s", response.StatusCode, body)
	}
	flipped := make(chan struct{})
	go func() {
		server.server.refreshAvailability()
		close(flipped)
	}()
	select {
	case <-flipped:
		t.Fatal("the availability flip did not wait for the in-flight write")
	case <-time.After(50 * time.Millisecond):
	}
	server.server.availabilityMu.RUnlock()
	select {
	case <-flipped:
	case <-time.After(5 * time.Second):
		t.Fatal("the availability flip did not complete")
	}
	server.server.mu.Lock()
	unavailable := server.server.unavailable
	server.server.mu.Unlock()
	if !unavailable {
		t.Fatal("the availability state did not flip")
	}
}

// TestAvailabilityFlipStopsDistribution covers a state change that no
// downstream request drives: once the relay is paused or no longer live, an
// established channel is closed and a held poll answers immediately, so the
// relay never confirms policy it can no longer vouch for.
func TestAvailabilityFlipStopsDistribution(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	var unavailable atomic.Bool
	server := newTestServer(t, mirror, &testServerOptions{paused: unavailable.Load})

	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, conn); kind != "ack" {
		t.Fatalf("first frame = %q", kind)
	}

	type result struct {
		status int
		body   []byte
	}
	polled := make(chan result, 1)
	go func() {
		response, body := server.post(server.defaultClient(),
			adsync.APIPrefix+"/channel/poll?wait=30", pollBody(t, testClientInstanceID))
		polled <- result{status: response.StatusCode, body: body}
	}()
	// The channel and the poll share one session identity, so the counter
	// reaches two only while both are active.
	heldSessions := func() int {
		server.server.mu.Lock()
		defer server.server.mu.Unlock()
		total := 0
		for _, count := range server.server.sessions {
			total += count
		}
		return total
	}
	waitFor(t, 2*time.Second, "the poll to be held", func() bool { return heldSessions() == 2 })

	unavailable.Store(true)

	// The watcher closes the channel without waiting for a downstream frame.
	code, reason := readClose(t, conn)
	if code != websocket.ClosePolicyViolation || reason != "distribution_paused" {
		t.Errorf("closed channel = %d %q", code, reason)
	}

	// The held poll answers long before its 30-second wait expires.
	select {
	case got := <-polled:
		if got.status != http.StatusServiceUnavailable || string(got.body) != `{"error":"distribution_paused"}` {
			t.Errorf("held poll = status %d body %s", got.status, got.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a held poll was not answered after distribution stopped")
	}

	// Recovery reopens distribution without a restart.
	unavailable.Store(false)
	response, body := server.get(server.defaultClient(), adsync.APIPrefix+"/services", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("services after recovery: status %d body %s", response.StatusCode, body)
	}
	reconnected, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial after recovery: %v", err)
	}
	defer reconnected.Close()
	writeJSONFrame(t, reconnected, helloFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, reconnected); kind != "ack" {
		t.Fatalf("first frame after recovery = %q", kind)
	}
}

// TestDistributionChangeSerializesWithAConfirmingWrite proves a caller's
// state change runs while no confirming response is in flight: the change
// waits for the write's availability read lock and flips the distribution
// state before it returns.
func TestDistributionChangeSerializesWithAConfirmingWrite(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	var paused atomic.Bool
	server := newTestServer(t, mirror, &testServerOptions{paused: paused.Load})
	server.server.availabilityMu.RLock()
	changed := make(chan struct{})
	applied := make(chan struct{})
	go func() {
		server.server.DistributionChange(func() {
			close(applied)
			paused.Store(true)
		})
		close(changed)
	}()
	select {
	case <-applied:
		server.server.availabilityMu.RUnlock()
		t.Fatal("the state change ran while a confirming write was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	server.server.availabilityMu.RUnlock()
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("the state change did not complete")
	}
	server.server.mu.Lock()
	unavailable := server.server.unavailable
	server.server.mu.Unlock()
	if !unavailable {
		t.Fatal("distribution did not flip with the state change")
	}
}

// TestCloseWaitsForAnInFlightWrite proves Close and a held poll's final ack
// cannot interleave: Close takes the availability write lock, so it either
// waits for a confirming write or is observed by that write's check.
func TestCloseWaitsForAnInFlightWrite(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	server.server.availabilityMu.RLock()
	closed := make(chan struct{})
	go func() {
		server.server.Close()
		close(closed)
	}()
	select {
	case <-closed:
		server.server.availabilityMu.RUnlock()
		t.Fatal("Close did not wait for the in-flight write")
	case <-time.After(50 * time.Millisecond):
	}
	server.server.availabilityMu.RUnlock()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete")
	}
}

// TestStopTransitionWaitsForAConfirmingWrite covers the store's wiring: an
// explicit machine stop runs through the downstream availability lock via the
// stop hook, so no confirming write from a pre-stop check can be in flight.
func TestStopTransitionWaitsForAConfirmingWrite(t *testing.T) {
	machine, err := adsync.NewMachine(adsync.MachineOptions{})
	if err != nil {
		t.Fatalf("new machine: %v", err)
	}
	mirror, _, _ := newPopulatedMirror(t)
	server, err := NewServer(Config{
		InstanceID: testProxyID,
		Mirror:     mirror,
		Paused: func() bool {
			instance := machine.Instance()
			return instance.Paused || instance.Liveness != adsync.LivenessLive
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()
	machine.SetStopHook(func(apply func()) { server.DistributionChange(apply) })
	server.availabilityMu.RLock()
	stopped := make(chan struct{})
	go func() {
		machine.Stop(adsync.ReasonRevoked)
		close(stopped)
	}()
	select {
	case <-stopped:
		server.availabilityMu.RUnlock()
		t.Fatal("the stop did not wait for the in-flight write")
	case <-time.After(50 * time.Millisecond):
	}
	server.availabilityMu.RUnlock()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the stop did not complete")
	}
	if instance := machine.Instance(); instance.Liveness != adsync.LivenessStopped {
		t.Fatalf("machine liveness after the stop = %v", instance.Liveness)
	}
}
