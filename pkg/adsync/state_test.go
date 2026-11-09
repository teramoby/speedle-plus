// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

const testServiceID3 = "01HF7YAT000000000000000103"

type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, time.November, 8, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(duration time.Duration) { c.now = c.now.Add(duration) }

func newTestMachine(t *testing.T) (*Machine, *fakeClock) {
	t.Helper()
	clock := newFakeClock()
	machine, err := NewMachine(MachineOptions{Clock: clock})
	if err != nil {
		t.Fatalf("NewMachine: %v", err)
	}
	return machine, clock
}

// testServiceState builds a verified copy whose state hash depends on the
// revision, so a copy at the wrong revision is detectable.
func testServiceState(id, name string, revision uint64) *ServiceState {
	snapshot := Snapshot{Name: name, Policies: []Policy{}, RolePolicies: []Policy{}}
	if revision > 0 {
		snapshot.Policies = []Policy{{
			ID:   fmt.Sprintf("policy-%d", revision),
			Name: fmt.Sprintf("policy-%d", revision),
			SPDL: "grant read",
		}}
	}
	hash := StateHash(snapshot)
	return &ServiceState{
		ServiceID:  id,
		SourceID:   id,
		Epoch:      id,
		Revision:   revision,
		StateHash:  hash,
		CommitHash: hash,
		Snapshot:   snapshot,
	}
}

func testAckService(id, name string, revision uint64) Service {
	return Service{
		ID:            id,
		Name:          name,
		Revision:      revision,
		Epoch:         id,
		HeadStateHash: testServiceState(id, name, revision).StateHash,
	}
}

// ackWith always returns a confirmed ack, even with no services.
func ackWith(services ...Service) Ack {
	return Ack{Services: append([]Service{}, services...)}
}

func mustAck(t *testing.T, machine *Machine, ack Ack) {
	t.Helper()
	if err := machine.Ack(ack); err != nil {
		t.Fatalf("Ack: %v", err)
	}
}

func mustApply(t *testing.T, machine *Machine, state *ServiceState) {
	t.Helper()
	if err := machine.Applied(machine.BeginSync(state.ServiceID), state); err != nil {
		t.Fatalf("Applied: %v", err)
	}
}

func requireLiveness(t *testing.T, machine *Machine, want Liveness) InstanceStatus {
	t.Helper()
	status := machine.Instance()
	if status.Liveness != want {
		t.Fatalf("liveness = %q (reason %q), want %q", status.Liveness, status.Reason, want)
	}
	return status
}

func requireServiceState(t *testing.T, machine *Machine, name string, want SyncState, wantReason SyncReason) ServiceStatus {
	t.Helper()
	status, known := machine.Status(name)
	if !known {
		t.Fatalf("Status(%q): service is not known", name)
	}
	if status.Service.State != want || status.Service.Reason != wantReason {
		t.Fatalf("service %q = %q (%q), want %q (%q)",
			name, status.Service.State, status.Service.Reason, want, wantReason)
	}
	return status.Service
}

func TestLivenessThresholds(t *testing.T) {
	cases := []struct {
		name   string
		age    time.Duration
		want   Liveness
		reason Reason
	}{
		{name: "just confirmed", age: 0, want: LivenessLive},
		{name: "one heartbeat later", age: 30 * time.Second, want: LivenessLive},
		{name: "at the warning threshold", age: 60 * time.Second, want: LivenessLive},
		{name: "just past the warning threshold", age: 60*time.Second + time.Nanosecond, want: LivenessStale},
		{name: "at the stop threshold", age: 120 * time.Second, want: LivenessStale},
		{name: "just past the stop threshold", age: 120*time.Second + time.Nanosecond, want: LivenessStopped, reason: ReasonHeartbeatLost},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			machine, clock := newTestMachine(t)
			mustAck(t, machine, ackWith())
			clock.Advance(testCase.age)
			status := requireLiveness(t, machine, testCase.want)
			if status.Reason != testCase.reason {
				t.Fatalf("reason = %q, want %q", status.Reason, testCase.reason)
			}
		})
	}
}

func TestStartupGracePeriodRunsFromCreation(t *testing.T) {
	machine, clock := newTestMachine(t)
	clock.Advance(59 * time.Second)
	requireLiveness(t, machine, LivenessLive)
	clock.Advance(2 * time.Second)
	requireLiveness(t, machine, LivenessStale)
	clock.Advance(60 * time.Second)
	status := requireLiveness(t, machine, LivenessStopped)
	if status.Reason != ReasonHeartbeatLost {
		t.Fatalf("reason = %q, want %q", status.Reason, ReasonHeartbeatLost)
	}
	if !status.LastConfirmedAckAt.IsZero() {
		t.Fatalf("LastConfirmedAckAt = %v, want zero", status.LastConfirmedAckAt)
	}
	if want := clock.Now().Add(-121 * time.Second); !status.Since.Equal(want) {
		t.Fatalf("Since = %v, want %v", status.Since, want)
	}
}

