// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSyncAcceptsLargeProtocolSnapshotAndCache(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	spdl := strings.Repeat("<", 128) + strings.Repeat("s", maxSPDLBytes-128)
	policies := make([]Policy, maxPoliciesPerList)
	rolePolicies := make([]Policy, maxPoliciesPerList)
	for index := 0; index < maxPoliciesPerList; index++ {
		policies[index] = Policy{
			ID: fmt.Sprintf("policy-%04d", index), Name: "read-books", SPDL: spdl,
		}
		rolePolicies[index] = Policy{
			ID: fmt.Sprintf("role-%04d", index), Name: "reader", SPDL: spdl,
		}
	}
	large := Snapshot{Name: "books", Policies: policies, RolePolicies: rolePolicies}
	encoded, err := json.Marshal(large)
	if err != nil {
		t.Fatalf("encode large snapshot: %v", err)
	}
	if len(encoded) <= 6<<20 {
		t.Fatalf("large snapshot is only %d bytes; test must exceed the old transport cap", len(encoded))
	}
	fixture.source.configure(func(source *fixtureSource) {
		current := source.services[service.ID]
		current.State = large
		current.StateHash = StateHash(large)
		current.CommitHash = current.StateHash
	})

	directory := t.TempDir()
	syncer := fixture.newSyncer(directory, func(config *Config) {
		config.RequestTimeout = 10 * time.Second
		config.SnapshotTimeout = 10 * time.Second
	})
	state := syncFixtureService(t, syncer, "books")
	if len(state.Snapshot.Policies) != maxPoliciesPerList || len(state.Snapshot.RolePolicies) != maxPoliciesPerList {
		t.Fatalf("large snapshot policy counts = %d/%d", len(state.Snapshot.Policies), len(state.Snapshot.RolePolicies))
	}
	cache, err := NewCache(directory, fixture.keys)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	info, err := os.Stat(cache.path(service.ID))
	if err != nil {
		t.Fatalf("stat large cache: %v", err)
	}
	if info.Size() <= 8<<20 {
		t.Fatalf("large cache is only %d bytes; test must exceed the old cache cap", info.Size())
	}

	restarted := fixture.newSyncer(directory, nil)
	reloaded := syncFixtureService(t, restarted, "books")
	if reloaded.StateHash != state.StateHash {
		t.Error("large cache did not survive restart")
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 1 {
		t.Errorf("snapshots after large-cache restart = %d, want 1", count)
	}
}

func TestChangesAcceptsLargeProtocolPage(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	client := fixture.newClient(func(config *Config) {
		config.RequestTimeout = 10 * time.Second
	})
	snapshot, _, err := client.Snapshot(context.Background(), service.ID, "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	spdl := strings.Repeat("s", maxSPDLBytes)
	for commitIndex := 0; commitIndex < 2; commitIndex++ {
		changes := make([]Change, maxChangesPerCommit)
		for index := range changes {
			changes[index] = Change{
				Op:   "upsert",
				Kind: "authz",
				ID:   fmt.Sprintf("policy-%d-%03d", commitIndex, index),
				Name: stringPointer("read-books"),
				SPDL: &spdl,
			}
		}
		service.publish(t, fixture.signer, changes...)
	}

	page, err := client.Changes(context.Background(), service.ID, snapshot.SourceID, snapshot.Epoch,
		snapshot.Revision, snapshot.CommitHash, maxCommitsPerPage)
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(page.Commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(page.Commits))
	}
	for _, commit := range page.Commits {
		if len(commit.Changes) != maxChangesPerCommit {
			t.Errorf("commit %d changes = %d, want %d", commit.Revision, len(commit.Changes), maxChangesPerCommit)
		}
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("encode large page: %v", err)
	}
	if len(encoded) <= 1<<20 {
		t.Errorf("large changes page is only %d bytes; test must exceed the old transport cap", len(encoded))
	}
}
