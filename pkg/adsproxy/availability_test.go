// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"bufio"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

type heldDataWriter struct {
	*httptest.ResponseRecorder
	entered                     chan struct{}
	release                     chan struct{}
	readDeadline, writeDeadline time.Time
}

func (w *heldDataWriter) WriteHeader(code int) {
	close(w.entered)
	<-w.release
	w.ResponseRecorder.WriteHeader(code)
}

func (w *heldDataWriter) SetReadDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		w.readDeadline = deadline
	}
	return nil
}

func (w *heldDataWriter) SetWriteDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		w.writeDeadline = deadline
	}
	return nil
}

// Every data route, including conditional snapshots and forwarded authz,
// must finish its response before a pause/stop or Close can complete.
func TestDataResponsesSerializeWithDistributionStop(t *testing.T) {
	for _, transition := range []string{"pause/stop", "close"} {
		for _, route := range []string{
			"/services", "/services/" + testServiceID1 + "/state",
			"/services/" + testServiceID1 + "/snapshot",
			"/services/" + testServiceID1 + "/snapshot?conditional",
			"/services/" + testServiceID1 + "/changes?since=1",
			"/authz-check/v1/is-allowed", "/authz-check/v1/all-granted-roles",
			"/authz-check/v1/all-granted-permissions",
		} {
			t.Run(transition+route, func(t *testing.T) {
				mirror, books, _ := newPopulatedMirror(t)
				var paused atomic.Bool
				server, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror, Paused: paused.Load})
				if err != nil {
					t.Fatal(err)
				}
				defer server.Close()
				authz := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
				handler, err := DownstreamHandler(server, AuthzHandlers{authz, authz, authz})
				if err != nil {
					t.Fatal(err)
				}
				path, method, want := adsync.APIPrefix+route, http.MethodGet, http.StatusOK
				if strings.HasPrefix(route, "/authz-check/") {
					path, method = route, http.MethodPost
				}
				request := httptest.NewRequest(method, path, nil)
				if strings.HasSuffix(route, "?conditional") {
					request.URL.RawQuery = ""
					request.Header.Set("If-None-Match", snapshotETag(*books.snapshotResponse(t)))
					want = http.StatusNotModified
				}
				cert := &x509.Certificate{Raw: []byte("client")}
				request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}
				writer := &heldDataWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
				var once sync.Once
				release := func() { once.Do(func() { close(writer.release) }) }
				defer release()
				answered := make(chan struct{})
				go func() { handler.ServeHTTP(writer, request); close(answered) }()
				select {
				case <-writer.entered:
				case <-time.After(5 * time.Second):
					t.Fatal("response did not reach the writer")
				}
				started, changed := make(chan struct{}), make(chan struct{})
				go func() {
					close(started)
					if transition == "close" {
						server.Close()
					} else {
						server.DistributionChange(func() { paused.Store(true) })
					}
					close(changed)
				}()
				<-started
				select {
				case <-changed:
					t.Fatal("transition completed during a data response")
				case <-time.After(20 * time.Millisecond):
				}
				release()
				select {
				case <-answered:
				case <-time.After(5 * time.Second):
					t.Fatal("response did not finish")
				}
				select {
				case <-changed:
				case <-time.After(5 * time.Second):
					t.Fatal("transition did not finish")
				}
				if writer.Code != want {
					t.Fatalf("response = %d, want %d", writer.Code, want)
				}
				if writer.readDeadline.IsZero() || writer.writeDeadline.IsZero() {
					t.Fatal("response had no network deadline")
				}
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				code := "distribution_paused"
				if transition == "close" {
					code = "source_unavailable"
				}
				if recorder.Code != http.StatusServiceUnavailable || recorder.Body.String() != `{"error":"`+code+`"}` {
					t.Fatalf("request after transition = %d %s", recorder.Code, recorder.Body.String())
				}
			})
		}
	}
}