func TestOnlyConfirmedAcksExtendLiveness(t *testing.T) {
	machine, clock := newTestMachine(t)

	// An ack without a services list does not confirm the heartbeat.
	if err := machine.Ack(Ack{}); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	clock.Advance(61 * time.Second)
	requireLiveness(t, machine, LivenessStale)
	if err := machine.Ack(Ack{}); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	requireLiveness(t, machine, LivenessStale)

	// An ack with a services list confirms, even when the set is empty.
	mustAck(t, machine, ackWith())
	requireLiveness(t, machine, LivenessLive)
	if got := machine.Instance().LastConfirmedAckAt; !got.Equal(clock.Now()) {
		t.Fatalf("LastConfirmedAckAt = %v, want %v", got, clock.Now())
	}
}

func TestDiscoveryDoesNotConfirmHeartbeatOrHead(t *testing.T) {
	machine, clock := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	if err := machine.Discover([]Service{books}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonCatchUp)
	if !machine.Instance().LastConfirmedAckAt.IsZero() {
		t.Fatal("a discovery data pull confirmed a heartbeat")
	}

	clock.Advance(61 * time.Second)
	requireLiveness(t, machine, LivenessStale)
	if err := machine.Discover([]Service{books}); err != nil {
		t.Fatalf("second Discover: %v", err)
	}
	requireLiveness(t, machine, LivenessStale)
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonCatchUp)

	mustAck(t, machine, ackWith(books))
	requireLiveness(t, machine, LivenessLive)
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestSeveralServicesInDifferentStates(t *testing.T) {
	machine, _ := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	orders := testAckService(testServiceID2, "orders", 3)
	search := testAckService(testServiceID3, "search", 7)
	mustAck(t, machine, ackWith(books, orders, search))

	// An ack alone gives every service a head but no local copy.
	for _, name := range []string{"books", "orders", "search"} {
		requireServiceState(t, machine, name, SyncStateSyncing, SyncReasonBootstrap)
	}

	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateInSync, "")
	requireServiceState(t, machine, "orders", SyncStateSyncing, SyncReasonBootstrap)
	requireServiceState(t, machine, "search", SyncStateSyncing, SyncReasonBootstrap)

	// The next ack is authoritative: search is no longer in the set.
	mustAck(t, machine, ackWith(books, orders))
	requireServiceState(t, machine, "books", SyncStateInSync, "")
	requireServiceState(t, machine, "search", SyncStateRemoved, "")
	status, known := machine.Status("search")
	if !known || status.Service.State != SyncStateRemoved {
		t.Fatalf("removed service status = %+v, known %v", status.Service, known)
	}
	if _, known := machine.Status("never-assigned"); known {
		t.Fatal("an unknown service name must not be reported")
	}

	// An in-flight sync must not resurrect a removed service.
	mustApply(t, machine, testServiceState(testServiceID3, "search", 7))
	requireServiceState(t, machine, "search", SyncStateRemoved, "")

	// Re-adding the service bootstraps it again.
	mustAck(t, machine, ackWith(books, orders, search))
	requireServiceState(t, machine, "search", SyncStateSyncing, SyncReasonBootstrap)
	mustApply(t, machine, testServiceState(testServiceID3, "search", 7))
	requireServiceState(t, machine, "search", SyncStateInSync, "")

	// The mixed states do not affect instance liveness.
	requireLiveness(t, machine, LivenessLive)

	statuses := machine.Services()
	if len(statuses) != 3 {
		t.Fatalf("Services() = %d entries, want 3", len(statuses))
	}
	for index := 1; index < len(statuses); index++ {
		if statuses[index-1].ID > statuses[index].ID {
			t.Fatalf("Services() is not sorted by ID: %+v", statuses)
		}
	}
}

