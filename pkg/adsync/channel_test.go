// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// channelSource is a local policy-source channel server. It upgrades WebSocket
// requests and answers long polls with the same acks, so both transports can be
// tested against identical expectations.
type channelSource struct {
	t *testing.T

	mu              sync.Mutex
	credential      string
	services        map[string]*fixtureService
	order           []string
	servicesVersion int64

	rejectWebSocket       bool
	silentFirstConnection bool
	noAck                 bool
	// keepaliveWhileNoAck sends protocol pings on connections that never get
	// an ack, so the first-ack deadline has to be absolute.
	keepaliveWhileNoAck bool
	closeAfterAck       bool
	// ackInterval makes the source push an ack on that interval, which lets a
	// test change the effective cadence between heartbeats.
	ackInterval time.Duration
	commands    []channelCommand
	pollHold    time.Duration
	// pollStatus, pollRetryAfter and pollBody override the poll response when
	// set, so tests can exercise failure and malformed-response handling.
	pollStatus     int
	pollRetryAfter string
	pollBody       []byte
	// failPollFrom makes every poll from that request number fail with 503.
	failPollFrom int

	upgradeAttempts int
	connections     int
	hellos          []channelHello
	heartbeats      []channelHeartbeat
	pollRequests    int
	pings           int
	commandResults  []channelCommandResult

	// writeMu serializes data-frame writes: the read loop, the unsolicited ack
	// ticker and notify flushes all write to the same connection.
	writeMu sync.Mutex

	notifies chan channelNotify
	wake     chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
}

var channelUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(*http.Request) bool { return true },
}

func newChannelSource(t *testing.T, services ...*fixtureService) *channelSource {
	t.Helper()
	source := &channelSource{
		t:               t,
		credential:      testCredential,
		services:        make(map[string]*fixtureService, len(services)),
		servicesVersion: 1,
		pollHold:        20 * time.Millisecond,
		notifies:        make(chan channelNotify, 16),
		wake:            make(chan struct{}, 1),
		stop:            make(chan struct{}),
	}
	for _, service := range services {
		source.services[service.ID] = service
		source.order = append(source.order, service.ID)
	}
	return source
}

func (f *channelSource) close() {
	f.stopOnce.Do(func() { close(f.stop) })
}

func (f *channelSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	credential := f.credential
	f.mu.Unlock()
	if credential != "" && r.Header.Get("Authorization") != "Bearer "+credential {
		writeFixtureError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	switch r.URL.Path {
	case APIPrefix + "/channel":
		f.serveWebSocket(w, r)
	case APIPrefix + "/channel/poll":
		f.servePoll(w, r)
	default:
		writeFixtureError(w, http.StatusNotFound, "service_not_found", "")
	}
}

func (f *channelSource) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.upgradeAttempts++
	reject := f.rejectWebSocket
	f.mu.Unlock()
	if reject {
		writeFixtureError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}

	conn, err := channelUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	f.mu.Lock()
	f.connections++
	connectionNumber := f.connections
	silent := f.silentFirstConnection && connectionNumber == 1
	noAck := f.noAck
	keepalive := f.keepaliveWhileNoAck
	closeAfterAck := f.closeAfterAck
	ackInterval := f.ackInterval
	f.mu.Unlock()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	messageType, payload, err := conn.ReadMessage()
	if err != nil || messageType != websocket.TextMessage {
		return
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(payload, &envelope) != nil || envelope.Type != channelTypeHello {
		return
	}
	var hello channelHello
	if json.Unmarshal(payload, &hello) != nil {
		return
	}
	f.recordHello(hello)

	if noAck {
		if keepalive {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
						return
					}
				case <-f.stop:
					return
				}
			}
		}
		<-f.stop
		return
	}
	if err := f.writeJSON(conn, f.ack()); err != nil {
		return
	}
	if closeAfterAck {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second))
		return
	}
	if silent {
		<-f.stop
		return
	}
	if ackInterval > 0 {
		go func() {
			ticker := time.NewTicker(ackInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := f.writeJSON(conn, f.ack()); err != nil {
						return
					}
				case <-f.stop:
					return
				}
			}
		}()
	}

	conn.SetPingHandler(func(message string) error {
		f.mu.Lock()
		f.pings++
		f.mu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(time.Second))
	})

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil || messageType != websocket.TextMessage {
			return
		}
		if json.Unmarshal(payload, &envelope) != nil {
			return
		}
		switch envelope.Type {
		case channelTypeHeartbeat:
			var heartbeat channelHeartbeat
			if json.Unmarshal(payload, &heartbeat) != nil {
				return
			}
			f.recordHeartbeat(heartbeat)
			if err := f.flushNotifies(conn); err != nil {
				return
			}
			if err := f.writeJSON(conn, f.ack()); err != nil {
				return
			}
		case channelTypeCommandResult:
			var result channelCommandResult
			if json.Unmarshal(payload, &result) != nil {
				return
			}
			f.mu.Lock()
			f.commandResults = append(f.commandResults, result)
			f.mu.Unlock()
		default:
			return
		}
	}
}

func (f *channelSource) writeJSON(conn *websocket.Conn, value any) error {
	f.writeMu.Lock()
	defer f.writeMu.Unlock()
	return conn.WriteJSON(value)
}

