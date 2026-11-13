// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func pollBody(t *testing.T, instanceID string) []byte {
	t.Helper()
	payload, err := json.Marshal(heartbeatFrame(instanceID))
	if err != nil {
		t.Fatalf("encode heartbeat: %v", err)
	}
	return payload
}

func TestChannelWebSocketHelloAndHeartbeat(t *testing.T) {
	mirror, books, _ := newPopulatedMirror(t)
	books.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	server := newTestServer(t, mirror, nil)
	conn, response, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial channel: %v (response %v)", err, response)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	kind, payload := readChannelFrame(t, conn)
	if kind != "ack" {
		t.Fatalf("first frame = %q", kind)
	}
	ack := decodeBody[ackMessage](t, payload)
	if len(ack.Services) != 2 {
		t.Fatalf("ack services = %+v", ack.Services)
	}
	for _, service := range ack.Services {
		if service.ID == testServiceID1 && service.Head != 2 {
			t.Errorf("books head = %d, want 2", service.Head)
		}
	}
	if ack.HeartbeatInterval != "30s" || ack.StaleAfter != "60s" || ack.StopAfter != "120s" {
		t.Errorf("thresholds = %s/%s/%s", ack.HeartbeatInterval, ack.StaleAfter, ack.StopAfter)
	}
	if ack.Commands == nil || len(ack.Commands) != 0 {
		t.Errorf("commands = %+v", ack.Commands)
	}
	for _, service := range ack.Services {
		if service.Head != service.OriginHead || service.HeadStateHash.IsZero() {
			t.Errorf("ack service = %+v", service)
		}
	}

	writeJSONFrame(t, conn, heartbeatFrame(testClientInstanceID))
	kind, _ = readChannelFrame(t, conn)
	if kind != "ack" {
		t.Fatalf("heartbeat response = %q", kind)
	}
}

// TestChannelRefusesChainLoops proves a relay refuses a downstream that
// reports the relay's own instance id (an upstream URL aimed at the relay's
// own downstream endpoint) or one of the relay's upstream ancestors (a
// two-node cycle), over both transports. A legitimate child is still served.
func TestChannelRefusesChainLoops(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, &testServerOptions{
		chain: []string{testUpstreamID, testProxyID},
	})
	cases := []struct {
		name       string
		instanceID string
	}{
		{name: "own instance", instanceID: testProxyID},
		{name: "upstream ancestor", instanceID: testUpstreamID},
	}
	for _, testCase := range cases {
		t.Run("websocket "+testCase.name, func(t *testing.T) {
			conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close()
			writeJSONFrame(t, conn, helloFrame(testCase.instanceID))
			code, reason := readClose(t, conn)
			if code != websocket.ClosePolicyViolation || reason != "chain_loop" {
				t.Fatalf("close = %d %q", code, reason)
			}
		})
		t.Run("poll "+testCase.name, func(t *testing.T) {
			response, body := server.post(server.defaultClient(),
				adsync.APIPrefix+"/channel/poll?wait=0", pollBody(t, testCase.instanceID))
			if response.StatusCode != http.StatusForbidden || string(body) != `{"error":"chain_loop"}` {
				t.Fatalf("poll = status %d body %s", response.StatusCode, body)
			}
			if count := server.server.DownstreamCount(); count != 0 {
				t.Fatalf("refused polls counted %d sessions", count)
			}
		})
	}

	t.Run("legitimate child", func(t *testing.T) {
		conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
		if kind, _ := readChannelFrame(t, conn); kind != "ack" {
			t.Fatalf("first frame = %q", kind)
		}
	})
}

