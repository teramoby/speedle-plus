// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	defaultHeartbeatInterval = 30 * time.Second
	defaultStaleAfter        = 60 * time.Second
	defaultStopAfter         = 120 * time.Second

	minHeartbeatInterval = 15 * time.Second
	maxHeartbeatInterval = 5 * time.Minute
	minStopAfter         = 60 * time.Second
	maxStopAfter         = 15 * time.Minute

	// maxTrackedServices bounds the records the machine keeps, including
	// tombstones for removed services. The live set is bounded by the
	// protocol's maxServicesPerSource; the extra room only absorbs service
	// churn over the lifetime of a process.
	maxTrackedServices = 100
)

// Clock supplies the current time. The state machine never reads the wall
// clock directly, so tests can drive it with a deterministic clock.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Liveness is the instance-level state. It is independent of the per-service
// sync state: an instance can be stale while every service still has a usable
// local copy.
type Liveness string

const (
	// LivenessLive means the last confirmed ack is within the stale
	// threshold. Requests are served according to each service's sync state.
	LivenessLive Liveness = "live"
	// LivenessStale means the last confirmed ack is older than the stale
	// threshold but not yet past the stop threshold. A service with a complete
	// copy still evaluates locally; readiness reports not ready.
	LivenessStale Liveness = "stale"
	// LivenessStopped means the instance must not return an authorization
	// decision at all. Requests fail closed with a reason.
	LivenessStopped Liveness = "stopped"
)

// SyncState is the per-service synchronization state.
type SyncState string

const (
	// SyncStateSyncing means the service has no complete copy or is behind
	// the source head. Requests for it are routed to the fallback or fail
	// closed.
	SyncStateSyncing SyncState = "syncing"
	// SyncStateInSync means the verified local revision equals the head from
	// the last confirmed ack. Requests are evaluated locally.
	SyncStateInSync SyncState = "in_sync"
	// SyncStateRemoved means the service is no longer in the instance's
	// service set or the source reported it gone. It is not served.
	SyncStateRemoved SyncState = "removed"
)

// SyncReason says why a service is syncing. The three causes share the same
// routing behaviour but are reported separately for logs and metrics.
type SyncReason string

const (
	// SyncReasonBootstrap means there is no verified local copy yet.
	SyncReasonBootstrap SyncReason = "bootstrap"
	// SyncReasonCatchUp means a copy exists but is behind the source head, or
	// its identity (epoch or state hash) does not match the head.
	SyncReasonCatchUp SyncReason = "catch_up"
	// SyncReasonResync means an explicit resync command invalidated the copy.
	SyncReasonResync SyncReason = "resync"
)

// Reason is a machine-readable cause reported with unavailable authorization
// responses and in status views. The values match the distribution protocol.
type Reason string

const (
	ReasonHeartbeatLost       Reason = "heartbeat_lost"
	ReasonRevoked             Reason = "revoked"
	ReasonSourceGone          Reason = "source_gone"
	ReasonResyncRequired      Reason = "resync_required"
	ReasonFallbackUnavailable Reason = "fallback_unavailable"
	ReasonQuotaExhausted      Reason = "quota_exhausted"
	ReasonConfigError         Reason = "config_error"
	ReasonStoppedByCommand    Reason = "stopped_by_command"
	ReasonIntegrityFailure    Reason = "integrity_failure"
	ReasonDistributionPaused  Reason = "distribution_paused"
)

// Terminal reports whether the reason needs operator action, such as new
// credentials or corrected configuration, before the instance can serve
// again. A terminal stop is never cleared automatically: the process must be
// reconfigured or restarted.
func (r Reason) Terminal() bool {
	switch r {
	case ReasonRevoked, ReasonSourceGone, ReasonConfigError, ReasonStoppedByCommand:
		return true
	default:
		return false
	}
}

