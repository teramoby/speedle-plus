// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSyncInitialSnapshotForSeveralServices(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	orders := newFixtureService(testServiceID2, "orders")
	fixture := newFixture(t, books, orders)
	syncer := fixture.newSyncer("", nil)

	for _, service := range []*fixtureService{books, orders} {
		state := syncFixtureService(t, syncer, service.Name)
		if state.ServiceID != service.ID || state.SourceID != service.ID || state.Epoch != service.Epoch {
			t.Errorf("state identity = %s/%s/%s", state.ServiceID, state.SourceID, state.Epoch)
		}
		if state.Revision != service.Revision {
			t.Errorf("state revision = %d, want %d", state.Revision, service.Revision)
		}
		if state.StateHash != service.StateHash || state.CommitHash != service.CommitHash {
			t.Errorf("state hashes do not match the source")
		}
		if state.Snapshot.Name != service.Name {
			t.Errorf("snapshot name = %q, want %q", state.Snapshot.Name, service.Name)
		}
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 1 {
		t.Errorf("first service snapshots = %d, want 1", count)
	}
	if count := fixture.requestCount(testServiceID2 + "/snapshot"); count != 1 {
		t.Errorf("second service snapshots = %d, want 1", count)
	}
}

func TestSyncAppliesPagedChanges(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	fixture.source.configure(func(source *fixtureSource) { source.pageSize = 2 })
	syncer := fixture.newSyncer("", nil)

	first := syncFixtureService(t, syncer, "books")
	if first.Revision != 1 {
		t.Fatalf("initial revision = %d", first.Revision)
	}

	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
			Name: stringPointer("read-books"), SPDL: stringPointer(`grant read on "/books/*"`)})
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "role", ID: "01HF7YAT000000000000000301",
			Name: stringPointer("reader"), SPDL: stringPointer(`grant role "reader" to read`)})
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "service", ID: service.ID, Name: stringPointer("books-v2")})
	service.publish(t, fixture.signer,
		Change{Op: "delete", Kind: "authz", ID: "01HF7YAT000000000000000201"})
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000202",
			Name: stringPointer("read-orders"), SPDL: stringPointer(`grant read on "/orders/*"`)})

	state := syncFixtureService(t, syncer, "books-v2")
	if state.Revision != service.Revision || state.Revision != 6 {
		t.Errorf("revision = %d, want 6", state.Revision)
	}
	if state.StateHash != service.StateHash || state.CommitHash != service.CommitHash {
		t.Errorf("state hashes do not match the fixture head")
	}
	if state.Snapshot.Name != "books-v2" {
		t.Errorf("service name = %q, want books-v2", state.Snapshot.Name)
	}
	if len(state.Snapshot.Policies) != 1 || state.Snapshot.Policies[0].ID != "01HF7YAT000000000000000202" {
		t.Errorf("policies = %+v", state.Snapshot.Policies)
	}
	if len(state.Snapshot.RolePolicies) != 1 || state.Snapshot.RolePolicies[0].ID != "01HF7YAT000000000000000301" {
		t.Errorf("role policies = %+v", state.Snapshot.RolePolicies)
	}
	if count := fixture.requestCount("/snapshot"); count != 1 {
		t.Errorf("snapshots = %d, want 1 (the initial one)", count)
	}
	if count := fixture.requestCount("/changes"); count < 3 {
		t.Errorf("changes pages = %d, want at least 3", count)
	}
}

func TestSyncRevisionGapFallsBackToSnapshot(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	syncFixtureService(t, syncer, "books")

	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
			Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000202",
			Name: stringPointer("write-books"), SPDL: stringPointer("grant write")})
	fixture.source.configure(func(source *fixtureSource) { source.dropCommitRevision = 2 })

	state := syncFixtureService(t, syncer, "books")
	if state.Revision != service.Revision || state.StateHash != service.StateHash {
		t.Fatalf("state did not recover through a snapshot")
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 2 {
		t.Errorf("snapshots = %d, want 2", count)
	}
}

