// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	service.publish(t, Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
		Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	state := syncFixtureService(t, syncer, "books")

	cache, err := NewCache(directory)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	path := cache.path(service.ID)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("cache mode = %o, want 0600", info.Mode().Perm())
	}

	loaded, err := cache.Load(service.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned no state")
	}
	if loaded.ServiceID != state.ServiceID || loaded.SourceID != state.SourceID ||
		loaded.Epoch != state.Epoch || loaded.Revision != state.Revision ||
		loaded.StateHash != state.StateHash || loaded.CommitHash != state.CommitHash {
		t.Errorf("loaded identity does not match the saved state")
	}
	if got, want := StateHash(loaded.Snapshot), state.StateHash; got != want {
		t.Errorf("loaded state hash = %s, want %s", got, want)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read cache directory: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(path) {
			t.Errorf("unexpected file left in the cache directory: %s", entry.Name())
		}
	}
}

func TestCacheRejectsCorruption(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")
	cache, err := NewCache(directory)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	path := cache.path(testServiceID1)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}

	corrupted := bytes.Replace(data, []byte(`"name":"books"`), []byte(`"name":"booxs"`), 1)
	if bytes.Equal(corrupted, data) {
		t.Fatal("could not find the payload name to corrupt")
	}
	if err := os.WriteFile(path, corrupted, 0o600); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}
	if _, err := cache.Load(testServiceID1); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("Load error = %v, want ErrCacheCorrupt", err)
	}

	// A corruption that also recomputes the checksum is still caught by the
	// canonical state hash.
	var envelope cacheEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	payloadBytes := bytes.Replace(envelope.Payload, []byte(`"name":"books"`), []byte(`"name":"booxs"`), 1)
	updated := cacheEnvelope{
		Format:   envelope.Format,
		Checksum: hashOf(payloadBytes),
		Payload:  payloadBytes,
	}
	encoded, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("encode corrupt envelope: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}
	if _, err := cache.Load(testServiceID1); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("Load error = %v, want ErrCacheCorrupt", err)
	}
}

func TestCacheForgedIncrementalStateIsNotUsed(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")
	service.publish(t, Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
		Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	syncFixtureService(t, syncer, "books")

	cache, err := NewCache(directory)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	path := cache.path(service.ID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var envelope cacheEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var payload cachePayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.Snapshot.Policies) != 1 {
		t.Fatalf("cached policies = %d, want 1", len(payload.Snapshot.Policies))
	}

	// Recompute every unkeyed value needed to make the forged file a valid
	// cache envelope. Its new state hash cannot equal unchanged discovery, so
	// Sync must discard the copy and fetch the signed source snapshot.
	payload.Snapshot.Policies[0].SPDL = "grant write"
	payload.StateHash = StateHash(payload.Snapshot)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode forged payload: %v", err)
	}
	envelope.Payload = payloadBytes
	envelope.Checksum = hashOf(payloadBytes)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode forged envelope: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write forged cache: %v", err)
	}

	restarted := fixture.newSyncer(directory, nil)
	state := syncFixtureService(t, restarted, "books")
	if got := state.Snapshot.Policies[0].SPDL; got != "grant read" {
		t.Errorf("restarted policy SPDL = %q, want the signed source policy", got)
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 2 {
		t.Errorf("snapshots after forged cache = %d, want 2", count)
	}
}

func TestCacheRejectsUnsafePermissions(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")
	cache, err := NewCache(directory)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	path := cache.path(testServiceID1)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod cache: %v", err)
	}
	if _, err := cache.Load(testServiceID1); !errors.Is(err, ErrUnsafeCache) {
		t.Fatalf("Load error = %v, want ErrUnsafeCache", err)
	}

	// The Syncer treats an unusable cache as a miss and rewrites it 0600.
	restarted := fixture.newSyncer(directory, nil)
	syncFixtureService(t, restarted, "books")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cache: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("cache mode after rewrite = %o, want 0600", info.Mode().Perm())
	}
}

func TestCacheRejectsUnsafeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory permissions are not available on Windows")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o777); err != nil {
		t.Fatalf("chmod cache directory: %v", err)
	}
	if _, err := NewCache(directory); !errors.Is(err, ErrUnsafeCache) {
		t.Fatalf("NewCache error = %v, want ErrUnsafeCache", err)
	}
}

