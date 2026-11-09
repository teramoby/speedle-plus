// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

// This file composes one in-process ADS instance for the end-to-end
// distribution suite: the policy-source store, an evaluator and its REST
// router, the optional proxy server for downstream instances, and the
// instance-level notification channel. It also applies the commands the
// channel delivers (pause, resume, resync, set_source, stop), so the suite can
// drive the same behaviour an operations command would.

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsproxy"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
)

const (
	// e2eHeartbeatCadence is the real-time cadence of the test instances. The
	// design's 30s default would make a reconnect or a pause take too long for a
	// unit-sized suite; the state machine thresholds stay at the protocol floor
	// and are moved by the injected clock instead.
	e2eHeartbeatCadence = 250 * time.Millisecond
	e2eReconcilePeriod  = 50 * time.Millisecond
	e2eMirrorPeriod     = 30 * time.Millisecond
)

// e2eAgentOptions configures one in-process ADS instance.
type e2eAgentOptions struct {
	InstanceID string
	Source     e2eSource
	Fallback   *e2eSource
	Clock      *testClock
	CacheDir   string
	Chain      []string
	// Downstream enables the proxy role with the given credentials.
	Downstream      []adsproxy.Credential
	DisableFallback bool
	Services        []string
}

type e2eProxyRequest struct {
	Path          string
	Authorization string
}

// e2eAgent is one in-process ADS instance.
type e2eAgent struct {
	t          *testing.T
	instanceID string
	clock      *testClock
	sourceURL  string

	store     *Store
	evaluator eval.InternalEvaluator
	router    http.Handler
	server    *httptest.Server
	client    *http.Client
	caFile    string

	mirror *adsproxy.Mirror
	proxy  *adsproxy.Server
	paused atomic.Bool

	channel        *adsync.Channel
	channelStopped atomic.Bool

	runMu     sync.Mutex
	runCancel context.CancelFunc
	runDone   chan struct{}

	syncCh  chan struct{}
	stopOne sync.Once

	mu            sync.Mutex
	commands      []adsync.Command
	seenCommands  map[string]struct{}
	proxyRequests []e2eProxyRequest
	mirrorIDs     map[string]struct{}
}

func newE2EAgent(t *testing.T, options e2eAgentOptions) *e2eAgent {
	t.Helper()
	if options.InstanceID == "" {
		t.Fatal("an instance id is required")
	}
	if options.Clock == nil {
		t.Fatal("a clock is required")
	}
	if len(options.Chain) == 0 {
		options.Chain = []string{options.InstanceID}
	}
	if options.Source.URL == "" || options.Source.Credential == "" || options.Source.Keys == nil {
		t.Fatal("a complete source configuration is required")
	}

	var fallbackURL, fallbackCredential string
	if options.Fallback != nil {
		fallbackURL = options.Fallback.URL
		fallbackCredential = options.Fallback.Credential
	}
	store, err := Open(context.Background(), Options{
		PolicySourceURL:   options.Source.URL,
		Credential:        options.Source.Credential,
		PublicKeys:        options.Source.Keys,
		CAFile:            options.Source.CAFile,
		Clock:             options.Clock,
		CacheDir:          options.CacheDir,
		Services:          options.Services,
		HeartbeatInterval: 15 * time.Second,
		StaleAfter:        30 * time.Second,
		StopAfter:         60 * time.Second,
		RequestTimeout:    2 * time.Second,
		Retry: adsync.RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
		},
		DisableFallback:    options.DisableFallback,
		FallbackURL:        fallbackURL,
		FallbackCredential: fallbackCredential,
	})
	if err != nil {
		t.Fatalf("open policy source store for %s: %v", options.InstanceID, err)
	}

	evaluator, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, store)
	if err != nil {
		t.Fatalf("build evaluator for %s: %v", options.InstanceID, err)
	}
	router, err := adsrest.NewRouter(evaluator)
	if err != nil {
		t.Fatalf("build REST router for %s: %v", options.InstanceID, err)
	}

	agent := &e2eAgent{
		t:            t,
		instanceID:   options.InstanceID,
		clock:        options.Clock,
		sourceURL:    options.Source.URL,
		store:        store,
		evaluator:    evaluator,
		router:       router,
		syncCh:       make(chan struct{}, 1),
		seenCommands: make(map[string]struct{}),
		mirrorIDs:    make(map[string]struct{}),
	}

	handler := http.Handler(router)
	if len(options.Downstream) > 0 {
		mirror, err := adsproxy.NewMirror(adsproxy.MirrorOptions{PublicKeys: options.Source.Keys})
		if err != nil {
			t.Fatalf("build mirror for %s: %v", options.InstanceID, err)
		}
		credentials, err := adsproxy.NewCredentialSet(options.Downstream)
		if err != nil {
			t.Fatalf("build downstream credentials for %s: %v", options.InstanceID, err)
		}
		proxy, err := adsproxy.NewServer(adsproxy.Config{
			InstanceID:        options.InstanceID,
			Chain:             options.Chain,
			Mirror:            mirror,
			Credentials:       credentials,
			HeartbeatInterval: 15 * time.Second,
			StaleAfter:        30 * time.Second,
			StopAfter:         60 * time.Second,
			CoalesceWindow:    20 * time.Millisecond,
			Paused:            agent.paused.Load,
		})
		if err != nil {
			t.Fatalf("build proxy for %s: %v", options.InstanceID, err)
		}
		agent.mirror = mirror
		agent.proxy = proxy
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, adsync.APIPrefix) {
				agent.recordProxyRequest(r)
				proxy.ServeHTTP(w, r)
				return
			}
			router.ServeHTTP(w, r)
		})
	}

	agent.server = httptest.NewUnstartedServer(handler)
	agent.server.Config.ErrorLog = log.New(io.Discard, "", 0)
	agent.server.StartTLS()
	agent.caFile = e2eWriteCAFile(t, agent.server.Certificate())
	agent.client = agent.server.Client()
	agent.client.Timeout = 5 * time.Second

	upstreamInstanceID := ""
	if len(options.Chain) > 1 {
		upstreamInstanceID = options.Chain[len(options.Chain)-2]
	}
	channel, err := adsync.NewChannel(store.client, adsync.ChannelOptions{
		InstanceID:         options.InstanceID,
		AgentVersion:       e2eAgentVersion,
		Chain:              options.Chain,
		Machine:            store.machine,
		Transport:          adsync.TransportWebSocket,
		HeartbeatInterval:  e2eHeartbeatCadence,
		Backoff:            adsync.Backoff{Initial: 5 * time.Millisecond, Max: 50 * time.Millisecond, Jitter: 0.01},
		UpstreamInstanceID: upstreamInstanceID,
		Fallback:           agent.fallbackState,
		OnNotify:           func(adsync.Notify) { agent.triggerSync() },
		OnServiceSetChange: func(adsync.ServiceSetChange) { agent.triggerSync() },
		OnCommand:          agent.handleCommand,
	})
	if err != nil {
		t.Fatalf("build channel for %s: %v", options.InstanceID, err)
	}
	agent.channel = channel

	if agent.mirror != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		agent.syncMirror(ctx)
		cancel()
	}

	agent.start()
	t.Cleanup(agent.stop)
	return agent
}

