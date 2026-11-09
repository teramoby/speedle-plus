// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"errors"
	"fmt"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func newTestMirror(t *testing.T, keys *adsync.PublicKeySet, mutate func(*MirrorOptions)) *Mirror {
	t.Helper()
	options := MirrorOptions{PublicKeys: keys}
	if mutate != nil {
		mutate(&options)
	}
	mirror, err := NewMirror(options)
	if err != nil {
		t.Fatalf("new mirror: %v", err)
	}
	return mirror
}

func snapshotAt(t *testing.T, service *fixtureService, signer *testSigner) *adsync.SnapshotResponse {
	t.Helper()
	return service.snapshotResponse(t, signer)
}

func TestMirrorServesSignedSnapshotAndChanges(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")

	initial := snapshotAt(t, service, signer)
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read-books", `grant read on "/books/*"`))
	service.publish(t, signer, upsertRole(testServiceID1+"R1", "reader", `grant role "reader" to read`))

	if err := mirror.ApplySnapshot(initial); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}

	services := mirror.Services()
	if len(services) != 1 {
		t.Fatalf("services = %d, want 1", len(services))
	}
	if services[0].Revision != service.Revision || services[0].HeadStateHash != service.StateHash {
		t.Errorf("service = %+v, want revision %d hash %s", services[0], service.Revision, service.StateHash)
	}
	if services[0].Name != "books" {
		t.Errorf("service name = %q", services[0].Name)
	}

	snapshot, found := mirror.Snapshot(testServiceID1)
	if !found {
		t.Fatal("snapshot not found")
	}
	if snapshot.Revision != 1 {
		t.Errorf("snapshot revision = %d, want the signed anchor 1", snapshot.Revision)
	}
	if snapshot.Signature != initial.Signature {
		t.Error("snapshot signature was not passed through unchanged")
	}
	if snapshot.StateHash != initial.StateHash || snapshot.CommitHash != initial.CommitHash {
		t.Error("snapshot hashes were not passed through unchanged")
	}

	page, err := mirror.Changes(testServiceID1, 1, 100)
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	if page.From != 2 || page.To != 3 || page.Head != 3 || page.NextSince != 4 {
		t.Errorf("page bounds = from %d to %d head %d next %d", page.From, page.To, page.Head, page.NextSince)
	}
	if len(page.Commits) != 2 {
		t.Fatalf("commits = %d, want 2", len(page.Commits))
	}
	if page.Commits[0].Signature != service.commits[0].Signature {
		t.Error("commit signature was not passed through unchanged")
	}
	if page.HeadStateHash != service.StateHash {
		t.Error("head state hash does not match the applied state")
	}

	empty, err := mirror.Changes(testServiceID1, 3, 100)
	if err != nil {
		t.Fatalf("changes at head: %v", err)
	}
	if len(empty.Commits) != 0 || empty.From != 4 || empty.To != 3 || empty.Head != 3 {
		t.Errorf("empty page = %+v", empty)
	}

	if _, err := mirror.Changes(testServiceID1, 0, 100); !errors.Is(err, ErrHistoryUnavailable) {
		t.Errorf("changes before the anchor: %v, want ErrHistoryUnavailable", err)
	}
	if _, err := mirror.Changes(testServiceID1, 4, 100); !errors.Is(err, ErrHistoryUnavailable) {
		t.Errorf("changes after the head: %v, want ErrHistoryUnavailable", err)
	}
	if _, err := mirror.Changes(testUnknownID, 1, 100); !errors.Is(err, ErrUnknownService) {
		t.Errorf("unknown service: %v, want ErrUnknownService", err)
	}
}

func TestMirrorStaleSnapshotCannotRollBack(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")

	initial := snapshotAt(t, service, signer)
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read-books", "grant read"))
	if err := mirror.ApplySnapshot(initial); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	// A late delivery of the same snapshot must not roll the copy back.
	if err := mirror.ApplySnapshot(initial); err != nil {
		t.Fatalf("re-apply snapshot: %v", err)
	}
	if got := mirror.Services()[0].Revision; got != 2 {
		t.Errorf("revision after stale snapshot = %d, want 2", got)
	}
	if page, err := mirror.Changes(testServiceID1, 1, 100); err != nil || len(page.Commits) != 1 {
		t.Errorf("log after stale snapshot: %v %+v", err, page.Commits)
	}
}

func TestMirrorRejectsBadSnapshot(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")

	tamperedHash := service.snapshotResponse(t, signer)
	tamperedHash.StateHash = adsync.StateHash(adsync.Snapshot{Name: "other"})
	if err := mirror.ApplySnapshot(tamperedHash); !errors.Is(err, adsync.ErrIntegrity) {
		t.Errorf("tampered state hash: %v, want ErrIntegrity", err)
	}

	tamperedSignature := service.snapshotResponse(t, signer)
	tamperedSignature.Signature.Value = signer.sign([]byte("different")).Value
	if err := mirror.ApplySnapshot(tamperedSignature); !errors.Is(err, adsync.ErrIntegrity) {
		t.Errorf("tampered signature: %v, want ErrIntegrity", err)
	}

	otherSigner := newTestSigner(t)
	wrongKey := service.snapshotResponse(t, otherSigner)
	if err := mirror.ApplySnapshot(wrongKey); !errors.Is(err, adsync.ErrIntegrity) {
		t.Errorf("signature from an unknown key: %v, want ErrIntegrity", err)
	}
}

func TestMirrorRejectsPageGap(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")

	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read-books", "grant read"))
	service.publish(t, signer, upsertAuthz(testServiceID1+"A2", "read-more", "grant read"))
	service.publish(t, signer, upsertAuthz(testServiceID1+"A3", "read-all", "grant read"))
	// The page starts at revision 3 while the copy is still at 1: revision 2
	// is missing, so the whole page must be rejected.
	page := pageFor(t, service, 2, 100)
	if page.From != 3 {
		t.Fatalf("fixture page starts at %d", page.From)
	}
	if err := mirror.ApplyChanges(page); !errors.Is(err, ErrSnapshotRequired) {
		t.Fatalf("page gap: %v, want ErrSnapshotRequired", err)
	}
	if got := mirror.Services()[0].Revision; got != 1 {
		t.Errorf("revision after rejected page = %d, want 1", got)
	}
}

func TestMirrorLogBoundStopsServingUntilSnapshot(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), func(options *MirrorOptions) {
		options.MaxLogCommits = 2
	})
	service := newFixtureService(testServiceID1, "books")

	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	for index := 0; index < 3; index++ {
		service.publish(t, signer, upsertAuthz(testServiceID1+string(rune('A'+index)), "policy", "grant read"))
		if err := mirror.ApplyChanges(pageFor(t, service, service.Revision-1, 100)); err != nil {
			t.Fatalf("apply changes at revision %d: %v", service.Revision, err)
		}
	}
	if services := mirror.Services(); len(services) != 0 {
		t.Fatalf("overflowed service is still advertised: %+v", services)
	}
	if needs := mirror.NeedsSnapshot(); len(needs) != 1 || needs[0] != testServiceID1 {
		t.Errorf("NeedsSnapshot = %v", needs)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceNeedsSnapshot {
		t.Errorf("availability = %v, want ServiceNeedsSnapshot", availability)
	}

	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("refresh snapshot: %v", err)
	}
	if services := mirror.Services(); len(services) != 1 {
		t.Fatalf("refreshed service is not advertised")
	}
	if needs := mirror.NeedsSnapshot(); len(needs) != 0 {
		t.Errorf("NeedsSnapshot after refresh = %v", needs)
	}
}

func TestMirrorSourceChangeResetsLogAndEpoch(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")

	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read-books", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	version := mirror.Version()

	// The source changed: a new epoch and a snapshot at a different revision.
	replacement := newFixtureServiceWithEpoch(testServiceID1, testServiceID2, "books")
	replacement.publish(t, signer, upsertRole(testServiceID1+"R9", "auditor", "grant role"))
	if err := mirror.ApplySnapshot(snapshotAt(t, replacement, signer)); err != nil {
		t.Fatalf("apply replacement snapshot: %v", err)
	}
	services := mirror.Services()
	if len(services) != 1 {
		t.Fatalf("services = %d", len(services))
	}
	if services[0].Epoch != testServiceID2 || services[0].Revision != replacement.Revision {
		t.Errorf("service after source change = %+v", services[0])
	}
	if mirror.Version() <= version {
		t.Error("service-set version did not advance on a source change")
	}
	if _, err := mirror.Changes(testServiceID1, 1, 100); !errors.Is(err, ErrHistoryUnavailable) {
		t.Errorf("old revision after source change: %v, want ErrHistoryUnavailable", err)
	}
	if page, err := mirror.Changes(testServiceID1, replacement.Revision, 100); err != nil || len(page.Commits) != 0 {
		t.Errorf("page at the new anchor: %v %+v", err, page.Commits)
	}
	if snapshot, found := mirror.Snapshot(testServiceID1); !found || snapshot.Epoch != testServiceID2 {
		t.Errorf("snapshot epoch = %q, want %q", snapshot.Epoch, testServiceID2)
	}
}

func TestMirrorServiceRemovalAndDelete(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")

	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplySnapshot(service.snapshotResponse(t, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	remove := adsync.Change{Op: "delete", Kind: "service", ID: testServiceID1}
	service.publish(t, signer, remove)
	if err := mirror.ApplyChanges(pageFor(t, service, 2, 100)); !errors.Is(err, adsync.ErrSourceGone) {
		t.Fatalf("service delete: %v, want ErrSourceGone", err)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceRemoved {
		t.Errorf("availability = %v, want ServiceRemoved", availability)
	}
	if _, err := mirror.Changes(testServiceID1, 1, 100); !errors.Is(err, ErrServiceRemoved) {
		t.Errorf("changes for a removed service: %v, want ErrServiceRemoved", err)
	}
	if _, found := mirror.Snapshot(testServiceID1); found {
		t.Error("snapshot served for a removed service")
	}

	// A fresh snapshot revives the service.
	if err := mirror.ApplySnapshot(service.snapshotResponse(t, signer)); err != nil {
		t.Fatalf("revive snapshot: %v", err)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceAvailable {
		t.Errorf("availability after revive = %v", availability)
	}
}

func TestMirrorAppliesRename(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")

	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, signer, renameService(testServiceID1, "books-v2"))
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply rename: %v", err)
	}
	if services := mirror.Services(); len(services) != 1 || services[0].Name != "books-v2" {
		t.Errorf("services after rename = %+v", services)
	}
	if info, availability := mirror.Lookup(testServiceID1); availability != ServiceAvailable || info.Name != "books-v2" {
		t.Errorf("lookup after rename = %+v %v", info, availability)
	}
}

func TestMirrorBoundsServices(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	for index := 0; index < MaxServices; index++ {
		id := fmt.Sprintf("01HF7YAT00000000000000%02X00", index)
		service := newFixtureService(id, "books")
		if err := mirror.ApplySnapshot(service.snapshotResponse(t, signer)); err != nil {
			t.Fatalf("apply snapshot %d: %v", index, err)
		}
	}
	extra := newFixtureService("01HF7YAT000000000000000199", "extra")
	if err := mirror.ApplySnapshot(extra.snapshotResponse(t, signer)); !errors.Is(err, ErrTooManyServices) {
		t.Errorf("extra service: %v, want ErrTooManyServices", err)
	}
}

func TestMirrorRequiresPublicKeys(t *testing.T) {
	if _, err := NewMirror(MirrorOptions{}); !errors.Is(err, ErrConfig) {
		t.Errorf("no keys: %v, want ErrConfig", err)
	}
}

func pageFor(t *testing.T, service *fixtureService, since uint64, limit int) *adsync.ChangesResponse {
	t.Helper()
	page := service.changesPage(since, limit)
	return &page
}

func TestMirrorServiceSetVersionTracksVisibility(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), func(options *MirrorOptions) {
		options.MaxLogCommits = 1
	})
	service := newFixtureService(testServiceID1, "books")

	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	advertised, version := mirror.ServicesVersion()
	if len(advertised) != 1 {
		t.Fatalf("initial services = %+v", advertised)
	}

	// The second commit pushes the log past its bound. The service stops being
	// advertised, and the version must advance so a downstream notices.
	for index := 0; index < 2; index++ {
		service.publish(t, signer, upsertAuthz(testServiceID1+string(rune('A'+index)), "policy", "grant read"))
		if err := mirror.ApplyChanges(pageFor(t, service, service.Revision-1, 100)); err != nil {
			t.Fatalf("apply changes at revision %d: %v", service.Revision, err)
		}
	}
	advertised, overflowVersion := mirror.ServicesVersion()
	if len(advertised) != 0 {
		t.Fatalf("overflowed service is advertised: %+v", advertised)
	}
	if overflowVersion <= version {
		t.Errorf("version did not advance when overflow hid the service: %d -> %d", version, overflowVersion)
	}

	// A fresh signed snapshot brings the service back; the version advances
	// again.
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("refresh snapshot: %v", err)
	}
	advertised, revivedVersion := mirror.ServicesVersion()
	if len(advertised) != 1 {
		t.Fatalf("revived service is not advertised")
	}
	if revivedVersion <= overflowVersion {
		t.Errorf("version did not advance when a snapshot revived the service: %d -> %d",
			overflowVersion, revivedVersion)
	}

	// A removal hides the service and another snapshot revives it; both
	// transitions must advance the version as well.
	mirror.RemoveService(testServiceID1)
	advertised, removedVersion := mirror.ServicesVersion()
	if len(advertised) != 0 {
		t.Fatalf("removed service is advertised: %+v", advertised)
	}
	if removedVersion <= revivedVersion {
		t.Errorf("version did not advance on removal: %d -> %d", revivedVersion, removedVersion)
	}
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("revive snapshot: %v", err)
	}
	advertised, revivalVersion := mirror.ServicesVersion()
	if len(advertised) != 1 {
		t.Fatalf("revived service is not advertised")
	}
	if revivalVersion <= removedVersion {
		t.Errorf("version did not advance on revival: %d -> %d", removedVersion, revivalVersion)
	}
}

func TestMirrorServicesVersionIsAtomic(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	snapshot := snapshotAt(t, service, signer)
	if err := mirror.ApplySnapshot(snapshot); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}

	// Each writer step makes exactly one visibility transition, so the parity
	// of the version says whether the service is advertised: an advertised
	// service always comes with an even version. A list and a version taken
	// from different moments would break the pairing.
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stop:
				return
			default:
			}
			mirror.RemoveService(testServiceID1)
			if err := mirror.ApplySnapshot(snapshot); err != nil {
				t.Errorf("revive snapshot: %v", err)
				return
			}
		}
	}()
	for index := 0; index < 2000; index++ {
		advertised, version := mirror.ServicesVersion()
		switch {
		case len(advertised) == 1 && version%2 != 0:
			t.Fatalf("advertised service with version %d, want an even version", version)
		case len(advertised) == 0 && version%2 == 0:
			t.Fatalf("hidden service with version %d, want an odd version", version)
		}
	}
	close(stop)
	<-stopped
}