// Ack confirms a heartbeat and is the authoritative statement of which
// services an instance serves and where each service's head is. The
// notification channel decodes the wire ack into this value; the machine does
// not parse the wire format itself.
//
// Only an ack that carries a services list confirms the heartbeat. A nil
// Services slice means the message did not confirm anything and is ignored.
// An instance with no services therefore confirms with an empty, non-nil
// slice.
type Ack struct {
	// Services lists every service the instance serves, with its current
	// head in the Revision field. The list is authoritative: a service that
	// is tracked but absent from it is removed.
	Services []Service

	// HeartbeatInterval, StaleAfter and StopAfter are the source's requested
	// thresholds. Zero means the source did not ask to change the current
	// value. The source can tighten the local thresholds but never loosen
	// them.
	HeartbeatInterval time.Duration
	StaleAfter        time.Duration
	StopAfter         time.Duration
}

// Confirmed reports whether the ack confirms the heartbeat.
func (a Ack) Confirmed() bool { return a.Services != nil }

// Head announces that one service has a new revision. It is an optimisation
// that lets the instance stop serving a service before the next ack; only a
// confirmed ack can make a service in_sync. StateHash may be zero when the
// notification carries no hash.
type Head struct {
	ID        string
	Epoch     string
	Revision  uint64
	StateHash Hash
}

// Thresholds are the effective heartbeat, warning and stop intervals.
type Thresholds struct {
	HeartbeatInterval time.Duration
	StaleAfter        time.Duration
	StopAfter         time.Duration
}

// InstanceStatus is the instance-level view.
type InstanceStatus struct {
	Liveness Liveness
	// Reason explains a stopped liveness. It is empty otherwise.
	Reason Reason
	// Paused is the distribution pause flag. Pausing only closes downstream
	// distribution; it does not affect liveness, synchronization or local
	// evaluation.
	Paused bool
	// LastConfirmedAckAt is when the last confirming ack arrived. It is zero
	// until the first one; the startup grace period is measured from Since
	// instead.
	LastConfirmedAckAt time.Time
	// Since is when the machine was created.
	Since time.Time
}

// ServiceStatus is the per-service view.
type ServiceStatus struct {
	ID    string
	Name  string
	State SyncState
	// Reason explains a syncing state: bootstrap, catch_up or resync. It is
	// empty in any other state.
	Reason SyncReason
	// Revision is the verified local revision. It is zero when no verified
	// copy is held.
	Revision uint64
	// Head is the source head from the most recent ack or notification.
	Head uint64
}

// LagRevisions is how many revisions the verified copy is behind the head.
func (s ServiceStatus) LagRevisions() uint64 {
	if s.Head > s.Revision {
		return s.Head - s.Revision
	}
	return 0
}

// Status is a consistent snapshot of the instance and one service.
type Status struct {
	Instance InstanceStatus
	Service  ServiceStatus
}

// StatusProvider is the read-only view entry points use. Status returns false
// when the instance does not know the name at all; a service that was removed
// is still reported, with State set to removed.
type StatusProvider interface {
	Status(serviceName string) (Status, bool)
}

// MachineOptions configures a Machine. Zero values select the defaults from
// the design.
type MachineOptions struct {
	// Clock supplies the current time. Nil uses the system clock.
	Clock Clock
	// HeartbeatInterval is how often the notification channel sends a
	// heartbeat. The default is 30s; the protocol range is 15s to 5m.
	HeartbeatInterval time.Duration
	// StaleAfter is the warning threshold. The default is 60s; it must be
	// greater than the heartbeat interval.
	StaleAfter time.Duration
	// StopAfter is the fail-closed threshold. The default is 120s; the
	// protocol range is 60s to 15m and it must be greater than the stale
	// threshold.
	StopAfter time.Duration
}

// Machine tracks the instance liveness and the per-service sync state. It is
// driven entirely by events: confirmed acks, head notifications, completed
// syncs, failures and administrative actions. It never talks to the network
// and never evaluates policy.
//
// The caller delivers acks and notifications in the order the source produced
// them. Within one epoch the machine additionally tolerates reordering; an
// epoch change discards the superseded copy, so a delayed message from an old
// epoch can only force a fresh verified copy before serving resumes.
//
// Two rules implement the hysteresis that keeps states from flapping:
//
//   - Leaving stale and becoming live again requires a confirming ack, which
//     also carries every service's head.
//   - Leaving stopped requires a confirming ack after the stop, followed by a
//     freshly verified copy for every service in the set. A heartbeat alone
//     never recovers a stopped instance.
//
// All methods are safe for concurrent use.
type Machine struct {
	mu    sync.Mutex
	clock Clock
	since time.Time

	localHeartbeat time.Duration
	localStale     time.Duration
	localStop      time.Duration

	cloudHeartbeat time.Duration
	cloudStale     time.Duration
	cloudStop      time.Duration

	paused bool

	stopped      bool
	stopReason   Reason
	ackSinceStop bool

	lastConfirmedAckAt time.Time

	setKnown bool
	records  map[string]*serviceRecord
	nextSeq  uint64
	// attemptSeq numbers the events that invalidate a synchronization attempt
	// already in flight: a service record is created, removed or resynced.
	attemptSeq uint64
}

