// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	observed := &adsync.ServiceState{
		ServiceID:  testServiceID1,
		SourceID:   verified.SourceID,
		Epoch:      verified.Epoch,
		Revision:   verified.Revision + 1,
		StateHash:  adsync.StateHash(next),
		CommitHash: commitHash,
		Snapshot:   next,
	}
	store.observeDownstreamChanges(&adsync.ChangesResponse{
		ServiceID:     testServiceID1,
		SourceID:      verified.SourceID,
		Epoch:         verified.Epoch,
		From:          verified.Revision + 1,
		To:            verified.Revision + 1,
		Head:          verified.Revision + 1,
		HeadStateHash: observed.StateHash,
		Commits: []adsync.Commit{{
			Revision: verified.Revision + 1,
			PrevHash: verified.CommitHash,
			Hash:     commitHash,
			At:       1762500000000,
			Changes:  changes,
		}},
		NextSince: verified.Revision + 2,
	})
	// An observed page is not servable until the pass that fetched it
	// publishes the verified state.
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceNeedsSnapshot {
		t.Errorf("availability after an observed page = %v, want ServiceNeedsSnapshot", availability)
	}
	store.publishDownstream(observed)
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
	// Publishing an older verified state does not clear the withhold: the
	// mirror ignores the stale snapshot, so the pass did not confirm the copy
	// that would be served.
	store.publishDownstream(verified)
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceNeedsSnapshot {
		t.Errorf("availability after a stale publish = %v, want ServiceNeedsSnapshot", availability)
	}
	store.publishDownstream(observed)
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceAvailable {
		t.Errorf("recovered availability = %v", availability)
	}

	// Removing a service stops serving it and keeps the deletion guard.
	store.removeDownstream(testServiceID1)
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceRemoved {
		t.Errorf("removed availability = %v", availability)
	}
}

// downstreamRequest performs one policy-source request through the relay's
// composed handler with a verified client certificate, as the dedicated TLS
// listener would deliver it.
func downstreamRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	certificate := &x509.Certificate{Raw: []byte("downstream-client")}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{certificate},
		VerifiedChains:   [][]*x509.Certificate{{certificate}},
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// TestServeDownstreamDropsServicesRemovedFromTheAuthoritativeSet covers a
// service the source stops assigning to the relay: it must leave the
// downstream mirror, not only the local copies, so a child cannot keep pulling
// policy the source no longer authorizes.
func TestServeDownstreamDropsServicesRemovedFromTheAuthoritativeSet(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()
	if _, availability := store.downstream.mirror.Lookup(testServiceID2); availability != adsproxy.ServiceAvailable {
		t.Fatalf("orders availability before the removal = %v", availability)
	}

	fixture.source.removeService("orders")
	if err := store.Sync(context.Background()); err != nil {
		t.Fatalf("sync after the removal: %v", err)
	}
	if _, availability := store.downstream.mirror.Lookup(testServiceID2); availability != adsproxy.ServiceRemoved {
		t.Errorf("removed service availability = %v, want ServiceRemoved", availability)
	}
	response := downstreamRequest(store.DownstreamHandler(), adsync.APIPrefix+"/services")
	if response.Code != http.StatusOK {
		t.Fatalf("services: status %d", response.Code)
	}
	for _, advertised := range decodeServices(t, response.Body.Bytes()) {
		if advertised.ID == testServiceID2 {
			t.Errorf("a service removed from the authoritative set is still advertised: %+v", advertised)
		}
	}
	if info, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceAvailable || info.Revision == 0 {
		t.Errorf("the remaining service is no longer served: %+v %v", info, availability)
	}
}

// TestServeDownstreamFollowsPauseAndStop covers the Cloud control actions:
// a pause command closes downstream distribution without affecting local
// evaluation, a resume reopens it, and a stop or revocation keeps serving
// nothing.
func TestServeDownstreamFollowsPauseAndStop(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	store.runtimeMu.Lock()
	store.runtime = &distributionRuntime{seenCommands: make(map[string]struct{})}
	store.runtimeMu.Unlock()
	defer func() {
		// The stub runtime has no loops to join, so clear it before Close.
		store.runtimeMu.Lock()
		store.runtime = nil
		store.runtimeMu.Unlock()
		store.Close()
	}()
	handler := store.DownstreamHandler()

	if response := downstreamRequest(handler, adsync.APIPrefix+"/services"); response.Code != http.StatusOK {
		t.Fatalf("services before the command: status %d", response.Code)
	}

	for _, command := range []adsync.Command{
		{ID: "pause-1", Action: "pause"},
		{ID: "resume-1", Action: "resume"},
	} {
		result := store.handleRuntimeCommand(command)
		if result == nil || result.Status != "acked" {
			t.Fatalf("%s command result = %+v", command.Action, result)
		}
		want := http.StatusServiceUnavailable
		if command.Action == "resume" {
			want = http.StatusOK
		}
		if response := downstreamRequest(handler, adsync.APIPrefix+"/services"); response.Code != want {
			t.Errorf("services after %s: status %d body %s", command.Action, response.Code, response.Body.String())
		}
	}

	// A stop command makes the instance stopped, which no longer vouches for
	// the copies it holds.
	if result := store.handleRuntimeCommand(adsync.Command{ID: "stop-1", Action: "stop"}); result == nil || result.Status != "acked" {
		t.Fatalf("stop command result = %+v", result)
	}
	response := downstreamRequest(handler, adsync.APIPrefix+"/services")
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("services while stopped: status %d", response.Code)
	}
	if response := downstreamRequest(handler, adsync.APIPrefix+"/services/"+testServiceID1+"/snapshot"); response.Code != http.StatusServiceUnavailable {
		t.Errorf("snapshot while stopped: status %d", response.Code)
	}
}