// start runs the channel, the reconciliation loop and, for a proxy, the mirror
// loop. It is also how a partition heals.
func (a *e2eAgent) start() {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.runCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.channelStopped.Store(false)
	var running sync.WaitGroup
	launch := func(run func(context.Context)) {
		running.Add(1)
		go func() {
			defer running.Done()
			run(ctx)
		}()
	}
	launch(func(ctx context.Context) {
		_ = a.channel.Run(ctx)
		a.channelStopped.Store(true)
	})
	launch(a.reconcile)
	if a.mirror != nil {
		launch(a.mirrorLoop)
	}
	done := make(chan struct{})
	go func() {
		running.Wait()
		close(done)
	}()
	a.runCancel = cancel
	a.runDone = done
}

// stopRun stops every background loop. The store keeps its copies, which is
// what a network partition looks like from this instance.
func (a *e2eAgent) stopRun() {
	a.runMu.Lock()
	cancel, done := a.runCancel, a.runDone
	a.runCancel, a.runDone = nil, nil
	a.runMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

// partition cuts this instance off from its source.
func (a *e2eAgent) partition() {
	a.stopRun()
}

// heal reconnects this instance to its source.
func (a *e2eAgent) heal() {
	a.start()
}

// stop tears the instance down for good.
func (a *e2eAgent) stop() {
	a.stopOne.Do(func() {
		a.stopRun()
		a.server.Close()
		e2eCloseEvaluator(a.evaluator)
		a.store.StopWatch()
		if a.proxy != nil {
			a.proxy.Close()
		}
	})
}

// reconcile drives the store's synchronization passes. The store owns its
// state machine, and the channel only reports head advances, so this loop is
// how the instance confirms liveness and how a stopped instance re-verifies
// its copies after a reconnect.
func (a *e2eAgent) reconcile(ctx context.Context) {
	ticker := time.NewTicker(e2eReconcilePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-a.syncCh:
		}
		_ = a.store.Sync(ctx)
	}
}

// triggerSync asks the reconciliation loop for a pass without blocking the
// channel's read loop.
func (a *e2eAgent) triggerSync() {
	select {
	case a.syncCh <- struct{}{}:
	default:
	}
}