type serviceRecord struct {
	id   string
	name string

	state  SyncState
	reason SyncReason

	setMember bool

	headKnown     bool
	headConfirmed bool
	epoch         string
	headRevision  uint64
	headStateHash Hash

	hasLocal       bool
	localRevision  uint64
	localEpoch     string
	localStateHash Hash

	pendingResync     bool
	verifiedSinceStop bool

	removedSeq uint64
	// generation changes whenever a result fetched earlier must no longer be
	// reported: on creation, removal and resync of the record.
	generation uint64
}

// SyncAttempt identifies one synchronization attempt for one service. A caller
// obtains it with BeginSync before it starts fetching the service and passes it
// to Applied with the verified result. An attempt that started before a
// resync, a removal or a re-add of the service is stale, and Applied ignores
// its result.
type SyncAttempt struct {
	serviceID  string
	generation uint64
}

// ServiceID returns the service the attempt belongs to.
func (a SyncAttempt) ServiceID() string { return a.serviceID }

// NewMachine returns a Machine. A nil clock uses the system clock.
func NewMachine(options MachineOptions) (*Machine, error) {
	clock := options.Clock
	if clock == nil {
		clock = systemClock{}
	}
	heartbeat := options.HeartbeatInterval
	if heartbeat == 0 {
		heartbeat = defaultHeartbeatInterval
	}
	stale := options.StaleAfter
	if stale == 0 {
		stale = defaultStaleAfter
	}
	stop := options.StopAfter
	if stop == 0 {
		stop = defaultStopAfter
	}
	if heartbeat < minHeartbeatInterval || heartbeat > maxHeartbeatInterval {
		return nil, fmt.Errorf("adsync: heartbeat interval %s is outside %s..%s", heartbeat, minHeartbeatInterval, maxHeartbeatInterval)
	}
	if stale <= heartbeat {
		return nil, fmt.Errorf("adsync: stale threshold %s must be greater than the heartbeat interval %s", stale, heartbeat)
	}
	if stop < minStopAfter || stop > maxStopAfter {
		return nil, fmt.Errorf("adsync: stop threshold %s is outside %s..%s", stop, minStopAfter, maxStopAfter)
	}
	if stop <= stale {
		return nil, fmt.Errorf("adsync: stop threshold %s must be greater than the stale threshold %s", stop, stale)
	}
	return &Machine{
		clock:          clock,
		since:          clock.Now(),
		localHeartbeat: heartbeat,
		localStale:     stale,
		localStop:      stop,
		records:        make(map[string]*serviceRecord),
	}, nil
}

// Ack applies a confirming ack from the policy source or an upstream proxy.
// An ack that does not carry a services list is ignored: a heartbeat only
// counts as confirmed when the source answers with the service set and each
// service's head.
//
// An ack never lowers a known head within one epoch, so a late or reordered
// ack cannot undo a newer notification. A new epoch replaces the head, because
// revisions are no longer comparable across epochs.
func (m *Machine) Ack(ack Ack) error {
	if !ack.Confirmed() {
		return nil
	}
	if err := validateAck(ack); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	m.advanceLocked(now)
	if err := m.applyThresholdsLocked(ack); err != nil {
		return err
	}
	m.lastConfirmedAckAt = now
	if m.stopped {
		m.ackSinceStop = true
	}
	m.applySetLocked(ack.Services, true)
	if m.stopped {
		m.tryRecoverLocked(now)
	}
	return nil
}

