// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

// These tests, like the existing log-capture tests, must not run in parallel:
// they temporarily configure the package's process-wide logger and limiter.
func captureRevisionWarnings(t *testing.T) (*bytes.Buffer, *fakeClock) {
	t.Helper()
	clock := newFakeClock()
	revisionWarnings.mu.Lock()
	oldStart, oldEmitted, oldSuppressed := revisionWarnings.windowStart, revisionWarnings.emitted, revisionWarnings.suppressed
	oldClock, oldLast, oldCleanup := revisionWarnings.clock, revisionWarnings.last, revisionWarnings.lastCleanup
	revisionWarnings.clock, revisionWarnings.last, revisionWarnings.lastCleanup = clock, nil, time.Time{}
	revisionWarnings.windowStart, revisionWarnings.emitted, revisionWarnings.suppressed = time.Time{}, 0, 0
	revisionWarnings.mu.Unlock()
	logger := log.StandardLogger()
	out, formatter, level := logger.Out, logger.Formatter, logger.GetLevel()
	buffer := new(bytes.Buffer)
	logger.SetOutput(buffer)
	logger.SetFormatter(&log.JSONFormatter{})
	logger.SetLevel(log.WarnLevel)
	t.Cleanup(func() {
		logger.SetOutput(out)
		logger.SetFormatter(formatter)
		logger.SetLevel(level)
		revisionWarnings.mu.Lock()
		revisionWarnings.clock, revisionWarnings.last, revisionWarnings.lastCleanup = oldClock, oldLast, oldCleanup
		revisionWarnings.windowStart, revisionWarnings.emitted, revisionWarnings.suppressed = oldStart, oldEmitted, oldSuppressed
		revisionWarnings.mu.Unlock()
	})
	return buffer, clock
}

func requireRevisionWarning(t *testing.T, buffer *bytes.Buffer, id, epoch, source string, upstream, local uint64, localSources ...string) {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buffer.Bytes()), &entry); err != nil {
		t.Fatalf("expected exactly one JSON warning, got %q: %v", buffer.String(), err)
	}
	localSource := "memory"
	if local == 0 {
		localSource = "none"
	}
	if len(localSources) != 0 {
		localSource = localSources[0]
	}
	for field, want := range map[string]any{
		"msg":   "policy-source revision is below the local copy or known head in the same epoch; ignoring rollback",
		"level": "warning", "service_id": id, "epoch": epoch, "source": source,
		"upstream_revision": float64(upstream), "local_revision": float64(local),
		"local_source": localSource,
	} {
		if entry[field] != want {
			t.Errorf("warning %s = %v, want %v", field, entry[field], want)
		}
	}
}

func TestLowerHeadRevisionWarnsWithoutRollback(t *testing.T) {
	for _, source := range []string{"ack", "notification", "discovery", "notification without epoch"} {
		t.Run(source, func(t *testing.T) {
			buffer, _ := captureRevisionWarnings(t)
			machine, _ := newTestMachine(t)
			mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
			mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
			for range 3 {
				switch source {
				case "ack":
					mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 4)))
				case "discovery":
					if err := machine.Discover([]Service{testAckService(testServiceID1, "books", 4)}); err != nil {
						t.Fatal(err)
					}
				default:
					epoch := testServiceID1
					if source == "notification without epoch" {
						epoch = ""
					}
					if err := machine.ObserveHead(Head{ID: testServiceID1, Epoch: epoch, Revision: 4}); err != nil {
						t.Fatal(err)
					}
				}
			}
			status := requireServiceState(t, machine, "books", SyncStateInSync, "")
			if status.Head != 5 || status.Revision != 5 {
				t.Fatalf("head/local revision = %d/%d, want 5/5", status.Head, status.Revision)
			}
			if source == "notification without epoch" {
				source = "notification"
			}
			requireRevisionWarning(t, buffer, testServiceID1, testServiceID1, source, 4, 5)
		})
	}
}