// TestServeDownstreamRevocationStopsSnapshotServing covers the other route to
// a stopped instance: the source rejects the credential during a pass.
func TestServeDownstreamRevocationStopsSnapshotServing(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()
	handler := store.DownstreamHandler()
	if response := downstreamRequest(handler, adsync.APIPrefix+"/services/"+testServiceID1+"/snapshot"); response.Code != http.StatusOK {
		t.Fatalf("snapshot before the revocation: status %d", response.Code)
	}

	fixture.source.rejectCredential()
	if err := store.Sync(context.Background()); err == nil {
		t.Fatal("a revoked credential did not fail the pass")
	}
	for _, path := range []string{
		adsync.APIPrefix + "/services",
		adsync.APIPrefix + "/services/" + testServiceID1 + "/snapshot",
	} {
		if response := downstreamRequest(handler, path); response.Code != http.StatusServiceUnavailable {
			t.Errorf("%s after the revocation: status %d", path, response.Code)
		}
	}
}

// TestServeDownstreamNeverExposesAnUnconfirmedCandidate covers a lagging
// source: its snapshot starts below the discovered head and the catch-up
// fails, so the revision the mirror observed must never reach a downstream
// instance. A later successful pass publishes the advertised head.
func TestServeDownstreamNeverExposesAnUnconfirmedCandidate(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()

	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = append(snapshot.Policies, policy("p-carol", "carol", "grant user carol read books"))
	})
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = append(snapshot.Policies, policy("p-dave", "dave", "grant user dave read books"))
	})
	books, known := fixture.source.service("books")
	if !known {
		t.Fatal("the fixture source has no books service")
	}
	head, laggingRevision := books.Revision, books.Revision-1
	fixture.source.serveLaggingSnapshot(true)
	fixture.source.holdSnapshots(testServiceID1)
	before := fixture.source.requestCount()
	syncDone := make(chan error, 1)
	go func() { syncDone <- store.Sync(context.Background()) }()
	// The discovery request and the held snapshot request both reach the
	// source; the fetch is in flight once the second one arrived.
	waitForCondition(t, 5*time.Second, "the snapshot fetch to be in flight", func() bool {
		return fixture.source.requestCount() >= before+2
	})
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability == adsproxy.ServiceAvailable {
		t.Fatal("the mirror kept serving a copy while a newer head was being fetched")
	}
	fixture.source.releaseSnapshots(testServiceID1)
	if err := <-syncDone; err == nil {
		t.Fatal("a lagging snapshot with no usable change log did not fail the pass")
	}

	// The candidate was applied internally but stays invisible.
	info, availability := store.downstream.mirror.Lookup(testServiceID1)
	if availability == adsproxy.ServiceAvailable {
		t.Fatalf("the unconfirmed candidate is served: %+v %v", info, availability)
	}
	candidate, current := store.downstream.mirror.CurrentInfo(testServiceID1)
	if !current || candidate.Revision != laggingRevision {
		t.Errorf("the observed candidate revision = %+v %v, want revision %d", candidate, current, laggingRevision)
	}
	if services := store.downstream.mirror.Services(); len(services) != 0 {
		t.Errorf("advertised services while unconfirmed = %+v", services)
	}

	// Once the source stops lagging, the pass reaches the advertised head and
	// the service becomes servable again.
	fixture.source.serveLaggingSnapshot(false)
	if err := store.Sync(context.Background()); err != nil {
		t.Fatalf("sync after the source caught up: %v", err)
	}
	info, availability = store.downstream.mirror.Lookup(testServiceID1)
	if availability != adsproxy.ServiceAvailable || info.Revision != head {
		t.Fatalf("published copy = %+v, availability %v", info, availability)
	}
}