// Discover applies the authoritative service set and heads returned by the
// discovery endpoint. A data pull is not a confirmed heartbeat: it neither
// refreshes liveness nor confirms a head. Only a channel ack can do that.
func (m *Machine) Discover(services []Service) error {
	if services == nil {
		return nil
	}
	if err := validateAck(Ack{Services: services}); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceLocked(m.clock.Now())
	m.applySetLocked(services, false)
	return nil
}

// ObserveHead applies a head notification for one service. A notification for
// a service the instance does not serve is ignored, because only an ack may
// add a service to the set. A notification that does not advance the head is a
// no-op.
func (m *Machine) ObserveHead(head Head) error {
	if !validEntityID(head.ID) {
		return fmt.Errorf("%w: invalid service id %q", ErrProtocol, head.ID)
	}
	if !validRevision(head.Revision) {
		return fmt.Errorf("%w: service %q revision %d is out of range", ErrProtocol, head.ID, head.Revision)
	}
	if head.Epoch != "" && !validEntityID(head.Epoch) {
		return fmt.Errorf("%w: invalid epoch %q", ErrProtocol, head.Epoch)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceLocked(m.clock.Now())
	record := m.records[head.ID]
	if record == nil || (m.setKnown && !record.setMember) || record.state == SyncStateRemoved {
		return nil
	}
	m.updateRecordHeadLocked(record, head.Epoch, head.Revision, head.StateHash, false)
	m.recomputeRecordLocked(record)
	return nil
}

// updateRecordHeadLocked applies one head update. An epoch transition
// invalidates every attempt that began under the previous epoch: a result
// fetched from replaced policy must not be publishable afterwards, not even
// when a delayed message moves the head back to the old epoch.
func (m *Machine) updateRecordHeadLocked(record *serviceRecord, epoch string, revision uint64, stateHash Hash, confirmed bool) {
	if record.headKnown && epoch != "" && record.epoch != "" && epoch != record.epoch {
		m.invalidateAttemptsLocked(record)
	}
	record.updateHead(epoch, revision, stateHash, confirmed)
}

// BeginSync returns the attempt a caller passes to Applied for a fetch of one
// service that it is about to start. Call it before the fetch, never after:
// the attempt captures the point after which a resync or a removal makes the
// fetched result stale.
func (m *Machine) BeginSync(serviceID string) SyncAttempt {
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt := SyncAttempt{serviceID: serviceID}
	if record := m.records[serviceID]; record != nil {
		attempt.generation = record.generation
	}
	return attempt
}

// AttemptCurrent reports whether nothing has invalidated the attempt yet.
// Applied makes the same check atomically; callers use this only to avoid
// publishing a result that Applied would ignore.
func (m *Machine) AttemptCurrent(attempt SyncAttempt) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attemptCurrentLocked(attempt)
}

func (m *Machine) attemptCurrentLocked(attempt SyncAttempt) bool {
	record := m.records[attempt.serviceID]
	if record == nil {
		// The service was unknown when the attempt began and still is.
		return attempt.generation == 0
	}
	return record.generation == attempt.generation
}