func TestChannelWebSocketFrameValidation(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)

	cases := []struct {
		name  string
		frame any
	}{
		{name: "invalid hello type", frame: map[string]any{"type": "heartbeat", "instanceId": testClientInstanceID}},
		{name: "invalid instance id", frame: helloFrame("not-a-uuid")},
		{name: "duplicate service", frame: map[string]any{
			"type": "hello", "instanceId": testClientInstanceID, "agentVersion": "test",
			"services": []map[string]any{
				{"id": testServiceID1, "name": "books", "revision": 1, "state": "in_sync"},
				{"id": testServiceID1, "name": "books", "revision": 1, "state": "in_sync"},
			},
		}},
		{name: "unknown field", frame: map[string]any{
			"type": "hello", "instanceId": testClientInstanceID, "bogus": true,
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close()
			writeJSONFrame(t, conn, testCase.frame)
			code, _ := readClose(t, conn)
			if code != websocket.CloseProtocolError {
				t.Fatalf("close code = %d", code)
			}
		})
	}

	t.Run("heartbeat instance change", func(t *testing.T) {
		conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
		if kind, _ := readChannelFrame(t, conn); kind != "ack" {
			t.Fatalf("first frame = %q", kind)
		}
		writeJSONFrame(t, conn, heartbeatFrame(testClientInstanceB))
		code, reason := readClose(t, conn)
		if code != websocket.CloseProtocolError || reason != "instance_changed" {
			t.Fatalf("close = %d %q", code, reason)
		}
	})

	t.Run("duplicate hello", func(t *testing.T) {
		conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
		if kind, _ := readChannelFrame(t, conn); kind != "ack" {
			t.Fatalf("first frame = %q", kind)
		}
		writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
		code, _ := readClose(t, conn)
		if code != websocket.CloseProtocolError {
			t.Fatalf("close code = %d", code)
		}
	})

	t.Run("unknown frame", func(t *testing.T) {
		conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
		if kind, _ := readChannelFrame(t, conn); kind != "ack" {
			t.Fatalf("first frame = %q", kind)
		}
		writeJSONFrame(t, conn, map[string]any{"type": "nonsense"})
		code, _ := readClose(t, conn)
		if code != websocket.CloseProtocolError {
			t.Fatalf("close code = %d", code)
		}
	})
}

func TestChannelAuthenticationFailsWithoutTrustedCertificate(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)

	cases := []struct {
		name string
		dial func() (*websocket.Conn, *http.Response, error)
	}{
		{name: "missing certificate", dial: server.dialAnonymous},
		{name: "expired certificate", dial: func() (*websocket.Conn, *http.Response, error) {
			return server.dial(server.pki.expiredCertFile, server.pki.expiredKeyFile)
		}},
		{name: "untrusted certificate", dial: func() (*websocket.Conn, *http.Response, error) {
			return server.dial(server.pki.untrustedCertFile, server.pki.untrustedKeyFile)
		}},
		{name: "revoked certificate", dial: func() (*websocket.Conn, *http.Response, error) {
			return server.dial(server.pki.revokedCertFile, server.pki.revokedKeyFile)
		}},
		{name: "serverAuth-only certificate", dial: func() (*websocket.Conn, *http.Response, error) {
			return server.dial(server.pki.serverOnlyCertFile, server.pki.serverOnlyKeyFile)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			conn, _, err := testCase.dial()
			if err == nil {
				conn.Close()
				t.Fatal("channel was accepted without a usable certificate")
			}
		})
	}

	// The same failures hit ordinary data requests.
	for _, files := range [][2]string{
		{server.pki.expiredCertFile, server.pki.expiredKeyFile},
		{server.pki.untrustedCertFile, server.pki.untrustedKeyFile},
	} {
		client := server.client(files[0], files[1])
		if _, err := client.Get(server.address + adsync.APIPrefix + "/services"); err == nil {
			t.Fatalf("%s: data request was accepted", files[0])
		}
	}
}

