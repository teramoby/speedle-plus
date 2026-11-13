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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstanceIDIsAPersistedUUIDv7(t *testing.T) {
	id, err := NewInstanceID()
	if err != nil || !ValidInstanceID(id) || validULID(id) {
		t.Fatalf("NewInstanceID = %q, %v", id, err)
	}
	if ValidInstanceID(strings.ToUpper(id)) || ValidInstanceID(testServiceID1) {
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

func TestNewClientValidatesKeyAndInstanceID(t *testing.T) {
	f := newFixture(t)
	cases := map[string]func(*Config){
		"no key":           func(config *Config) { config.APIKey = "" },
		"malformed key":    func(config *Config) { config.APIKey = "bad key " + testAPIKey },
		"ULID instance id": func(config *Config) { config.InstanceID = testServiceID1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewClient(f.config(mutate))
			if err == nil {
				t.Fatal("configuration accepted")
			}
			if strings.Contains(err.Error(), "fixtureSecretValue") {
				t.Fatal("error echoes the key")
			}
		})
	}
	client := f.newClient(nil)
	if id, err := client.InstanceID(); err != nil || id != testInstanceID {
		t.Fatalf("InstanceID = %q, %v", id, err)
	}
}

// TestInstanceIDIsCreatedOnFirstUseAndPersisted checks that a client creates
// no identity until it first needs one, persists it in the cache directory
// before it is sent, and reuses it on the next start.
func TestInstanceIDIsCreatedOnFirstUseAndPersisted(t *testing.T) {
	f := newFixture(t, newFixtureService(testServiceID1, "books"))
	dir := filepath.Join(t.TempDir(), "cache")
	persisted := func(config *Config) { config.InstanceID = ""; config.InstanceIDDir = dir }
	client := f.newClient(persisted)
	if _, err := os.Stat(filepath.Join(dir, InstanceIDFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("instance id created before first use: %v", err)
	}
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, InstanceIDFile))
	if err != nil {
		t.Fatalf("instance id not persisted: %v", err)
	}
	id := strings.TrimSpace(string(data))
	if !ValidInstanceID(id) {
		t.Fatalf("persisted id %q", id)
	}
	if sent := f.lastRequest().Instance; sent != id {
		t.Fatalf("sent instance %q, persisted %q", sent, id)
	}
	restarted := f.newClient(persisted)
	if again, err := restarted.InstanceID(); err != nil || again != id {
		t.Fatalf("restart id = %q, %v; want %q", again, err, id)
	}

	// Without a cache directory every client generates a fresh identity.
	ephemeral := func(config *Config) { config.InstanceID = "" }
	first, err := f.newClient(ephemeral).InstanceID()
	if err != nil || !ValidInstanceID(first) {
		t.Fatalf("ephemeral id = %q, %v", first, err)
	}
	if second, _ := f.newClient(ephemeral).InstanceID(); second == first {
		t.Fatal("two starts without a cache directory share an id")
	}

	// An unusable directory fails the request; nothing is sent without an
	// identity.
	broken := f.newClient(func(config *Config) { config.InstanceID = ""; config.InstanceIDDir = unusableDir(t) })
	before := f.requestTotal()
	if _, err := broken.Services(context.Background()); err == nil {
		t.Fatal("request sent without a persisted identity")
	}
	if f.requestTotal() != before {
		t.Fatal("a request was sent without an identity")
	}
}

func unusableDir(t *testing.T) string {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(blocker, "sub")
}

// TestKeyAndInstanceHeaderOnEveryRequest syncs through the fixture: every
// request carries the key and the instance header.
func TestKeyAndInstanceHeaderOnEveryRequest(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	f := newFixture(t, service)
	f.source.configure(func(*fixtureSource) {
		service.publish(t, Change{Op: "upsert", Kind: "authz", ID: "p1", Name: stringPointer("p1"), SPDL: stringPointer("grant user a read r")})
	})
	syncer := f.newSyncer(t.TempDir(), nil)
	if state := syncFixtureService(t, syncer, "books"); state.Revision != 2 {
		t.Fatalf("state = revision %d", state.Revision)
	}
	f.source.mu.Lock()
	defer f.source.mu.Unlock()
	for _, request := range f.source.requests {
		if request.Authorization != "Bearer "+testAPIKey || request.Instance != testInstanceID {
			t.Fatalf("request %s without the API key or instance header %q", request.Path, request.Instance)
		}
	}
}