// Applied records that one service now has a verified local copy. The caller
// reports the result of a successful Sync, including a re-verification of an
// unchanged copy, together with the attempt it obtained from BeginSync before
// the fetch.
//
// Applied ignores a stale attempt: a fetch that started before a resync, a
// removal or a re-add of the service cannot make its result servable, even
// when the result is byte for byte what a fresh fetch would return. It is also
// ignored for a service that was removed: a sync that was already in flight
// must not resurrect it.
func (m *Machine) Applied(attempt SyncAttempt, state *ServiceState) error {
	if state == nil {
		return fmt.Errorf("adsync: nil service state")
	}
	if attempt.serviceID != state.ServiceID {
		return fmt.Errorf("adsync: sync attempt for service %q reports service %q", attempt.serviceID, state.ServiceID)
	}
	if !validEntityID(state.ServiceID) {
		return fmt.Errorf("%w: invalid service id %q", ErrProtocol, state.ServiceID)
	}
	if !validServiceName(state.Snapshot.Name) {
		return fmt.Errorf("%w: invalid service name %q", ErrProtocol, state.Snapshot.Name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	m.advanceLocked(now)
	if !m.attemptCurrentLocked(attempt) {
		return nil
	}
	record := m.records[state.ServiceID]
	if record == nil {
		if m.setKnown {
			return nil
		}
		// Until the first authoritative ack the set is unknown, but the
		// protocol still bounds how many services one source can serve, so a
		// caller cannot grow the record map without bound.
		if len(m.records) >= maxServicesPerSource {
			return fmt.Errorf("%w: more than %d services reported before the first ack", ErrProtocol, maxServicesPerSource)
		}
		record = m.newRecordLocked(state.ServiceID, state.Snapshot.Name)
	}
	if m.setKnown && !record.setMember {
		return nil
	}
	if !m.setKnown {
		// No authoritative set yet, so the snapshot is the best name source.
		record.name = state.Snapshot.Name
	}
	record.hasLocal = true
	record.localRevision = state.Revision
	record.localEpoch = state.Epoch
	record.localStateHash = state.StateHash
	record.pendingResync = false
	record.verifiedSinceStop = true
	m.recomputeRecordLocked(record)
	if m.stopped {
		m.tryRecoverLocked(now)
	}
	return nil
}

// SyncFailed records the outcome of a failed sync for one service.
//
// An authentication failure stops the instance as revoked. A service the
// source reports missing or gone is removed. Every other failure leaves the
// state unchanged; the caller retries, and a persistent integrity failure is
// reported with Stop(ReasonIntegrityFailure).
func (m *Machine) SyncFailed(serviceID string, err error) {
	if err == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceLocked(m.clock.Now())
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
		m.stopLocked(ReasonRevoked)
		return
	}
	record := m.records[serviceID]
	if record == nil {
		return
	}
	if !errors.Is(err, ErrServiceNotFound) && !errors.Is(err, ErrSourceGone) {
		return
	}
	if m.setKnown && !record.setMember {
		return
	}
	m.removeRecordLocked(record)
}

// Resync marks one service for a forced re-synchronization after a resync
// command. The service stops serving local decisions immediately and reports
// the resync reason until a freshly verified copy arrives. The caller must
// discard the old copy and fetch a new one, for example with Syncer.Forget
// followed by Syncer.Sync.
//
// Resync invalidates every attempt that began before it, so a fetch already
// in flight when the command arrives cannot report its result as the fresh
// copy: Applied ignores it, and only an attempt begun after Resync can bring
// the service back to in_sync.
func (m *Machine) Resync(serviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceLocked(m.clock.Now())
	record := m.records[serviceID]
	if record == nil || (m.setKnown && !record.setMember) || record.state == SyncStateRemoved {
		return
	}
	m.invalidateAttemptsLocked(record)
	record.pendingResync = true
	record.hasLocal = false
	record.localRevision = 0
	record.localEpoch = ""
	record.localStateHash = Hash{}
	record.verifiedSinceStop = false
	m.recomputeRecordLocked(record)
}

// Stop moves the instance to stopped with the given reason. A non-terminal
// stop is cleared by a confirming ack followed by a verified copy of every
// service; a terminal stop is final for the lifetime of the machine.
func (m *Machine) Stop(reason Reason) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceLocked(m.clock.Now())
	m.stopLocked(reason)
}

// Pause closes downstream distribution. It leaves liveness, synchronization
// and local evaluation untouched.
func (m *Machine) Pause() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceLocked(m.clock.Now())
	m.paused = true
}

// Resume reopens downstream distribution.
func (m *Machine) Resume() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceLocked(m.clock.Now())
	m.paused = false
}

// Status returns the instance status and the state of one service by name.
// The boolean is false when the instance does not know the name at all; a
// removed service is reported with State removed.
func (m *Machine) Status(serviceName string) (Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	status := Status{Instance: m.instanceStatusLocked(now)}
	record := m.recordByNameLocked(serviceName)
	if record == nil {
		return status, false
	}
	status.Service = record.status()
	return status, true
}

// Instance returns the instance-level status.
func (m *Machine) Instance() InstanceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.instanceStatusLocked(m.clock.Now())
}

// Services returns the status of every tracked service, including removed
// tombstones, sorted by service ID.
func (m *Machine) Services() []ServiceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	services := make([]ServiceStatus, 0, len(m.records))
	for _, record := range m.records {
		services = append(services, record.status())
	}
	sort.Slice(services, func(left, right int) bool {
		return services[left].ID < services[right].ID
	})
	return services
}