func TestHubCoalescesNotifications(t *testing.T) {
	hub, err := NewHub(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	defer hub.Close()
	first, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	second, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	hub.Publish(testServiceID1, 2)
	hub.Publish(testServiceID1, 3)
	hub.Publish(testServiceID1, 4)
	hub.Publish(testServiceID2, 1)
	hub.Publish(testServiceID2, 2)

	received := collectNotifications(t, first.C(), 2, 2*time.Second)
	if received[testServiceID1] != 4 || received[testServiceID2] != 2 {
		t.Errorf("coalesced heads = %v", received)
	}
	secondReceived := collectNotifications(t, second.C(), 2, 2*time.Second)
	if secondReceived[testServiceID1] != 4 || secondReceived[testServiceID2] != 2 {
		t.Errorf("second subscriber heads = %v", secondReceived)
	}
	select {
	case extra := <-first.C():
		t.Errorf("unexpected notification %+v", extra)
	case <-time.After(200 * time.Millisecond):
	}
	hub.Unsubscribe(first)
	hub.Unsubscribe(first)
	if hub.Subscribers() != 1 {
		t.Errorf("subscribers = %d", hub.Subscribers())
	}
}

func collectNotifications(t *testing.T, incoming <-chan Notify, want int, timeout time.Duration) map[string]uint64 {
	t.Helper()
	received := make(map[string]uint64, want)
	deadline := time.After(timeout)
	for len(received) < want {
		select {
		case notification := <-incoming:
			received[notification.ServiceID] = notification.Head
		case <-deadline:
			t.Fatalf("received %v, want %d notifications", received, want)
		}
	}
	return received
}

func TestHubHasNoSubscriberLimit(t *testing.T) {
	hub, err := NewHub(time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	defer hub.Close()
	subscribers := make([]*Subscriber, 0, 200)
	for index := 0; index < 200; index++ {
		subscriber, err := hub.Subscribe()
		if err != nil {
			t.Fatalf("subscribe %d: %v", index, err)
		}
		subscribers = append(subscribers, subscriber)
	}
	if got := hub.Subscribers(); got != 200 {
		t.Errorf("subscribers = %d, want 200", got)
	}
	for _, subscriber := range subscribers {
		hub.Unsubscribe(subscriber)
	}
	if hub.Subscribers() != 0 {
		t.Errorf("subscribers after unsubscribe = %d", hub.Subscribers())
	}
	hub.Close()
	if _, err := hub.Subscribe(); err == nil {
		t.Error("closed hub accepted a subscriber")
	}
}

func TestPollWaitBounds(t *testing.T) {
	for _, testCase := range []struct {
		text string
		want time.Duration
		ok   bool
	}{
		{text: "", want: defaultPollWait, ok: true},
		{text: "0", want: 0, ok: true},
		{text: "25", want: 25 * time.Second, ok: true},
		{text: "30", want: 30 * time.Second, ok: true},
		{text: "31", ok: false},
		{text: "01", ok: false},
		{text: "+5", ok: false},
		{text: "-1", ok: false},
		{text: "9223372036854775807", ok: false},
		{text: "99999999999999999999999999", ok: false},
		{text: "1.5", ok: false},
	} {
		got, ok := parsePollWait(testCase.text)
		if ok != testCase.ok || got != testCase.want {
			t.Errorf("parsePollWait(%q) = %v, %v; want %v, %v", testCase.text, got, ok, testCase.want, testCase.ok)
		}
	}
}

func TestPollReturnsAck(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	response, body := server.post(server.defaultClient(),
		adsync.APIPrefix+"/channel/poll?wait=0", pollBody(t, testClientInstanceID))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("poll: status %d body %s", response.StatusCode, body)
	}
	ack := decodeBody[ackMessage](t, body)
	if ack.Type != "ack" || len(ack.Services) != 2 || ack.ServicesVersion <= 0 {
		t.Errorf("ack = %+v", ack)
	}
}

func TestPollRejectsInvalidRequests(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.defaultClient()
	for _, wait := range []string{"31", "01", "9223372036854775807", "9223372037"} {
		path := adsync.APIPrefix + "/channel/poll?wait=" + wait
		response, body := server.post(client, path, pollBody(t, testClientInstanceID))
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("wait=%s: status %d body %s", wait, response.StatusCode, body)
		}
	}
	for _, body := range [][]byte{
		[]byte(`not json`),
		[]byte(`{"type":"heartbeat"}`),
		[]byte(`{"type":"heartbeat","instanceId":"not-a-uuid"}`),
		[]byte(`{"type":"heartbeat","instanceId":"` + testClientInstanceID + `","bogus":true}`),
		[]byte(`{"type":"hello","instanceId":"` + testClientInstanceID + `"}`),
	} {
		response, payload := server.post(client, adsync.APIPrefix+"/channel/poll?wait=0", body)
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status %d payload %s", body, response.StatusCode, payload)
		}
	}
	oversized := make([]byte, maxChannelFrameBytes+1)
	oversized[0] = '{'
	response, _ := server.post(client, adsync.APIPrefix+"/channel/poll?wait=0", oversized)
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: status %d", response.StatusCode)
	}
}

