// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func heartbeatFrame(chain []string) []byte {
	payload, err := json.Marshal(heartbeatMessage{
		Type:     "heartbeat",
		Seq:      1,
		Services: []heartbeatService{},
		Liveness: "live",
		Chain:    chain,
		Fallback: fallbackMessage{Services: []string{}},
	})
	if err != nil {
		panic(err)
	}
	return payload
}

func TestChannelChainValidationLongPoll(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		chain: []string{testUpstreamID, testProxyID},
	}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	path := adsync.APIPrefix + "/channel/poll?wait=0"

	response, body := server.post(path, token, heartbeatFrame([]string{testProxyID, testDownstreamID}))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid chain: status %d body %s", response.StatusCode, body)
	}
	ack := decodeBody[ackMessage](t, body)
	if ack.Type != "ack" || len(ack.Services) != 2 || ack.ServicesVersion <= 0 {
		t.Errorf("ack = %+v", ack)
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

	response, body = server.post(path, token, heartbeatFrame([]string{testProxyID, testDownstreamID, testProxyID}))
	if response.StatusCode != http.StatusForbidden || string(body) != `{"error":"chain_loop"}` {
		t.Errorf("repeated instance: status %d body %s", response.StatusCode, body)
	}

	// The sender is one of this proxy's own upstream instances: a cycle.
	response, body = server.post(path, token, heartbeatFrame([]string{testProxyID, testUpstreamID}))
	if response.StatusCode != http.StatusForbidden || string(body) != `{"error":"chain_loop"}` {
		t.Errorf("ancestor cycle: status %d body %s", response.StatusCode, body)
	}

	response, body = server.post(path, token, heartbeatFrame(
		[]string{testProxyID, testDownstreamID, testDownstreamB, testUpstreamID, testUnknownID}))
	if response.StatusCode != http.StatusForbidden || string(body) != `{"error":"chain_too_deep"}` {
		t.Errorf("depth: status %d body %s", response.StatusCode, body)
	}

	response, body = server.post(path, token, heartbeatFrame([]string{"not-a-ulid"}))
	if response.StatusCode != http.StatusForbidden {
		t.Errorf("malformed chain: status %d body %s", response.StatusCode, body)
	}

	response, body = server.post(path, token, []byte(`{"type":"heartbeat","bogus":true}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown field: status %d body %s", response.StatusCode, body)
	}
}

func TestChannelWebSocketHelloAndHeartbeat(t *testing.T) {
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
	conn, response, err := server.dial(token)
	if err != nil {
		t.Fatalf("dial channel: %v (response %v)", err, response)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloMessage{
		Type:         "hello",
		InstanceID:   testDownstreamID,
		Services:     []helloService{},
		Chain:        []string{testProxyID, testDownstreamID},
		AgentVersion: "test",
	})
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

	writeJSONFrame(t, conn, heartbeatMessage{
		Type:     "heartbeat",
		Seq:      1,
		Services: []heartbeatService{},
		Liveness: "live",
		Chain:    []string{testProxyID, testDownstreamID},
		Fallback: fallbackMessage{Services: []string{}},
	})
	kind, _ = readChannelFrame(t, conn)
	if kind != "ack" {
		t.Fatalf("heartbeat response = %q", kind)
	}
}

func TestChannelWebSocketChainRejections(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, orders := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		chain: []string{testUpstreamID, testProxyID},
	}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
		services:     []string{testServiceID1},
	})
	_ = orders

	// The sender is one of this proxy's own ancestors: a loop.
	conn, _, err := server.dial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	writeJSONFrame(t, conn, helloMessage{
		Type:         "hello",
		InstanceID:   testUpstreamID,
		Services:     []helloService{},
		Chain:        []string{testProxyID, testUpstreamID},
		AgentVersion: "test",
	})
	code, reason := readClose(t, conn)
	conn.Close()
	if code != websocket.ClosePolicyViolation || reason != "chain_loop" {
		t.Errorf("ancestor loop: close %d %q", code, reason)
	}

	// A chain deeper than the protocol allows.
	conn, _, err = server.dial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	writeJSONFrame(t, conn, helloMessage{
		Type:         "hello",
		InstanceID:   testDownstreamID,
		Services:     []helloService{},
		Chain:        []string{testProxyID, testDownstreamID, testDownstreamB, testUpstreamID, testUnknownID},
		AgentVersion: "test",
	})
	code, reason = readClose(t, conn)
	conn.Close()
	if code != websocket.ClosePolicyViolation || reason != "chain_too_deep" {
		t.Errorf("depth: close %d %q", code, reason)
	}

	// The hello subscribes to a service outside the credential's subset.
	conn, _, err = server.dial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	writeJSONFrame(t, conn, helloMessage{
		Type:         "hello",
		InstanceID:   testDownstreamID,
		Services:     []helloService{{ID: testServiceID2, Name: "orders"}},
		Chain:        []string{testProxyID, testDownstreamID},
		AgentVersion: "test",
	})
	code, reason = readClose(t, conn)
	conn.Close()
	if code != websocket.ClosePolicyViolation || reason != "forbidden" {
		t.Errorf("subset: close %d %q", code, reason)
	}

	// A downstream claiming this proxy's own instance id.
	conn, _, err = server.dial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	writeJSONFrame(t, conn, helloMessage{
		Type:         "hello",
		InstanceID:   testProxyID,
		Services:     []helloService{},
		Chain:        []string{testProxyID},
		AgentVersion: "test",
	})
	code, reason = readClose(t, conn)
	conn.Close()
	if code != websocket.ClosePolicyViolation || reason != "chain_loop" {
		t.Errorf("self claim: close %d %q", code, reason)
	}
}

func TestChannelWebSocketAuthentication(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	for _, credential := range []string{"", testToken(testDownstreamB, 7)} {
		conn, response, err := server.dial(credential)
		if err == nil {
			conn.Close()
			t.Fatal("unauthenticated channel was accepted")
		}
		if !errors.Is(err, websocket.ErrBadHandshake) {
			t.Fatalf("dial error = %v", err)
		}
		if response == nil || response.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %v", response)
		}
	}
}

func TestDownstreamChannelEndToEnd(t *testing.T) {
	signer := newTestSigner(t)
	mirror, books, orders := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		coalesceWindow: 300 * time.Millisecond,
	}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})

	client := server.newClient(token)
	machine, err := adsync.NewMachine(adsync.MachineOptions{})
	if err != nil {
		t.Fatalf("new machine: %v", err)
	}
	updates := make(chan adsync.Notify, 16)
	channel, err := adsync.NewChannel(client, adsync.ChannelOptions{
		InstanceID:   testDownstreamID,
		AgentVersion: "test",
		Chain:        []string{testProxyID, testDownstreamID},
		Machine:      machine,
		Transport:    adsync.TransportWebSocket,
		OnNotify: func(notification adsync.Notify) {
			select {
			case updates <- notification:
			default:
			}
		},
	})
	if err != nil {
		t.Fatalf("new channel: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- channel.Run(ctx) }()
	defer func() {
		cancel()
		// The downstream client's WebSocket read only notices cancellation at
		// its read deadline; closing the proxy's sessions makes the test quick.
		server.server.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("channel did not stop")
		}
	}()

	waitFor(t, 5*time.Second, "the first ack", func() bool {
		status, found := machine.Status("books")
		return found && status.Service.Head == 1
	})

	// Two revisions inside one coalescing window produce one notification
	// carrying the latest head.
	books.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 1, 100)); err != nil {
		t.Fatalf("apply first change: %v", err)
	}
	books.publish(t, signer, upsertAuthz(testServiceID1+"A2", "read-more", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 2, 100)); err != nil {
		t.Fatalf("apply second change: %v", err)
	}

	select {
	case notification := <-updates:
		if notification.ServiceID != testServiceID1 || notification.Head != 3 {
			t.Errorf("notification = %+v, want books at 3", notification)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notification arrived")
	}
	select {
	case extra := <-updates:
		t.Errorf("unexpected extra notification %+v", extra)
	case <-time.After(500 * time.Millisecond):
	}
	_ = orders
}

func TestLongPollChannelEndToEnd(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	client := server.newClient(token)
	machine, err := adsync.NewMachine(adsync.MachineOptions{})
	if err != nil {
		t.Fatalf("new machine: %v", err)
	}
	channel, err := adsync.NewChannel(client, adsync.ChannelOptions{
		InstanceID:   testDownstreamID,
		AgentVersion: "test",
		Chain:        []string{testProxyID, testDownstreamID},
		Machine:      machine,
		Transport:    adsync.TransportLongPoll,
		PollWait:     time.Second,
	})
	if err != nil {
		t.Fatalf("new channel: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- channel.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("channel did not stop")
		}
	}()

	waitFor(t, 5*time.Second, "a confirming long-poll ack", func() bool {
		status, found := machine.Status("orders")
		return found && status.Service.Head == 1
	})
}

func TestHubCoalescesNotifications(t *testing.T) {
	hub, err := NewHub(10, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	defer hub.Close()
	all, err := hub.Subscribe("edge-1", nil)
	if err != nil {
		t.Fatalf("subscribe all: %v", err)
	}
	onlyOrders, err := hub.Subscribe("edge-2", func(serviceID string) bool {
		return serviceID == testServiceID2
	})
	if err != nil {
		t.Fatalf("subscribe orders: %v", err)
	}

	hub.Publish(testServiceID1, 2)
	hub.Publish(testServiceID1, 3)
	hub.Publish(testServiceID1, 4)
	hub.Publish(testServiceID2, 1)
	hub.Publish(testServiceID2, 2)

	received := collectNotifications(t, all.C(), 2, 2*time.Second)
	if received[testServiceID1] != 4 || received[testServiceID2] != 2 {
		t.Errorf("coalesced heads = %v", received)
	}
	filtered := collectNotifications(t, onlyOrders.C(), 1, 2*time.Second)
	if len(filtered) != 1 || filtered[testServiceID2] != 2 {
		t.Errorf("filtered notifications = %v", filtered)
	}
	select {
	case extra := <-all.C():
		t.Errorf("unexpected notification %+v", extra)
	case <-time.After(200 * time.Millisecond):
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

func TestHubFanoutLimit(t *testing.T) {
	hub, err := NewHub(50, time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	defer hub.Close()
	for index := 0; index < 50; index++ {
		if _, err := hub.Subscribe("edge-"+string(rune('A'+index)), nil); err != nil {
			t.Fatalf("subscribe %d: %v", index, err)
		}
	}
	if _, err := hub.Subscribe("edge-overflow", nil); !errors.Is(err, ErrFanoutLimit) {
		t.Fatalf("51st downstream: %v, want ErrFanoutLimit", err)
	}
	if err := hub.Allow("edge-overflow"); !errors.Is(err, ErrFanoutLimit) {
		t.Fatalf("Allow: %v, want ErrFanoutLimit", err)
	}
	if got := hub.Downstreams(); got != 50 {
		t.Errorf("downstreams = %d", got)
	}

	// A second registration from an existing downstream keeps its slot.
	second, err := hub.Subscribe("edge-A", nil)
	if err != nil {
		t.Fatalf("second registration: %v", err)
	}
	if got := hub.Downstreams(); got != 50 {
		t.Errorf("downstreams after reconnect = %d", got)
	}
	hub.Unsubscribe(second)
}

func TestServerFanoutLimit(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	hub, err := NewHub(1, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if _, err := hub.Subscribe(testDownstreamB, nil); err != nil {
		t.Fatalf("reserve downstream: %v", err)
	}
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{hub: hub}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})

	conn, response, err := server.dial(token)
	if err == nil {
		conn.Close()
		t.Fatal("channel beyond the fan-out limit was accepted")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("channel status = %v", response)
	}
	if response.Header.Get("Retry-After") != "30" {
		t.Errorf("Retry-After = %q", response.Header.Get("Retry-After"))
	}
	response, body := server.post(adsync.APIPrefix+"/channel/poll?wait=0", token, heartbeatFrame([]string{testProxyID}))
	if response.StatusCode != http.StatusServiceUnavailable || string(body) != `{"error":"fanout_limit"}` {
		t.Errorf("poll beyond the limit: status %d body %s", response.StatusCode, body)
	}
	// Data endpoints are not part of the fan-out limit.
	response, _ = server.get(adsync.APIPrefix+"/services", token, nil)
	if response.StatusCode != http.StatusOK {
		t.Errorf("services beyond the limit: status %d", response.StatusCode)
	}
}

func TestCredentialReloadClosesChannelsWhoseGrantsChanged(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		subset string
	}{
		{name: "removed", subset: ""},
		{name: "narrowed", subset: testServiceID1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			signer := newTestSigner(t)
			mirror, _, _ := newPopulatedMirror(t, signer)
			token := testToken(testDownstreamID, 1)
			server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
				downstreamID: testDownstreamID,
				token:        token,
			})
			conn, _, err := server.dial(token)
			if err != nil {
				t.Fatalf("dial: %v", err)
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
				t.Fatal("channel did not receive its initial ack")
			}

			line := testDownstreamB + ":" + hashToken(testToken(testDownstreamB, 2)) + ":*"
			if testCase.subset != "" {
				line = testDownstreamID + ":" + hashToken(token) + ":" + testCase.subset
			}
			server.writeCredentialFile(line)
			if err := server.server.ReloadCredentials(); err != nil {
				t.Fatalf("ReloadCredentials: %v", err)
			}
			code, reason := readClose(t, conn)
			if code != websocket.CloseTryAgainLater || reason != "credentials_reloaded" {
				t.Fatalf("close = %d %q", code, reason)
			}

			if testCase.name == "removed" {
				response, _ := server.get(adsync.APIPrefix+"/services", token, nil)
				if response.StatusCode != http.StatusUnauthorized {
					t.Fatalf("removed token HTTP status = %d", response.StatusCode)
				}
				return
			}
			response, _ := server.get(adsync.APIPrefix+"/services/"+testServiceID2+"/snapshot", token, nil)
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("narrowed service status = %d", response.StatusCode)
			}
			reconnected, _, err := server.dial(token)
			if err != nil {
				t.Fatalf("reconnect with narrowed grant: %v", err)
			}
			defer reconnected.Close()
			writeJSONFrame(t, reconnected, helloMessage{
				Type:         "hello",
				InstanceID:   testDownstreamID,
				Services:     []helloService{},
				Chain:        []string{testProxyID, testDownstreamID},
				AgentVersion: "test",
			})
			kind, payload := readChannelFrame(t, reconnected)
			if kind != "ack" {
				t.Fatalf("reconnected frame = %q", kind)
			}
			ack := decodeBody[ackMessage](t, payload)
			if len(ack.Services) != 1 || ack.Services[0].ID != testServiceID1 {
				t.Fatalf("reconnected service set = %+v", ack.Services)
			}
		})
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

func TestPollRejectsOverflowingWait(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	for _, wait := range []string{"31", "01", "9223372036854775807", "9223372037"} {
		path := adsync.APIPrefix + "/channel/poll?wait=" + wait
		response, body := server.post(path, token, heartbeatFrame([]string{testProxyID, testDownstreamID}))
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("wait=%s: status %d body %s", wait, response.StatusCode, body)
		}
	}
}

func TestHeartbeatAncestorChainWithoutThisProxy(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		chain: []string{testUpstreamID, testProxyID},
	}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	path := adsync.APIPrefix + "/channel/poll?wait=0"

	// The chain lists only an ancestor and omits this proxy. The last entry
	// still names the sender, so the ancestor check must reject it: allowing it
	// would let a cycle form if the ancestor synchronizes from this proxy.
	response, body := server.post(path, token, heartbeatFrame([]string{testUpstreamID}))
	if response.StatusCode != http.StatusForbidden || string(body) != `{"error":"chain_loop"}` {
		t.Errorf("ancestor without this proxy: status %d body %s", response.StatusCode, body)
	}

	// An unrelated nonempty chain and the documented empty chain stay
	// legitimate: this proxy has no position in them to check.
	for _, chain := range [][]string{{testDownstreamB}, {}, {testUnknownID}} {
		response, body := server.post(path, token, heartbeatFrame(chain))
		if response.StatusCode != http.StatusOK {
			t.Errorf("chain %v: status %d body %s", chain, response.StatusCode, body)
		}
	}
}

func TestWebSocketHeartbeatAncestorChainWithoutThisProxy(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		chain: []string{testUpstreamID, testProxyID},
	}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	conn, _, err := server.dial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
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
		t.Fatalf("frame = %q", kind)
	}
	writeJSONFrame(t, conn, heartbeatMessage{
		Type:     "heartbeat",
		Services: []heartbeatService{},
		Liveness: "live",
		Chain:    []string{testUpstreamID},
		Fallback: fallbackMessage{Services: []string{}},
	})
	code, reason := readClose(t, conn)
	if code != websocket.ClosePolicyViolation || reason != "chain_loop" {
		t.Fatalf("close = %d %q", code, reason)
	}
}

func TestWebSocketHelloFromAnAncestor(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		chain: []string{testUpstreamID, testProxyID},
	}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	conn, _, err := server.dial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloMessage{
		Type:         "hello",
		InstanceID:   testUpstreamID,
		Services:     []helloService{},
		Chain:        []string{testUpstreamID},
		AgentVersion: "test",
	})
	code, reason := readClose(t, conn)
	if code != websocket.ClosePolicyViolation || reason != "chain_loop" {
		t.Fatalf("close = %d %q", code, reason)
	}
}
