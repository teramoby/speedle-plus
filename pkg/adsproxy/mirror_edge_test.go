// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func TestMirrorOptionValidation(t *testing.T) {
	for name, mutate := range map[string]func(*MirrorOptions){
		"commits": func(options *MirrorOptions) { options.MaxLogCommits = -1 },
		"bytes":   func(options *MirrorOptions) { options.MaxLogBytes = -1 },
	} {
		options := MirrorOptions{}
		mutate(&options)
		if _, err := NewMirror(options); !errors.Is(err, ErrConfig) {
			t.Errorf("%s: %v, want ErrConfig", name, err)
		}
	}
}

func TestMirrorChangesEdgeCases(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, upsertAuthz(testServiceID1+"A1", "policy", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	low, err := mirror.Changes(testServiceID1, 1, 0)
	if err != nil || len(low.Commits) != 1 {
		t.Errorf("limit below one: %v %+v", err, low.Commits)
	}
	high, err := mirror.Changes(testServiceID1, 1, 1000)
	if err != nil || len(high.Commits) != 1 {
		t.Errorf("limit above the maximum: %v %+v", err, high.Commits)
	}
	if _, err := mirror.Changes(testServiceID1, 1, 100); err != nil {
		t.Fatalf("changes: %v", err)
	}

	overflowed := newTestMirror(t, func(options *MirrorOptions) {
		options.MaxLogCommits = 1
	})
	other := newFixtureService(testServiceID2, "orders")
	if err := overflowed.ApplySnapshot(snapshotAt(t, other)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	for index := 0; index < 2; index++ {
		other.publish(t, upsertAuthz(testServiceID2+string(rune('A'+index)), "policy", "grant read"))
		if err := overflowed.ApplyChanges(pageFor(t, other, other.Revision-1, 100)); err != nil {
			t.Fatalf("apply changes: %v", err)
		}
	}
	if _, err := overflowed.Changes(testServiceID2, 1, 100); !errors.Is(err, ErrSnapshotRequired) {
		t.Errorf("overflowed changes: %v, want ErrSnapshotRequired", err)
	}
}

func TestMirrorNilInputs(t *testing.T) {
	mirror := newTestMirror(t, nil)
	if err := mirror.ApplySnapshot(nil); !errors.Is(err, ErrConfig) {
		t.Errorf("nil snapshot: %v, want ErrConfig", err)
	}
	if err := mirror.ApplyChanges(nil); !errors.Is(err, ErrConfig) {
		t.Errorf("nil page: %v, want ErrConfig", err)
	}
	if err := mirror.ApplyChanges(&adsync.ChangesResponse{ServiceID: testUnknownID}); !errors.Is(err, ErrUnknownService) {
		t.Errorf("unknown service: %v, want ErrUnknownService", err)
	}
}

func TestMirrorSnapshotValidation(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	for _, testCase := range []struct {
		name   string
		mutate func(*adsync.SnapshotResponse)
	}{
		{name: "invalid service id", mutate: func(response *adsync.SnapshotResponse) { response.ServiceID = "bad" }},
		{name: "invalid source id", mutate: func(response *adsync.SnapshotResponse) { response.SourceID = "bad" }},
		{name: "invalid epoch", mutate: func(response *adsync.SnapshotResponse) { response.Epoch = "bad" }},
		{name: "invalid revision", mutate: func(response *adsync.SnapshotResponse) { response.Revision = 1<<53 + 1 }},
		{name: "invalid name", mutate: func(response *adsync.SnapshotResponse) { response.Snapshot.Name = "-bad" }},
		{name: "missing hashes", mutate: func(response *adsync.SnapshotResponse) { response.StateHash = adsync.Hash{} }},
	} {
		response := service.snapshotResponse(t)
		testCase.mutate(response)
		if err := mirror.ApplySnapshot(response); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("%s: %v, want ErrProtocol", testCase.name, err)
		}
	}
}

func TestMirrorChangePageValidation(t *testing.T) {
	newMirrorWithCommit := func(t *testing.T) (*Mirror, *fixtureService) {
		t.Helper()
		mirror := newTestMirror(t, nil)
		service := newFixtureService(testServiceID1, "books")
		if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
			t.Fatalf("apply snapshot: %v", err)
		}
		service.publish(t, upsertAuthz(testServiceID1+"A1", "policy", "grant read"))
		return mirror, service
	}
	valid := func(t *testing.T, service *fixtureService) *adsync.ChangesResponse {
		t.Helper()
		return pageFor(t, service, 1, 100)
	}

	t.Run("already applied", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		if err := mirror.ApplyChanges(page); err != nil {
			t.Fatalf("first delivery: %v", err)
		}
		if err := mirror.ApplyChanges(page); err != nil {
			t.Errorf("duplicate delivery: %v", err)
		}
	})

	t.Run("identity changed", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.Epoch = testServiceID2
		if err := mirror.ApplyChanges(page); !errors.Is(err, ErrIdentityChanged) {
			t.Errorf("epoch change: %v, want ErrIdentityChanged", err)
		}
	})

	t.Run("revision out of range", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.From = 1<<53 + 1
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("from out of range: %v, want ErrProtocol", err)
		}
	})

	t.Run("ends after head", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.To = page.Head + 1
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("to after head: %v, want ErrProtocol", err)
		}
	})

	t.Run("no commits", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.Commits = nil
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("empty page: %v, want ErrProtocol", err)
		}
	})

	t.Run("not contiguous", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		service.publish(t, upsertAuthz(testServiceID1+"A2", "policy", "grant read"))
		page := pageFor(t, service, 1, 100)
		if len(page.Commits) != 2 {
			t.Fatalf("fixture commits = %d", len(page.Commits))
		}
		page.Commits[1].Revision = page.Commits[0].Revision
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("non-contiguous page: %v, want ErrProtocol", err)
		}
	})

	t.Run("broken chain", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.Commits[0].PrevHash = adsync.Hash{}
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrIntegrity) {
			t.Errorf("broken chain: %v, want ErrIntegrity", err)
		}
	})

	t.Run("hash mismatch", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		name := "tampered"
		page.Commits[0].Changes = []adsync.Change{upsertAuthz(testServiceID1+"A9", name, "grant read")}
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrIntegrity) {
			t.Errorf("hash mismatch: %v, want ErrIntegrity", err)
		}
	})

	t.Run("service change targets another service", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		service.publish(t, renameService(testServiceID2, "other"))
		page := pageFor(t, service, 1, 100)
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("foreign service change: %v, want ErrProtocol", err)
		}
	})

	t.Run("last commit before page end", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		service.publish(t, upsertAuthz(testServiceID1+"A2", "policy", "grant read"))
		page := pageFor(t, service, 1, 1)
		page.To = page.Head
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("short page: %v, want ErrProtocol", err)
		}
	})

	t.Run("head state hash mismatch", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.HeadStateHash = adsync.StateHash(adsync.Snapshot{Name: "other"})
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrIntegrity) {
			t.Errorf("head hash: %v, want ErrIntegrity", err)
		}
	})
}