func TestPollWaitsForANotification(t *testing.T) {
	mirror, books, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	result := make(chan *http.Response, 1)
	go func() {
		// The hold is shorter than the wait, so a returned ack proves the
		// notification woke the poll.
		response, _ := server.post(server.defaultClient(),
			adsync.APIPrefix+"/channel/poll?wait=30", pollBody(t, testClientInstanceID))
		result <- response
	}()
	waitFor(t, 2*time.Second, "the poll to be held", func() bool {
		return server.server.DownstreamCount() == 1
	})
	books.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	select {
	case response := <-result:
		if response.StatusCode != http.StatusOK {
			t.Fatalf("poll status = %d", response.StatusCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the poll was not woken by the notification")
	}
}

func TestChannelRejectsNonCanonicalQuery(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	client := server.defaultClient()
	for _, path := range []string{
		adsync.APIPrefix + "/services?unknown=1",
		adsync.APIPrefix + "/services/" + testServiceID1 + "/state?x=1&x=2",
		adsync.APIPrefix + "/services/" + testServiceID1 + "/snapshot?unknown=1",
		adsync.APIPrefix + "/channel?unknown=1",
	} {
		response, body := server.get(client, path, nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s: status %d body %s", path, response.StatusCode, body)
		}
	}
	for _, query := range []string{"wait=01", "wait=1&wait=2", "wait=", "unknown=1"} {
		path := adsync.APIPrefix + "/channel/poll?" + query
		response, body := server.post(client, path, pollBody(t, testClientInstanceID))
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("POST %s: status %d body %s", path, response.StatusCode, body)
		}
	}
}

func TestChannelRejectsNonTextFrames(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3}); err != nil {
		t.Fatalf("write binary frame: %v", err)
	}
	code, _ := readClose(t, conn)
	if code != websocket.CloseProtocolError {
		t.Fatalf("close code = %d", code)
	}
}

func TestChannelRejectsTrailingJSON(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	payload, err := json.Marshal(helloFrame(testClientInstanceID))
	if err != nil {
		t.Fatalf("encode hello: %v", err)
	}
	payload = append(payload, []byte(`{"type":"hello"}`)...)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	code, _ := readClose(t, conn)
	if code != websocket.CloseProtocolError {
		t.Fatalf("close code = %d", code)
	}
}

func TestChannelRequiresUpgrade(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	request, err := http.NewRequest(http.MethodGet, server.address+adsync.APIPrefix+"/channel", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := server.defaultClient().Do(request)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestChannelInitialAckRespectsPause(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	var paused atomic.Bool
	server := newTestServer(t, mirror, &testServerOptions{paused: paused.Load})
	conn, _, err := server.dial(server.pki.clientCertFile, server.pki.clientKeyFile)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	paused.Store(true)
	writeJSONFrame(t, conn, helloFrame(testClientInstanceID))
	code, reason := readClose(t, conn)
	if code != websocket.ClosePolicyViolation || reason != "distribution_paused" {
		t.Fatalf("close = %d %q", code, reason)
	}
}

func TestPollBodyIsStrictlyDecoded(t *testing.T) {
	mirror, _, _ := newPopulatedMirror(t)
	server := newTestServer(t, mirror, nil)
	body := pollBody(t, testClientInstanceID)
	body = bytes.Replace(body, []byte(`"seq":1`), []byte(`"seq":1,"unknown":"x"`), 1)
	response, payload := server.post(server.defaultClient(), adsync.APIPrefix+"/channel/poll?wait=0", body)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d body %s", response.StatusCode, payload)
	}
}
