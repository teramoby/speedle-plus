// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func TestSessionIdentityDeduplicatesReconnects(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)

	first, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer first.Close()
	writeJSONFrame(t, first, helloFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, first); kind != "ack" {
		t.Fatalf("first frame = %q", kind)
	}
	waitFor(t, 2*time.Second, "the first session", func() bool {
		return server.server.DownstreamCount() == 1
	})

	second, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	defer second.Close()
	writeJSONFrame(t, second, helloFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, second); kind != "ack" {
		t.Fatalf("second first frame = %q", kind)
	}

	// The replaced channel is closed and the identity counts once.
	code, reason := readClose(t, first)
	if code != websocket.CloseGoingAway || reason != "session replaced" {
		t.Errorf("replaced session close = %d %q", code, reason)
	}
	if count := server.server.DownstreamCount(); count != 1 {
		t.Errorf("DownstreamCount = %d, want 1", count)
	}

	second.Close()
	waitFor(t, 2*time.Second, "the session to end", func() bool {
		return server.server.DownstreamCount() == 0
	})
}

func TestSessionUUIDCollisionUnderAnotherCertificateKeepsBoth(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)

	first, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer first.Close()
	writeJSONFrame(t, first, helloFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, first); kind != "ack" {
		t.Fatalf("first frame = %q", kind)
	}

	second, _, err := server.dial(server.pki.clientBCertFile, server.pki.clientBKeyFile)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	defer second.Close()
	// The same UUID under another certificate is a distinct session and must
	// not evict the legitimate one.
	writeJSONFrame(t, second, helloFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, second); kind != "ack" {
		t.Fatalf("second first frame = %q", kind)
	}
	if count := server.server.DownstreamCount(); count != 2 {
		t.Fatalf("DownstreamCount = %d, want 2", count)
	}

	// The first session is still live.
	writeJSONFrame(t, first, heartbeatFrame(testClientInstanceID))
	if kind, _ := readChannelFrame(t, first); kind != "ack" {
		t.Fatalf("first heartbeat response = %q", kind)
	}
}

func TestSessionCountCountsHeldPollsOncePerIdentity(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.defaultClient()
	body := pollBody(t, testClientInstanceID)

	cancelFirst, doneFirst := startHeldPoll(t, client, server, body)
	defer cancelFirst()
	waitFor(t, 2*time.Second, "the first poll", func() bool {
		return server.server.DownstreamCount() == 1
	})
	cancelSecond, doneSecond := startHeldPoll(t, client, server, body)
	defer cancelSecond()
	// A second held poll of the same session identity is not a new session.
	if count := server.server.DownstreamCount(); count != 1 {
		t.Errorf("DownstreamCount with two held polls = %d, want 1", count)
	}
	cancelFirst()
	doneFirst()
	waitFor(t, 2*time.Second, "the first poll to end", func() bool {
		return server.server.DownstreamCount() == 1
	})
	cancelSecond()
	doneSecond()
	waitFor(t, 2*time.Second, "the session to end", func() bool {
		return server.server.DownstreamCount() == 0
	})
}

func TestSessionCountIgnoresConnectionsWithoutHello(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// A connection that never reports its UUID is not a session.
	if count := server.server.DownstreamCount(); count != 0 {
		t.Errorf("DownstreamCount = %d, want 0", count)
	}
	conn.Close()
}

func TestSessionDataRequestsAreNotSessions(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	if response, _ := server.get(server.defaultClient(), adsync.APIPrefix+"/services", nil); response.StatusCode != http.StatusOK {
		t.Fatalf("services: status %d", response.StatusCode)
	}
	if count := server.server.DownstreamCount(); count != 0 {
		t.Errorf("DownstreamCount = %d, want 0", count)
	}
}

// startHeldPoll issues one long poll that stays held until cancel runs. The
// returned done function waits for its goroutine to finish.
func startHeldPoll(t *testing.T, client *http.Client, server *testServer, body []byte) (cancel func(), done func()) {
	t.Helper()
	ctx, cancelContext := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		request, err := http.NewRequestWithContext(ctx, http.MethodPost,
			server.address+adsync.APIPrefix+"/channel/poll?wait=30", strings.NewReader(string(body)))
		if err != nil {
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	return cancelContext, func() { <-finished }
}