func TestMirrorRemovedAndOverflowedPages(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, upsertAuthz(testServiceID1+"A1", "policy", "grant read"))
	page := pageFor(t, service, 1, 100)
	mirror.RemoveService(testServiceID1)
	if err := mirror.ApplyChanges(page); !errors.Is(err, ErrServiceRemoved) {
		t.Errorf("removed service: %v, want ErrServiceRemoved", err)
	}

	overflowed := newTestMirror(t, func(options *MirrorOptions) {
		options.MaxLogCommits = 1
	})
	other := newFixtureService(testServiceID2, "orders")
	if err := overflowed.ApplySnapshot(snapshotAt(t, other)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	for index := 0; index < 2; index++ {
		other.publish(t, upsertAuthz(testServiceID2+string(rune('A'+index)), "policy", "grant read"))
		if err := overflowed.ApplyChanges(pageFor(t, other, other.Revision-1, 100)); err != nil {
			t.Fatalf("apply changes: %v", err)
		}
	}
	next := other.changesPage(other.Revision, 100)
	if err := overflowed.ApplyChanges(&next); !errors.Is(err, ErrSnapshotRequired) {
		t.Errorf("overflowed service: %v, want ErrSnapshotRequired", err)
	}
}

func TestRemoveServiceIgnoresUnknownAndRemoved(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	version := mirror.Version()
	mirror.RemoveService(testUnknownID)
	if mirror.Version() != version {
		t.Error("removing an unknown service advanced the version")
	}
	mirror.RemoveService(testServiceID1)
	removedVersion := mirror.Version()
	mirror.RemoveService(testServiceID1)
	if mirror.Version() != removedVersion {
		t.Error("removing a removed service advanced the version")
	}
}

