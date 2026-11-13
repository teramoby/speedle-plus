// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testAPIKey           = "spdl_01HF7YAT000000000000000K01_directSecretValue0123456789"
	testDirectInstanceID = "01a1d38d-8c69-7ac1-b3d6-cb9ade5c63f4"
)

// directFixture is the shared fixture switched to an API key.
func directFixture(t *testing.T, services ...*fixtureService) *fixture {
	t.Helper()
	f := newFixture(t, services...)
	f.source.configure(func(source *fixtureSource) { source.credential = testAPIKey })
	return f
}

func directConfig(config *Config) {
	config.Credential = ""
	config.APIKey = testAPIKey
	config.InstanceID = testDirectInstanceID
	config.PublicKeys = nil
}

func TestInstanceIDIsAPersistedUUIDv7(t *testing.T) {
	id, err := NewInstanceID()
	if err != nil || !ValidInstanceID(id) || validULID(id) {
		t.Fatalf("NewInstanceID = %q, %v", id, err)
	}
	if ValidInstanceID(strings.ToUpper(id)) || ValidInstanceID(testInstanceID) {
		t.Fatal("ValidInstanceID accepted a non-canonical id")
	}

	dir := filepath.Join(t.TempDir(), "cache")
	first, created, err := LoadOrCreateInstanceID(dir)
	if err != nil || !created || !ValidInstanceID(first) {
		t.Fatalf("first LoadOrCreateInstanceID = %q, %v, %v", first, created, err)
	}
	info, err := os.Stat(filepath.Join(dir, InstanceIDFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("instance id file: %v, %v", info, err)
	}
	second, created, err := LoadOrCreateInstanceID(dir)
	if err != nil || created || second != first {
		t.Fatalf("second LoadOrCreateInstanceID = %q, %v, %v; want %q", second, created, err, first)
	}

	if _, _, err := LoadOrCreateInstanceID(""); err == nil {
		t.Fatal("empty directory accepted")
	}
	path := filepath.Join(dir, InstanceIDFile)
	if err := os.WriteFile(path, []byte("not-an-id"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateInstanceID(dir); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("corrupt id error = %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 200)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateInstanceID(dir); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("oversized id error = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateInstanceID(dir); !errors.Is(err, ErrUnsafeCache) {
		t.Fatalf("public id file error = %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateInstanceID(dir); !errors.Is(err, ErrUnsafeCache) {
		t.Fatalf("directory id error = %v", err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateInstanceID(filepath.Join(blocker, "sub")); err == nil {
		t.Fatal("unusable directory accepted")
	}
}

func TestValidateAPIKey(t *testing.T) {
	if err := ValidateAPIKey(testAPIKey); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	for _, bad := range []string{"", "has space", "tab\tkey", strings.Repeat("k", maxAPIKeyBytes+1), "ключ"} {
		err := ValidateAPIKey(bad)
		if err == nil {
			t.Fatalf("key %q accepted", bad)
		}
		if bad != "" && strings.Contains(err.Error(), bad) {
			t.Fatal("error echoes the key")
		}
	}
}

func TestNewClientDirectModeValidation(t *testing.T) {
	f := directFixture(t)
	cases := map[string]func(*Config){
		"credential and key": func(config *Config) { directConfig(config); config.Credential = testCredential },
		"malformed key":      func(config *Config) { directConfig(config); config.APIKey = "bad key " + testAPIKey },
		"no instance id":     func(config *Config) { directConfig(config); config.InstanceID = "" },
		"ULID instance id":   func(config *Config) { directConfig(config); config.InstanceID = testInstanceID },
		"id without key":     func(config *Config) { config.InstanceID = testDirectInstanceID },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewClient(f.config(mutate))
			if err == nil {
				t.Fatal("configuration accepted")
			}
			if strings.Contains(err.Error(), "directSecretValue") {
				t.Fatal("error echoes the key")
			}
		})
	}
	client := f.newClient(directConfig)
	if !client.DirectMode() || client.InstanceID() != testDirectInstanceID || client.SignaturesVerified() {
		t.Fatal("direct-mode client reports the wrong mode")
	}
	legacy := f.newClient(nil)
	if legacy.DirectMode() || legacy.InstanceID() != "" || !legacy.SignaturesVerified() || legacy.fallbackSlots != nil {
		t.Fatal("credential client reports the wrong mode")
	}
}

// TestDirectModeSendsKeyAndInstanceHeader syncs through the shared fixture
// with an API key and no public keys: every request carries the key and the
// instance header, and the signatures the fixture still sends are ignored.
func TestDirectModeSendsKeyAndInstanceHeader(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	f := directFixture(t, service)
	f.source.configure(func(*fixtureSource) {
		service.publish(t, f.signer, Change{Op: "upsert", Kind: "authz", ID: "p1", Name: stringPointer("p1"), SPDL: stringPointer("grant user a read r")})
	})
	syncer := f.newSyncer(t.TempDir(), directConfig)
	state := syncFixtureService(t, syncer, "books")
	if state.Revision != 2 || state.signature != (Signature{}) {
		t.Fatalf("state = revision %d signature %+v", state.Revision, state.signature)
	}
	if _, err := syncer.client.Changes(context.Background(), testServiceID1, testServiceID1, testServiceID1, 1, service.commits[0].PrevHash, 0); err != nil {
		t.Fatalf("changes without verification: %v", err)
	}
	f.source.mu.Lock()
	defer f.source.mu.Unlock()
	for _, request := range f.source.requests {
		if request.Authorization != "Bearer "+testAPIKey || request.Instance != testDirectInstanceID {
			t.Fatalf("request %s without the API key or instance header %q", request.Path, request.Instance)
		}
	}
}

// TestDirectModeAcceptsResponsesWithoutSignatureFields decodes snapshot and
// changes bodies that carry no signature fields at all.
func TestDirectModeAcceptsResponsesWithoutSignatureFields(t *testing.T) {
	snapshot := Snapshot{Name: "books", Policies: []Policy{}, RolePolicies: []Policy{}}
	hash := StateHash(snapshot)
	change := Change{Op: "upsert", Kind: "authz", ID: "p1", Name: stringPointer("p1"), SPDL: stringPointer("grant user a read r")}
	commitHash, err := CommitHash(hash, []Change{change})
	if err != nil {
		t.Fatal(err)
	}
	next := cloneSnapshot(snapshot)
	if err := applyChanges(&next, []Change{change}); err != nil {
		t.Fatal(err)
	}
	var changesStatus atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status := changesStatus.Load(); status != 0 && strings.HasSuffix(r.URL.Path, "/changes") {
			w.WriteHeader(int(status))
			_, _ = io.WriteString(w, `{"error":"failure"}`)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/snapshot"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"serviceId": testServiceID1, "sourceId": testServiceID1, "epoch": testServiceID1,
				"revision": 1, "stateHash": hash, "commitHash": hash, "snapshot": snapshot,
			})
		case strings.HasSuffix(r.URL.Path, "/changes"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"serviceId": testServiceID1, "sourceId": testServiceID1, "epoch": testServiceID1,
				"from": 2, "to": 2, "head": 2, "headStateHash": StateHash(next), "nextSince": 3,
				"commits": []map[string]any{{"revision": 2, "prevHash": hash, "hash": commitHash, "at": 1, "changes": []Change{change}}},
			})
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: testAPIKey, InstanceID: testDirectInstanceID, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Snapshot(context.Background(), testServiceID1, ""); err != nil {
		t.Fatalf("unsigned snapshot: %v", err)
	}
	if _, err := client.Changes(context.Background(), testServiceID1, testServiceID1, testServiceID1, 1, hash, 0); err != nil {
		t.Fatalf("unsigned changes: %v", err)
	}
	syncer, err := NewSyncer(client, filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := syncer.SyncAtLeast(context.Background(), Service{ID: testServiceID1, Name: "books", Revision: 1, Epoch: testServiceID1, HeadStateHash: hash}, 2)
	if err != nil || state.Revision != 1 {
		t.Fatalf("first sync = %+v, %v", state, err)
	}
	// A failed fetch of a notified head keeps the copy at the ack head, and
	// a service the source no longer serves is forgotten.
	ackHead := Service{ID: testServiceID1, Name: "books", Revision: 1, Epoch: testServiceID1, HeadStateHash: hash}
	changesStatus.Store(http.StatusServiceUnavailable)
	if _, err := syncer.SyncAtLeast(context.Background(), ackHead, 2); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("failed notified fetch = %v", err)
	}
	if current, found := syncer.State(testServiceID1); !found || current.Revision != 1 {
		t.Fatal("a failed notified fetch dropped the copy")
	}
	changesStatus.Store(http.StatusNotFound)
	if _, err := syncer.SyncAtLeast(context.Background(), ackHead, 2); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("missing service = %v", err)
	}
	if _, found := syncer.State(testServiceID1); found {
		t.Fatal("a missing service kept its copy")
	}
	changesStatus.Store(0)
	if _, err := syncer.Sync(context.Background(), ackHead); err != nil {
		t.Fatal(err)
	}
	// A notified head above the ack is fetched from the local copy.
	state, err = syncer.SyncAtLeast(context.Background(), Service{ID: testServiceID1, Name: "books", Revision: 1, Epoch: testServiceID1, HeadStateHash: hash}, 2)
	if err != nil || state.Revision != 2 {
		t.Fatalf("notified sync = %+v, %v", state, err)
	}
	// A copy ahead of the ack is kept, never rolled back.
	state, err = syncer.Sync(context.Background(), Service{ID: testServiceID1, Name: "books", Revision: 1, Epoch: testServiceID1, HeadStateHash: hash})
	if err != nil || state.Revision != 2 {
		t.Fatalf("ahead sync = %+v, %v", state, err)
	}
	if cached := syncer.CachedServices(); len(cached) != 1 || cached[0].ID != testServiceID1 || cached[0].Name != "books" {
		t.Fatalf("cached services = %+v", cached)
	}
	// A reloaded snapshot of another epoch fails closed.
	if _, err := syncer.Sync(context.Background(), Service{ID: testServiceID1, Name: "books", Revision: 1, Epoch: testServiceID3, HeadStateHash: hash}); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("epoch mismatch error = %v", err)
	}
}

func TestDirectModeChannelMessages(t *testing.T) {
	f := directFixture(t)
	machine, _ := newTestMachine(t)
	client := f.newClient(directConfig)
	for name, options := range map[string]ChannelOptions{
		"other instance id": {InstanceID: testInstanceID},
		"credential id":     {CredentialID: testInstanceID},
		"chain":             {Chain: []string{testInstanceID}},
		"upstream":          {UpstreamInstanceID: testInstanceID},
	} {
		options.Machine, options.AgentVersion = machine, "test"
		if _, err := NewChannel(client, options); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	channel, err := NewChannel(client, ChannelOptions{Machine: machine, AgentVersion: "test/1", HostName: "edge-1"})
	if err != nil {
		t.Fatal(err)
	}
	decode := func(value any) map[string]any {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		_ = json.Unmarshal(encoded, &decoded)
		return decoded
	}
	for _, message := range []map[string]any{decode(channel.helloMessage(TransportWebSocket)), decode(channel.heartbeatMessage(TransportLongPoll))} {
		if message["instanceId"] != testDirectInstanceID || message["agentVersion"] != "test/1" || message["hostName"] != "edge-1" {
			t.Fatalf("message = %v", message)
		}
		if _, found := message["credentialId"]; found {
			t.Fatal("credential id sent")
		}
		if chain, ok := message["chain"].([]any); !ok || len(chain) != 0 {
			t.Fatalf("chain = %v", message["chain"])
		}
	}
	header := channel.requestHeaders()
	if header.Get("Authorization") != "Bearer "+testAPIKey || header.Get(InstanceHeader) != testDirectInstanceID {
		t.Fatalf("channel headers = %v", header)
	}
	if err := channel.terminalStatusError(http.MethodPost, "/poll", http.StatusConflict, []byte(`{"error":"quota_exceeded","limit":10}`)); err != nil {
		t.Fatalf("quota_exceeded is terminal in direct mode: %v", err)
	}
	if odd, err := NewChannel(client, ChannelOptions{Machine: machine, AgentVersion: "x", HostName: "bad\nhost", InstanceID: testDirectInstanceID}); err != nil || odd.hostName != "" {
		t.Fatalf("control characters in the host name were kept: %v", err)
	}

	// A credential channel keeps its messages and its terminal 409.
	legacyMachine, _ := newTestMachine(t)
	legacy, err := NewChannel(newFixture(t).newClient(nil), ChannelOptions{
		InstanceID: testInstanceID, Machine: legacyMachine, AgentVersion: "test/1", HostName: "edge-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	heartbeat := decode(legacy.heartbeatMessage(TransportLongPoll))
	hello := decode(legacy.helloMessage(TransportWebSocket))
	if _, found := heartbeat["instanceId"]; found {
		t.Fatal("credential heartbeat carries an instance id")
	}
	if _, found := heartbeat["hostName"]; found || hello["hostName"] != nil || hello["instanceId"] != testInstanceID {
		t.Fatalf("credential messages changed: %v %v", hello, heartbeat)
	}
	if legacy.requestHeaders().Get(InstanceHeader) != "" {
		t.Fatal("credential channel sends an instance header")
	}
	if err := legacy.terminalStatusError(http.MethodPost, "/poll", http.StatusConflict, nil); !isTerminalError(err) {
		t.Fatalf("credential 409 = %v, want terminal", err)
	}
}

func TestChannelReportsEveryAck(t *testing.T) {
	f := newFixture(t)
	machine, _ := newTestMachine(t)
	var acks atomic.Int32
	channel, err := NewChannel(f.newClient(nil), ChannelOptions{
		InstanceID: testInstanceID, Machine: machine, AgentVersion: "test",
		OnAck: func() { acks.Add(1) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, acked := channel.AckedServices(); acked {
		t.Fatal("acked before the first ack")
	}
	ack := &channelAck{Type: "ack", ServicesVersion: 1, Services: []channelAckService{}, HeartbeatInterval: "30s", StaleAfter: "60s", StopAfter: "120s"}
	for range 2 {
		if err := channel.applyAck(ack, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if services, acked := channel.AckedServices(); !acked || len(services) != 0 || acks.Load() != 2 {
		t.Fatalf("AckedServices = %v, %v; OnAck calls %d", services, acked, acks.Load())
	}
}

func newReconcilingMachine(t *testing.T) *Machine {
	t.Helper()
	machine, err := NewMachine(MachineOptions{Clock: newFakeClock(), ReconcileOnAck: true})
	if err != nil {
		t.Fatal(err)
	}
	return machine
}

func serviceState(t *testing.T, machine *Machine, name string) ServiceStatus {
	t.Helper()
	status, known := machine.Status(name)
	if !known {
		t.Fatalf("service %q unknown", name)
	}
	return status.Service
}

func TestReconcileOnAckFailsAnAheadCopyClosed(t *testing.T) {
	machine := newReconcilingMachine(t)
	at3 := testAckService(testServiceID1, "books", 3)
	if err := machine.Ack(ackWith(at3)); err != nil {
		t.Fatal(err)
	}
	if err := machine.Applied(testServiceState(testServiceID1, "books", 3)); err != nil {
		t.Fatal(err)
	}
	if state := serviceState(t, machine, "books"); state.State != SyncStateInSync {
		t.Fatalf("state = %+v", state)
	}
	if err := machine.Ack(ackWith(testAckService(testServiceID1, "books", 2))); err != nil {
		t.Fatal(err)
	}
	if state := serviceState(t, machine, "books"); state.Reason != SyncReasonIntegrity || state.Revision != 3 {
		t.Fatalf("ahead copy = %+v", state)
	}
	if err := machine.Ack(ackWith(at3)); err != nil {
		t.Fatal(err)
	}
	if state := serviceState(t, machine, "books"); state.State != SyncStateInSync {
		t.Fatalf("after the ack caught up = %+v", state)
	}

	// Without ReconcileOnAck the same sequence never fails closed.
	legacy, _ := newTestMachine(t)
	_ = legacy.Ack(ackWith(at3))
	_ = legacy.Applied(testServiceState(testServiceID1, "books", 3))
	_ = legacy.Ack(ackWith(testAckService(testServiceID1, "books", 2)))
	legacy.FailService(testServiceID1)
	if state := serviceState(t, legacy, "books"); state.Reason == SyncReasonIntegrity {
		t.Fatalf("credential machine failed a service closed: %+v", state)
	}
}

func TestFailServiceHoldsUntilTheHeadChanges(t *testing.T) {
	machine := newReconcilingMachine(t)
	at2 := testAckService(testServiceID1, "books", 2)
	_ = machine.Ack(ackWith(at2, testAckService(testServiceID3, "orders", 1)))
	_ = machine.Applied(testServiceState(testServiceID3, "orders", 1))
	machine.FailService(testServiceID1)
	machine.FailService("unknown")
	if state := serviceState(t, machine, "books"); state.Reason != SyncReasonIntegrity {
		t.Fatalf("failed service = %+v", state)
	}
	if state := serviceState(t, machine, "orders"); state.State != SyncStateInSync {
		t.Fatalf("other service = %+v", state)
	}
	// The same head keeps it failed, and so does a copy that does not match.
	_ = machine.Ack(ackWith(at2, testAckService(testServiceID3, "orders", 1)))
	_ = machine.Applied(testServiceState(testServiceID1, "books", 1))
	if state := serviceState(t, machine, "books"); state.Reason != SyncReasonIntegrity {
		t.Fatalf("after the same head = %+v", state)
	}
	// A copy that matches the head clears it.
	_ = machine.Applied(testServiceState(testServiceID1, "books", 2))
	if state := serviceState(t, machine, "books"); state.State != SyncStateInSync {
		t.Fatalf("after a matching copy = %+v", state)
	}
	// A new head clears a failure as well.
	machine.FailService(testServiceID1)
	_ = machine.Ack(ackWith(testAckService(testServiceID1, "books", 3), testAckService(testServiceID3, "orders", 1)))
	if state := serviceState(t, machine, "books"); state.Reason != SyncReasonCatchUp {
		t.Fatalf("after a new head = %+v", state)
	}
	// A removal clears every flag.
	machine.FailService(testServiceID1)
	_ = machine.Ack(ackWith(testAckService(testServiceID3, "orders", 1)))
	if state := serviceState(t, machine, "books"); state.State != SyncStateRemoved || state.Reason != "" {
		t.Fatalf("after removal = %+v", state)
	}
}

func TestTrackRecordsCachedServicesAsSyncing(t *testing.T) {
	machine := newReconcilingMachine(t)
	if err := machine.Track("bad", "books"); err == nil {
		t.Fatal("invalid id accepted")
	}
	if err := machine.Track(testServiceID1, "bad name!"); err == nil {
		t.Fatal("invalid name accepted")
	}
	if err := machine.Track(testServiceID1, "books"); err != nil {
		t.Fatal(err)
	}
	if err := machine.Track(testServiceID1, "books"); err != nil {
		t.Fatal(err)
	}
	if state := serviceState(t, machine, "books"); state.State != SyncStateSyncing || state.Reason != SyncReasonBootstrap {
		t.Fatalf("tracked = %+v", state)
	}
	for index := 0; len(machine.Services()) < maxServicesPerSource; index++ {
		id := fmt.Sprintf("01HF7YAT0000000000000004%02d", index)
		if err := machine.Track(id, "svc"+string(rune('a'+index))); err != nil {
			t.Fatal(err)
		}
	}
	if err := machine.Track(testServiceID3, "orders"); err == nil {
		t.Fatal("more services than the protocol bound were tracked")
	}
	_ = machine.Ack(ackWith(testAckService(testServiceID3, "orders", 1)))
	if err := machine.Track("01HF7YAT000000000000000999", "late"); err != nil {
		t.Fatal(err)
	}
	if _, known := machine.Status("late"); known {
		t.Fatal("Track after the first ack created a record")
	}
}

func newDirectFallbackClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(Config{
		BaseURL: server.URL, APIKey: testAPIKey, InstanceID: testDirectInstanceID, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestDirectModeForwardsToTheUpstream(t *testing.T) {
	var gotAuth, gotInstance, gotPath string
	client := newDirectFallbackClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotInstance, gotPath = r.Header.Get("Authorization"), r.Header.Get(InstanceHeader), r.URL.Path
		_, _ = w.Write([]byte(`{"allowed":true}`))
	})
	if !client.FallbackAvailable() {
		t.Fatal("direct mode has no forwarding")
	}
	response, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.DecisionSource != DecisionSourceUpstream || gotAuth != "Bearer "+testAPIKey ||
		gotInstance != testDirectInstanceID || gotPath != AuthzCheckAPIPrefix+"is-allowed" {
		t.Fatalf("forwarded key %v instance %q path %q, source %q", gotAuth == "Bearer "+testAPIKey, gotInstance, gotPath, response.DecisionSource)
	}
}

func TestDirectModeForwardingLimitsInFlightRequests(t *testing.T) {
	release := make(chan struct{})
	arrived := make(chan struct{}, 2*MaxUpstreamInFlight)
	client := newDirectFallbackClient(t, func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"allowed":false}`))
	})
	client.fallbackTimeout = 10 * time.Second
	var done sync.WaitGroup
	for range MaxUpstreamInFlight {
		done.Add(1)
		go func() {
			defer done.Done()
			_, _ = client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
		}()
	}
	for range MaxUpstreamInFlight {
		<-arrived
	}
	_, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
	close(release)
	done.Wait()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("request beyond the limit = %v", err)
	}
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); err != nil {
		t.Fatalf("request after the burst = %v", err)
	}
}

func TestDirectModeHonoursRetryAfterUpToAMinute(t *testing.T) {
	retryAfter := "30"
	client := newDirectFallbackClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", retryAfter)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"rate_limited"}`)
	})
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("error = %v", err)
	}
	now = now.Add(29 * time.Second)
	if client.FallbackAvailable() {
		t.Fatal("forwarding resumed before Retry-After")
	}
	now = now.Add(2 * time.Second)
	if !client.FallbackAvailable() {
		t.Fatal("forwarding did not resume after Retry-After")
	}
	retryAfter = "3600"
	_, _ = client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
	now = now.Add(MaxUpstreamRetryAfter + time.Second)
	if !client.FallbackAvailable() {
		t.Fatal("Retry-After was not capped at a minute")
	}
}

// TestDirectModeKeyNeverAppearsInErrors drives the failure paths that build
// error messages and checks none of them carries the key.
func TestDirectModeKeyNeverAppearsInErrors(t *testing.T) {
	status := http.StatusUnauthorized
	client := newDirectFallbackClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
	})
	var errs []error
	_, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
	errs = append(errs, err)
	_, err = client.Services(context.Background())
	errs = append(errs, err)
	status = http.StatusInternalServerError
	_, _, err = client.Snapshot(context.Background(), testServiceID1, "")
	errs = append(errs, err)
	unreachable, _ := NewClient(Config{BaseURL: "https://127.0.0.1:1", APIKey: testAPIKey, InstanceID: testDirectInstanceID,
		Retry: RetryPolicy{MaxAttempts: 1}})
	_, err = unreachable.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
	errs = append(errs, err)
	_, err = unreachable.Services(context.Background())
	errs = append(errs, err)
	for _, err := range errs {
		if err == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(err.Error(), "directSecretValue") {
			t.Fatalf("error carries the key: %v", err)
		}
	}
}