func TestCascadeRetainedCacheRejectsHashedServiceIDs(t *testing.T) {
	cache, err := NewCache(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	id := "service/with/slashes"
	if err := cache.Save(&ServiceState{ServiceID: id}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("Save accepted an ID requiring a hashed filename: %v", err)
	}
	// Such IDs are not supported by the protocol. A file manually placed at
	// their hashed path must fail startup, even if its contents are corrupt.
	if err := os.WriteFile(cache.path(id), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cache.validateRetained(); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("hashed retained filename bypassed validation: %v", err)
	}
}

func TestCacheInputValidation(t *testing.T) {
	if _, err := NewCache(""); err == nil {
		t.Error("NewCache accepted an empty directory")
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := NewCache(file); err == nil {
		t.Error("NewCache accepted a regular file")
	}

	directory := filepath.Join(t.TempDir(), "nested", "cache")
	cache, err := NewCache(directory)
	if err != nil {
		t.Fatalf("NewCache nested directory: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatalf("stat cache directory: %v", err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("cache directory mode = %o, want 0700", info.Mode().Perm())
		}
	}
	if err := cache.Save(nil); err == nil {
		t.Error("Save accepted a nil state")
	}
	if err := cache.Save(&ServiceState{}); !errors.Is(err, ErrProtocol) {
		t.Errorf("Save invalid state error = %v, want ErrProtocol", err)
	}
	if _, err := cache.Load("invalid"); !errors.Is(err, ErrCacheCorrupt) {
		t.Errorf("Load invalid id error = %v, want ErrCacheCorrupt", err)
	}
	if err := cache.Remove("invalid"); err == nil {
		t.Error("Remove accepted an invalid service id")
	}
	if name := cacheFileName("service/with/slashes"); strings.Contains(name, "/") {
		t.Errorf("hashed cache filename contains a slash: %q", name)
	}
}

func TestSyncCacheWriteFailureKeepsPreviousState(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")
	service.publish(t, Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
		Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})

	syncer.cache.dir = filepath.Join(directory, "missing", "cache")
	defer func() { syncer.cache.dir = directory }()
	state, err := syncer.Sync(context.Background(), discoveredService(service))
	if err == nil {
		t.Fatal("Sync succeeded with an unwritable cache")
	}
	if state != nil {
		t.Fatalf("failed Sync returned revision %d", state.Revision)
	}
	current, found := syncer.State(service.ID)
	if !found || current.Revision != 1 {
		t.Fatalf("state after cache failure = %+v, found %v; want revision 1", current, found)
	}

	syncer.cache.dir = directory
	updated, err := syncer.Sync(context.Background(), discoveredService(service))
	if err != nil {
		t.Fatalf("Sync retry: %v", err)
	}
	if updated.Revision != 2 {
		t.Errorf("retry revision = %d, want 2", updated.Revision)
	}
	cached, err := syncer.cache.Load(service.ID)
	if err != nil {
		t.Fatalf("Load after retry: %v", err)
	}
	if cached == nil || cached.Revision != 2 {
		t.Errorf("cache after retry = %+v, want revision 2", cached)
	}
	if count := fixture.requestCount("/changes"); count < 2 {
		t.Errorf("changes requests = %d, want a retry after the persistence failure", count)
	}
}

func TestForgetCacheFailureKeepsStateUntilRemoval(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")

	path := syncer.cache.path(service.ID)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove cache file: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("replace cache file with directory: %v", err)
	}
	child := filepath.Join(path, "child")
	if err := os.WriteFile(child, []byte("block removal"), 0o600); err != nil {
		t.Fatalf("write cache directory child: %v", err)
	}
	if err := syncer.Forget(service.ID); err == nil {
		t.Fatal("Forget succeeded despite the cache removal failure")
	}
	if _, found := syncer.State(service.ID); !found {
		t.Error("Forget discarded memory before the cache removal succeeded")
	}

	if err := os.Remove(child); err != nil {
		t.Fatalf("remove cache directory child: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove cache directory: %v", err)
	}
	if err := syncer.Forget(service.ID); err != nil {
		t.Fatalf("Forget retry: %v", err)
	}
	if _, found := syncer.State(service.ID); found {
		t.Error("state survived successful Forget retry")
	}
	cached, err := syncer.cache.Load(service.ID)
	if err != nil || cached != nil {
		t.Errorf("cache after Forget retry = %+v, %v", cached, err)
	}
}