// Thresholds returns the effective heartbeat, warning and stop intervals.
func (m *Machine) Thresholds() Thresholds {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.effectiveThresholdsLocked()
}

var _ StatusProvider = (*Machine)(nil)

func validateAck(ack Ack) error {
	if len(ack.Services) > maxServicesPerSource {
		return fmt.Errorf("%w: %d services exceed the protocol bound", ErrProtocol, len(ack.Services))
	}
	seenIDs := make(map[string]struct{}, len(ack.Services))
	seenNames := make(map[string]struct{}, len(ack.Services))
	for index := range ack.Services {
		if err := ack.Services[index].validate(); err != nil {
			return err
		}
		if _, duplicate := seenIDs[ack.Services[index].ID]; duplicate {
			return fmt.Errorf("%w: duplicate service id %q", ErrProtocol, ack.Services[index].ID)
		}
		seenIDs[ack.Services[index].ID] = struct{}{}
		// Service names are unique per account and the request gate resolves a
		// service by name, so an ack with two active services of one name is
		// ambiguous.
		if _, duplicate := seenNames[ack.Services[index].Name]; duplicate {
			return fmt.Errorf("%w: duplicate service name %q", ErrProtocol, ack.Services[index].Name)
		}
		seenNames[ack.Services[index].Name] = struct{}{}
	}
	return nil
}

func (m *Machine) applyThresholdsLocked(ack Ack) error {
	cloudHeartbeat, cloudStale, cloudStop := m.cloudHeartbeat, m.cloudStale, m.cloudStop
	if ack.HeartbeatInterval >= minHeartbeatInterval && ack.HeartbeatInterval <= maxHeartbeatInterval {
		cloudHeartbeat = ack.HeartbeatInterval
	}
	if ack.StaleAfter > 0 && ack.StaleAfter <= maxStopAfter {
		cloudStale = ack.StaleAfter
	}
	if ack.StopAfter >= minStopAfter && ack.StopAfter <= maxStopAfter {
		cloudStop = ack.StopAfter
	}
	thresholds := m.effectiveThresholds(cloudHeartbeat, cloudStale, cloudStop)
	if thresholds.StaleAfter <= thresholds.HeartbeatInterval || thresholds.StopAfter <= thresholds.StaleAfter {
		return fmt.Errorf("%w: source thresholds produce an impossible heartbeat cadence", ErrProtocol)
	}
	m.cloudHeartbeat, m.cloudStale, m.cloudStop = cloudHeartbeat, cloudStale, cloudStop
	return nil
}

func (m *Machine) effectiveThresholdsLocked() Thresholds {
	return m.effectiveThresholds(m.cloudHeartbeat, m.cloudStale, m.cloudStop)
}

func (m *Machine) effectiveThresholds(cloudHeartbeat, cloudStale, cloudStop time.Duration) Thresholds {
	thresholds := Thresholds{
		HeartbeatInterval: m.localHeartbeat,
		StaleAfter:        m.localStale,
		StopAfter:         m.localStop,
	}
	if cloudHeartbeat != 0 && cloudHeartbeat < thresholds.HeartbeatInterval {
		thresholds.HeartbeatInterval = cloudHeartbeat
	}
	if cloudStale != 0 && cloudStale < thresholds.StaleAfter {
		thresholds.StaleAfter = cloudStale
	}
	if cloudStop != 0 && cloudStop < thresholds.StopAfter {
		thresholds.StopAfter = cloudStop
	}
	return thresholds
}

func (m *Machine) applySetLocked(services []Service, confirmed bool) {
	m.setKnown = true
	for _, record := range m.records {
		record.setMember = false
	}
	for _, service := range services {
		record := m.records[service.ID]
		if record == nil {
			record = m.newRecordLocked(service.ID, service.Name)
		} else if record.state == SyncStateRemoved {
			// A re-added service starts over: nothing fetched while it was
			// removed may be reported for it.
			m.invalidateAttemptsLocked(record)
		}
		record.setMember = true
		record.name = service.Name
		m.updateRecordHeadLocked(record, service.Epoch, service.Revision, service.HeadStateHash, confirmed)
	}
	for _, record := range m.records {
		m.recomputeRecordLocked(record)
	}
	m.evictLocked()
}