func (f *channelSource) flushNotifies(conn *websocket.Conn) error {
	for {
		select {
		case notify := <-f.notifies:
			if err := f.writeJSON(conn, notify); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (f *channelSource) servePoll(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.pollRequests++
	hold := f.pollHold
	status := f.pollStatus
	if f.failPollFrom > 0 && f.pollRequests >= f.failPollFrom && status == 0 {
		status = http.StatusServiceUnavailable
	}
	retryAfter := f.pollRetryAfter
	override := append([]byte(nil), f.pollBody...)
	f.mu.Unlock()

	body, err := io.ReadAll(io.LimitReader(r.Body, maxChannelFrameBytes+1))
	if err != nil {
		writeFixtureError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	var heartbeat channelHeartbeat
	if err := json.Unmarshal(body, &heartbeat); err != nil || heartbeat.Type != channelTypeHeartbeat {
		writeFixtureError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	f.recordHeartbeat(heartbeat)

	select {
	case <-f.wake:
	case <-time.After(hold):
	case <-r.Context().Done():
		return
	case <-f.stop:
		writeFixtureError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	if status != 0 {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		if len(override) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(override)
			return
		}
		writeFixtureError(w, status, "source_unavailable", "")
		return
	}
	if len(override) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(override)
		return
	}
	writeFixtureJSON(w, http.StatusOK, f.ack())
}

func (f *channelSource) ack() channelAck {
	f.mu.Lock()
	defer f.mu.Unlock()
	ack := channelAck{
		Type:              channelTypeAck,
		ServicesVersion:   f.servicesVersion,
		ServerTime:        time.Now().UnixMilli(),
		HeartbeatInterval: "30s",
		StaleAfter:        "60s",
		StopAfter:         "120s",
		Commands:          append([]channelCommand(nil), f.commands...),
	}
	for _, id := range f.order {
		service := f.services[id]
		ack.Services = append(ack.Services, channelAckService{
			ID:            service.ID,
			Name:          service.Name,
			Head:          service.Revision,
			OriginHead:    service.Revision,
			Epoch:         service.Epoch,
			HeadStateHash: service.StateHash,
		})
	}
	return ack
}

func (f *channelSource) recordHello(hello channelHello) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hellos = append(f.hellos, hello)
}

func (f *channelSource) recordHeartbeat(heartbeat channelHeartbeat) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heartbeats = append(f.heartbeats, heartbeat)
}

func (f *channelSource) sendNotify(notify channelNotify) {
	f.notifies <- notify
}

func (f *channelSource) signal() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

func (f *channelSource) stats() (upgradeAttempts, connections, pollRequests, pings int, hellos []channelHello, heartbeats []channelHeartbeat) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.upgradeAttempts, f.connections, f.pollRequests, f.pings,
		append([]channelHello(nil), f.hellos...),
		append([]channelHeartbeat(nil), f.heartbeats...)
}

func (f *channelSource) configure(update func(*channelSource)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	update(f)
}

func (f *channelSource) serviceState(serviceID string) *ServiceState {
	f.mu.Lock()
	defer f.mu.Unlock()
	service := f.services[serviceID]
	return &ServiceState{
		ServiceID:  service.ID,
		SourceID:   service.ID,
		Epoch:      service.Epoch,
		Revision:   service.Revision,
		StateHash:  service.StateHash,
		CommitHash: service.CommitHash,
		Snapshot:   cloneSnapshot(service.State),
	}
}

func (f *channelSource) setHead(serviceID string, revision uint64, stateHash Hash) {
	f.mu.Lock()
	defer f.mu.Unlock()
	service := f.services[serviceID]
	service.Revision = revision
	service.StateHash = stateHash
}

func (f *channelSource) addService(service *fixtureService) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.services[service.ID] = service
	f.order = append(f.order, service.ID)
	f.servicesVersion++
}

func (f *channelSource) renameService(serviceID, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if service := f.services[serviceID]; service != nil && service.Name != name {
		service.Name = name
		f.servicesVersion++
	}
}

func (f *channelSource) removeService(serviceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.services, serviceID)
	kept := f.order[:0]
	for _, id := range f.order {
		if id != serviceID {
			kept = append(kept, id)
		}
	}
	f.order = kept
	f.servicesVersion++
}

func (f *channelSource) setCommands(commands ...channelCommand) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = commands
}

func (f *channelSource) results() []channelCommandResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]channelCommandResult(nil), f.commandResults...)
}

// startChannelFixture starts an HTTPS server for the channel under test. The
// cleanup closes the fixture before the server so blocked handlers exit.
func startChannelFixture(t *testing.T, source *channelSource) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(source)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(func() {
		source.close()
		server.Close()
	})
	return server
}

func newChannelClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	vectors := loadGolden(t)
	client, err := NewClient(Config{
		BaseURL:        server.URL,
		Credential:     testCredential,
		PublicKeys:     goldenPublicKeys(t, vectors),
		CAFile:         writeServerCA(t, server),
		RequestTimeout: 2 * time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func writeServerCA(t *testing.T, server *httptest.Server) string {
	t.Helper()
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("test server has no certificate")
	}
	return writeTestFile(t, t.TempDir(), "ca.pem",
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o644)
}

func newChannelMachine(t *testing.T) *Machine {
	t.Helper()
	machine, err := NewMachine(MachineOptions{})
	if err != nil {
		t.Fatalf("NewMachine: %v", err)
	}
	return machine
}

// newTestChannel builds a Channel with short timings. Tests that need a
// different value overwrite the unexported fields afterwards.
func newTestChannel(t *testing.T, client *Client, machine *Machine, options ChannelOptions) *Channel {
	t.Helper()
	options.Machine = machine
	if options.InstanceID == "" {
		options.InstanceID = testInstanceID
	}
	if options.AgentVersion == "" {
		options.AgentVersion = "test-1.0"
	}
	if options.Chain == nil {
		options.Chain = []string{testInstanceID}
	}
	channel, err := NewChannel(client, options)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	channel.heartbeatOverride = 25 * time.Millisecond
	channel.ackTimeout = 300 * time.Millisecond
	channel.probeInterval = 200 * time.Millisecond
	channel.pollWait = 0
	channel.backoff = Backoff{
		Initial: 5 * time.Millisecond,
		Max:     20 * time.Millisecond,
		Jitter:  0.2,
		Rand:    func() float64 { return 0.5 },
	}
	return channel
}

func runChannel(t *testing.T, channel *Channel) (cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	ctx, cancelCtx := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- channel.Run(ctx) }()
	t.Cleanup(func() {
		cancelCtx()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("channel did not stop")
		}
	})
	return cancelCtx, finished
}

