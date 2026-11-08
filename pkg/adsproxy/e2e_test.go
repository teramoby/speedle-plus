// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"context"
	"errors"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

// TestSignaturePassThroughToDownstreamClient proves the core proxy promise: the
// snapshot and commit signatures downstream instances verify are the Cloud
// signatures, byte for byte. The proxy only holds public keys, so it could not
// produce them itself.
func TestSignaturePassThroughToDownstreamClient(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")

	initial := service.snapshotResponse(t, signer)
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read-books", `grant read on "/books/*"`))
	service.publish(t, signer, upsertRole(testServiceID1+"R1", "reader", `grant role "reader" to read`))
	if err := mirror.ApplySnapshot(initial); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}

	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})

	// A downstream ADS that only trusts the Cloud public key must verify both
	// the snapshot and every commit, and end at the proxy's revision.
	client := server.newClient(token)
	syncer, err := adsync.NewSyncer(client, "")
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}
	state := syncToHead(t, syncer, service.service())
	if state.Revision != service.Revision || state.StateHash != service.StateHash || state.CommitHash != service.CommitHash {
		t.Errorf("downstream state = revision %d state %s commit %s, want %d %s %s",
			state.Revision, state.StateHash, state.CommitHash,
			service.Revision, service.StateHash, service.CommitHash)
	}
	if state.Snapshot.Name != "books" || len(state.Snapshot.Policies) != 1 || len(state.Snapshot.RolePolicies) != 1 {
		t.Errorf("downstream snapshot = %+v", state.Snapshot)
	}

	// The exact signed bytes are observable over HTTP as well.
	response, body := server.get(
		adsync.APIPrefix+"/services/"+testServiceID1+"/snapshot", token, nil)
	if response.StatusCode != 200 {
		t.Fatalf("snapshot: status %d body %s", response.StatusCode, body)
	}
	snapshot := decodeBody[adsync.SnapshotResponse](t, body)
	if snapshot.Signature != initial.Signature {
		t.Error("snapshot signature changed in transit")
	}
	if snapshot.SourceID != initial.SourceID || snapshot.Epoch != initial.Epoch ||
		snapshot.Revision != initial.Revision || snapshot.StateHash != initial.StateHash ||
		snapshot.CommitHash != initial.CommitHash {
		t.Error("snapshot identity changed in transit")
	}

	// A downstream that trusts a different key must reject the data, proving
	// the proxy is not re-signing with a key of its own.
	otherSigner := newTestSigner(t)
	wrongKeyServer := newTestServer(t, mirror, otherSigner.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	wrongClient := wrongKeyServer.newClient(token)
	wrongSyncer, err := adsync.NewSyncer(wrongClient, "")
	if err != nil {
		t.Fatalf("new wrong-key syncer: %v", err)
	}
	if _, err := wrongSyncer.Sync(context.Background(), service.service()); !errors.Is(err, adsync.ErrIntegrity) {
		t.Errorf("wrong-key downstream: %v, want ErrIntegrity", err)
	}
}

func TestDownstreamChangesPagesVerify(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	initial := service.snapshotResponse(t, signer)
	for _, policy := range []string{"A1", "A2", "A3"} {
		service.publish(t, signer, upsertAuthz(testServiceID1+policy, "policy-"+policy, "grant read"))
	}
	if err := mirror.ApplySnapshot(initial); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}

	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	client := server.newClient(token)
	ctx := context.Background()

	first, err := client.Changes(ctx, testServiceID1, service.ID, service.Epoch, 1, initial.CommitHash, 1)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if first.From != 2 || first.To != 2 || first.Head != 4 || first.NextSince != 3 {
		t.Errorf("first page = %+v", first)
	}
	if len(first.Commits) != 1 || first.Commits[0].Signature != service.commits[0].Signature {
		t.Error("first page commit signature changed in transit")
	}
	second, err := client.Changes(ctx, testServiceID1, service.ID, service.Epoch, first.To, first.Commits[0].Hash, 100)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if second.From != 3 || second.To != 4 || second.NextSince != 5 {
		t.Errorf("second page = %+v", second)
	}
	if second.HeadStateHash != service.StateHash {
		t.Error("second page head hash does not match the source")
	}

	if _, err := client.Changes(ctx, testServiceID1, service.ID, service.Epoch, 99, first.Commits[0].Hash, 100); !errors.Is(err, adsync.ErrSnapshotRequired) {
		t.Errorf("changes after head: %v, want ErrSnapshotRequired", err)
	}
	if _, err := client.Changes(ctx, testUnknownID, service.ID, service.Epoch, 1, initial.CommitHash, 100); !errors.Is(err, adsync.ErrServiceNotFound) {
		t.Errorf("unknown service: %v, want ErrServiceNotFound", err)
	}
	if _, _, err := client.Snapshot(ctx, testUnknownID, ""); !errors.Is(err, adsync.ErrServiceNotFound) {
		t.Errorf("unknown snapshot: %v, want ErrServiceNotFound", err)
	}
}

func TestDownstreamRecoversFromEpochChange(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	initial := service.snapshotResponse(t, signer)
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read-books", "grant read"))
	if err := mirror.ApplySnapshot(initial); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}

	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	client := server.newClient(token)
	syncer, err := adsync.NewSyncer(client, "")
	if err != nil {
		t.Fatalf("new syncer: %v", err)
	}
	oldService := service.service()
	if state := syncToHead(t, syncer, oldService); state.Epoch != service.Epoch {
		t.Fatalf("downstream epoch = %q", state.Epoch)
	}

	// The source changed: a new epoch with new content. A downstream that
	// still holds the old epoch must reload the snapshot and land on the new
	// identity.
	replacement := newFixtureServiceWithEpoch(testServiceID1, testUnknownID, "books")
	replacement.publish(t, signer, upsertRole(testServiceID1+"R9", "auditor", "grant role"))
	if err := mirror.ApplySnapshot(replacement.snapshotResponse(t, signer)); err != nil {
		t.Fatalf("apply replacement snapshot: %v", err)
	}

	newService := replacement.service()
	state := syncToHead(t, syncer, newService)
	if state.Epoch != replacement.Epoch || state.Revision != replacement.Revision ||
		state.StateHash != replacement.StateHash {
		t.Errorf("downstream after epoch change = %+v", state)
	}
	if len(state.Snapshot.RolePolicies) != 1 || len(state.Snapshot.Policies) != 0 {
		t.Errorf("downstream snapshot after epoch change = %+v", state.Snapshot)
	}

	// A stale revision from the old epoch can no longer be resumed.
	if _, err := mirror.Changes(testServiceID1, 1, 100); !errors.Is(err, ErrHistoryUnavailable) {
		t.Errorf("old since after epoch change: %v, want ErrHistoryUnavailable", err)
	}

	// The proxy advertises the new identity and version.
	response, body := server.get(adsync.APIPrefix+"/services", token, nil)
	if response.StatusCode != 200 {
		t.Fatalf("services: status %d", response.StatusCode)
	}
	services := decodeBody[adsync.ServicesResponse](t, body)
	if len(services.Services) != 1 || services.Services[0].Epoch != replacement.Epoch {
		t.Errorf("services after epoch change = %+v", services.Services)
	}
}
