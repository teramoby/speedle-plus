// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"testing"
	"time"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
)

func readRequest(user string) adsapi.RequestContext {
	return adsapi.RequestContext{
		Subject:     &adsapi.Subject{Principals: []*adsapi.Principal{{Type: adsapi.PRINCIPAL_TYPE_USER, Name: user}}},
		ServiceName: "books",
		Resource:    "books",
		Action:      "read",
	}
}

// The evaluator refuses to run this store without watching, and a
// synchronization pass then reaches requests through the FULL_RELOAD event.
func TestEvaluatorInstallPrecedesInSync(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store, err := Open(nil, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, store)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := engine.(*eval.PolicyEvalImpl)
	t.Cleanup(func() { _ = evaluator.Close() })
	confirmStoreGeneration(t, store)

	alice := readRequest("alice")
	carol := readRequest("carol")
	if allowed, _, err := evaluator.IsAllowed(alice); !allowed || err != nil {
		t.Fatalf("initial alice decision: %v %v", allowed, err)
	}

	fixture.source.holdSnapshots(testServiceID2)
	defer fixture.source.releaseSnapshots(testServiceID2)
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{policy("p-carol", "carol", "grant user carol read books")}
	})
	fixture.source.publishService("orders", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{policy("p-dave", "dave", "grant user dave read orders")}
	})

	snapshotsBefore := fixture.source.requestsForSuffix("/services/" + testServiceID2 + "/snapshot")
	syncDone := make(chan error, 1)
	go func() { syncDone <- store.Sync(nil) }()
	deadline := time.Now().Add(3 * time.Second)
	for fixture.source.requestsForSuffix("/services/"+testServiceID2+"/snapshot") <= snapshotsBefore {
		if time.Now().After(deadline) {
			t.Fatal("the orders snapshot was not held")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// A channel ack can confirm both new heads while the second service's
	// snapshot is still in flight. Books must nevertheless remain syncing until
	// its new assembly is installed by the evaluator.
	confirmStoreGeneration(t, store)
	if status, _ := store.Status("books"); status.Service.State != adsync.SyncStateSyncing {
		t.Fatalf("books became %s before evaluator installation", status.Service.State)
	}
	if allowed, _, err := evaluator.IsAllowed(alice); !allowed || err != nil {
		t.Fatalf("the evaluator lost the old installed generation: %v %v", allowed, err)
	}
	if allowed, _, err := evaluator.IsAllowed(carol); allowed || err != nil {
		t.Fatalf("the evaluator saw the unpublished generation: %v %v", allowed, err)
	}

	fixture.source.releaseSnapshots(testServiceID2)
	if err := <-syncDone; err != nil {
		t.Fatalf("Sync: %v", err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		allowed, _, err := evaluator.IsAllowed(carol)
		status, _ := store.Status("books")
		if err == nil && allowed && status.Service.State == adsync.SyncStateInSync {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("installed decision/state did not converge: allowed=%v err=%v status=%+v", allowed, err, status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEvaluatorReloadsThroughTheStoreEvents(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)
	engine, err := eval.NewWithStore(&cfg.Config{EnableWatch: false}, store)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, ok := engine.(*eval.PolicyEvalImpl)
	if !ok {
		t.Fatalf("evaluator type %T", engine)
	}
	t.Cleanup(func() { _ = evaluator.Close() })

	alice := readRequest("alice")
	carol := readRequest("carol")
	if allowed, _, err := evaluator.IsAllowed(alice); !allowed || err != nil {
		t.Fatalf("initial alice decision: %v %v", allowed, err)
	}
	if allowed, _, err := evaluator.IsAllowed(carol); allowed || err != nil {
		t.Fatalf("initial carol decision: %v %v", allowed, err)
	}

	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-read-books", "read-books", "grant user carol read books"),
		}
	})
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		aliceAllowed, _, aliceErr := evaluator.IsAllowed(alice)
		carolAllowed, _, carolErr := evaluator.IsAllowed(carol)
		if aliceErr == nil && carolErr == nil && !aliceAllowed && carolAllowed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the evaluator did not reload: alice=%v/%v carol=%v/%v",
				aliceAllowed, aliceErr, carolAllowed, carolErr)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