func waitFor(t *testing.T, timeout time.Duration, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func requireInSync(t *testing.T, machine *Machine, name string) {
	t.Helper()
	waitFor(t, 3*time.Second, "service "+name+" to be in sync", func() bool {
		status, known := machine.Status(name)
		return known && status.Service.State == SyncStateInSync
	})
}

func TestChannelWebSocketMultiService(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	orders := newFixtureService(testServiceID2, "orders")
	reports := newFixtureService(testServiceID3, "reports")
	source := newChannelSource(t, books, orders, reports)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	for _, service := range []*fixtureService{books, orders, reports} {
		if err := machine.ApplyAttempt(machine.BeginSync(service.ID), source.serviceState(service.ID)); err != nil {
			t.Fatalf("Applied: %v", err)
		}
	}

	changes := make(chan ServiceSetChange, 16)
	notifications := make(chan Notify, 16)
	channel := newTestChannel(t, client, machine, ChannelOptions{
		Transport: TransportWebSocket,
		OnServiceSetChange: func(change ServiceSetChange) {
			select {
			case changes <- change:
			default:
			}
		},
		OnNotify: func(notify Notify) {
			select {
			case notifications <- notify:
			default:
			}
		},
	})
	// ApplyAttempt already marks these services in sync, so requireInSync does
	// not wait for the channel. Services() stays empty until the first ack.
	// The short test ack deadline can also expire before that ack on a busy
	// machine and open a second session; this test expects exactly one.
	channel.ackTimeout = 2 * time.Second
	runChannel(t, channel)

	waitFor(t, 3*time.Second, "the channel to ack every service", func() bool {
		return len(channel.Services()) == 3 && channel.CurrentTransport() == TransportWebSocket
	})
	for _, service := range []string{"books", "orders", "reports"} {
		requireInSync(t, machine, service)
	}
	if got := channel.CurrentTransport(); got != TransportWebSocket {
		t.Fatalf("CurrentTransport = %q, want websocket", got)
	}
	if services := channel.Services(); len(services) != 3 {
		t.Fatalf("channel services = %d, want 3", len(services))
	}

	upgradeAttempts, connections, _, _, hellos, _ := source.stats()
	if upgradeAttempts != 1 || connections != 1 {
		t.Fatalf("upgradeAttempts = %d, connections = %d, want 1 and 1", upgradeAttempts, connections)
	}
	if len(hellos) != 1 {
		t.Fatalf("got %d hellos, want 1", len(hellos))
	}
	hello := hellos[0]
	if hello.InstanceID != testInstanceID {
		t.Errorf("hello instanceId = %q", hello.InstanceID)
	}
	if hello.AgentVersion != "test-1.0" || hello.Transport != string(TransportWebSocket) {
		t.Errorf("hello = %+v", hello)
	}
	if len(hello.Services) != 3 {
		t.Fatalf("hello carries %d services, want 3", len(hello.Services))
	}
	if got := hello.Chain; len(got) != 1 || got[0] != testInstanceID {
		t.Errorf("hello chain = %v", got)
	}
	waitFor(t, 2*time.Second, "a heartbeat carrying every service", func() bool {
		_, _, _, _, _, heartbeats := source.stats()
		for _, heartbeat := range heartbeats {
			if len(heartbeat.Services) == 3 && heartbeat.Transport == string(TransportWebSocket) {
				return true
			}
		}
		return false
	})
	waitFor(t, 2*time.Second, "protocol pings", func() bool {
		_, _, _, pings, _, _ := source.stats()
		return pings > 0
	})

	select {
	case change := <-changes:
		if len(change.Added) != 3 || len(change.Removed) != 0 {
			t.Fatalf("first change = %+v, want three additions", change)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no service-set change reported")
	}

	// A head notification advances the machine without waiting for an ack.
	newHash := StateHash(Snapshot{
		Name:         "books",
		Policies:     []Policy{{ID: "p2", Name: "p2", SPDL: "grant read"}},
		RolePolicies: []Policy{},
	})
	source.setHead(testServiceID1, 2, newHash)
	source.sendNotify(channelNotify{Type: channelTypeNotify, ServiceID: testServiceID1, Head: 2})

	waitFor(t, 3*time.Second, "the notified head", func() bool {
		status, known := machine.Status("books")
		return known && status.Service.Head == 2
	})
	select {
	case notify := <-notifications:
		if notify.ServiceID != testServiceID1 || notify.Head != 2 {
			t.Fatalf("notification = %+v", notify)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the notification callback did not run")
	}
}

func TestChannelAckHeadAdvanceNotifies(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	notifications := make(chan Notify, 64)
	channel := newTestChannel(t, client, machine, ChannelOptions{
		Transport: TransportWebSocket,
		OnNotify: func(notify Notify) {
			select {
			case notifications <- notify:
			default:
			}
		},
	})
	runChannel(t, channel)
	requireInSync(t, machine, "books")

	// Only an ack reports the new head here; no notification frame is sent.
	newHash := StateHash(Snapshot{
		Name:         "books",
		Policies:     []Policy{{ID: "p3", Name: "p3", SPDL: "grant read"}},
		RolePolicies: []Policy{},
	})
	source.setHead(books.ID, 2, newHash)

	waitFor(t, 3*time.Second, "an ack-driven head notification", func() bool {
		for {
			select {
			case notify := <-notifications:
				if notify.ServiceID == books.ID && notify.Head == 2 {
					return true
				}
			default:
				return false
			}
		}
	})
}

func TestChannelLongPollMultiService(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	orders := newFixtureService(testServiceID2, "orders")
	source := newChannelSource(t, books, orders)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	for _, service := range []*fixtureService{books, orders} {
		if err := machine.ApplyAttempt(machine.BeginSync(service.ID), source.serviceState(service.ID)); err != nil {
			t.Fatalf("Applied: %v", err)
		}
	}
	changes := make(chan ServiceSetChange, 16)
	channel := newTestChannel(t, client, machine, ChannelOptions{
		Transport: TransportLongPoll,
		OnServiceSetChange: func(change ServiceSetChange) {
			select {
			case changes <- change:
			default:
			}
		},
	})
	runChannel(t, channel)

	requireInSync(t, machine, "books")
	requireInSync(t, machine, "orders")
	if got := channel.CurrentTransport(); got != TransportLongPoll {
		t.Fatalf("CurrentTransport = %q, want longpoll", got)
	}
	waitFor(t, 2*time.Second, "long-poll heartbeats", func() bool {
		_, _, polls, _, _, heartbeats := source.stats()
		return polls > 0 && len(heartbeats) > 0
	})
	_, _, _, _, hellos, heartbeats := source.stats()
	if len(hellos) != 0 {
		t.Fatalf("long polling sent %d hello frames, want 0", len(hellos))
	}
	if len(heartbeats[0].Services) != 2 {
		t.Fatalf("poll body carries %d services, want 2", len(heartbeats[0].Services))
	}
	if heartbeats[0].Transport != string(TransportLongPoll) {
		t.Fatalf("poll body transport = %q", heartbeats[0].Transport)
	}
	select {
	case change := <-changes:
		if len(change.Added) != 2 {
			t.Fatalf("service-set change = %+v", change)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no service-set change reported")
	}
}

func TestChannelAutoFallsBackOnUpgradeFailure(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	source.configure(func(f *channelSource) { f.rejectWebSocket = true })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportAuto})
	runChannel(t, channel)

	requireInSync(t, machine, "books")
	if got := channel.CurrentTransport(); got != TransportLongPoll {
		t.Fatalf("CurrentTransport = %q, want longpoll after a failed upgrade", got)
	}
	waitFor(t, 2*time.Second, "a WebSocket attempt and a poll", func() bool {
		attempts, _, polls, _, _, _ := source.stats()
		return attempts > 0 && polls > 0
	})
}

func TestChannelAutoFallsBackWhenNoAck(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	source.configure(func(f *channelSource) { f.noAck = true })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportAuto})
	channel.ackTimeout = 100 * time.Millisecond
	runChannel(t, channel)

	requireInSync(t, machine, "books")
	if got := channel.CurrentTransport(); got != TransportLongPoll {
		t.Fatalf("CurrentTransport = %q, want longpoll when the upgrade never acks", got)
	}
}