func TestAppliedBeforeAckWaitsForConfirmation(t *testing.T) {
	machine, _ := newTestMachine(t)
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonCatchUp)

	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestHeadAdvanceRequiresAckConfirmation(t *testing.T) {
	machine, _ := newTestMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	// A notification stops local evaluation immediately, before the next ack.
	if err := machine.ObserveHead(Head{ID: testServiceID1, Epoch: testServiceID1, Revision: 6}); err != nil {
		t.Fatalf("ObserveHead: %v", err)
	}
	status := requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonCatchUp)
	if status.Head != 6 || status.Revision != 5 || status.LagRevisions() != 1 {
		t.Fatalf("status = %+v, want head 6, revision 5, lag 1", status)
	}

	// Applying the new revision is not enough while no ack covers it.
	mustApply(t, machine, testServiceState(testServiceID1, "books", 6))
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonCatchUp)

	// The ack that covers the new head makes the service local again.
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 6)))
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestJitteredAcksDoNotFlap(t *testing.T) {
	machine, clock := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	mustAck(t, machine, ackWith(books))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	intervals := []time.Duration{
		25 * time.Second, 35 * time.Second, 28 * time.Second, 32 * time.Second,
		29 * time.Second, 31 * time.Second, 27 * time.Second, 33 * time.Second,
		30 * time.Second, 30 * time.Second,
	}
	for _, interval := range intervals {
		for elapsed := time.Duration(0); elapsed < interval; elapsed += time.Second {
			clock.Advance(time.Second)
			requireLiveness(t, machine, LivenessLive)
			requireServiceState(t, machine, "books", SyncStateInSync, "")
		}
		mustAck(t, machine, ackWith(books))
	}

	// Duplicate and out-of-order notifications are no-ops.
	if err := machine.ObserveHead(Head{ID: testServiceID1, Epoch: testServiceID1, Revision: 5}); err != nil {
		t.Fatalf("ObserveHead: %v", err)
	}
	requireServiceState(t, machine, "books", SyncStateInSync, "")
	if err := machine.ObserveHead(Head{ID: testServiceID1, Epoch: testServiceID1, Revision: 4}); err != nil {
		t.Fatalf("ObserveHead: %v", err)
	}
	status := requireServiceState(t, machine, "books", SyncStateInSync, "")
	if status.Head != 5 {
		t.Fatalf("head = %d, want 5", status.Head)
	}

	// One genuinely late ack produces one stale window, then recovery.
	clock.Advance(61 * time.Second)
	requireLiveness(t, machine, LivenessStale)
	mustAck(t, machine, ackWith(books))
	requireLiveness(t, machine, LivenessLive)
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestAnOlderAckDoesNotLowerTheHead(t *testing.T) {
	machine, _ := newTestMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 6)))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 6))
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	// A reordered ack describing an older revision must not make the service
	// look behind or ready again.
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
	status := requireServiceState(t, machine, "books", SyncStateInSync, "")
	if status.Head != 6 {
		t.Fatalf("head = %d, want 6", status.Head)
	}
}

func TestStoppedRecoversOnlyAfterVerifiedSync(t *testing.T) {
	machine, clock := newTestMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	clock.Advance(121 * time.Second)
	requireLiveness(t, machine, LivenessStopped)

	// The ack that reconnects updates the head but does not recover: the
	// service is behind the latest revision.
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 6)))
	requireLiveness(t, machine, LivenessStopped)
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonCatchUp)

	mustApply(t, machine, testServiceState(testServiceID1, "books", 6))
	status := requireLiveness(t, machine, LivenessLive)
	requireServiceState(t, machine, "books", SyncStateInSync, "")
	if !status.LastConfirmedAckAt.Equal(clock.Now()) {
		t.Fatalf("LastConfirmedAckAt = %v, want %v", status.LastConfirmedAckAt, clock.Now())
	}
}

func TestAckAloneDoesNotRecoverStoppedInstance(t *testing.T) {
	machine, clock := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	mustAck(t, machine, ackWith(books))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))

	clock.Advance(121 * time.Second)
	requireLiveness(t, machine, LivenessStopped)

	// The copy is already at the head, but the heartbeat alone must not
	// recover the instance: the copy has to be verified again first.
	mustAck(t, machine, ackWith(books))
	requireLiveness(t, machine, LivenessStopped)
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireLiveness(t, machine, LivenessLive)
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestStoppedRecoveryWaitsForEveryService(t *testing.T) {
	machine, clock := newTestMachine(t)
	books5 := testAckService(testServiceID1, "books", 5)
	orders3 := testAckService(testServiceID2, "orders", 3)
	mustAck(t, machine, ackWith(books5, orders3))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	mustApply(t, machine, testServiceState(testServiceID2, "orders", 3))

	clock.Advance(121 * time.Second)
	requireLiveness(t, machine, LivenessStopped)

	books6 := testAckService(testServiceID1, "books", 6)
	orders4 := testAckService(testServiceID2, "orders", 4)
	mustAck(t, machine, ackWith(books6, orders4))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 6))
	requireLiveness(t, machine, LivenessStopped)

	mustApply(t, machine, testServiceState(testServiceID2, "orders", 4))
	requireLiveness(t, machine, LivenessLive)
	requireServiceState(t, machine, "books", SyncStateInSync, "")
	requireServiceState(t, machine, "orders", SyncStateInSync, "")
}