// TestResponsesWithoutSignatureFields decodes snapshot and
// changes bodies that carry no signature fields at all.
func TestResponsesWithoutSignatureFields(t *testing.T) {
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
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: testAPIKey, InstanceID: testInstanceID, HTTPClient: server.Client()})
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

func TestChannelMessagesCarryIdentityAndHost(t *testing.T) {
	f := newFixture(t)
	machine, _ := newTestMachine(t)
	client := f.newClient(nil)
	ips := []string{"not-an-ip", "2001:db8::1"}
	for index := range 20 {
		ips = append(ips, fmt.Sprintf("10.0.0.%d", index))
	}
	host := HostInfo{Hostname: "edge-1", OS: "linux", Arch: "amd64", IPs: ips, GoVersion: "go1", PID: 7, Container: true, StartedAt: "2026-11-13T07:00:00Z"}
	channel, err := NewChannel(client, ChannelOptions{
		Machine: machine, AgentVersion: "test/1",
		HostInfo: func(time.Time) HostInfo { return host },
	})
	if err != nil {
		t.Fatal(err)
	}
	channel.instanceID, _ = client.InstanceID()
	decode := func(value any) map[string]any {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		_ = json.Unmarshal(encoded, &decoded)
		return decoded
	}
	hello := decode(channel.helloMessage(TransportWebSocket))
	poll := decode(channel.heartbeatMessage(TransportLongPoll))
	for _, message := range []map[string]any{hello, poll} {
		if message["instanceId"] != testInstanceID || message["agentVersion"] != "test/1" {
			t.Fatalf("message = %v", message)
		}
		if _, found := message["credentialId"]; found {
			t.Fatal("credential id sent")
		}
		sent, ok := message["host"].(map[string]any)
		if !ok || sent["hostname"] != "edge-1" || sent["os"] != "linux" || sent["container"] != true {
			t.Fatalf("host = %v", message["host"])
		}
		if ips, _ := sent["ips"].([]any); len(ips) != maxHostIPs {
			t.Fatalf("ips not bounded: %d", len(ips))
		}
	}
	// The hello keeps the start time and transport inside the host block and
	// names no chain; the heartbeat has no chain or top-level start time but
	// keeps its transport.
	for _, key := range []string{"chain", "startedAt", "transport"} {
		if _, found := hello[key]; found {
			t.Fatalf("hello carries a top-level %s: %v", key, hello)
		}
	}
	for _, key := range []string{"chain", "startedAt"} {
		if _, found := poll[key]; found {
			t.Fatalf("heartbeat carries a top-level %s: %v", key, poll)
		}
	}
	if poll["transport"] != string(TransportLongPoll) {
		t.Fatalf("heartbeat transport = %v", poll["transport"])
	}
	for name, message := range map[string]map[string]any{"hello": hello, "heartbeat": poll} {
		if started, _ := message["host"].(map[string]any)["startedAt"].(string); started == "" {
			t.Fatalf("%s host startedAt = %v", name, message["host"])
		} else if _, err := time.Parse(time.RFC3339, started); err != nil {
			t.Fatalf("%s host startedAt %q: %v", name, started, err)
		}
	}
	if hello["host"].(map[string]any)["transport"] != string(TransportWebSocket) ||
		poll["host"].(map[string]any)["transport"] != string(TransportLongPoll) {
		t.Fatalf("host transport = %v / %v", hello["host"], poll["host"])
	}
	// A WebSocket heartbeat follows a hello and carries no host object.
	if _, found := decode(channel.heartbeatMessage(TransportWebSocket))["host"]; found {
		t.Fatal("WebSocket heartbeat carries the host object")
	}
	header, err := channel.requestHeaders()
	if err != nil || header.Get("Authorization") != "Bearer "+testAPIKey || header.Get(InstanceHeader) != testInstanceID {
		t.Fatalf("channel headers = %v, %v", header, err)
	}
	if err := channel.terminalStatusError(http.MethodPost, "/poll", http.StatusConflict, []byte(`{"error":"quota_exceeded","limit":10}`)); err != nil {
		t.Fatalf("quota_exceeded is terminal: %v", err)
	}
	// A long poll before any collection sends an empty host object.
	fresh, err := NewChannel(client, ChannelOptions{Machine: machine, AgentVersion: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if sent, ok := decode(fresh.heartbeatMessage(TransportLongPoll))["host"].(map[string]any); !ok || sent["transport"] != string(TransportLongPoll) {
		t.Fatalf("host before collection = %v", sent)
	}
}

// TestChannelRunFailsWithoutAnIdentity stops the machine when the instance ID
// cannot be persisted, before anything is sent.
func TestChannelRunFailsWithoutAnIdentity(t *testing.T) {
	f := newFixture(t)
	client := f.newClient(func(config *Config) { config.InstanceID = ""; config.InstanceIDDir = unusableDir(t) })
	machine, _ := newTestMachine(t)
	channel, err := NewChannel(client, ChannelOptions{Machine: machine, AgentVersion: "x", Transport: TransportLongPoll})
	if err != nil {
		t.Fatal(err)
	}
	if err := channel.Run(context.Background()); !isTerminalError(err) {
		t.Fatalf("Run = %v, want terminal", err)
	}
	if machine.Instance().Liveness != LivenessStopped {
		t.Fatalf("instance = %+v", machine.Instance())
	}
	if f.requestTotal() != 0 {
		t.Fatal("a request was sent without an identity")
	}
}

func TestHostInfoBounds(t *testing.T) {
	host := HostInfo{
		Hostname:  strings.Repeat("h", maxHostnameBytes+1),
		OS:        strings.Repeat("o", maxHostPlatformBytes+1),
		Arch:      "amd64",
		OSVersion: "bad\nversion",
		GoVersion: strings.Repeat("g", maxHostVersionBytes),
		Transport: strings.Repeat("t", maxHostVersionBytes+1),
		IPs:       []string{"::1", "999.0.0.1", "192.0.2.1"},
		PID:       -1,
		StartedAt: "yesterday",
	}.bounded()
	if host.Hostname != "" || host.OS != "" || host.OSVersion != "" || host.Transport != "" || host.PID != 0 || host.StartedAt != "" {
		t.Fatalf("oversized fields kept: %+v", host)
	}
	if host.Arch != "amd64" || len(host.GoVersion) != maxHostVersionBytes {
		t.Fatalf("valid fields dropped: %+v", host)
	}
	if len(host.IPs) != 2 || host.IPs[0] != "::1" || host.IPs[1] != "192.0.2.1" {
		t.Fatalf("ips = %v", host.IPs)
	}
}

func TestCollectHostInfo(t *testing.T) {
	started := time.UnixMilli(1762500000000)
	host := CollectHostInfo(started)
	if host.OS != runtime.GOOS || host.Arch != runtime.GOARCH || host.GoVersion != runtime.Version() ||
		host.PID != os.Getpid() || host.StartedAt != started.UTC().Format(time.RFC3339) || host.IPs == nil || len(host.IPs) > maxHostIPs {
		t.Fatalf("host = %+v", host)
	}
	for _, ip := range host.IPs {
		if ip == "127.0.0.1" || ip == "::1" {
			t.Fatalf("loopback address reported: %v", host.IPs)
		}
	}
}

func TestChannelReportsEveryAck(t *testing.T) {
	f := newFixture(t)
	machine, _ := newTestMachine(t)
	var acks atomic.Int32
	channel, err := NewChannel(f.newClient(nil), ChannelOptions{
		Machine: machine, AgentVersion: "test",
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
	_ = machine.Ack(ackWith(testAckService(testServiceID3, "orders", 1)))
	if err := machine.Track("01HF7YAT000000000000000999", "late"); err != nil {
		t.Fatal(err)
	}
	if _, known := machine.Status("late"); known {
		t.Fatal("Track after the first ack created a record")
	}
}

func newForwardingClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(Config{
		BaseURL: server.URL, APIKey: testAPIKey, InstanceID: testInstanceID, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestUpstreamForwardsToTheUpstream(t *testing.T) {
	var gotAuth, gotInstance, gotPath string
	client := newForwardingClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotInstance, gotPath = r.Header.Get("Authorization"), r.Header.Get(InstanceHeader), r.URL.Path
		_, _ = w.Write([]byte(`{"allowed":true}`))
	})
	if !client.FallbackAvailable() {
		t.Fatal("no forwarding")
	}
	response, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.DecisionSource != DecisionSourceUpstream || gotAuth != "Bearer "+testAPIKey ||
		gotInstance != testInstanceID || gotPath != AuthzCheckAPIPrefix+"is-allowed" {
		t.Fatalf("forwarded key %v instance %q path %q, source %q", gotAuth == "Bearer "+testAPIKey, gotInstance, gotPath, response.DecisionSource)
	}
}

func TestUpstreamForwardingLimitsInFlightRequests(t *testing.T) {
	release := make(chan struct{})
	arrived := make(chan struct{}, 2*MaxUpstreamInFlight)
	client := newForwardingClient(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestUpstreamHonoursRetryAfterUpToAMinute(t *testing.T) {
	retryAfter := "30"
	client := newForwardingClient(t, func(w http.ResponseWriter, r *http.Request) {
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

// TestUpstreamHonoursDailyQuotaRetryAfterUntilMidnight honours the full
// Retry-After of an exhausted daily quota, bounded by the next UTC midnight.
func TestUpstreamHonoursDailyQuotaRetryAfterUntilMidnight(t *testing.T) {
	retryAfter := "7200"
	client := newForwardingClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", retryAfter)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"daily_quota_exceeded"}`)
	})
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); !errors.Is(err, ErrFallbackQuotaExhausted) {
		t.Fatalf("error = %v", err)
	}
	now = now.Add(119 * time.Minute)
	if client.FallbackAvailable() {
		t.Fatal("forwarding resumed before the daily quota Retry-After")
	}
	now = now.Add(2 * time.Minute)
	if !client.FallbackAvailable() {
		t.Fatal("forwarding did not resume after the daily quota Retry-After")
	}
	// A Retry-After past midnight is bounded by the quota reset.
	now = time.Date(2026, 11, 13, 23, 0, 0, 0, time.UTC)
	retryAfter = "86400"
	_, _ = client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
	now = now.Add(59 * time.Minute)
	if client.FallbackAvailable() {
		t.Fatal("forwarding resumed before midnight")
	}
	now = now.Add(2 * time.Minute)
	if !client.FallbackAvailable() {
		t.Fatal("daily quota Retry-After was not bounded by the next UTC midnight")
	}
}

// TestUpstreamKeyNeverAppearsInErrors drives the failure paths that build
// error messages and checks none of them carries the key.
func TestUpstreamKeyNeverAppearsInErrors(t *testing.T) {
	status := http.StatusUnauthorized
	client := newForwardingClient(t, func(w http.ResponseWriter, r *http.Request) {
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
	unreachable, _ := NewClient(Config{BaseURL: "https://127.0.0.1:1", APIKey: testAPIKey, InstanceID: testInstanceID,
		Retry: RetryPolicy{MaxAttempts: 1}})
	_, err = unreachable.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
	errs = append(errs, err)
	_, err = unreachable.Services(context.Background())
	errs = append(errs, err)
	for _, err := range errs {
		if err == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(err.Error(), "fixtureSecretValue") {
			t.Fatalf("error carries the key: %v", err)
		}
	}
}

// TestUpstreamRetryDeadlineSurvivesOutOfOrderResponses runs two forwarded
// requests concurrently. The one started first completes last; neither its
// success nor its shorter Retry-After may shorten or clear the deadline the
// other response set.
func TestUpstreamRetryDeadlineSurvivesOutOfOrderResponses(t *testing.T) {
	for name, lateResponse := range map[string]func(http.ResponseWriter){
		"success": func(w http.ResponseWriter) {
			_, _ = io.WriteString(w, `{"allowed":true,"reason":0}`)
		},
		"shorter Retry-After": func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"rate_limited"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			arrived := make(chan struct{})
			release := make(chan struct{})
			client := newForwardingClient(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if string(body) == `{"slow":true}` {
					close(arrived)
					<-release
					lateResponse(w)
					return
				}
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, `{"error":"rate_limited"}`)
			})
			start := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
			var clock atomic.Int64
			clock.Store(start.UnixNano())
			client.now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }

			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{"slow":true}`))
			}()
			<-arrived
			if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); !errors.Is(err, ErrRateLimited) {
				t.Fatalf("fast request error = %v", err)
			}
			close(release)
			<-done

			clock.Store(start.Add(10 * time.Second).UnixNano())
			if client.FallbackAvailable() {
				t.Fatal("a late response shortened or cleared the retry deadline")
			}
			clock.Store(start.Add(31 * time.Second).UnixNano())
			if !client.FallbackAvailable() {
				t.Fatal("forwarding did not resume after the latest deadline")
			}
		})
	}
}