func TestChannelAutoFallsBackWhenNoAckWithKeepalive(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	source.configure(func(f *channelSource) {
		f.noAck = true
		f.keepaliveWhileNoAck = true
	})
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportAuto})
	channel.ackTimeout = 100 * time.Millisecond
	runChannel(t, channel)

	requireInSync(t, machine, "books")
	if got := channel.CurrentTransport(); got != TransportLongPoll {
		t.Fatalf("CurrentTransport = %q, want longpoll: keepalive traffic must not hide a missing ack", got)
	}
	if _, connections, polls, _, _, _ := source.stats(); connections == 0 || polls == 0 {
		t.Fatalf("connections = %d, polls = %d, want both transports", connections, polls)
	}
}

func TestChannelAutoFallsBackAfterRepeatedDrops(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	source.configure(func(f *channelSource) { f.closeAfterAck = true })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportAuto})
	runChannel(t, channel)

	waitFor(t, 3*time.Second, "two dropped WebSocket connections", func() bool {
		_, connections, _, _, _, _ := source.stats()
		return connections >= 2
	})
	waitFor(t, 3*time.Second, "the long-poll fallback", func() bool {
		return channel.CurrentTransport() == TransportLongPoll
	})
	waitFor(t, 3*time.Second, "an ack through long polling", func() bool {
		_, known := machine.Status("books")
		return known
	})
}

func TestChannelHalfOpenConnectionReconnects(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	source.configure(func(f *channelSource) { f.silentFirstConnection = true })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	runChannel(t, channel)

	waitFor(t, 3*time.Second, "a second connection after the half-open one", func() bool {
		_, connections, _, _, _, _ := source.stats()
		return connections >= 2
	})
	waitFor(t, 3*time.Second, "heartbeats on the new connection", func() bool {
		_, _, _, pings, _, heartbeats := source.stats()
		return pings > 0 && len(heartbeats) > 0
	})
	if _, known := machine.Status("books"); !known {
		t.Fatal("the reconnected channel did not deliver an ack")
	}
}

func TestChannelRunStopsWhenCanceledWithLivePeer(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	// The peer keeps sending acks, so cancellation must close the connection
	// rather than wait for a read deadline that traffic keeps extending.
	source.configure(func(f *channelSource) { f.ackInterval = 10 * time.Millisecond })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- channel.Run(ctx) }()
	requireInSync(t, machine, "books")

	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancellation with a live peer")
	}
}

func TestChannelRunStopsWhenCanceledWithSilentPeer(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	// The peer acks once and then stays silent; without closing the connection
	// on cancellation the reader waits for the long read deadline.
	source.configure(func(f *channelSource) { f.silentFirstConnection = true })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	channel.heartbeatOverride = 10 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- channel.Run(ctx) }()
	requireInSync(t, machine, "books")

	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancellation with a silent peer")
	}
}

func TestChannelLongPollThrottlesImmediateResponses(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	// The source answers wait-length polls immediately, as a nonconforming
	// endpoint would.
	source.configure(func(f *channelSource) { f.pollHold = 0 })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportLongPoll})
	channel.pollWait = maxPollWait
	runChannel(t, channel)
	requireInSync(t, machine, "books")

	time.Sleep(300 * time.Millisecond)
	if _, _, polls, _, _, _ := source.stats(); polls > 2 {
		t.Fatalf("polls = %d in 300ms, want at most 2: immediate responses must be throttled", polls)
	}
}

func TestChannelAutoReprobesWebSocket(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	source.configure(func(f *channelSource) { f.rejectWebSocket = true })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportAuto})
	channel.probeInterval = 100 * time.Millisecond
	runChannel(t, channel)

	waitFor(t, 2*time.Second, "a first upgrade attempt", func() bool {
		attempts, _, _, _, _, _ := source.stats()
		return attempts > 0
	})
	source.configure(func(f *channelSource) { f.rejectWebSocket = false })
	waitFor(t, 3*time.Second, "the WebSocket probe to succeed", func() bool {
		_, connections, _, _, _, _ := source.stats()
		return connections > 0 && channel.CurrentTransport() == TransportWebSocket
	})
}

func TestChannelServiceSetChange(t *testing.T) {
	for _, transport := range []Transport{TransportWebSocket, TransportLongPoll} {
		t.Run(string(transport), func(t *testing.T) {
			books := newFixtureService(testServiceID1, "books")
			orders := newFixtureService(testServiceID2, "orders")
			source := newChannelSource(t, books, orders)
			server := startChannelFixture(t, source)
			client := newChannelClient(t, server)

			machine := newChannelMachine(t)
			if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
				t.Fatalf("Applied: %v", err)
			}
			changes := make(chan ServiceSetChange, 16)
			channel := newTestChannel(t, client, machine, ChannelOptions{
				Transport: transport,
				OnServiceSetChange: func(change ServiceSetChange) {
					select {
					case changes <- change:
					default:
					}
				},
			})
			runChannel(t, channel)
			requireInSync(t, machine, "books")

			reports := newFixtureService(testServiceID3, "reports")
			source.removeService(orders.ID)
			source.addService(reports)
			source.signal()

			waitFor(t, 3*time.Second, "the service-set change in the machine", func() bool {
				if status, known := machine.Status("orders"); !known || status.Service.State != SyncStateRemoved {
					return false
				}
				status, known := machine.Status("reports")
				return known && status.Service.State == SyncStateSyncing
			})
			waitFor(t, 3*time.Second, "the change callback", func() bool {
				for {
					select {
					case change := <-changes:
						if len(change.Added) == 1 && change.Added[0].ID == reports.ID &&
							len(change.Removed) == 1 && change.Removed[0].ID == orders.ID {
							return true
						}
					default:
						return false
					}
				}
			})
			services := channel.Services()
			if len(services) != 2 {
				t.Fatalf("channel services = %+v, want books and reports", services)
			}

			// A rename is reported separately: the id stays, the name changes.
			source.renameService(books.ID, "books-v2")
			source.signal()
			waitFor(t, 3*time.Second, "the renamed service in the machine", func() bool {
				status, known := machine.Status("books-v2")
				return known && status.Service.ID == books.ID
			})
			waitFor(t, 3*time.Second, "the rename callback", func() bool {
				for {
					select {
					case change := <-changes:
						if len(change.Renamed) == 1 && change.Renamed[0].ID == books.ID &&
							change.Renamed[0].Name == "books-v2" {
							return true
						}
					default:
						return false
					}
				}
			})
		})
	}
}

