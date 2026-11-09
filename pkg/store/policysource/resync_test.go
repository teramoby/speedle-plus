// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

// A resync command that arrives while a pass is already fetching the service
// must not be completed by that fetch. Only a fetch that begins after the
// command can make the service servable again.
func TestResyncIsNotCompletedByAFetchAlreadyInFlight(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)
	if status, _ := store.Status("books"); status.Service.State != adsync.SyncStateInSync {
		t.Fatalf("books = %s before the resync", status.Service.State)
	}

	// The next pass fetches a new revision. The command arrives after the
	// fetch completed and before the pass stores its result.
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{policy("p-carol", "carol", "grant user carol read books")}
	})
	snapshotPath := "/services/" + testServiceID1 + "/snapshot"
	before := fixture.source.requestsForSuffix(snapshotPath)
	fetched := make(chan struct{})
	resumed := make(chan struct{})
	var once sync.Once
	store.afterFetch = func(string) {
		once.Do(func() {
			close(fetched)
			<-resumed
		})
	}

	syncDone := make(chan error, 1)
	go func() { syncDone <- store.Sync(nil) }()
	select {
	case <-fetched:
	case err := <-syncDone:
		t.Fatalf("Sync finished without fetching: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the in-flight fetch")
	}
	store.resync(testServiceID1)
	close(resumed)
	if err := <-syncDone; err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// The channel confirms the head and the evaluator installs whatever is
	// published. The fetch began before the resync, so books stays syncing.
	confirmStoreGeneration(t, store)
	status, _ := store.Status("books")
	if status.Service.State != adsync.SyncStateSyncing || status.Service.Reason != adsync.SyncReasonResync {
		t.Fatalf("books = %s (%s) after a fetch that began before the resync", status.Service.State, status.Service.Reason)
	}

	// The next pass fetches a fresh copy and completes the resync.
	if err := store.Sync(nil); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	confirmStoreGeneration(t, store)
	head, _ := fixture.source.service("books")
	status, _ = store.Status("books")
	if status.Service.State != adsync.SyncStateInSync || status.Service.Revision != head.Revision {
		t.Fatalf("books = %s at revision %d after a fresh fetch, want in_sync at %d",
			status.Service.State, status.Service.Revision, head.Revision)
	}
	if got := fixture.source.requestsForSuffix(snapshotPath) - before; got != 2 {
		t.Fatalf("%d snapshot fetches, want the in-flight one and a fresh one", got)
	}
}

// A service whose state is reset while its content does not change returns
// to in_sync on the next pass, without waiting for a content change that
// would trigger another evaluator reload.
func TestUnchangedReaddedServiceReturnsToInSync(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)

	discovery, err := store.client.Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The channel removes and re-adds books between two passes; the local copy
	// and the installed assembly are unchanged.
	if err := store.machine.Ack(adsync.Ack{Services: []adsync.Service{}}); err != nil {
		t.Fatal(err)
	}
	if err := store.machine.Ack(adsync.Ack{Services: discovery.Services}); err != nil {
		t.Fatal(err)
	}
	if status, _ := store.Status("books"); status.Service.State != adsync.SyncStateSyncing {
		t.Fatalf("books = %s after a re-add", status.Service.State)
	}

	if err := store.Sync(nil); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if status, _ := store.Status("books"); status.Service.State != adsync.SyncStateInSync {
		t.Fatalf("books = %s (%s) after re-verifying the installed content", status.Service.State, status.Service.Reason)
	}
}