func TestMirrorSnapshotConflictAndRename(t *testing.T) {
	mirror := newTestMirror(t, nil)
	first := newFixtureService(testServiceID1, "books")
	first.publish(t, upsertAuthz(testServiceID1+"A1", "one", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, first)); err != nil {
		t.Fatalf("apply first snapshot: %v", err)
	}
	second := newFixtureService(testServiceID1, "books")
	second.publish(t, upsertAuthz(testServiceID1+"B1", "two", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, second)); !errors.Is(err, adsync.ErrIntegrity) {
		t.Errorf("same revision, different state hash: %v, want ErrIntegrity", err)
	}

	renamed := newTestMirror(t, nil)
	base := newFixtureService(testServiceID1, "books")
	if err := renamed.ApplySnapshot(snapshotAt(t, base)); err != nil {
		t.Fatalf("apply base snapshot: %v", err)
	}
	version := renamed.Version()
	updated := newFixtureService(testServiceID1, "books")
	updated.publish(t, renameService(testServiceID1, "books-v2"))
	if err := renamed.ApplySnapshot(snapshotAt(t, updated)); err != nil {
		t.Fatalf("apply renamed snapshot: %v", err)
	}
	services := renamed.Services()
	if len(services) != 1 || services[0].Name != "books-v2" {
		t.Fatalf("services after rename = %+v", services)
	}
	if renamed.Version() <= version {
		t.Error("a rename through a snapshot did not advance the version")
	}
}

func TestMirrorOversizedCommitOverflows(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, upsertAuthz(testServiceID1+"A1", "policy", strings.Repeat("x", 1200<<10)))
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply oversized commit: %v", err)
	}
	if services := mirror.Services(); len(services) != 0 {
		t.Errorf("service with an unsendable commit is advertised: %+v", services)
	}
	if needs := mirror.NeedsSnapshot(); len(needs) != 1 || needs[0] != testServiceID1 {
		t.Errorf("NeedsSnapshot = %v", needs)
	}
}

func TestMirrorRejectsCommitThatCannotApply(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	changes := []adsync.Change{{Op: "upsert", Kind: "service", ID: testServiceID1}}
	hash, err := adsync.CommitHash(service.CommitHash, changes)
	if err != nil {
		t.Fatalf("commit hash: %v", err)
	}
	commit := adsync.Commit{
		Revision: service.Revision + 1,
		PrevHash: service.CommitHash,
		Hash:     hash,
		At:       1762500000000,
		Changes:  changes,
	}
	page := &adsync.ChangesResponse{
		ServiceID: service.ID,
		SourceID:  service.ID,
		Epoch:     service.Epoch,
		From:      service.Revision + 1,
		To:        service.Revision + 1,
		Head:      service.Revision + 1,
		Commits:   []adsync.Commit{commit},
	}
	if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
		t.Errorf("commit without a service name: %v, want ErrProtocol", err)
	}
}

func TestMirrorDelayedSnapshotCannotReviveASignedDelete(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	before := snapshotAt(t, service)
	service.publish(t, deleteChange("service", testServiceID1))
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); !errors.Is(err, adsync.ErrSourceGone) {
		t.Fatalf("signed delete: %v, want ErrSourceGone", err)
	}

	// The pre-deletion snapshot was fetched before the delete reached the
	// source. It must not bring the deleted policies back.
	if err := mirror.ApplySnapshot(before); err != nil {
		t.Fatalf("delayed snapshot: %v", err)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceRemoved {
		t.Errorf("availability after delayed snapshot = %v, want ServiceRemoved", availability)
	}
	if services := mirror.Services(); len(services) != 0 {
		t.Errorf("delayed snapshot revived the service: %+v", services)
	}

	// Newer source state does revive it: a later revision is past the delete.
	service.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("newer snapshot: %v", err)
	}
	if services := mirror.Services(); len(services) != 1 || services[0].Revision != service.Revision {
		t.Errorf("newer snapshot did not revive the service: %+v", services)
	}
}

func TestMirrorDelayedSnapshotCannotReviveAnAdministrativeRemoval(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	before := snapshotAt(t, service)
	mirror.RemoveService(testServiceID1)
	if err := mirror.ApplySnapshot(before); err != nil {
		t.Fatalf("delayed snapshot: %v", err)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceRemoved {
		t.Errorf("availability after delayed snapshot = %v, want ServiceRemoved", availability)
	}

	service.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
		t.Fatalf("newer snapshot: %v", err)
	}
	if services := mirror.Services(); len(services) != 1 {
		t.Errorf("newer snapshot did not revive the service: %+v", services)
	}
}