func TestChannelFirstAckRemovesPersistedService(t *testing.T) {
	for _, transport := range []Transport{TransportWebSocket, TransportLongPoll} {
		t.Run(string(transport), func(t *testing.T) {
			books := newFixtureService(testServiceID1, "books")
			orders := newFixtureService(testServiceID2, "orders")
			source := newChannelSource(t, books, orders)
			// The process restarts with orders cached locally, but the source
			// has dropped it since the cache was written.
			persisted := source.serviceState(orders.ID)
			source.removeService(orders.ID)
			server := startChannelFixture(t, source)
			client := newChannelClient(t, server)

			machine := newChannelMachine(t)
			if err := machine.ApplyAttempt(machine.BeginSync(persisted.ServiceID), persisted); err != nil {
				t.Fatalf("Applied: %v", err)
			}
			changes := make(chan ServiceSetChange, 16)
			channel := newTestChannel(t, client, machine, ChannelOptions{
				Transport: transport,
				OnServiceSetChange: func(change ServiceSetChange) {
					select {
					case changes <- change:
					default:
					}
				},
			})
			runChannel(t, channel)

			waitFor(t, 3*time.Second, "the persisted service to be removed", func() bool {
				status, known := machine.Status("orders")
				return known && status.Service.State == SyncStateRemoved
			})
			select {
			case change := <-changes:
				if len(change.Removed) != 1 || change.Removed[0].ID != orders.ID {
					t.Fatalf("first change = %+v, want orders removed", change)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("the removal of a persisted service was not reported")
			}
		})
	}
}

func TestChannelWebSocketOnlyStopsOnUpgradeConfigError(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	// The upgrade is rejected with 400, which retrying cannot fix.
	source.configure(func(f *channelSource) { f.rejectWebSocket = true })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := channel.Run(ctx)
	if err == nil {
		t.Fatal("Run returned nil, want a terminal error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run retried a permanent upgrade failure: %v", err)
	}
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("Run error = %v, want a protocol error", err)
	}
	if status := machine.Instance(); status.Liveness != LivenessStopped || status.Reason != ReasonConfigError {
		t.Fatalf("machine = %+v, want stopped (config_error)", status)
	}
	if upgradeAttempts, _, _, _, _, _ := source.stats(); upgradeAttempts != 1 {
		t.Fatalf("upgradeAttempts = %d, want 1", upgradeAttempts)
	}
}

func TestChannelCommands(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	resync := channelCommand{
		ID:        "01HF7YAT000000000000000301",
		Action:    commandActionResync,
		ServiceID: testServiceID1,
		ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
	}
	expired := channelCommand{
		ID:        "01HF7YAT000000000000000302",
		Action:    commandActionResume,
		ExpiresAt: time.Now().Add(-time.Minute).UnixMilli(),
	}
	setSource := channelCommand{
		ID:     "01HF7YAT000000000000000303",
		Action: commandActionSetSource,
		Params: &channelCommandParams{
			URL:               "https://proxy.example:443",
			CredentialProfile: "upstream",
		},
		ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
	}
	source.setCommands(resync, expired, setSource)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	delivered := make(chan Command, 1024)
	channel := newTestChannel(t, client, machine, ChannelOptions{
		Transport: TransportWebSocket,
		OnCommand: func(command Command) *CommandResult {
			select {
			case delivered <- command:
			default:
			}
			return &CommandResult{Status: commandStatusAcked, Result: map[string]string{"ok": "true"}}
		},
	})
	runChannel(t, channel)

	seen := map[string]Command{}
	waitFor(t, 3*time.Second, "the commands", func() bool {
		for {
			select {
			case command := <-delivered:
				seen[command.ID] = command
			default:
				_, foundResync := seen[resync.ID]
				_, foundSetSource := seen[setSource.ID]
				return foundResync && foundSetSource
			}
		}
	})
	if _, found := seen[expired.ID]; found {
		t.Fatal("an expired command was delivered")
	}
	if command := seen[resync.ID]; command.ServiceID != testServiceID1 || command.Action != commandActionResync {
		t.Fatalf("resync command = %+v", command)
	}
	if command := seen[setSource.ID]; command.SourceURL != "https://proxy.example:443" ||
		command.CredentialProfile != "upstream" {
		t.Fatalf("set_source command = %+v", command)
	}
	waitFor(t, 3*time.Second, "the command results", func() bool {
		results := source.results()
		if len(results) < 2 {
			return false
		}
		for _, result := range results {
			if result.ID != resync.ID && result.ID != setSource.ID {
				return false
			}
			if result.Status != commandStatusAcked || result.Type != channelTypeCommandResult {
				return false
			}
		}
		return true
	})
}

func TestChannelUnauthorizedStopsInstance(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	// A well-formed credential the fixture does not accept.
	client.credential = "adsi_" + testInstanceID + "_" + strings.Repeat("b", 43)

	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportLongPoll})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := channel.Run(ctx)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Run = %v, want ErrUnauthorized", err)
	}
	instance := machine.Instance()
	if instance.Liveness != LivenessStopped || instance.Reason != ReasonRevoked {
		t.Fatalf("instance = %+v, want stopped/revoked", instance)
	}
}

func TestBackoffDelays(t *testing.T) {
	backoff, err := Backoff{Initial: time.Second, Max: 60 * time.Second}.normalized()
	if err != nil {
		t.Fatalf("normalized: %v", err)
	}
	backoff.Rand = func() float64 { return 0.5 } // no jitter offset
	want := []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		60 * time.Second,
		60 * time.Second,
	}
	for index, expected := range want {
		if got := backoff.delay(index + 1); got != expected {
			t.Errorf("delay(%d) = %s, want %s", index+1, got, expected)
		}
	}

	// The jitter stays within the configured fraction.
	backoff.Rand = func() float64 { return 0 }
	if got := backoff.delay(1); got != time.Second*7/10 {
		t.Errorf("low jitter delay = %s, want 700ms", got)
	}
	backoff.Rand = func() float64 { return 0.999999 }
	if got := backoff.delay(1); got < time.Second*1299/1000 || got > time.Second*13/10 {
		t.Errorf("high jitter delay = %s, want about 1.3s", got)
	}
}