func TestLowerAckWarnsForLocalCopyBeforeFirstHead(t *testing.T) {
	buffer, _ := captureRevisionWarnings(t)
	machine, _ := newTestMachine(t)
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 4)))
	requireRevisionWarning(t, buffer, testServiceID1, testServiceID1, "ack", 4, 5)
}

func TestLowerHeadWarningReportsAppliedRevision(t *testing.T) {
	for _, localRevision := range []uint64{0, 3, 8} {
		t.Run(fmt.Sprint(localRevision), func(t *testing.T) {
			buffer, _ := captureRevisionWarnings(t)
			machine, _ := newTestMachine(t)
			mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
			if localRevision != 0 {
				mustApply(t, machine, testServiceState(testServiceID1, "books", localRevision))
			}
			mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 4)))
			requireRevisionWarning(t, buffer, testServiceID1, testServiceID1, "ack", 4, localRevision)
			var entry map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(buffer.Bytes()), &entry); err != nil {
				t.Fatal(err)
			}
			if entry["known_head_revision"] != float64(5) {
				t.Fatalf("known_head_revision = %v, want 5", entry["known_head_revision"])
			}
		})
	}
}

func TestLowerDiscoveryRevisionWarnsForDiskCacheOnStartup(t *testing.T) {
	buffer, _ := captureRevisionWarnings(t)
	service := newFixtureService(testServiceID1, "books")
	staleService := newFixtureService(testServiceID1, "books")
	for range 4 {
		service.publish(t, Change{Op: "upsert", Kind: "service", ID: service.ID, Name: stringPointer("books")})
	}
	fixture := newFixture(t, service)
	cacheDir := t.TempDir()
	initial := syncFixtureService(t, fixture.newSyncer(cacheDir, nil), "books")
	fixture.source.configure(func(source *fixtureSource) { source.services[service.ID] = staleService })
	// A restarted syncer knows only the disk cache; no machine has received
	// its newer revision. It still warns and keeps the cached copy.
	syncer := fixture.newSyncer(cacheDir, nil)
	if _, found := syncer.State(service.ID); found {
		t.Fatal("new syncer already holds an in-memory copy")
	}
	for range 3 {
		state := syncFixtureService(t, syncer, "books")
		if !reflect.DeepEqual(state, initial) {
			t.Fatalf("lower discovery revision replaced disk-cache copy: %+v", state)
		}
	}
	if count := fixture.requestCount(service.ID + "/snapshot"); count != 1 {
		t.Fatalf("snapshots = %d, want only the initial snapshot", count)
	}
	requireRevisionWarning(t, buffer, service.ID, service.Epoch, "discovery", 1, 5, "cache")
}

func TestNotifiedCatchUpDoesNotWarnForReusedDiscovery(t *testing.T) {
	buffer, _ := captureRevisionWarnings(t)
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	syncer := fixture.newSyncer("", nil)
	syncFixtureService(t, syncer, "books")
	discovery, err := syncer.Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fixture.source.configure(func(source *fixtureSource) {
		service.publish(t, Change{Op: "upsert", Kind: "service", ID: service.ID, Name: stringPointer("books")})
	})
	// A notification advances the known head to 2, while discovery still
	// carries 1. The first call catches up; later calls reuse that old result.
	for range 3 {
		state, err := syncer.SyncAtLeast(context.Background(), discovery.Services[0], 2)
		if err != nil {
			t.Fatal(err)
		}
		if state.Revision != 2 {
			t.Fatalf("notified head = %d, want 2", state.Revision)
		}
	}
	// Plain Sync does not carry minRevision and may reuse the same result.
	if _, err := syncer.Sync(context.Background(), discovery.Services[0]); err != nil {
		t.Fatal(err)
	}
	if buffer.Len() != 0 {
		t.Fatalf("reusing discovery after notified catch-up warned: %s", buffer.String())
	}
	// SyncByName instead reads a fresh upstream result. A newly reported
	// lower revision must warn, even if the local copy advanced by notification.
	fixture.source.configure(func(source *fixtureSource) {
		source.services[service.ID] = newFixtureService(service.ID, "books")
	})
	if _, err := syncer.SyncByName(context.Background(), "books"); err != nil {
		t.Fatal(err)
	}
	requireRevisionWarning(t, buffer, service.ID, service.Epoch, "discovery", 1, 2)
}

