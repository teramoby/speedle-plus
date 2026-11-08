// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
			Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	state := syncFixtureService(t, syncer, "books")

	cache, err := NewCache(directory, fixture.keys)
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
	cache, err := NewCache(directory, fixture.keys)
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

func TestCacheRejectsUnsafePermissions(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")
	cache, err := NewCache(directory, fixture.keys)
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
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
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
	cache, err := NewCache(directory, fixture.keys)
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
	cache, err := NewCache(directory, fixture.keys)
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