func TestNewChannelValidation(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	machine := newChannelMachine(t)
	options := func() ChannelOptions {
		return ChannelOptions{
			InstanceID:   testInstanceID,
			AgentVersion: "test-1.0",
			Machine:      machine,
		}
	}

	if _, err := NewChannel(nil, options()); err == nil {
		t.Error("NewChannel without a client succeeded")
	}
	withoutMachine := options()
	withoutMachine.Machine = nil
	if _, err := NewChannel(client, withoutMachine); err == nil {
		t.Error("NewChannel without a machine succeeded")
	}

	longChain := make([]string, maxChainEntries+1)
	for index := range longChain {
		longChain[index] = testInstanceID
	}
	cases := []struct {
		name    string
		options ChannelOptions
	}{
		{"invalid instance id", func() ChannelOptions {
			value := options()
			value.InstanceID = "not-a-ulid"
			return value
		}()},
		{"invalid credential id", func() ChannelOptions {
			value := options()
			value.CredentialID = "not-a-ulid"
			return value
		}()},
		{"empty agent version", func() ChannelOptions {
			value := options()
			value.AgentVersion = ""
			return value
		}()},
		{"long agent version", func() ChannelOptions {
			value := options()
			value.AgentVersion = strings.Repeat("a", 65)
			return value
		}()},
		{"chain too long", func() ChannelOptions {
			value := options()
			value.Chain = longChain
			return value
		}()},
		{"invalid chain entry", func() ChannelOptions {
			value := options()
			value.Chain = []string{"not-a-ulid"}
			return value
		}()},
		{"invalid upstream instance id", func() ChannelOptions {
			value := options()
			value.UpstreamInstanceID = "not-a-ulid"
			return value
		}()},
		{"unknown transport", func() ChannelOptions {
			value := options()
			value.Transport = Transport("carrier-pigeon")
			return value
		}()},
		{"negative heartbeat", func() ChannelOptions {
			value := options()
			value.HeartbeatInterval = -time.Second
			return value
		}()},
		{"negative poll wait", func() ChannelOptions {
			value := options()
			value.PollWait = -time.Second
			return value
		}()},
		{"long poll wait", func() ChannelOptions {
			value := options()
			value.PollWait = maxPollWait + time.Second
			return value
		}()},
		{"negative ack timeout", func() ChannelOptions {
			value := options()
			value.AckTimeout = -time.Second
			return value
		}()},
		{"negative probe interval", func() ChannelOptions {
			value := options()
			value.ProbeInterval = -time.Second
			return value
		}()},
		{"negative backoff initial", func() ChannelOptions {
			value := options()
			value.Backoff = Backoff{Initial: -time.Second}
			return value
		}()},
		{"backoff max below initial", func() ChannelOptions {
			value := options()
			value.Backoff = Backoff{Initial: 2 * time.Second, Max: time.Second}
			return value
		}()},
		{"backoff jitter out of range", func() ChannelOptions {
			value := options()
			value.Backoff = Backoff{Jitter: 1.5}
			return value
		}()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewChannel(client, testCase.options); err == nil {
				t.Fatal("NewChannel succeeded, want an error")
			}
		})
	}
	if _, err := NewChannel(client, options()); err != nil {
		t.Fatalf("NewChannel with valid options: %v", err)
	}
}

func TestChannelRunGuards(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{})

	if err := channel.Run(nil); err == nil {
		t.Error("Run with a nil context succeeded")
	}
	channel.running.Store(true)
	if err := channel.Run(context.Background()); err == nil {
		t.Error("Run while running succeeded")
	}
}

func TestChannelHeartbeatReportsTopology(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	orders := newFixtureService(testServiceID2, "orders")
	source := newChannelSource(t, books, orders)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	both := []Service{
		{ID: books.ID, Name: books.Name, Revision: books.Revision, Epoch: books.Epoch, HeadStateHash: books.StateHash},
		{ID: orders.ID, Name: orders.Name, Revision: orders.Revision, Epoch: orders.Epoch, HeadStateHash: orders.StateHash},
	}
	if err := machine.Ack(Ack{Services: both}); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	// The second ack removes orders, so the heartbeat must skip its tombstone.
	if err := machine.Ack(Ack{Services: both[:1]}); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	downstreams := uint64(4)
	channel, err := NewChannel(client, ChannelOptions{
		InstanceID:         testInstanceID,
		AgentVersion:       "test-1.0",
		Machine:            machine,
		Chain:              []string{testInstanceID},
		UpstreamInstanceID: testInstanceID,
		DownstreamCount:    &downstreams,
		Fallback: func() FallbackState {
			return FallbackState{
				Active:      true,
				Services:    []string{testServiceID1, "not a service", testServiceID2},
				Since:       time.Now(),
				RoutedCalls: 7,
			}
		},
	})
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}

	message := channel.heartbeatMessage(TransportLongPoll)
	if len(message.Services) != 1 || message.Services[0].ID != books.ID {
		t.Fatalf("heartbeat services = %+v, want only books", message.Services)
	}
	if !message.Fallback.Active || message.Fallback.RoutedCalls != 7 {
		t.Fatalf("fallback = %+v", message.Fallback)
	}
	if message.Fallback.Since == nil {
		t.Fatal("fallback since is missing")
	}
	if len(message.Fallback.Services) != 2 || message.Fallback.Services[0] != testServiceID1 ||
		message.Fallback.Services[1] != testServiceID2 {
		t.Fatalf("fallback services = %v", message.Fallback.Services)
	}
	if message.UpstreamInstanceID == nil || *message.UpstreamInstanceID != testInstanceID {
		t.Fatalf("upstream instance id = %v", message.UpstreamInstanceID)
	}
	if message.DownstreamCount == nil || *message.DownstreamCount != downstreams {
		t.Fatalf("downstream count = %v", message.DownstreamCount)
	}
	if message.Seq != 1 || message.Liveness != string(LivenessLive) {
		t.Fatalf("heartbeat = %+v", message)
	}
	if next := channel.heartbeatMessage(TransportLongPoll); next.Seq != 2 {
		t.Fatalf("second heartbeat seq = %d, want 2", next.Seq)
	}
}