// Forwarding can revoke the relay itself. It must neither deadlock on the
// availability lock nor publish the response prepared before that revocation.
func TestAuthorizationRevocationDiscardsPreparedResponse(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	var paused atomic.Bool
	server, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror, Paused: paused.Load})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	authz := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "stale")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"allowed":true}`))
		server.DistributionChange(func() { paused.Store(true) })
	})
	handler, err := DownstreamHandler(server, AuthzHandlers{authz, authz, authz})
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{Raw: []byte("client")}
	for _, operation := range []string{"is-allowed", "all-granted-roles", "all-granted-permissions"} {
		server.DistributionChange(func() { paused.Store(false) })
		request := httptest.NewRequest(http.MethodPost, "/authz-check/v1/"+operation, nil)
		request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}
		recorder := httptest.NewRecorder()
		answered := make(chan struct{})
		go func() { handler.ServeHTTP(recorder, request); close(answered) }()
		select {
		case <-answered:
		case <-time.After(5 * time.Second):
			t.Fatal("authorization revocation deadlocked")
		}
		if recorder.Code != http.StatusServiceUnavailable || recorder.Body.String() != `{"error":"distribution_paused"}` || recorder.Header().Get("ETag") != "" {
			t.Fatalf("response after revocation = %d %s headers %v", recorder.Code, recorder.Body.String(), recorder.Header())
		}
	}
}

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

// silentPipeWebsocket returns a websocket whose peer completes the handshake
// and then never reads another byte. net.Pipe is unbuffered, so a frame the
// relay writes blocks until the peer reads or the write deadline expires:
// it behaves like a downstream that stopped reading its socket.
func silentPipeWebsocket(t *testing.T) *websocket.Conn {
	t.Helper()
	peer, local := net.Pipe()
	t.Cleanup(func() {
		_ = local.Close()
		_ = peer.Close()
	})
	handshaken := make(chan error, 1)
	go func() {
		request, err := http.ReadRequest(bufio.NewReader(peer))
		if err != nil {
			handshaken <- err
			return
		}
		accept := sha1.Sum([]byte(request.Header.Get("Sec-WebSocket-Key") +
			"258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, err = io.WriteString(peer, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\nConnection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(accept[:])+"\r\n\r\n")
		handshaken <- err
	}()
	conn, _, err := websocket.NewClient(local,
		&url.URL{Scheme: "ws", Host: "relay.test", Path: adsync.APIPrefix + "/channel"}, nil, 0, 0)
	if err != nil {
		t.Fatalf("open silent pipe websocket: %v", err)
	}
	if err := <-handshaken; err != nil {
		t.Fatalf("silent pipe websocket handshake: %v", err)
	}
	return conn
}

// TestAvailabilityFlipDoesNotWaitForASlowPeer proves the close frames of a
// pause or stop are written outside the availability write lock. A downstream
// that never reads must not delay the flip, and requests must keep being
// answered while its pending close frame blocks.
func TestAvailabilityFlipDoesNotWaitForASlowPeer(t *testing.T) {
	transitions := []string{"watcher", "distribution change"}
	for _, transition := range transitions {
		t.Run(transition, func(t *testing.T) {
			mirror, _, _ := newPopulatedMirror(t)
			var paused atomic.Bool
			server, err := NewServer(Config{InstanceID: testProxyID, Mirror: mirror, Paused: paused.Load})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			slow := silentPipeWebsocket(t)
			identity := sessionIdentity{fingerprint: "slow-peer", uuid: testClientInstanceB}
			server.mu.Lock()
			server.channels[identity] = slow
			server.sessions[identity] = 1
			server.mu.Unlock()

			paused.Store(true)
			finished := make(chan struct{})
			go func() {
				if transition == "watcher" {
					server.refreshAvailability()
				} else {
					server.DistributionChange(func() {})
				}
				close(finished)
			}()
			select {
			case <-finished:
			case <-time.After(2 * time.Second):
				t.Fatal("the availability flip waited for a peer that never reads")
			}

			// The pending close frame must not hold the availability lock, so
			// a data request is answered while its write is still blocked.
			request := httptest.NewRequest(http.MethodGet, adsync.APIPrefix+"/services", nil)
			cert := &x509.Certificate{Raw: []byte("client")}
			request.TLS = &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
				VerifiedChains:   [][]*x509.Certificate{{cert}},
			}
			recorder := httptest.NewRecorder()
			answered := make(chan struct{})
			go func() {
				server.ServeHTTP(recorder, request)
				close(answered)
			}()
			select {
			case <-answered:
			case <-time.After(2 * time.Second):
				t.Fatal("a data response waited for a peer that never reads")
			}
			if recorder.Code != http.StatusServiceUnavailable ||
				recorder.Body.String() != `{"error":"distribution_paused"}` {
				t.Fatalf("data response while the close frame is blocked = %d %s",
					recorder.Code, recorder.Body.String())
			}
		})
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