func TestSyncBadCommitSignatureFallsBackToSnapshot(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	syncFixtureService(t, syncer, "books")

	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
			Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	fixture.source.configure(func(source *fixtureSource) { source.corruptCommitSignature = true })

	state := syncFixtureService(t, syncer, "books")
	if state.Revision != service.Revision || state.StateHash != service.StateHash {
		t.Fatalf("state did not recover through a snapshot")
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 2 {
		t.Errorf("snapshots = %d, want 2", count)
	}
}

func TestSyncBadHeadStateHashFallsBackToSnapshot(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	syncFixtureService(t, syncer, "books")

	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
			Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	fixture.source.configure(func(source *fixtureSource) { source.corruptHeadStateHash = true })

	state := syncFixtureService(t, syncer, "books")
	if state.Revision != service.Revision || state.StateHash != service.StateHash {
		t.Fatalf("state did not recover through a snapshot")
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 2 {
		t.Errorf("snapshots = %d, want 2", count)
	}
}

func TestSyncEpochChangeReloadsSnapshot(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	first := syncFixtureService(t, syncer, "books")
	if first.Epoch != service.Epoch {
		t.Fatalf("initial epoch = %q", first.Epoch)
	}

	const newEpoch = "01HF7YAT000000000000000103"
	fixture.source.configure(func(source *fixtureSource) {
		updated := source.services[testServiceID1]
		updated.Epoch = newEpoch
		updated.Revision = 1
		updated.State = Snapshot{Name: "books", Policies: []Policy{}, RolePolicies: []Policy{}}
		updated.StateHash = StateHash(updated.State)
		updated.CommitHash = updated.StateHash
		updated.commits = nil
	})

	state := syncFixtureService(t, syncer, "books")
	if state.Epoch != newEpoch || state.Revision != 1 {
		t.Fatalf("state = epoch %q revision %d", state.Epoch, state.Revision)
	}
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 2 {
		t.Errorf("snapshots = %d, want 2", count)
	}
}

func TestSyncSnapshotRequiredReloadsSnapshot(t *testing.T) {
	for _, reason := range []string{"history_unavailable", "epoch_mismatch", "signature_required"} {
		t.Run(reason, func(t *testing.T) {
			service := newFixtureService(testServiceID1, "books")
			fixture := newFixture(t, service)
			syncer := fixture.newSyncer("", nil)
			syncFixtureService(t, syncer, "books")

			service.publish(t, fixture.signer,
				Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
					Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
			fixture.source.configure(func(source *fixtureSource) { source.changesReason = reason })

			state := syncFixtureService(t, syncer, "books")
			if state.Revision != service.Revision || state.StateHash != service.StateHash {
				t.Fatalf("state did not recover through a snapshot")
			}
			if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 2 {
				t.Errorf("snapshots = %d, want 2", count)
			}
		})
	}
}

func TestSyncSourceGoneForgetsTheService(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	syncFixtureService(t, syncer, "books")

	service.publish(t, fixture.signer,
		Change{Op: "delete", Kind: "authz", ID: "01HF7YAT000000000000000201"})
	fixture.source.configure(func(source *fixtureSource) { source.changesReason = "source_gone" })

	services, err := syncer.Services(context.Background())
	if err != nil {
		t.Fatalf("Services: %v", err)
	}
	_, err = syncer.Sync(context.Background(), services.Services[0])
	if !errors.Is(err, ErrSourceGone) {
		t.Fatalf("Sync error = %v, want ErrSourceGone", err)
	}
	if _, found := syncer.State(testServiceID1); found {
		t.Error("the local copy survived source_gone")
	}
}

func TestSyncSnapshotIntegrityFailure(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	fixture.source.configure(func(source *fixtureSource) {
		source.corruptSnapshotHash = true
		source.corruptSnapshotSignature = true
	})
	syncer := fixture.newSyncer("", nil)

	services, err := syncer.Services(context.Background())
	if err != nil {
		t.Fatalf("Services: %v", err)
	}
	if _, err := syncer.Sync(context.Background(), services.Services[0]); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Sync error = %v, want ErrIntegrity", err)
	}
	if _, found := syncer.State(testServiceID1); found {
		t.Error("an unverified snapshot was stored")
	}
}

func TestSyncDiscoveryHashMismatchReloadsSnapshot(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	syncFixtureService(t, syncer, "books")

	fixture.source.configure(func(source *fixtureSource) { source.corruptDiscoveryHash = true })
	syncFixtureService(t, syncer, "books")
	if count := fixture.requestCount(testServiceID1 + "/snapshot"); count != 2 {
		t.Errorf("snapshots = %d, want 2", count)
	}
}

func TestSyncUnknownService(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	syncer := fixture.newSyncer("", nil)
	missing := Service{
		ID:            "01HF7YAT000000000000000999",
		Name:          "missing",
		Revision:      1,
		Epoch:         "01HF7YAT000000000000000999",
		HeadStateHash: mustHash(t, "sha256:"+strings.Repeat("a", 64)),
	}
	if _, err := syncer.Sync(context.Background(), missing); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Sync error = %v, want ErrServiceNotFound", err)
	}
}

func TestSyncByNameUnknownService(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	syncer := fixture.newSyncer("", nil)
	if _, err := syncer.SyncByName(context.Background(), "missing"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("SyncByName error = %v, want ErrServiceNotFound", err)
	}
}

func TestApplyChangesServiceRename(t *testing.T) {
	snapshot := Snapshot{Name: "books", Policies: []Policy{}, RolePolicies: []Policy{}}
	if err := applyChanges(&snapshot, []Change{
		{Op: "upsert", Kind: "service", ID: testServiceID1, Name: stringPointer("books-v2")},
	}); err != nil {
		t.Fatalf("applyChanges: %v", err)
	}
	if snapshot.Name != "books-v2" {
		t.Errorf("name = %q, want books-v2", snapshot.Name)
	}
}

func TestApplyChangesServiceDeleteIsSourceGone(t *testing.T) {
	snapshot := Snapshot{Name: "books", Policies: []Policy{}, RolePolicies: []Policy{}}
	err := applyChanges(&snapshot, []Change{{Op: "delete", Kind: "service", ID: testServiceID1}})
	if !errors.Is(err, ErrSourceGone) {
		t.Fatalf("applyChanges error = %v, want ErrSourceGone", err)
	}
}

func TestServiceStateCloneIsIndependent(t *testing.T) {
	state := &ServiceState{
		ServiceID: testServiceID1,
		Snapshot: Snapshot{
			Name:     "books",
			Policies: []Policy{{ID: "p1", Name: "p", SPDL: "grant"}},
		},
	}
	cloned := state.clone()
	cloned.Snapshot.Policies[0].Name = "changed"
	if state.Snapshot.Policies[0].Name != "p" {
		t.Error("clone shares its policy slice with the original")
	}
	if !reflect.DeepEqual(state.Snapshot.RolePolicies, cloned.Snapshot.RolePolicies) {
		t.Error("clone changed role policies")
	}
}