func TestLowerCachedRevisionWarningUsesNotifiedHead(t *testing.T) {
	for _, notified := range []uint64{2, 5, 6} {
		t.Run(fmt.Sprint(notified), func(t *testing.T) {
			buffer, _ := captureRevisionWarnings(t)
			service := newFixtureService(testServiceID1, "books")
			fixture := newFixture(t, service)
			cacheDir := t.TempDir()
			syncer := fixture.newSyncer(cacheDir, nil)
			local := syncFixtureService(t, syncer, "books")
			local.Revision = 5
			if err := syncer.store(local); err != nil {
				t.Fatal(err)
			}
			syncer = fixture.newSyncer(cacheDir, nil)
			state, err := syncer.SyncAtLeast(context.Background(), testAckService(service.ID, "books", 1), notified)
			if err != nil || state.Revision != 5 {
				t.Fatalf("cached copy changed: %+v, error %v", state, err)
			}
			if notified < 5 {
				requireRevisionWarning(t, buffer, service.ID, service.Epoch, "discovery", 1, 5, "cache")
				var entry map[string]any
				if err := json.Unmarshal(buffer.Bytes(), &entry); err != nil {
					t.Fatal(err)
				}
				if entry["known_head_revision"] != float64(notified) {
					t.Fatalf("known head = %v, want %d", entry["known_head_revision"], notified)
				}
			} else if buffer.Len() != 0 {
				t.Fatalf("notification already accounts for the cached revision: %s", buffer.String())
			}
		})
	}
}

func TestLowerSnapshotRevisionWarnsAndKeepsProtocolError(t *testing.T) {
	for _, localSource := range []string{"memory", "cache"} {
		t.Run(localSource, func(t *testing.T) {
			buffer, _ := captureRevisionWarnings(t)
			service := newFixtureService(testServiceID1, "books")
			fixture := newFixture(t, service)
			cacheDir := t.TempDir()
			syncer := fixture.newSyncer(cacheDir, nil)
			initial := syncFixtureService(t, syncer, "books")
			// Force a snapshot reload with a higher discovered revision; the valid
			// snapshot is now below the retained local copy in the same epoch.
			local := initial.clone()
			local.Revision = 5
			if err := syncer.store(local); err != nil {
				t.Fatal(err)
			}
			if localSource == "cache" {
				syncer = fixture.newSyncer(cacheDir, nil)
			}
			fixture.source.configure(func(source *fixtureSource) {
				service.Revision = 6
				response := fixtureSnapshot(service)
				response.Revision = 4
				source.snapshotOverride = response
				source.changesReason = "history_unavailable"
			})
			for range 3 {
				_, err := syncer.Sync(context.Background(), testAckService(testServiceID1, "books", 6))
				want := fmt.Sprintf("%v: snapshot revision 4 would roll back local revision 5", ErrProtocol)
				if !errors.Is(err, ErrProtocol) || err.Error() != want {
					t.Fatalf("Sync error = %v, want %q", err, want)
				}
			}
			current, found := syncer.State(service.ID)
			if (localSource == "cache" && found) || (localSource == "memory" && (!found || !reflect.DeepEqual(current, local))) {
				t.Fatalf("snapshot rollback changed the in-memory copy: %+v", current)
			}
			cached, err := syncer.cache.Load(service.ID)
			if err != nil || !reflect.DeepEqual(cached, local) {
				t.Fatalf("snapshot rollback changed disk cache: %+v, error %v", cached, err)
			}
			requireRevisionWarning(t, buffer, service.ID, service.Epoch, "snapshot", 4, 5, localSource)
		})
	}
}