func TestMirrorServiceChurnDoesNotExhaustCapacity(t *testing.T) {
	mirror := newTestMirror(t, nil)
	const churn = 20
	ids := make([]string, 0, churn)
	for index := 0; index < churn; index++ {
		id := fmt.Sprintf("01HF7YAT%018d", 100+index)
		ids = append(ids, id)
		service := newFixtureService(id, fmt.Sprintf("service-%d", index))
		if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
			t.Fatalf("service %d: %v", index, err)
		}
	}
	for _, id := range ids {
		mirror.RemoveService(id)
	}
	if services := mirror.Services(); len(services) != 0 {
		t.Fatalf("removed services are advertised: %+v", services)
	}

	// Tombstones must not hold the service slots: a fresh set of ids replaces
	// the removed ones.
	for index := 0; index < churn; index++ {
		id := fmt.Sprintf("01HF7YAT%018d", 200+index)
		service := newFixtureService(id, fmt.Sprintf("fresh-%d", index))
		if err := mirror.ApplySnapshot(snapshotAt(t, service)); err != nil {
			t.Fatalf("fresh service %d: %v", index, err)
		}
	}
	if services := mirror.Services(); len(services) != churn {
		t.Errorf("advertised services = %d, want %d", len(services), churn)
	}

	// Active services have no engine-side cap.
	extra := newFixtureService("01HF7YAT000000000000000999", "extra")
	if err := mirror.ApplySnapshot(snapshotAt(t, extra)); err != nil {
		t.Errorf("additional active service: %v", err)
	}
}

func TestMirrorIgnoresSnapshotsFromRetiredIdentities(t *testing.T) {
	mirror := newTestMirror(t, nil)
	retired := newFixtureServiceWithEpoch(testServiceID1, "01HF7YAT000000000000000201", "books")
	retired.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	retiredSnapshot := snapshotAt(t, retired)
	if err := mirror.ApplySnapshot(retiredSnapshot); err != nil {
		t.Fatalf("apply first identity: %v", err)
	}

	current := newFixtureServiceWithEpoch(testServiceID1, "01HF7YAT000000000000000202", "books")
	current.publish(t, upsertAuthz(testServiceID1+"B1", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, current)); err != nil {
		t.Fatalf("apply second identity: %v", err)
	}

	// A delayed snapshot from the identity the source moved away from carries
	// replaced policy. Revisions of different identities cannot be compared, so
	// the identity itself must keep it out.
	if err := mirror.ApplySnapshot(retiredSnapshot); err != nil {
		t.Fatalf("delayed retired snapshot: %v", err)
	}
	info, availability := mirror.Lookup(testServiceID1)
	if availability != ServiceAvailable || info.Epoch != current.Epoch || info.Revision != current.Revision {
		t.Errorf("copy rolled back to a retired identity: %+v availability %v", info, availability)
	}

	// The same holds for a removed copy: the retired identity cannot revive it,
	// while newer state of the current identity can.
	mirror.RemoveService(testServiceID1)
	if err := mirror.ApplySnapshot(retiredSnapshot); err != nil {
		t.Fatalf("retired snapshot after removal: %v", err)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceRemoved {
		t.Errorf("retired identity revived a removed service: availability %v", availability)
	}
	current.publish(t, upsertAuthz(testServiceID1+"B2", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, current)); err != nil {
		t.Fatalf("revive current identity: %v", err)
	}
	if services := mirror.Services(); len(services) != 1 || services[0].Epoch != current.Epoch {
		t.Errorf("current identity did not revive the service: %+v", services)
	}
}

func TestMirrorKeepsTombstonesAtTheBound(t *testing.T) {
	mirror := newTestMirror(t, nil)
	var (
		firstID       string
		firstSnapshot *adsync.SnapshotResponse
	)
	for index := 0; index < maxTombstones; index++ {
		id := fmt.Sprintf("01HF7YAT%018d", 2000+index)
		service := newFixtureService(id, "books")
		snapshot := snapshotAt(t, service)
		if index == 0 {
			firstID, firstSnapshot = id, snapshot
		}
		if err := mirror.ApplySnapshot(snapshot); err != nil {
			t.Fatalf("service %d: %v", index, err)
		}
		mirror.RemoveService(id)
	}
	// The tombstone store is full. Losing a tombstone would let a delayed
	// snapshot resurrect deleted policy, so a new service is refused instead.
	extra := newFixtureService(fmt.Sprintf("01HF7YAT%018d", 5000), "extra")
	if err := mirror.ApplySnapshot(snapshotAt(t, extra)); !errors.Is(err, ErrTooManyServices) {
		t.Errorf("new service with a full tombstone store: %v, want ErrTooManyServices", err)
	}

	// The oldest tombstone still guards its pre-removal snapshot.
	if err := mirror.ApplySnapshot(firstSnapshot); err != nil {
		t.Fatalf("delayed snapshot: %v", err)
	}
	if _, availability := mirror.Lookup(firstID); availability != ServiceRemoved {
		t.Errorf("availability of the oldest tombstone = %v, want ServiceRemoved", availability)
	}
}

func TestMirrorRejectsReplaysFromRetiredIdentities(t *testing.T) {
	mirror := newTestMirror(t, nil)
	var first *adsync.SnapshotResponse
	for index := 0; index < 12; index++ {
		epoch := fmt.Sprintf("01HF7YAT%018d", 6000+index)
		service := newFixtureServiceWithEpoch(testServiceID1, epoch, "books")
		service.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
		snapshot := snapshotAt(t, service)
		if index == 0 {
			first = snapshot
		}
		if err := mirror.ApplySnapshot(snapshot); err != nil {
			t.Fatalf("identity %d: %v", index, err)
		}
	}
	// Replay protection outlives the transition: more identity changes than any
	// retention bound must not make the oldest snapshot acceptable again.
	if err := mirror.ApplySnapshot(first); err != nil {
		t.Fatalf("replayed snapshot: %v", err)
	}
	info, availability := mirror.Lookup(testServiceID1)
	if availability != ServiceAvailable {
		t.Fatalf("availability = %v", availability)
	}
	if info.Epoch != fmt.Sprintf("01HF7YAT%018d", 6011) {
		t.Errorf("copy rolled back to %q", info.Epoch)
	}
}

func TestMirrorRejectsConflictingCommitHashAtOneRevision(t *testing.T) {
	mirror := newTestMirror(t, nil)
	service := newFixtureService(testServiceID1, "books")
	original := snapshotAt(t, service)
	if err := mirror.ApplySnapshot(original); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}

	conflicting := snapshotAt(t, service)
	conflicting.CommitHash = adsync.StateHash(adsync.Snapshot{Name: "other history"})
	if err := mirror.ApplySnapshot(conflicting); !errors.Is(err, adsync.ErrIntegrity) {
		t.Errorf("conflicting commit hash: %v, want ErrIntegrity", err)
	}
	anchor, found := mirror.Snapshot(testServiceID1)
	if !found || anchor.CommitHash != original.CommitHash {
		t.Errorf("anchor was replaced: %+v", anchor)
	}

	// The snapshot entity tag covers the commit hash, so a downstream holding
	// the old anchor never sees a 304 for a different history.
	if snapshotETag(*original) == snapshotETag(*conflicting) {
		t.Error("snapshot ETag ignores the commit hash")
	}
}