func (a *e2eAgent) fallbackState() adsync.FallbackState {
	state := adsync.FallbackState{Active: false}
	for _, service := range a.store.Services() {
		if service.State == adsync.SyncStateSyncing {
			state.Active = true
			state.Services = append(state.Services, service.ID)
		}
	}
	return state
}

// --- Mirror feeding ---

func (a *e2eAgent) mirrorLoop(ctx context.Context) {
	ticker := time.NewTicker(e2eMirrorPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		a.syncMirror(ctx)
	}
}

// syncMirror brings the proxy copy to the source head with the same public
// client API a downstream instance would use.
func (a *e2eAgent) syncMirror(ctx context.Context) {
	if a.mirror == nil {
		return
	}
	discovery, err := a.store.client.Services(ctx)
	if err != nil {
		return
	}
	live := make(map[string]struct{}, len(discovery.Services))
	for _, service := range discovery.Services {
		live[service.ID] = struct{}{}
		info, availability := a.mirror.Lookup(service.ID)
		sameIdentity := availability == adsproxy.ServiceAvailable &&
			info.SourceID == service.ID && info.Epoch == service.Epoch
		switch {
		case sameIdentity && info.Revision == service.Revision:
			// Up to date.
		case sameIdentity && info.Revision < service.Revision:
			if err := a.catchUpMirror(ctx, service, info); err != nil {
				a.loadMirrorSnapshot(ctx, service)
			}
		default:
			a.loadMirrorSnapshot(ctx, service)
		}
	}
	a.mu.Lock()
	for id := range a.mirrorIDs {
		if _, found := live[id]; !found {
			a.mirror.RemoveService(id)
			delete(a.mirrorIDs, id)
		}
	}
	for id := range live {
		a.mirrorIDs[id] = struct{}{}
	}
	a.mu.Unlock()
}

func (a *e2eAgent) catchUpMirror(ctx context.Context, service adsync.Service, info adsproxy.ServiceInfo) error {
	since := info.Revision
	anchor := info.CommitHash
	for {
		page, err := a.store.client.Changes(ctx, service.ID, info.SourceID, info.Epoch, since, anchor, 100)
		if err != nil {
			return err
		}
		if err := a.mirror.ApplyChanges(page); err != nil {
			return err
		}
		if page.To >= page.Head || len(page.Commits) == 0 {
			return nil
		}
		since = page.NextSince
		anchor = page.Commits[len(page.Commits)-1].Hash
	}
}

func (a *e2eAgent) loadMirrorSnapshot(ctx context.Context, service adsync.Service) {
	snapshot, _, err := a.store.client.Snapshot(ctx, service.ID, "")
	if err != nil {
		return
	}
	_ = a.mirror.ApplySnapshot(snapshot, service)
}

// --- Commands ---

func (a *e2eAgent) handleCommand(command adsync.Command) *adsync.CommandResult {
	a.mu.Lock()
	if _, seen := a.seenCommands[command.ID]; seen {
		a.mu.Unlock()
		// A source may repeat a command in several acks.
		return nil
	}
	a.seenCommands[command.ID] = struct{}{}
	a.commands = append(a.commands, command)
	a.mu.Unlock()

	switch command.Action {
	case "pause":
		a.paused.Store(true)
		a.store.machine.Pause()
		return &adsync.CommandResult{Status: "acked"}
	case "resume":
		a.paused.Store(false)
		a.store.machine.Resume()
		a.triggerSync()
		return &adsync.CommandResult{Status: "acked"}
	case "resync":
		a.resync(command.ServiceID)
		return &adsync.CommandResult{Status: "acked"}
	case "set_source":
		// The suite's source is already the requested URL; any other profile
		// is unknown locally, as the design requires.
		if command.CredentialProfile == "" || command.SourceURL != a.sourceURL {
			return &adsync.CommandResult{Status: "rejected", Result: "unknown credential profile"}
		}
		a.triggerSync()
		return &adsync.CommandResult{Status: "acked"}
	case "stop":
		a.store.machine.Stop(adsync.ReasonStoppedByCommand)
		return &adsync.CommandResult{Status: "acked"}
	default:
		return &adsync.CommandResult{Status: "rejected", Result: "unknown action"}
	}
}

// resync discards the named service's copy and asks for a fresh one, which is
// what a forced re-synchronization command requires.
func (a *e2eAgent) resync(serviceID string) {
	var targets []string
	if serviceID != "" {
		targets = append(targets, serviceID)
	} else {
		for _, service := range a.store.Services() {
			if service.State != adsync.SyncStateRemoved {
				targets = append(targets, service.ID)
			}
		}
	}
	changed := false
	for _, id := range targets {
		a.store.machine.Resync(id)
		if err := a.store.syncer.Forget(id); err != nil {
			continue
		}
		a.store.dropCopy(id)
		changed = true
	}
	if changed {
		a.store.publish()
	}
	a.triggerSync()
}