func TestEpochChangeDoesNotWarn(t *testing.T) {
	for _, source := range []string{"ack", "notification", "snapshot"} {
		t.Run(source, func(t *testing.T) {
			buffer, _ := captureRevisionWarnings(t)
			if source == "snapshot" {
				service := newFixtureService(testServiceID1, "books")
				fixture := newFixture(t, service)
				syncer := fixture.newSyncer("", nil)
				initial := syncFixtureService(t, syncer, "books")
				initial.Epoch = testServiceID2
				initial.Revision = 5
				if err := syncer.store(initial); err != nil {
					t.Fatal(err)
				}
				state := syncFixtureService(t, syncer, "books")
				if state.Epoch != service.Epoch || state.Revision != 1 {
					t.Fatalf("epoch change did not replace copy: %+v", state)
				}
			} else {
				machine, _ := newTestMachine(t)
				mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
				mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
				service := testAckService(testServiceID1, "books", 1)
				service.Epoch = testServiceID2
				if source == "ack" {
					mustAck(t, machine, ackWith(service))
				} else if err := machine.ObserveHead(Head{ID: service.ID, Epoch: service.Epoch, Revision: 1}); err != nil {
					t.Fatal(err)
				}
				status, _ := machine.Status("books")
				if status.Service.Head != 1 || status.Service.Revision != 0 {
					t.Fatalf("epoch change did not replace head: %+v", status.Service)
				}
			}
			if buffer.Len() != 0 {
				t.Fatalf("epoch change warned: %s", buffer.String())
			}
		})
	}
}

func TestRevisionWarningBudgetSharedAcrossSourcesAndServicesIndependent(t *testing.T) {
	buffer, clock := captureRevisionWarnings(t)
	machine, _ := newTestMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5), testAckService(testServiceID2, "orders", 5)))
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 4), testAckService(testServiceID2, "orders", 5)))
	requireRevisionWarning(t, buffer, testServiceID1, testServiceID1, "ack", 4, 0)
	buffer.Reset()
	// The issue specifies a per-service budget, so an epoch change does not
	// create another budget for the same service within that minute.
	revisionWarnings.warn(revisionWarning{serviceID: testServiceID1, epoch: testServiceID2, upstream: 3, local: 5, head: 5, source: "ack", localSource: "memory"})
	if buffer.Len() != 0 {
		t.Fatalf("epoch change reset the service warning budget: %s", buffer.String())
	}
	clock.Advance(time.Minute - time.Nanosecond)
	if err := machine.ObserveHead(Head{ID: testServiceID1, Epoch: testServiceID1, Revision: 3}); err != nil {
		t.Fatal(err)
	}
	revisionWarnings.warn(revisionWarning{serviceID: testServiceID1, epoch: testServiceID1, upstream: 3, local: 5, head: 5, source: "snapshot", localSource: "memory"})
	if buffer.Len() != 0 {
		t.Fatalf("same service warned twice within a minute: %s", buffer.String())
	}
	if err := machine.ObserveHead(Head{ID: testServiceID2, Epoch: testServiceID2, Revision: 3}); err != nil {
		t.Fatal(err)
	}
	requireRevisionWarning(t, buffer, testServiceID2, testServiceID2, "notification", 3, 0)
	buffer.Reset()
	clock.Advance(time.Nanosecond)
	revisionWarnings.warn(revisionWarning{serviceID: testServiceID1, epoch: testServiceID1, upstream: 3, local: 5, head: 5, source: "snapshot", localSource: "memory"})
	requireRevisionWarning(t, buffer, testServiceID1, testServiceID1, "snapshot", 3, 5)
	clock.Advance(time.Minute)
	revisionWarnings.warn(revisionWarning{serviceID: testServiceID1, epoch: testServiceID1, upstream: 3, local: 5, head: 5, source: "snapshot", localSource: "memory"})
	revisionWarnings.mu.Lock()
	defer revisionWarnings.mu.Unlock()
	if len(revisionWarnings.last) != 1 {
		t.Fatalf("expired service warning entries retained: %v", revisionWarnings.last)
	}
}