func (m *Machine) newRecordLocked(id, name string) *serviceRecord {
	record := &serviceRecord{id: id, name: name}
	m.invalidateAttemptsLocked(record)
	m.records[id] = record
	return record
}

// invalidateAttemptsLocked gives the record a generation no earlier attempt
// carries. Generations come from one machine-wide sequence, so a record that
// is evicted and created again never reuses one.
func (m *Machine) invalidateAttemptsLocked(record *serviceRecord) {
	m.attemptSeq++
	record.generation = m.attemptSeq
}

// recomputeRecordLocked derives the service state from the local copy, the
// confirmed head and the set membership. It is the only place that assigns
// state and reason.
func (m *Machine) recomputeRecordLocked(record *serviceRecord) {
	if m.setKnown && !record.setMember {
		m.removeRecordLocked(record)
		return
	}
	switch {
	case record.pendingResync:
		record.state = SyncStateSyncing
		record.reason = SyncReasonResync
	case !record.hasLocal:
		record.state = SyncStateSyncing
		record.reason = SyncReasonBootstrap
	case !record.headKnown:
		record.state = SyncStateSyncing
		record.reason = SyncReasonCatchUp
	case record.localEpoch != "" && record.epoch != "" && record.localEpoch != record.epoch:
		record.state = SyncStateSyncing
		record.reason = SyncReasonCatchUp
	case record.headConfirmed &&
		record.localRevision == record.headRevision &&
		record.localStateHash == record.headStateHash:
		record.state = SyncStateInSync
		record.reason = ""
	default:
		record.state = SyncStateSyncing
		record.reason = SyncReasonCatchUp
	}
}

func (m *Machine) removeRecordLocked(record *serviceRecord) {
	if record.state != SyncStateRemoved {
		m.nextSeq++
		record.removedSeq = m.nextSeq
		m.invalidateAttemptsLocked(record)
	}
	record.state = SyncStateRemoved
	record.reason = ""
	record.setMember = false
	record.headKnown = false
	record.headConfirmed = false
	record.epoch = ""
	record.headRevision = 0
	record.headStateHash = Hash{}
	record.hasLocal = false
	record.localRevision = 0
	record.localEpoch = ""
	record.localStateHash = Hash{}
	record.pendingResync = false
	record.verifiedSinceStop = false
}

// updateHead advances the known head. Within one epoch a lower revision never
// wins; a different epoch replaces the head because revisions are not
// comparable across epochs. Only an ack confirms a head: a notification can
// advance it, but in_sync waits for the ack that covers the new revision.
func (r *serviceRecord) updateHead(epoch string, revision uint64, stateHash Hash, confirmed bool) {
	if !r.headKnown {
		r.headKnown = true
		r.headConfirmed = confirmed
		r.epoch = epoch
		r.headRevision = revision
		r.headStateHash = stateHash
		return
	}
	if r.epoch == "" {
		r.epoch = epoch
	} else if epoch != "" && epoch != r.epoch {
		r.epoch = epoch
		r.headRevision = revision
		r.headStateHash = stateHash
		r.headConfirmed = confirmed
		r.verifiedSinceStop = false
		// A copy from the previous epoch cannot be compared with the new
		// head, so discard it. A delayed message that still carries the old
		// epoch must not be able to make that copy servable again.
		r.hasLocal = false
		r.localRevision = 0
		r.localEpoch = ""
		r.localStateHash = Hash{}
		return
	}
	switch {
	case revision > r.headRevision:
		r.headRevision = revision
		r.headStateHash = stateHash
		r.headConfirmed = confirmed
	case revision == r.headRevision && confirmed:
		r.headConfirmed = true
		if !stateHash.IsZero() {
			r.headStateHash = stateHash
		}
	}
}

func (r *serviceRecord) status() ServiceStatus {
	return ServiceStatus{
		ID:       r.id,
		Name:     r.name,
		State:    r.state,
		Reason:   r.reason,
		Revision: r.localRevision,
		Head:     r.headRevision,
	}
}