// TestCascadeDelayFollowsEachThreshold wires three instances into a chain and
// asserts that the downstream instances stop after their own thresholds, not
// the upstream one. While an upstream is only stale it still serves, so the
// downstream stays live.
func TestCascadeDelayFollowsEachThreshold(t *testing.T) {
	clock := newFakeClock()
	newMachine := func() *Machine {
		machine, err := NewMachine(MachineOptions{Clock: clock})
		if err != nil {
			t.Fatalf("NewMachine: %v", err)
		}
		return machine
	}
	upstream := newMachine()
	middle := newMachine()
	downstream := newMachine()

	books := testAckService(testServiceID1, "books", 5)
	for _, machine := range []*Machine{upstream, middle, downstream} {
		mustAck(t, machine, ackWith(books))
		mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
		requireServiceState(t, machine, "books", SyncStateInSync, "")
	}

	const never = -time.Duration(1)
	staleAt := map[*Machine]time.Duration{upstream: never, middle: never, downstream: never}
	stopAt := map[*Machine]time.Duration{upstream: never, middle: never, downstream: never}
	lastAck := map[*Machine]time.Duration{upstream: 0, middle: 0, downstream: 0}

	upstreamLiveWhileStale := false
	for elapsed := time.Second; elapsed <= 400*time.Second; elapsed += time.Second {
		clock.Advance(time.Second)

		// The source of the upstream instance vanished at zero. A proxy keeps
		// answering its downstream until it stops, not until it is stale.
		if upstream.Instance().Liveness != LivenessStopped &&
			elapsed-lastAck[middle] >= 30*time.Second {
			mustAck(t, middle, ackWith(books))
			lastAck[middle] = elapsed
		}
		if middle.Instance().Liveness != LivenessStopped &&
			elapsed-lastAck[downstream] >= 30*time.Second {
			mustAck(t, downstream, ackWith(books))
			lastAck[downstream] = elapsed
		}

		if elapsed == 100*time.Second {
			upstreamLiveWhileStale = upstream.Instance().Liveness == LivenessStale &&
				middle.Instance().Liveness == LivenessLive &&
				downstream.Instance().Liveness == LivenessLive
		}
		for _, machine := range []*Machine{upstream, middle, downstream} {
			switch machine.Instance().Liveness {
			case LivenessStale:
				if staleAt[machine] == never {
					staleAt[machine] = elapsed
				}
			case LivenessStopped:
				if stopAt[machine] == never {
					stopAt[machine] = elapsed
				}
			}
		}
	}

	if !upstreamLiveWhileStale {
		t.Fatal("an upstream that is only stale must not stop or stale its downstream")
	}
	cases := []struct {
		name    string
		machine *Machine
		stale   time.Duration
		stop    time.Duration
	}{
		{name: "upstream", machine: upstream, stale: 61 * time.Second, stop: 121 * time.Second},
		{name: "middle", machine: middle, stale: 181 * time.Second, stop: 241 * time.Second},
		{name: "downstream", machine: downstream, stale: 301 * time.Second, stop: 361 * time.Second},
	}
	for _, testCase := range cases {
		if staleAt[testCase.machine] != testCase.stale {
			t.Errorf("%s went stale at %s, want %s", testCase.name, staleAt[testCase.machine], testCase.stale)
		}
		if stopAt[testCase.machine] != testCase.stop {
			t.Errorf("%s stopped at %s, want %s", testCase.name, stopAt[testCase.machine], testCase.stop)
		}
	}
}

func TestPauseDoesNotAffectSyncOrLiveness(t *testing.T) {
	machine, clock := newTestMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))

	machine.Pause()
	status, _ := machine.Status("books")
	if !status.Instance.Paused {
		t.Fatal("Paused = false after Pause")
	}
	requireLiveness(t, machine, LivenessLive)
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	// Synchronization and liveness keep working while distribution is paused.
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 6)))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 6))
	requireServiceState(t, machine, "books", SyncStateInSync, "")
	clock.Advance(61 * time.Second)
	requireLiveness(t, machine, LivenessStale)

	machine.Resume()
	if machine.Instance().Paused {
		t.Fatal("Paused = true after Resume")
	}
}