// TestServeDownstreamWithholdsForANotifiedHead covers a notification that
// announces a head newer than the last ack. The synchronizer catches up from
// the acknowledged revision and applies change pages the mirror must not serve
// until the whole pass verifies the notified head.
func TestServeDownstreamWithholdsForANotifiedHead(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()
	books, known := fixture.source.service("books")
	if !known {
		t.Fatal("the fixture source has no books service")
	}
	notified := books.Revision + 2
	if err := store.machine.ObserveHead(adsync.Head{
		ID: testServiceID1, Epoch: books.Epoch, Revision: notified,
	}); err != nil {
		t.Fatalf("observe head: %v", err)
	}
	fixture.source.holdChanges(testServiceID1)
	before := fixture.source.requestCount()
	syncDone := make(chan error, 1)
	go func() { syncDone <- store.Sync(context.Background()) }()
	// The discovery request and the held change-page request both reach the
	// source; the catch-up is in flight once the second one arrived.
	waitForCondition(t, 5*time.Second, "the catch-up to be in flight", func() bool {
		return fixture.source.requestCount() >= before+2
	})
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability == adsproxy.ServiceAvailable {
		t.Fatal("the mirror kept serving the acknowledged revision while a notified head was being caught up")
	}
	fixture.source.releaseChanges(testServiceID1)
	if err := <-syncDone; err == nil {
		t.Fatal("a missing change log did not fail the pass")
	}
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability == adsproxy.ServiceAvailable {
		t.Fatal("the mirror recovered without a verified pass")
	}
}

// TestServeDownstreamWithholdsAnObservedCopyThatFailsCompilation covers a
// fetch that starts while the mirror already serves the acknowledged head: the
// source advances during the fetch, the observer applies the candidate, and
// the pass then rejects it. The unconfirmed candidate must never be served.
func TestServeDownstreamWithholdsAnObservedCopyThatFailsCompilation(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()
	books, known := fixture.source.service("books")
	if !known {
		t.Fatal("the fixture source has no books service")
	}
	ackService := adsync.Service{
		ID:            testServiceID1,
		Name:          "books",
		Revision:      books.Revision,
		Epoch:         books.Epoch,
		HeadStateHash: books.StateHash,
	}
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = append(snapshot.Policies,
			adsync.Policy{ID: "p-invalid", Name: "invalid", SPDL: "this is not a policy"})
	})
	// Dropping the local copy forces a fetch even though the mirror already
	// matches the acknowledged head, which is the window this test covers.
	if err := store.syncer.Forget(testServiceID1); err != nil {
		t.Fatalf("forget local copy: %v", err)
	}
	if err := store.syncService(context.Background(), ackService); err == nil {
		t.Fatal("a copy that fails compilation did not fail the sync")
	}
	// The observer did apply the candidate internally; only the withhold keeps
	// it away from downstream instances.
	candidate, current := store.downstream.mirror.CurrentInfo(testServiceID1)
	if !current || candidate.Revision != ackService.Revision+1 {
		t.Fatalf("the observed candidate = %+v %v, want revision %d", candidate, current, ackService.Revision+1)
	}
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability == adsproxy.ServiceAvailable {
		t.Fatal("the rejected candidate is served downstream")
	}
}

// TestObservedCandidatesStayWithheldUntilPublished covers the window inside a
// pass: a snapshot or change page the synchronizer observes must not be
// servable before the pass publishes the verified state, even though the
// mirror served the previous head when the fetch started.
func TestObservedCandidatesStayWithheldUntilPublished(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()
	verified, found := store.syncer.State(testServiceID1)
	if !found {
		t.Fatal("the store has no verified state")
	}
	next := adsync.Snapshot{
		Name:         verified.Snapshot.Name,
		Policies:     append([]adsync.Policy(nil), verified.Snapshot.Policies...),
		RolePolicies: append([]adsync.Policy(nil), verified.Snapshot.RolePolicies...),
	}
	next.Policies = append(next.Policies, policy("p-audit", "audit", "grant user auditor read books"))
	nextHash := adsync.StateHash(next)
	store.observeDownstreamSnapshot(&adsync.SnapshotResponse{
		ServiceID:  testServiceID1,
		SourceID:   verified.SourceID,
		Epoch:      verified.Epoch,
		Revision:   verified.Revision + 1,
		StateHash:  nextHash,
		CommitHash: nextHash,
		Snapshot:   next,
	})
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceNeedsSnapshot {
		t.Fatalf("observed candidate availability = %v, want ServiceNeedsSnapshot", availability)
	}
	candidate, current := store.downstream.mirror.CurrentInfo(testServiceID1)
	if !current || candidate.Revision != verified.Revision+1 {
		t.Fatalf("observed candidate = %+v %v", candidate, current)
	}
	store.publishDownstream(&adsync.ServiceState{
		ServiceID:  testServiceID1,
		SourceID:   verified.SourceID,
		Epoch:      verified.Epoch,
		Revision:   verified.Revision + 1,
		StateHash:  nextHash,
		CommitHash: nextHash,
		Snapshot:   next,
	})
	info, availability := store.downstream.mirror.Lookup(testServiceID1)
	if availability != adsproxy.ServiceAvailable || info.Revision != verified.Revision+1 {
		t.Fatalf("published candidate = %+v, availability %v", info, availability)
	}
}