func (a *e2eAgent) commandReceived(action, serviceID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, command := range a.commands {
		if command.Action == action && command.ServiceID == serviceID {
			return true
		}
	}
	return false
}

// --- Requests ---

func (a *e2eAgent) recordProxyRequest(r *http.Request) {
	a.mu.Lock()
	a.proxyRequests = append(a.proxyRequests, e2eProxyRequest{
		Path:          r.URL.Path,
		Authorization: r.Header.Get("Authorization"),
	})
	a.mu.Unlock()
}

// proxyDataRequests counts snapshot and change pulls this proxy served.
func (a *e2eAgent) proxyDataRequests() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	count := 0
	for _, request := range a.proxyRequests {
		if strings.HasSuffix(request.Path, "/snapshot") || strings.HasSuffix(request.Path, "/changes") {
			count++
		}
	}
	return count
}

func (a *e2eAgent) proxyDataRequestsWithToken(token string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	expected := "Bearer " + token
	count := 0
	for _, request := range a.proxyRequests {
		if request.Authorization != expected {
			continue
		}
		if strings.HasSuffix(request.Path, "/snapshot") || strings.HasSuffix(request.Path, "/changes") {
			count++
		}
	}
	return count
}

func (a *e2eAgent) status(serviceName string) (adsync.Status, bool) {
	return a.store.Status(serviceName)
}

func (a *e2eAgent) liveness() adsync.Liveness {
	return a.store.Instance().Liveness
}

func (a *e2eAgent) channelFinished() bool {
	return a.channelStopped.Load()
}

func (a *e2eAgent) post(path string, body []byte) (int, http.Header, []byte) {
	request, err := http.NewRequest(http.MethodPost, a.server.URL+path, strings.NewReader(string(body)))
	if err != nil {
		a.t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		a.t.Fatalf("POST %s: %v", path, err)
	}
	defer response.Body.Close()
	payload := make([]byte, 0, 1024)
	buffer := make([]byte, 4096)
	for {
		read, readErr := response.Body.Read(buffer)
		payload = append(payload, buffer[:read]...)
		if readErr != nil {
			break
		}
	}
	return response.StatusCode, response.Header.Clone(), payload
}

func (a *e2eAgent) isAllowed(body []byte) (int, http.Header, []byte) {
	return a.post(svcs.PolicyAtzPath+"is-allowed", body)
}

func (a *e2eAgent) get(path string) (int, []byte) {
	request, err := http.NewRequest(http.MethodGet, a.server.URL+path, nil)
	if err != nil {
		a.t.Fatalf("build request: %v", err)
	}
	response, err := a.client.Do(request)
	if err != nil {
		a.t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	payload := make([]byte, 0, 1024)
	buffer := make([]byte, 4096)
	for {
		read, readErr := response.Body.Read(buffer)
		payload = append(payload, buffer[:read]...)
		if readErr != nil {
			break
		}
	}
	return response.StatusCode, payload
}

func (a *e2eAgent) readyz() (int, []byte) {
	return a.get("/readyz")
}

// proxyGet performs one downstream request against this agent's proxy.
func (a *e2eAgent) proxyGet(token, path string) (int, []byte) {
	request, err := http.NewRequest(http.MethodGet, a.server.URL+path, nil)
	if err != nil {
		a.t.Fatalf("build proxy request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := a.client.Do(request)
	if err != nil {
		a.t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		a.t.Fatalf("read proxy response: %v", err)
	}
	return response.StatusCode, payload
}

func (a *e2eAgent) healthOK() int {
	status, _ := a.get("/healthz")
	return status
}

// --- Request bodies used by the parity checks ---

type e2eCheck struct {
	Service    string
	Resource   string
	Action     string
	Principals []*adsrest.JsonPrincipal
	Attributes []*adsrest.JsonAttribute
}

func (c e2eCheck) body(t *testing.T) []byte {
	t.Helper()
	request := adsrest.JsonContext{
		Subject:     &adsrest.JsonSubject{Principals: c.Principals},
		ServiceName: c.Service,
		Resource:    c.Resource,
		Action:      c.Action,
		Attributes:  c.Attributes,
	}
	encoded, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("encode check: %v", err)
	}
	return encoded
}

func e2eUser(name string) []*adsrest.JsonPrincipal {
	return []*adsrest.JsonPrincipal{{Type: "user", Name: name}}
}

func e2eGroup(name string) []*adsrest.JsonPrincipal {
	return []*adsrest.JsonPrincipal{{Type: "group", Name: name}}
}

func e2eAttr(name, dataType string, value interface{}) []*adsrest.JsonAttribute {
	return []*adsrest.JsonAttribute{{Name: name, Type: dataType, Value: value}}
}