func TestCloudThresholdsCanOnlyTighten(t *testing.T) {
	t.Run("stop tightened", func(t *testing.T) {
		machine, clock := newTestMachine(t)
		mustAck(t, machine, Ack{Services: []Service{}, StopAfter: 90 * time.Second})
		if got := machine.Thresholds().StopAfter; got != 90*time.Second {
			t.Fatalf("stop threshold = %s, want 90s", got)
		}
		clock.Advance(90*time.Second + time.Nanosecond)
		requireLiveness(t, machine, LivenessStopped)
	})
	t.Run("stop cannot be loosened", func(t *testing.T) {
		machine, clock := newTestMachine(t)
		mustAck(t, machine, Ack{Services: []Service{}, StopAfter: 10 * time.Minute})
		if got := machine.Thresholds().StopAfter; got != 120*time.Second {
			t.Fatalf("stop threshold = %s, want the local 120s", got)
		}
		clock.Advance(121 * time.Second)
		requireLiveness(t, machine, LivenessStopped)
	})
	t.Run("warning tightened", func(t *testing.T) {
		machine, clock := newTestMachine(t)
		mustAck(t, machine, Ack{Services: []Service{}, HeartbeatInterval: 15 * time.Second, StaleAfter: 30 * time.Second})
		if got := machine.Thresholds().StaleAfter; got != 30*time.Second {
			t.Fatalf("stale threshold = %s, want 30s", got)
		}
		clock.Advance(31 * time.Second)
		requireLiveness(t, machine, LivenessStale)
	})
	t.Run("heartbeat taken from the source", func(t *testing.T) {
		machine, _ := newTestMachine(t)
		mustAck(t, machine, Ack{Services: []Service{}, HeartbeatInterval: 15 * time.Second})
		if got := machine.Thresholds().HeartbeatInterval; got != 15*time.Second {
			t.Fatalf("heartbeat interval = %s, want 15s", got)
		}
	})
	t.Run("heartbeat cannot be loosened", func(t *testing.T) {
		machine, _ := newTestMachine(t)
		mustAck(t, machine, Ack{
			Services:          []Service{},
			HeartbeatInterval: 5 * time.Minute,
			StaleAfter:        time.Minute,
			StopAfter:         2 * time.Minute,
		})
		thresholds := machine.Thresholds()
		if thresholds.HeartbeatInterval != 30*time.Second || thresholds.StaleAfter != time.Minute || thresholds.StopAfter != 2*time.Minute {
			t.Fatalf("thresholds = %+v, want local 30s/60s/120s", thresholds)
		}
	})
	t.Run("out of range values are ignored", func(t *testing.T) {
		machine, _ := newTestMachine(t)
		mustAck(t, machine, Ack{
			Services:          []Service{},
			HeartbeatInterval: time.Second,
			StaleAfter:        10 * time.Minute,
			StopAfter:         time.Second,
		})
		thresholds := machine.Thresholds()
		if thresholds.HeartbeatInterval != 30*time.Second ||
			thresholds.StaleAfter != 60*time.Second ||
			thresholds.StopAfter != 120*time.Second {
			t.Fatalf("thresholds = %+v, want the local defaults", thresholds)
		}
	})
}

func TestSyncFailureClassification(t *testing.T) {
	t.Run("unauthorized stops as revoked", func(t *testing.T) {
		machine, _ := newTestMachine(t)
		mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
		machine.SyncFailed(testServiceID1, fmt.Errorf("sync: %w", ErrUnauthorized))
		status := requireLiveness(t, machine, LivenessStopped)
		if status.Reason != ReasonRevoked {
			t.Fatalf("reason = %q, want %q", status.Reason, ReasonRevoked)
		}
	})
	t.Run("forbidden stops as revoked", func(t *testing.T) {
		machine, _ := newTestMachine(t)
		mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
		machine.SyncFailed(testServiceID1, ErrForbidden)
		if reason := machine.Instance().Reason; reason != ReasonRevoked {
			t.Fatalf("reason = %q, want %q", reason, ReasonRevoked)
		}
	})
	t.Run("missing service is removed", func(t *testing.T) {
		machine, _ := newTestMachine(t)
		mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
		machine.SyncFailed(testServiceID1, ErrServiceNotFound)
		requireServiceState(t, machine, "books", SyncStateRemoved, "")
	})
	t.Run("source gone is removed", func(t *testing.T) {
		machine, _ := newTestMachine(t)
		mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
		machine.SyncFailed(testServiceID1, &SnapshotRequiredError{Reason: "source_gone"})
		requireServiceState(t, machine, "books", SyncStateRemoved, "")
	})
	t.Run("transient failure keeps the service syncing", func(t *testing.T) {
		machine, _ := newTestMachine(t)
		mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
		machine.SyncFailed(testServiceID1, fmt.Errorf("sync: %w", ErrUnavailable))
		requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonBootstrap)
		requireLiveness(t, machine, LivenessLive)
	})
}

