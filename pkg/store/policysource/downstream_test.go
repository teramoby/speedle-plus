// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/adsproxy"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func TestServeDownstreamMirrorsVerifiedCopies(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) {
		options.ServeDownstream = true
	})
	defer store.Close()

	if store.DownstreamHandler() == nil {
		t.Fatal("ServeDownstream did not build a downstream handler")
	}
	if count := store.DownstreamCount(); count != 0 {
		t.Errorf("DownstreamCount = %d, want 0", count)
	}
	if count := store.downstreamSessionCount(); count != 0 {
		t.Errorf("heartbeat count = %d, want 0", count)
	}
	verified, found := store.syncer.State(testServiceID1)
	if !found {
		t.Fatal("the store has no verified state")
	}
	info, availability := store.downstream.mirror.Lookup(testServiceID1)
	if availability != adsproxy.ServiceAvailable || info.Revision != verified.Revision ||
		info.StateHash != verified.StateHash {
		t.Fatalf("mirrored copy = %+v %v", info, availability)
	}

	// A later source revision reaches the mirror through the syncer observer.
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = append(snapshot.Policies, policy("p-audit", "audit", "grant user auditor read books"))
	})
	if err := store.Sync(context.Background()); err != nil {
		t.Fatalf("sync after source change: %v", err)
	}
	advanced, found := store.syncer.State(testServiceID1)
	if !found || advanced.Revision != verified.Revision+1 {
		t.Fatalf("store did not advance: %+v %v", advanced, found)
	}
	if info, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceAvailable ||
		info.Revision != advanced.Revision || info.StateHash != advanced.StateHash {
		t.Fatalf("mirror did not follow the source change: %+v %v", info, availability)
	}
	verified = advanced

	// A verified change page reaches the mirror with its log intact, so
	// downstream instances can resume incrementally.
	name, spdl := "audit", "grant user auditor read orders"
	changes := []adsync.Change{{Op: "upsert", Kind: "authz", ID: "p-audit-2", Name: &name, SPDL: &spdl}}
	commitHash, err := adsync.CommitHash(verified.CommitHash, changes)
	if err != nil {
		t.Fatalf("commit hash: %v", err)
	}
	next := adsync.Snapshot{
		Name:         verified.Snapshot.Name,
		Policies:     append([]adsync.Policy(nil), verified.Snapshot.Policies...),
		RolePolicies: append([]adsync.Policy(nil), verified.Snapshot.RolePolicies...),
	}
	if err := adsync.ApplyChanges(&next, changes); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	store.observeDownstreamChanges(&adsync.ChangesResponse{
		ServiceID:     testServiceID1,
		SourceID:      verified.SourceID,
		Epoch:         verified.Epoch,
		From:          verified.Revision + 1,
		To:            verified.Revision + 1,
		Head:          verified.Revision + 1,
		HeadStateHash: adsync.StateHash(next),
		Commits: []adsync.Commit{{
			Revision: verified.Revision + 1,
			PrevHash: verified.CommitHash,
			Hash:     commitHash,
			At:       1762500000000,
			Changes:  changes,
		}},
		NextSince: verified.Revision + 2,
	})
	page, err := store.downstream.mirror.Changes(testServiceID1, verified.Revision, 10)
	if err != nil {
		t.Fatalf("mirror changes: %v", err)
	}
	if page.To != verified.Revision+1 || len(page.Commits) != 1 {
		t.Fatalf("mirror page = to %d, commits %d", page.To, len(page.Commits))
	}

	// A copy that is no longer confirmed at its advertised head is withheld
	// from downstream instances.
	store.withholdDownstream(testServiceID1)
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceNeedsSnapshot {
		t.Errorf("withheld availability = %v", availability)
	}
	store.publishDownstream(verified)
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceAvailable {
		t.Errorf("recovered availability = %v", availability)
	}

	// Removing a service stops serving it and keeps the deletion guard.
	store.removeDownstream(testServiceID1)
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceRemoved {
		t.Errorf("removed availability = %v", availability)
	}
}