func TestChannelPollTerminalStatus(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	source.configure(func(f *channelSource) { f.pollStatus = http.StatusBadRequest })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportLongPoll})

	err := channel.pollOnce(context.Background())
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("pollOnce = %v, want ErrProtocol", err)
	}
	if !isTerminalError(err) {
		t.Fatalf("pollOnce error is not terminal: %v", err)
	}
	instance := machine.Instance()
	if instance.Liveness != LivenessStopped || instance.Reason != ReasonConfigError {
		t.Fatalf("instance = %+v, want stopped/config_error", instance)
	}
}

func TestChannelPollTerminalStatusWithLargeBody(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			source := newChannelSource(t, books)
			source.configure(func(f *channelSource) {
				f.pollStatus = status
				f.pollBody = []byte(`{"error":"denied","pad":"` + strings.Repeat("x", maxChannelFrameBytes) + `"}`)
			})
			server := startChannelFixture(t, source)
			client := newChannelClient(t, server)
			machine := newChannelMachine(t)
			channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportLongPoll})

			err := channel.pollOnce(context.Background())
			want := ErrUnauthorized
			if status == http.StatusForbidden {
				want = ErrForbidden
			}
			if !errors.Is(err, want) {
				t.Fatalf("pollOnce = %v, want %v", err, want)
			}
			if !isTerminalError(err) {
				t.Fatalf("pollOnce error is not terminal: %v", err)
			}
			instance := machine.Instance()
			if instance.Liveness != LivenessStopped || instance.Reason != ReasonRevoked {
				t.Fatalf("instance = %+v, want stopped/revoked", instance)
			}
		})
	}
}

func TestChannelPollRetryableStatus(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	source.configure(func(f *channelSource) {
		f.pollStatus = http.StatusServiceUnavailable
		f.pollRetryAfter = "2"
	})
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportLongPoll})

	err := channel.pollOnce(context.Background())
	if err == nil {
		t.Fatal("pollOnce succeeded, want a status error")
	}
	if isTerminalError(err) {
		t.Fatalf("a retryable status is terminal: %v", err)
	}
	if got := retryAfterOf(err); got != 2*time.Second {
		t.Fatalf("Retry-After = %s, want 2s", got)
	}
	if instance := machine.Instance(); instance.Liveness == LivenessStopped {
		t.Fatalf("a retryable status stopped the instance: %+v", instance)
	}
}

func TestChannelPollRejectsBadResponses(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportLongPoll})

	source.configure(func(f *channelSource) {
		f.pollBody = []byte(`{"type":"notify","serviceId":"` + testServiceID1 + `","head":1}`)
	})
	if err := channel.pollOnce(context.Background()); !errors.Is(err, ErrProtocol) {
		t.Fatalf("non-ack response = %v, want ErrProtocol", err)
	}

	source.configure(func(f *channelSource) {
		f.pollBody = []byte(`{"type":"ack","pad":"` + strings.Repeat("x", maxChannelFrameBytes) + `"}`)
	})
	if err := channel.pollOnce(context.Background()); !errors.Is(err, ErrProtocol) {
		t.Fatalf("oversized response = %v, want ErrProtocol", err)
	}
}

func TestChannelSleepBeforeRetryHonoursRetryAfter(t *testing.T) {
	channel := &Channel{backoff: Backoff{Initial: time.Millisecond, Max: 100 * time.Millisecond}}
	cause := &statusError{retryAfter: 25 * time.Millisecond}
	start := time.Now()
	if err := channel.sleepBeforeRetry(context.Background(), 1, cause); err != nil {
		t.Fatalf("sleepBeforeRetry: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("slept %s, want at least the 25ms Retry-After", elapsed)
	}

	// A Retry-After beyond the maximum delay is capped.
	cause.retryAfter = time.Minute
	start = time.Now()
	if err := channel.sleepBeforeRetry(context.Background(), 1, cause); err != nil {
		t.Fatalf("sleepBeforeRetry: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond || elapsed < 80*time.Millisecond {
		t.Fatalf("slept %s, want the 100ms maximum delay", elapsed)
	}
}

func TestChannelLongPollProbeDeadline(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportLongPoll})

	if err := channel.runLongPollLoop(context.Background(), time.Now()); err != nil {
		t.Fatalf("runLongPollLoop with a due probe = %v, want nil", err)
	}
}

func TestChannelHeartbeatCadence(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportLongPoll})
	channel.heartbeatOverride = 0

	if got := channel.heartbeatInterval(); got != defaultHeartbeatInterval {
		t.Fatalf("heartbeatInterval = %s, want %s", got, defaultHeartbeatInterval)
	}
	if got := channel.pingInterval(); got != defaultPingInterval {
		t.Fatalf("pingInterval = %s, want %s", got, defaultPingInterval)
	}
	channel.heartbeatOverride = 5 * time.Second
	if got := channel.pingInterval(); got != 5*time.Second {
		t.Fatalf("pingInterval with a short heartbeat = %s, want 5s", got)
	}
}

func TestChannelAppliesShortenedHeartbeatAfterAck(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	// The source pushes acks, so a cadence change can be announced without
	// waiting for a heartbeat.
	source.configure(func(f *channelSource) { f.ackInterval = 500 * time.Millisecond })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	// Start close to the 30s default. The first ack that shortens the cadence
	// must reset the heartbeat timer instead of waiting for it to expire.
	channel.heartbeatOverride = 10 * time.Second
	runChannel(t, channel)
	requireInSync(t, machine, "books")

	channel.mu.Lock()
	channel.heartbeatOverride = 200 * time.Millisecond
	channel.mu.Unlock()
	waitFor(t, 3*time.Second, "a heartbeat at the shortened cadence", func() bool {
		_, _, _, _, _, heartbeats := source.stats()
		return len(heartbeats) > 0
	})
	if _, connections, _, _, _, _ := source.stats(); connections != 1 {
		t.Fatalf("connections = %d, want 1: the first connection must adopt the new cadence", connections)
	}
}

func TestChannelUnsolicitedAcksDoNotSuppressHeartbeats(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	// Acks arrive faster than the heartbeat interval and leave the cadence
	// unchanged, so they must not keep postponing the heartbeat.
	source.configure(func(f *channelSource) { f.ackInterval = 40 * time.Millisecond })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	channel.heartbeatOverride = 200 * time.Millisecond
	runChannel(t, channel)
	requireInSync(t, machine, "books")

	waitFor(t, 3*time.Second, "heartbeats at the configured cadence", func() bool {
		_, _, _, _, _, heartbeats := source.stats()
		return len(heartbeats) >= 2
	})
}