func TestConcurrentRevisionWarningsAreRateLimited(t *testing.T) {
	buffer, _ := captureRevisionWarnings(t)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			revisionWarnings.warn(revisionWarning{serviceID: testServiceID1, epoch: testServiceID1, upstream: 4, local: 5, head: 5, source: "snapshot", localSource: "memory"})
		})
	}
	wg.Wait()
	requireRevisionWarning(t, buffer, testServiceID1, testServiceID1, "snapshot", 4, 5)
}

func requireSuppressedRevisionWarnings(t *testing.T, buffer *bytes.Buffer, want uint64) {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buffer.Bytes()), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["suppressed_since_last"] != float64(want) {
		t.Fatalf("suppressed_since_last = %v, want %d", entry["suppressed_since_last"], want)
	}
}

func TestRevisionWarningSuppressedCountSurvivesCleanup(t *testing.T) {
	buffer, clock := captureRevisionWarnings(t)
	warning := revisionWarning{serviceID: testServiceID1, epoch: testServiceID1,
		upstream: 3, local: 5, head: 5, source: "ack", localSource: "memory"}
	warning.log()
	requireSuppressedRevisionWarnings(t, buffer, 0)
	buffer.Reset()
	for _, source := range []string{"notification", "snapshot", "discovery"} {
		warning.source = source
		warning.log()
	}
	if buffer.Len() != 0 {
		t.Fatalf("same-service warnings escaped suppression: %s", buffer.String())
	}
	// Cleanup of the per-service entries must not discard the pending count.
	clock.Advance(2 * time.Minute)
	warning.log()
	requireSuppressedRevisionWarnings(t, buffer, 3)
	buffer.Reset()
	clock.Advance(time.Minute)
	warning.log()
	requireSuppressedRevisionWarnings(t, buffer, 0)
}

func TestBulkRevisionWarningsHaveGlobalBudget(t *testing.T) {
	for _, source := range []string{"ack", "discovery"} {
		t.Run(source, func(t *testing.T) {
			buffer, clock := captureRevisionWarnings(t)
			machine, _ := newTestMachine(t)
			services := make([]Service, 100)
			for i := range services {
				id := fmt.Sprintf("%08x-0000-7000-8000-000000000000", i+1)
				services[i] = testAckService(id, fmt.Sprintf("service-%d", i), 5)
			}
			mustAck(t, machine, ackWith(services...))
			for i := range services {
				services[i].Revision = 4
			}
			if source == "ack" {
				mustAck(t, machine, ackWith(services...))
			} else if err := machine.Discover(services); err != nil {
				t.Fatal(err)
			}
			if count := bytes.Count(buffer.Bytes(), []byte("\n")); count != revisionWarningBurst {
				t.Fatalf("bulk %s emitted %d warnings, want %d", source, count, revisionWarningBurst)
			}
			for _, service := range services {
				status, found := machine.Status(service.Name)
				if !found || status.Service.Head != 5 {
					t.Fatalf("warning budget changed head: %+v", status)
				}
			}
			buffer.Reset()
			// A different caller and service still share the process budget.
			revisionWarnings.warn(revisionWarning{serviceID: testServiceID2, source: "snapshot"})
			if buffer.Len() != 0 {
				t.Fatalf("another source escaped the global budget: %s", buffer.String())
			}
			clock.Advance(time.Minute)
			revisionWarnings.warn(revisionWarning{serviceID: services[0].ID, source: source})
			requireSuppressedRevisionWarnings(t, buffer, uint64(len(services)-revisionWarningBurst+1))
		})
	}
}

func TestConcurrentServicesShareRevisionWarningBudget(t *testing.T) {
	for _, zeroClock := range []bool{false, true} {
		t.Run(fmt.Sprintf("zero-clock=%v", zeroClock), func(t *testing.T) {
			buffer, clock := captureRevisionWarnings(t)
			if zeroClock {
				clock.now = time.Time{}
			}
			var wg sync.WaitGroup
			for i := range 100 {
				wg.Go(func() { revisionWarnings.warn(revisionWarning{serviceID: fmt.Sprint(i)}) })
			}
			wg.Wait()
			if count := bytes.Count(buffer.Bytes(), []byte("\n")); count != revisionWarningBurst {
				t.Fatalf("concurrent services emitted %d warnings, want %d", count, revisionWarningBurst)
			}
			buffer.Reset()
			clock.Advance(time.Minute)
			revisionWarnings.warn(revisionWarning{serviceID: testServiceID1})
			requireSuppressedRevisionWarnings(t, buffer, 100-revisionWarningBurst)
		})
	}
}