func (m *Machine) evictLocked() {
	if len(m.records) <= maxTrackedServices {
		return
	}
	removed := make([]*serviceRecord, 0, len(m.records))
	for _, record := range m.records {
		if record.state == SyncStateRemoved {
			removed = append(removed, record)
		}
	}
	sort.Slice(removed, func(left, right int) bool {
		return removed[left].removedSeq < removed[right].removedSeq
	})
	for _, record := range removed {
		if len(m.records) <= maxTrackedServices {
			return
		}
		delete(m.records, record.id)
	}
}

func (m *Machine) confirmationBaseLocked() time.Time {
	if m.lastConfirmedAckAt.IsZero() {
		return m.since
	}
	return m.lastConfirmedAckAt
}

// advanceLocked materializes a threshold crossing before an input is applied,
// so an instinctive "the ack arrived, therefore we are live again" can never
// skip the stop state. If the instance is already stopped the crossing is
// irrelevant and the latch is kept.
func (m *Machine) advanceLocked(now time.Time) {
	if m.stopped {
		return
	}
	if now.Sub(m.confirmationBaseLocked()) > m.effectiveThresholdsLocked().StopAfter {
		m.enterStoppedLocked(ReasonHeartbeatLost)
	}
}

func (m *Machine) enterStoppedLocked(reason Reason) {
	m.stopped = true
	m.stopReason = reason
	m.ackSinceStop = false
	for _, record := range m.records {
		record.verifiedSinceStop = false
	}
}

func (m *Machine) stopLocked(reason Reason) {
	if !m.stopped {
		m.enterStoppedLocked(reason)
		return
	}
	if reason.Terminal() && !m.stopReason.Terminal() {
		m.stopReason = reason
	}
}

// tryRecoverLocked leaves stopped only when a confirmed ack arrived after the
// stop and every service in the authoritative set has a freshly verified copy
// at its head. A heartbeat alone therefore never recovers an instance.
func (m *Machine) tryRecoverLocked(now time.Time) {
	if !m.stopped || !m.ackSinceStop || m.stopReason.Terminal() {
		return
	}
	for _, record := range m.records {
		if m.setKnown && !record.setMember {
			continue
		}
		if record.state != SyncStateInSync || !record.verifiedSinceStop {
			return
		}
	}
	m.stopped = false
	m.stopReason = ""
	m.ackSinceStop = false
	// A completed verification is stronger evidence of reachability than an
	// ack and restarts the liveness window.
	m.lastConfirmedAckAt = now
}

func (m *Machine) livenessLocked(now time.Time) Liveness {
	if m.stopped {
		return LivenessStopped
	}
	age := now.Sub(m.confirmationBaseLocked())
	thresholds := m.effectiveThresholdsLocked()
	switch {
	case age > thresholds.StopAfter:
		return LivenessStopped
	case age > thresholds.StaleAfter:
		return LivenessStale
	default:
		return LivenessLive
	}
}

func (m *Machine) instanceStatusLocked(now time.Time) InstanceStatus {
	liveness := m.livenessLocked(now)
	status := InstanceStatus{
		Liveness:           liveness,
		Paused:             m.paused,
		LastConfirmedAckAt: m.lastConfirmedAckAt,
		Since:              m.since,
	}
	if liveness == LivenessStopped {
		if m.stopped {
			status.Reason = m.stopReason
		} else {
			status.Reason = ReasonHeartbeatLost
		}
	}
	return status
}

// recordByNameLocked resolves the current name of a service. An active service
// always wins over a removed tombstone with the same name, so a replacement
// service is not shadowed by its predecessor. A tombstone is returned only
// when no active record carries the name. Duplicate active names resolve to
// the lowest service ID so the result stays deterministic.
func (m *Machine) recordByNameLocked(name string) *serviceRecord {
	if name == "" {
		return nil
	}
	var active, removed *serviceRecord
	for _, record := range m.records {
		if record.name != name {
			continue
		}
		if record.state == SyncStateRemoved {
			if removed == nil || record.id < removed.id {
				removed = record
			}
			continue
		}
		if active == nil || record.id < active.id {
			active = record
		}
	}
	if active != nil {
		return active
	}
	return removed
}