func TestTerminalStopNeverRecovers(t *testing.T) {
	machine, clock := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	mustAck(t, machine, ackWith(books))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))

	machine.Stop(ReasonRevoked)
	status := requireLiveness(t, machine, LivenessStopped)
	if status.Reason != ReasonRevoked {
		t.Fatalf("reason = %q, want %q", status.Reason, ReasonRevoked)
	}

	// A reconnect and a full sync do not clear a terminal stop.
	clock.Advance(30 * time.Second)
	mustAck(t, machine, ackWith(books))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	status = requireLiveness(t, machine, LivenessStopped)
	if status.Reason != ReasonRevoked {
		t.Fatalf("reason = %q, want %q", status.Reason, ReasonRevoked)
	}

	// A later non-terminal reason cannot downgrade a terminal one.
	machine.Stop(ReasonHeartbeatLost)
	if reason := machine.Instance().Reason; reason != ReasonRevoked {
		t.Fatalf("reason = %q, want %q", reason, ReasonRevoked)
	}

	// The first terminal reason sticks; a later one does not replace it.
	machine.Stop(ReasonConfigError)
	if reason := machine.Instance().Reason; reason != ReasonRevoked {
		t.Fatalf("reason = %q, want %q", reason, ReasonRevoked)
	}

	// A terminal reason does replace a non-terminal one.
	other, _ := newTestMachine(t)
	other.Stop(ReasonHeartbeatLost)
	other.Stop(ReasonConfigError)
	if reason := other.Instance().Reason; reason != ReasonConfigError {
		t.Fatalf("reason = %q, want %q", reason, ReasonConfigError)
	}
}

func TestNonTerminalStopRecoversAfterVerification(t *testing.T) {
	machine, _ := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	mustAck(t, machine, ackWith(books))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))

	machine.Stop(ReasonIntegrityFailure)
	status := requireLiveness(t, machine, LivenessStopped)
	if status.Reason != ReasonIntegrityFailure {
		t.Fatalf("reason = %q, want %q", status.Reason, ReasonIntegrityFailure)
	}
	if reason := ReasonIntegrityFailure.Terminal(); reason {
		t.Fatal("integrity failure must not be terminal")
	}

	// Recovery still needs an ack and a freshly verified copy.
	mustAck(t, machine, ackWith(books))
	requireLiveness(t, machine, LivenessStopped)
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireLiveness(t, machine, LivenessLive)
}

func TestResyncForcesOneServiceToSyncing(t *testing.T) {
	machine, _ := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	mustAck(t, machine, ackWith(books))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	machine.Resync(testServiceID1)
	status := requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonResync)
	if status.Revision != 0 {
		t.Fatalf("revision = %d, want 0 while the old copy is discarded", status.Revision)
	}

	// A freshly verified copy completes the resync.
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestFetchInFlightBeforeResyncCannotCompleteIt(t *testing.T) {
	machine, _ := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	mustAck(t, machine, ackWith(books))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))

	// A pass starts fetching books, then the resync command arrives. The
	// completed fetch is byte for byte what a fresh fetch would return, yet it
	// began before the resync and must not end it.
	inFlight := machine.BeginSync(testServiceID1)
	machine.Resync(testServiceID1)
	if machine.AttemptCurrent(inFlight) {
		t.Fatal("an attempt that began before the resync is still current")
	}
	if err := machine.Applied(inFlight, testServiceState(testServiceID1, "books", 5)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	status := requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonResync)
	if status.Revision != 0 {
		t.Fatalf("revision = %d, want the discarded copy to stay discarded", status.Revision)
	}

	fresh := machine.BeginSync(testServiceID1)
	if !machine.AttemptCurrent(fresh) {
		t.Fatal("an attempt begun after the resync is not current")
	}
	if err := machine.Applied(fresh, testServiceState(testServiceID1, "books", 5)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestFetchInFlightBeforeRemovalCannotServeTheReaddedService(t *testing.T) {
	machine, _ := newTestMachine(t)
	books := testAckService(testServiceID1, "books", 5)
	mustAck(t, machine, ackWith(books))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))

	inFlight := machine.BeginSync(testServiceID1)
	mustAck(t, machine, Ack{Services: []Service{}})
	requireServiceState(t, machine, "books", SyncStateRemoved, "")
	mustAck(t, machine, ackWith(books))
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonBootstrap)

	if err := machine.Applied(inFlight, testServiceState(testServiceID1, "books", 5)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonBootstrap)

	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestAppliedRejectsAnAttemptForAnotherService(t *testing.T) {
	machine, _ := newTestMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
	if err := machine.Applied(machine.BeginSync(testServiceID2), testServiceState(testServiceID1, "books", 5)); err == nil {
		t.Fatal("Applied accepted an attempt that belongs to another service")
	}
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonBootstrap)
}