func TestMirrorBoundsIdentityChurn(t *testing.T) {
	mirror := newTestMirror(t, nil)
	current := newFixtureServiceWithEpoch(testServiceID1, fmt.Sprintf("01HF7YAT%018d", 7000), "books")
	current.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, current)); err != nil {
		t.Fatalf("first identity: %v", err)
	}
	for index := 1; index <= maxRetiredIdentities; index++ {
		next := newFixtureServiceWithEpoch(testServiceID1, fmt.Sprintf("01HF7YAT%018d", 7000+index), "books")
		next.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
		if err := mirror.ApplySnapshot(snapshotAt(t, next)); err != nil {
			t.Fatalf("identity %d: %v", index, err)
		}
		current = next
	}

	// One more identity replacement is refused, and the copy stops being
	// advertised rather than keeping replay state without bound.
	excess := newFixtureServiceWithEpoch(testServiceID1, fmt.Sprintf("01HF7YAT%018d", 7000+maxRetiredIdentities+1), "books")
	excess.publish(t, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, excess)); !errors.Is(err, ErrIdentityChurn) {
		t.Fatalf("identity churn: %v, want ErrIdentityChurn", err)
	}
	if services := mirror.Services(); len(services) != 0 {
		t.Errorf("service under identity churn is advertised: %+v", services)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceNeedsSnapshot {
		t.Errorf("availability = %v, want ServiceNeedsSnapshot", availability)
	}

	// The current identity can still be refreshed.
	current.publish(t, upsertAuthz(testServiceID1+"A2", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, current)); err != nil {
		t.Fatalf("refresh current identity: %v", err)
	}
	services := mirror.Services()
	if len(services) != 1 || services[0].Epoch != current.Epoch {
		t.Errorf("current identity did not revive the service: %+v", services)
	}
}