// TestPublishDownstreamRefusesADifferentCommitHistory covers a source that
// serves the same revision and state hash with another commit history. The
// mirror rejects the second history, so the withhold must stay instead of
// clearing it for the copy the mirror still holds.
func TestPublishDownstreamRefusesADifferentCommitHistory(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()
	verified, found := store.syncer.State(testServiceID1)
	if !found {
		t.Fatal("the store has no verified state")
	}
	other := *verified
	other.CommitHash[0] ^= 0xff
	store.withholdDownstream(testServiceID1)
	store.publishDownstream(&other)
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability == adsproxy.ServiceAvailable {
		t.Fatal("a copy with a different commit history was published")
	}
	store.publishDownstream(verified)
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability != adsproxy.ServiceAvailable {
		t.Fatal("the verified state did not publish")
	}
}

// TestServeDownstreamDropsServicesRemovedByAChannelAck covers the runtime
// removal path: an ack that drops a service must remove it from the mirror
// immediately, before the next discovery pass.
func TestServeDownstreamDropsServicesRemovedByAChannelAck(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()
	if _, availability := store.downstream.mirror.Lookup(testServiceID2); availability != adsproxy.ServiceAvailable {
		t.Fatalf("orders availability before the ack = %v", availability)
	}
	store.applyServiceSetChange(adsync.ServiceSetChange{
		Removed: []adsync.Service{{ID: testServiceID2, Name: "orders"}},
	})
	if _, availability := store.downstream.mirror.Lookup(testServiceID2); availability != adsproxy.ServiceRemoved {
		t.Fatalf("orders availability after the ack = %v, want ServiceRemoved", availability)
	}
	if services := store.downstream.mirror.Services(); len(services) != 1 || services[0].ID != testServiceID1 {
		t.Fatalf("advertised services after the ack = %+v", services)
	}
}

// TestInstallFetchedDoesNotReviveARemovedService covers a channel ack that
// removes a service while its fetch is in flight: the fetched copy must not
// revive a service the source no longer assigns to this relay.
func TestInstallFetchedDoesNotReviveARemovedService(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.ServeDownstream = true })
	defer store.Close()
	verified, found := store.syncer.State(testServiceID1)
	if !found {
		t.Fatal("the store has no verified state")
	}
	next := adsync.Snapshot{
		Name:         verified.Snapshot.Name,
		Policies:     append([]adsync.Policy(nil), verified.Snapshot.Policies...),
		RolePolicies: append([]adsync.Policy(nil), verified.Snapshot.RolePolicies...),
	}
	next.Policies = append(next.Policies, policy("p-audit", "audit", "grant user auditor read books"))
	nextHash := adsync.StateHash(next)
	service := adsync.Service{
		ID:            testServiceID1,
		Name:          "books",
		Revision:      verified.Revision,
		Epoch:         verified.Epoch,
		HeadStateHash: verified.StateHash,
	}
	attempt := store.machine.BeginSync(testServiceID1)
	store.applyServiceSetChange(adsync.ServiceSetChange{
		Removed: []adsync.Service{{ID: testServiceID1, Name: "books"}},
	})
	if err := store.installFetched(service, &adsync.ServiceState{
		ServiceID:  testServiceID1,
		SourceID:   verified.SourceID,
		Epoch:      verified.Epoch,
		Revision:   verified.Revision + 1,
		StateHash:  nextHash,
		CommitHash: nextHash,
		Snapshot:   next,
	}, attempt); err != nil {
		t.Fatalf("install fetched copy: %v", err)
	}
	if _, availability := store.downstream.mirror.Lookup(testServiceID1); availability == adsproxy.ServiceAvailable {
		t.Fatal("a removed service was revived by an in-flight publish")
	}
}

// waitForCondition polls a condition until it holds or the timeout expires.
func waitForCondition(t *testing.T, timeout time.Duration, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

// decodeServices decodes a services response body.
func decodeServices(t *testing.T, body []byte) []adsync.Service {
	t.Helper()
	var response adsync.ServicesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode services %s: %v", body, err)
	}
	return response.Services
}