func TestEpochChangeRequiresASnapshot(t *testing.T) {
	machine, _ := newTestMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	// The same revision number under a new epoch is a different state, and the
	// copy from the old epoch is discarded.
	newEpoch := testServiceID2
	service := testAckService(testServiceID1, "books", 1)
	service.Epoch = newEpoch
	mustAck(t, machine, ackWith(service))
	requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonBootstrap)

	state := testServiceState(testServiceID1, "books", 1)
	state.Epoch = newEpoch
	mustApply(t, machine, state)
	requireServiceState(t, machine, "books", SyncStateInSync, "")
}

func TestDelayedOldEpochMessageCannotRestoreServing(t *testing.T) {
	// Setup: epoch A revision 5 is confirmed, applied and served, then the
	// source moves to epoch B.
	setup := func(t *testing.T) (*Machine, Service) {
		t.Helper()
		machine, _ := newTestMachine(t)
		mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
		mustApply(t, machine, testServiceState(testServiceID1, "books", 5))
		requireServiceState(t, machine, "books", SyncStateInSync, "")

		moved := testAckService(testServiceID1, "books", 1)
		moved.Epoch = testServiceID2
		mustAck(t, machine, ackWith(moved))
		requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonBootstrap)
		return machine, moved
	}

	t.Run("delayed ack", func(t *testing.T) {
		machine, _ := setup(t)
		mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 5)))
		status := requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonBootstrap)
		if status.Revision != 0 || status.Head != 5 {
			t.Fatalf("status = %+v, want no verified revision and head 5", status)
		}
	})

	t.Run("delayed notification", func(t *testing.T) {
		machine, moved := setup(t)
		old := testAckService(testServiceID1, "books", 5)
		if err := machine.ObserveHead(Head{
			ID:        old.ID,
			Epoch:     old.Epoch,
			Revision:  old.Revision,
			StateHash: old.HeadStateHash,
		}); err != nil {
			t.Fatalf("ObserveHead: %v", err)
		}
		status := requireServiceState(t, machine, "books", SyncStateSyncing, SyncReasonBootstrap)
		if status.Revision != 0 || status.Head != 5 {
			t.Fatalf("status = %+v, want no verified revision and head 5", status)
		}

		// The current epoch can still be confirmed and synchronized.
		mustAck(t, machine, ackWith(moved))
		state := testServiceState(testServiceID1, "books", 1)
		state.Epoch = moved.Epoch
		mustApply(t, machine, state)
		requireServiceState(t, machine, "books", SyncStateInSync, "")
	})
}

func TestRemovedTombstoneDoesNotShadowItsReplacement(t *testing.T) {
	machine, _ := newTestMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 1)))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 1))
	requireServiceState(t, machine, "books", SyncStateInSync, "")

	// Removing the service keeps a tombstone under the old name.
	mustAck(t, machine, ackWith())
	requireServiceState(t, machine, "books", SyncStateRemoved, "")

	// A replacement with the same name has a higher service ID, so the old
	// lookup rule would keep resolving the removed tombstone.
	mustAck(t, machine, ackWith(testAckService(testServiceID2, "books", 1)))
	mustApply(t, machine, testServiceState(testServiceID2, "books", 1))
	status := requireServiceState(t, machine, "books", SyncStateInSync, "")
	if status.ID != testServiceID2 {
		t.Fatalf("Status(books) resolved service %q, want %q", status.ID, testServiceID2)
	}

	// With no active service left, the name still resolves to a tombstone so
	// the caller can report the service as removed.
	mustAck(t, machine, ackWith())
	requireServiceState(t, machine, "books", SyncStateRemoved, "")
}