type blockedRevisionLog struct {
	started chan struct{}
	release chan struct{}
}

func (w *blockedRevisionLog) Write(p []byte) (int, error) {
	close(w.started)
	<-w.release
	return len(p), nil
}

func TestRevisionWarningDoesNotHoldMachineLock(t *testing.T) {
	for _, source := range []string{"ack", "discovery", "notification"} {
		t.Run(source, func(t *testing.T) {
			captureRevisionWarnings(t)
			machine, _ := newTestMachine(t)
			mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
			writer := &blockedRevisionLog{started: make(chan struct{}), release: make(chan struct{})}
			log.SetOutput(writer)
			done := make(chan error, 1)
			t.Cleanup(func() {
				close(writer.release)
				if err := <-done; err != nil {
					t.Error(err)
				}
			})
			go func() {
				switch source {
				case "ack":
					done <- machine.Ack(ackWith(testAckService(testServiceID1, "books", 4)))
				case "discovery":
					done <- machine.Discover([]Service{testAckService(testServiceID1, "books", 4)})
				default:
					done <- machine.ObserveHead(Head{ID: testServiceID1, Epoch: testServiceID1, Revision: 4})
				}
			}()
			select {
			case <-writer.started:
			case <-time.After(5 * time.Second):
				t.Fatal("warning did not reach the blocked sink")
			}
			statusDone := make(chan Status, 1)
			go func() { status, _ := machine.Status("books"); statusDone <- status }()
			select {
			case status := <-statusDone:
				if status.Service.Head != 5 {
					t.Fatalf("head = %d, want 5", status.Service.Head)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("blocked log sink held the machine lock")
			}
		})
	}
}

func TestRevisionWarningDoesNotHoldSyncLock(t *testing.T) {
	for _, source := range []string{"cache", "snapshot"} {
		t.Run(source, func(t *testing.T) {
			captureRevisionWarnings(t)
			service := newFixtureService(testServiceID1, "books")
			fixture := newFixture(t, service)
			cacheDir := ""
			if source == "cache" {
				cacheDir = t.TempDir()
			}
			syncer := fixture.newSyncer(cacheDir, nil)
			local := syncFixtureService(t, syncer, "books")
			local.Revision = 5
			if err := syncer.store(local); err != nil {
				t.Fatal(err)
			}
			revision := uint64(1)
			if source == "cache" {
				syncer = fixture.newSyncer(cacheDir, nil)
			} else {
				revision = 6
				fixture.source.configure(func(source *fixtureSource) {
					response := fixtureSnapshot(service)
					response.Revision = 4
					source.snapshotOverride = response
					source.changesReason = "history_unavailable"
				})
			}
			writer := &blockedRevisionLog{started: make(chan struct{}), release: make(chan struct{})}
			log.SetOutput(writer)
			done := make(chan error, 1)
			t.Cleanup(func() {
				close(writer.release)
				err := <-done
				if source == "snapshot" {
					if !errors.Is(err, ErrProtocol) {
						t.Errorf("Sync error = %v, want ErrProtocol", err)
					}
				} else if err != nil {
					t.Error(err)
				}
			})
			go func() {
				_, err := syncer.Sync(context.Background(), testAckService(service.ID, "books", revision))
				done <- err
			}()
			select {
			case <-writer.started:
			case <-time.After(5 * time.Second):
				t.Fatal("warning did not reach the blocked sink")
			}
			forgotten := make(chan error, 1)
			go func() { forgotten <- syncer.Forget(service.ID) }()
			select {
			case err := <-forgotten:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("blocked log sink held the service sync lock")
			}
		})
	}
}