func TestChannelPollCanceledContext(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportLongPoll})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := channel.pollOnce(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("pollOnce with a canceled context = %v, want context.Canceled", err)
	}
}

func TestChannelApplyAckRejectsDuplicateNames(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportLongPoll})

	ack := &channelAck{
		Type:              channelTypeAck,
		ServicesVersion:   1,
		ServerTime:        time.Now().UnixMilli(),
		HeartbeatInterval: "30s",
		StaleAfter:        "60s",
		StopAfter:         "120s",
		Services: []channelAckService{
			{ID: testServiceID1, Name: "shared", Head: 1, OriginHead: 1, Epoch: testServiceID1, HeadStateHash: books.StateHash},
			{ID: testServiceID2, Name: "shared", Head: 1, OriginHead: 1, Epoch: testServiceID2, HeadStateHash: books.StateHash},
		},
	}
	if err := ack.validate(); err != nil {
		t.Fatalf("the wire ack should be valid: %v", err)
	}
	if err := channel.applyAck(ack, nil, nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("applyAck = %v, want ErrProtocol", err)
	}
}

func TestChannelDeliverCommands(t *testing.T) {
	command := func(id string, expires time.Time) channelCommand {
		return channelCommand{ID: id, Action: commandActionResync, ServiceID: testServiceID1, ExpiresAt: expires.UnixMilli()}
	}
	now := time.Now()
	channel := &Channel{}
	var delivered []Command
	channel.onCommand = func(command Command) *CommandResult {
		delivered = append(delivered, command)
		return nil
	}
	results := make(chan channelCommandResult, 4)
	channel.deliverCommands([]channelCommand{
		command("01HF7YAT000000000000000301", now.Add(time.Minute)),
		command("01HF7YAT000000000000000302", now.Add(-time.Minute)),
	}, results)
	if len(delivered) != 1 {
		t.Fatalf("delivered %d commands, want 1", len(delivered))
	}
	select {
	case result := <-results:
		t.Fatalf("a nil result was sent: %+v", result)
	default:
	}

	channel.onCommand = func(Command) *CommandResult { return &CommandResult{Status: "maybe"} }
	channel.deliverCommands([]channelCommand{command("01HF7YAT000000000000000303", now.Add(time.Minute))}, results)
	select {
	case result := <-results:
		if result.Status != commandStatusRejected || result.Result != "invalid result status" {
			t.Fatalf("invalid status result = %+v", result)
		}
	default:
		t.Fatal("an invalid command status produced no result")
	}

	channel.onCommand = func(Command) *CommandResult { return &CommandResult{} }
	channel.deliverCommands([]channelCommand{command("01HF7YAT000000000000000304", now.Add(time.Minute))}, results)
	select {
	case result := <-results:
		if result.Status != commandStatusAcked {
			t.Fatalf("empty status result = %+v, want acked", result)
		}
	default:
		t.Fatal("an empty command status produced no result")
	}

	// A transport without a result path still delivers the command.
	channel.onCommand = func(Command) *CommandResult { return &CommandResult{} }
	channel.deliverCommands([]channelCommand{command("01HF7YAT000000000000000305", now.Add(time.Minute))}, nil)
}

// newCustomTransportChannel returns a channel whose client uses a caller-owned
// HTTP client, so no WebSocket dialer is built.
func newCustomTransportChannel(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(Config{
		BaseURL:        server.URL,
		Credential:     testCredential,
		PublicKeys:     goldenPublicKeys(t, loadGolden(t)),
		HTTPClient:     server.Client(),
		RequestTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestChannelWithoutDialer(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newCustomTransportChannel(t, server)

	pinned := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportWebSocket})
	if err := pinned.Run(context.Background()); err == nil {
		t.Fatal("a WebSocket-only channel without a dialer started")
	}

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	auto := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportAuto})
	runChannel(t, auto)
	requireInSync(t, machine, "books")
	if got := auto.CurrentTransport(); got != TransportLongPoll {
		t.Fatalf("CurrentTransport = %q, want longpoll", got)
	}
}

func TestChannelAutoStopsOnRevokedUpgrade(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	client.credential = "adsi_" + testInstanceID + "_" + strings.Repeat("b", 43)

	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportAuto})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := channel.Run(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Run = %v, want ErrUnauthorized", err)
	}
	instance := machine.Instance()
	if instance.Liveness != LivenessStopped || instance.Reason != ReasonRevoked {
		t.Fatalf("instance = %+v, want stopped/revoked", instance)
	}
}

func TestChannelApplyAckRejectsInvalidDurations(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportLongPoll})

	valid := func() *channelAck {
		return &channelAck{
			Type:              channelTypeAck,
			ServicesVersion:   1,
			ServerTime:        time.Now().UnixMilli(),
			HeartbeatInterval: "30s",
			StaleAfter:        "60s",
			StopAfter:         "120s",
			Services: []channelAckService{{
				ID: testServiceID1, Name: "books", Head: 1, OriginHead: 1,
				Epoch: testServiceID1, HeadStateHash: books.StateHash,
			}},
		}
	}
	cases := []struct {
		name   string
		update func(*channelAck)
	}{
		{"heartbeat", func(ack *channelAck) { ack.HeartbeatInterval = "soon" }},
		{"stale", func(ack *channelAck) { ack.StaleAfter = "soon" }},
		{"stop", func(ack *channelAck) { ack.StopAfter = "soon" }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ack := valid()
			testCase.update(ack)
			if err := channel.applyAck(ack, nil, nil); !errors.Is(err, ErrProtocol) {
				t.Fatalf("applyAck = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestChannelLongPollLoopRetriesAfterHealthyPoll(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	// The first poll is answered, every later poll fails, so the loop has to
	// reset its attempt counter and retry.
	source.configure(func(f *channelSource) { f.failPollFrom = 2 })
	server := startChannelFixture(t, source)
	client := newChannelClient(t, server)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportLongPoll})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- channel.runLongPollLoop(ctx, time.Time{}) }()
	waitFor(t, 3*time.Second, "a failed poll after the first ack", func() bool {
		_, _, polls, _, _, _ := source.stats()
		return polls >= 2
	})
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runLongPollLoop = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runLongPollLoop did not stop")
	}
}