func TestAckValidation(t *testing.T) {
	machine, _ := newTestMachine(t)

	invalidName := testAckService(testServiceID1, "books", 5)
	invalidName.Name = "not a valid name"
	if err := machine.Ack(ackWith(invalidName)); !errors.Is(err, ErrProtocol) {
		t.Fatalf("invalid name error = %v, want ErrProtocol", err)
	}

	duplicate := ackWith(
		testAckService(testServiceID1, "books", 5),
		testAckService(testServiceID1, "books", 5),
	)
	if err := machine.Ack(duplicate); !errors.Is(err, ErrProtocol) {
		t.Fatalf("duplicate id error = %v, want ErrProtocol", err)
	}

	duplicateName := ackWith(
		testAckService(testServiceID1, "books", 5),
		testAckService(testServiceID2, "books", 5),
	)
	if err := machine.Ack(duplicateName); !errors.Is(err, ErrProtocol) {
		t.Fatalf("duplicate name error = %v, want ErrProtocol", err)
	}

	tooMany := make([]Service, maxServicesPerSource+1)
	for index := range tooMany {
		id := fmt.Sprintf("%026d", index+1)
		tooMany[index] = testAckService(id, fmt.Sprintf("service-%d", index), 1)
	}
	if err := machine.Ack(ackWith(tooMany...)); !errors.Is(err, ErrProtocol) {
		t.Fatalf("too many services error = %v, want ErrProtocol", err)
	}

	if services := machine.Services(); len(services) != 0 {
		t.Fatalf("an invalid ack changed the state: %+v", services)
	}

	if err := machine.ObserveHead(Head{ID: "bad", Revision: 1}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("invalid head error = %v, want ErrProtocol", err)
	}
	if err := machine.Applied(machine.BeginSync(""), nil); err == nil {
		t.Fatal("Applied(nil) must fail")
	}
	if err := machine.Applied(machine.BeginSync("bad"), &ServiceState{ServiceID: "bad", Snapshot: Snapshot{Name: "books"}}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("invalid applied state error = %v, want ErrProtocol", err)
	}
}

func TestMachineOptionsValidation(t *testing.T) {
	cases := []struct {
		name    string
		options MachineOptions
	}{
		{name: "heartbeat too short", options: MachineOptions{HeartbeatInterval: time.Second}},
		{name: "heartbeat too long", options: MachineOptions{HeartbeatInterval: 10 * time.Minute}},
		{name: "stale not after heartbeat", options: MachineOptions{HeartbeatInterval: 60 * time.Second, StaleAfter: 60 * time.Second}},
		{name: "stop too short", options: MachineOptions{StaleAfter: 30 * time.Second, StopAfter: 30 * time.Second}},
		{name: "stop not after stale", options: MachineOptions{StaleAfter: 120 * time.Second, StopAfter: 120 * time.Second}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewMachine(testCase.options); err == nil {
				t.Fatal("NewMachine accepted invalid options")
			}
		})
	}

	machine, err := NewMachine(MachineOptions{})
	if err != nil {
		t.Fatalf("NewMachine with defaults: %v", err)
	}
	thresholds := machine.Thresholds()
	if thresholds.HeartbeatInterval != 30*time.Second ||
		thresholds.StaleAfter != 60*time.Second ||
		thresholds.StopAfter != 120*time.Second {
		t.Fatalf("default thresholds = %+v", thresholds)
	}
}

func TestTrackedServicesAreBounded(t *testing.T) {
	machine, _ := newTestMachine(t)
	const batches = 15
	for batch := 0; batch < batches; batch++ {
		services := make([]Service, maxServicesPerSource)
		for index := range services {
			number := batch*maxServicesPerSource + index + 1
			id := fmt.Sprintf("%026d", number)
			name := fmt.Sprintf("service-%d", number)
			services[index] = testAckService(id, name, 1)
		}
		mustAck(t, machine, ackWith(services...))
	}

	if tracked := len(machine.Services()); tracked > maxTrackedServices {
		t.Fatalf("tracked services = %d, want at most %d", tracked, maxTrackedServices)
	}
	// The live set survives eviction.
	for index := 0; index < maxServicesPerSource; index++ {
		number := (batches-1)*maxServicesPerSource + index + 1
		requireServiceState(t, machine, fmt.Sprintf("service-%d", number), SyncStateSyncing, SyncReasonBootstrap)
	}
}

func TestAppliedBeforeTheFirstAckIsBounded(t *testing.T) {
	machine, _ := newTestMachine(t)
	for index := 0; index < maxServicesPerSource; index++ {
		number := index + 1
		state := testServiceState(fmt.Sprintf("%026d", number), fmt.Sprintf("service-%d", number), 1)
		mustApply(t, machine, state)
	}

	extra := testServiceState(
		fmt.Sprintf("%026d", maxServicesPerSource+1),
		fmt.Sprintf("service-%d", maxServicesPerSource+1),
		1,
	)
	if err := machine.Applied(machine.BeginSync(extra.ServiceID), extra); !errors.Is(err, ErrProtocol) {
		t.Fatalf("Applied beyond the pre-ack bound = %v, want ErrProtocol", err)
	}
	if tracked := len(machine.Services()); tracked != maxServicesPerSource {
		t.Fatalf("tracked services = %d, want %d", tracked, maxServicesPerSource)
	}

	// The first ack establishes the authoritative set and removes the rest.
	mustAck(t, machine, ackWith(testAckService(fmt.Sprintf("%026d", 1), "service-1", 1)))
	if tracked := len(machine.Services()); tracked != maxServicesPerSource {
		t.Fatalf("tracked services after ack = %d, want the bounded %d", tracked, maxServicesPerSource)
	}
}