func TestCacheStateRemainsTentativeAfterRefreshFailure(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")
	service.publish(t, Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
		Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	syncFixtureService(t, syncer, "books")

	restarted := fixture.newSyncer(directory, nil)
	service.publish(t, Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000202",
		Name: stringPointer("write-books"), SPDL: stringPointer("grant write")})
	fixture.source.configure(func(source *fixtureSource) {
		source.failRemaining = 1
		source.failStatus = 503
	})
	if _, err := restarted.Sync(context.Background(), discoveredService(service)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Sync refresh error = %v, want ErrUnavailable", err)
	}
	if _, found := restarted.State(service.ID); found {
		t.Fatal("failed refresh published a tentative disk-cache state")
	}

	updated, err := restarted.Sync(context.Background(), discoveredService(service))
	if err != nil {
		t.Fatalf("Sync retry: %v", err)
	}
	if updated.Revision != 3 {
		t.Errorf("retry revision = %d, want 3", updated.Revision)
	}
	published, found := restarted.State(service.ID)
	if !found || published.Revision != 3 || published.StateHash != service.StateHash {
		t.Errorf("published state after retry = %+v, found %v", published, found)
	}
}

func TestCacheRestartResumesFromCachedRevision(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	directory := t.TempDir()

	first := fixture.newSyncer(directory, nil)
	syncFixtureService(t, first, "books")
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 1 {
		t.Fatalf("initial snapshots = %d, want 1", count)
	}

	// A new process with the same cache directory reloads revision 1 and does
	// not fetch the snapshot again when discovery still reports the same head.
	second := fixture.newSyncer(directory, nil)
	state := syncFixtureService(t, second, "books")
	if state.Revision != 1 {
		t.Fatalf("restarted revision = %d, want 1", state.Revision)
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 1 {
		t.Errorf("snapshots after restart = %d, want 1", count)
	}

	// The cached revision is a real resume position: the next commit is
	// fetched as a change, not as a new snapshot.
	service.publish(t, Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
		Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	updated := syncFixtureService(t, second, "books")
	if updated.Revision != 2 || updated.StateHash != service.StateHash {
		t.Fatalf("resumed state = revision %d hash %s", updated.Revision, updated.StateHash)
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 1 {
		t.Errorf("snapshots after resume = %d, want 1", count)
	}
	if count := fixture.requestCount(testServiceID1 + "/changes"); count == 0 {
		t.Error("no changes request after resuming from the cache")
	}

	// A state that advanced through commits no longer carries a snapshot
	// signature; it must still reload from its checksum and state hash.
	third := fixture.newSyncer(directory, nil)
	reloaded := syncFixtureService(t, third, "books")
	if reloaded.Revision != 2 || reloaded.StateHash != service.StateHash {
		t.Fatalf("reloaded state = revision %d hash %s", reloaded.Revision, reloaded.StateHash)
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 1 {
		t.Errorf("snapshots after second restart = %d, want 1", count)
	}
}

func TestCacheRemove(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")
	cache, err := NewCache(directory)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if err := cache.Remove(testServiceID1); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	loaded, err := cache.Load(testServiceID1)
	if err != nil || loaded != nil {
		t.Fatalf("Load after Remove = %v, %v", loaded, err)
	}
	if err := cache.Remove(testServiceID1); err != nil {
		t.Fatalf("second Remove: %v", err)
	}
}

func TestForgetRemovesCachedFile(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")
	cache, err := NewCache(directory)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if err := syncer.Forget(testServiceID1); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, err := os.Stat(cache.path(testServiceID1)); !os.IsNotExist(err) {
		t.Errorf("cache file survived Forget: %v", err)
	}
	if _, found := syncer.State(testServiceID1); found {
		t.Error("in-memory state survived Forget")
	}
}
