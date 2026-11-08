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
