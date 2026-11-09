// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"errors"
	"testing"
	"time"
)

type syncCallResult struct {
	state *ServiceState
	err   error
}

func startSync(syncer *Syncer, service Service) <-chan syncCallResult {
	result := make(chan syncCallResult, 1)
	go func() {
		state, err := syncer.Sync(context.Background(), service)
		result <- syncCallResult{state: state, err: err}
	}()
	return result
}

func waitSyncResult(t *testing.T, result <-chan syncCallResult) syncCallResult {
	t.Helper()
	select {
	case completed := <-result:
		return completed
	case <-time.After(2 * time.Second):
		t.Fatal("Sync did not complete")
		return syncCallResult{}
	}
}

func discoveredService(service *fixtureService) Service {
	return Service{
		ID:            service.ID,
		Name:          service.Name,
		Epoch:         service.Epoch,
		Revision:      service.Revision,
		HeadStateHash: service.StateHash,
	}
}

func signedFixtureSnapshot(service *fixtureService, signer *fixtureSigner) *SnapshotResponse {
	response := &SnapshotResponse{
		ServiceID:  service.ID,
		SourceID:   service.ID,
		Epoch:      service.Epoch,
		Revision:   service.Revision,
		StateHash:  service.StateHash,
		CommitHash: service.CommitHash,
		Snapshot:   cloneSnapshot(service.State),
	}
	response.Signature = signer.sign(SnapshotSignedBytes(
		response.SourceID, response.Epoch, response.Revision, response.StateHash, response.CommitHash,
	))
	return response
}

func TestSyncRejectsSnapshotRollback(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	oldSnapshot := signedFixtureSnapshot(service, fixture.signer)
	syncFixtureService(t, syncer, "books")

	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
			Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	updated := syncFixtureService(t, syncer, "books")
	if updated.Revision != 2 {
		t.Fatalf("updated revision = %d, want 2", updated.Revision)
	}

	fixture.source.configure(func(source *fixtureSource) {
		source.snapshotOverride = oldSnapshot
	})
	discovered := discoveredService(service)
	discovered.Revision = oldSnapshot.Revision
	discovered.HeadStateHash = oldSnapshot.StateHash
	state, err := syncer.Sync(context.Background(), discovered)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("Sync rollback error = %v, want ErrProtocol", err)
	}
	if state != nil {
		t.Fatalf("Sync rollback returned state at revision %d", state.Revision)
	}
	current, found := syncer.State(service.ID)
	if !found || current.Revision != 2 || current.StateHash != updated.StateHash {
		t.Fatalf("local state rolled back: %+v, found %v", current, found)
	}
}

func TestSyncSerializesConcurrentSyncs(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	started := make(chan struct{})
	release := make(chan struct{})
	fixture.source.configure(func(source *fixtureSource) {
		source.snapshotStarted = started
		source.snapshotRelease = release
	})

	first := startSync(syncer, discoveredService(service))
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("first snapshot did not start")
	}
	secondEntered := make(chan struct{})
	second := make(chan syncCallResult, 1)
	go func() {
		close(secondEntered)
		state, err := syncer.Sync(context.Background(), discoveredService(service))
		second <- syncCallResult{state: state, err: err}
	}()
	<-secondEntered
	time.Sleep(50 * time.Millisecond)
	if count := fixture.requestCount("/snapshot"); count != 1 {
		t.Errorf("concurrent snapshots = %d, want 1", count)
	}
	close(release)

	for index, result := range []<-chan syncCallResult{first, second} {
		completed := waitSyncResult(t, result)
		if completed.err != nil {
			t.Errorf("Sync %d: %v", index+1, completed.err)
		}
		if completed.state == nil || completed.state.Revision != 1 {
			t.Errorf("Sync %d state = %+v", index+1, completed.state)
		}
	}
}

func TestForgetWaitsForInFlightSync(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, nil)
	syncFixtureService(t, syncer, "books")

	started := make(chan struct{})
	release := make(chan struct{})
	fixture.source.configure(func(source *fixtureSource) {
		source.snapshotStarted = started
		source.snapshotRelease = release
	})
	discovered := discoveredService(service)
	discovered.HeadStateHash = flipHash(discovered.HeadStateHash)
	syncResult := startSync(syncer, discovered)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("snapshot did not start")
	}

	forgetResult := make(chan error, 1)
	go func() { forgetResult <- syncer.Forget(service.ID) }()
	forgotEarly := false
	select {
	case err := <-forgetResult:
		forgotEarly = true
		if err != nil {
			t.Errorf("Forget: %v", err)
		}
	case <-time.After(50 * time.Millisecond):
	}
	if forgotEarly {
		t.Error("Forget completed while Sync was still in flight")
	}
	close(release)
	if completed := waitSyncResult(t, syncResult); completed.err != nil {
		t.Errorf("Sync: %v", completed.err)
	}
	if !forgotEarly {
		select {
		case err := <-forgetResult:
			if err != nil {
				t.Errorf("Forget: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Forget did not complete")
		}
	}

	if _, found := syncer.State(service.ID); found {
		t.Error("in-flight Sync resurrected the forgotten in-memory state")
	}
	cache, err := NewCache(directory, fixture.keys)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	cached, err := cache.Load(service.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cached != nil {
		t.Error("in-flight Sync resurrected the forgotten cache file")
	}
}
