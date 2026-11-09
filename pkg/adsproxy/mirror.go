// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

const (
	// DefaultMaxLogCommits bounds the per-service change log by revision count.
	// It matches the retention the distribution design gives the source's own
	// change log.
	DefaultMaxLogCommits = 1000
	// DefaultMaxLogBytes bounds the per-service change log by approximate
	// memory. One page is at most 1 MiB and a commit at most 200 changes, so
	// the byte bound usually wins for large policies.
	DefaultMaxLogBytes = 16 << 20
	// MaxServices is the protocol bound on the services one instance serves.
	MaxServices = 10
	// maxRetiredIdentities bounds the source identities one service copy
	// keeps replay protection for. Once a copy has moved away from this many
	// identities, it refuses further identity replacement and stops being
	// advertised rather than accumulating replay state without limit.
	maxRetiredIdentities = 64
	// maxTombstones bounds the removed-service tombstones kept next to the
	// active services. A tombstone answers source_gone and is the deletion
	// guard against a delayed snapshot reviving a removed service, so it is
	// never evicted: once the combined store is full, a new service is refused
	// (fail closed) instead of forgetting that guard.
	maxTombstones = 1024
	// maxChangesPageBytes is the contract's response-page bound. A page that
	// would exceed it is trimmed.
	maxChangesPageBytes = 1 << 20
	// maxCommitsPerPage is the contract's commit-count bound for one page.
	maxCommitsPerPage = 100
)

var (
	ulidPattern        = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
	uuidV7Pattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
)

func validEntityID(id string) bool {
	return ulidPattern.MatchString(id) || uuidV7Pattern.MatchString(id)
}

// MirrorOptions configures a Mirror.
type MirrorOptions struct {
	// PublicKeys is the out-of-band trust anchor for the source's snapshot and
	// commit signatures. It is required: a proxy only serves data whose Cloud
	// signature it has verified with keys the operator distributed separately.
	PublicKeys *adsync.PublicKeySet
	// MaxLogCommits bounds the per-service change log; zero uses
	// DefaultMaxLogCommits.
	MaxLogCommits int
	// MaxLogBytes bounds the approximate in-memory size of the per-service
	// change log; zero uses DefaultMaxLogBytes.
	MaxLogBytes int64
}

// HeadEvent announces that one service's verified revision advanced. It is how
// a Mirror tells the server's fan-out hub that a downstream may have something
// new to pull.
type HeadEvent struct {
	ServiceID string
	Epoch     string
	Revision  uint64
	StateHash adsync.Hash
}

// ServiceAvailability says whether the proxy can serve one service.
type ServiceAvailability int

const (
	// ServiceAvailable means the service has a signed snapshot and a usable
	// change log.
	ServiceAvailable ServiceAvailability = iota
	// ServiceUnknown means the proxy never held the service.
	ServiceUnknown
	// ServiceRemoved means the service was held and has been removed.
	ServiceRemoved
	// ServiceNeedsSnapshot means the change log reached its bound, so the
	// proxy needs a fresh signed snapshot before it can serve the service
	// again.
	ServiceNeedsSnapshot
)

// ServiceInfo is one service's current verified identity.
type ServiceInfo struct {
	ID         string
	Name       string
	SourceID   string
	Epoch      string
	Revision   uint64
	StateHash  adsync.Hash
	CommitHash adsync.Hash
}

// Mirror holds the verified copies of the services this ADS serves downstream.
// It is fed by the synchronization loop with the signed snapshots and change
// pages the ADS received, verifies them again, applies commits and keeps a
// bounded log.
//
// All methods are safe for concurrent use. Every value returned is a copy;
// callers may retain and read it without locking.
type Mirror struct {
	keys          *adsync.PublicKeySet
	maxLogCommits int
	maxLogBytes   int64

	mu        sync.RWMutex
	services  map[string]*serviceCopy
	version   int64
	observers map[uint64]func(HeadEvent)
	nextID    uint64
}

// serviceCopy is one verified service: the signed snapshot it is anchored to,
// the state after every log commit, and the log itself.
type serviceCopy struct {
	id       string
	name     string
	sourceID string
	epoch    string

	signed     *adsync.SnapshotResponse
	snapshot   adsync.Snapshot
	revision   uint64
	stateHash  adsync.Hash
	commitHash adsync.Hash

	log      []adsync.Commit
	logBytes int64

	removed  bool
	overflow bool
	// unavailable withholds a verified copy from downstream serving after the
	// runtime could not reach the head advertised for it. Unlike removed, the
	// copy and its log are kept: the runtime clears the flag with MarkAvailable
	// only after it has verified the advertised identity and revision, so a
	// multi-page catch-up stays withheld until it has completed.
	unavailable bool
	// tombstoneRevision is the revision that removed the service: a signed
	// delete commit or the revision current at an administrative removal. A
	// snapshot at or below it belongs to the state that was removed and cannot
	// revive the service; only newer source state can.
	tombstoneRevision uint64
	// retired holds every source identity this copy has moved away from. A
	// late snapshot from a retired identity belongs to policy the source
	// already replaced, and revisions of different identities cannot be
	// ordered, so it is ignored rather than allowed to replace the copy. The
	// set is never evicted: replay protection has to outlive the transition.
	retired map[retiredIdentity]struct{}
}

// retiredIdentity is one sourceId/epoch pair a service copy has moved away from.
type retiredIdentity struct {
	sourceID string
	epoch    string
}

// identityRetired reports whether sourceID/epoch is an identity this copy has
// moved away from.
func (s *serviceCopy) identityRetired(sourceID, epoch string) bool {
	_, retired := s.retired[retiredIdentity{sourceID: sourceID, epoch: epoch}]
	return retired
}

// retireIdentity records the identity the copy is leaving. Entries are never
// dropped: a replayed snapshot from any retired identity must stay rejected.
func (s *serviceCopy) retireIdentity(sourceID, epoch string) {
	if sourceID == "" {
		return
	}
	if s.retired == nil {
		s.retired = make(map[retiredIdentity]struct{})
	}
	s.retired[retiredIdentity{sourceID: sourceID, epoch: epoch}] = struct{}{}
}

// NewMirror creates an empty mirror.
func NewMirror(options MirrorOptions) (*Mirror, error) {
	if options.PublicKeys == nil {
		return nil, fmt.Errorf("%w: policy source public keys are required", ErrConfig)
	}
	maxLogCommits := options.MaxLogCommits
	if maxLogCommits == 0 {
		maxLogCommits = DefaultMaxLogCommits
	}
	if maxLogCommits < 1 {
		return nil, fmt.Errorf("%w: MaxLogCommits must be positive", ErrConfig)
	}
	maxLogBytes := options.MaxLogBytes
	if maxLogBytes == 0 {
		maxLogBytes = DefaultMaxLogBytes
	}
	if maxLogBytes < 1 {
		return nil, fmt.Errorf("%w: MaxLogBytes must be positive", ErrConfig)
	}
	return &Mirror{
		keys:          options.PublicKeys,
		maxLogCommits: maxLogCommits,
		maxLogBytes:   maxLogBytes,
		services:      make(map[string]*serviceCopy),
		version:       1,
		observers:     make(map[uint64]func(HeadEvent)),
	}, nil
}

// AddObserver registers a callback for verified head advances. It returns a
// function that removes the observer. Observers are called synchronously after
// the mirror's lock is released, so they must not block.
func (m *Mirror) AddObserver(observer func(HeadEvent)) func() {
	m.mu.Lock()
	id := m.nextID
	m.nextID++
	m.observers[id] = observer
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.observers, id)
		m.mu.Unlock()
	}
}

// ApplySnapshot records a signed snapshot, for example the one the ADS
// synchronized from its own source. The bytes are not trusted: the state hash,
// the commit hash and the signature are verified again here.
//
// A snapshot older than the copy already held for the same sourceId and epoch
// is ignored, so a late delivery cannot roll the proxy back. A snapshot for a
// different sourceId or epoch replaces the copy and starts a new log, which is
// how a source change reaches downstream instances.
//
// Callers synchronizing a discovered service pass that service as expected.
// The snapshot must then match both its id and epoch; sourceId is always bound
// to the requested service id, never to a different service's signed dataset.
func (m *Mirror) ApplySnapshot(response *adsync.SnapshotResponse, expected ...adsync.Service) error {
	if response == nil {
		return fmt.Errorf("%w: nil snapshot", ErrConfig)
	}
	if len(expected) > 1 {
		return fmt.Errorf("%w: ApplySnapshot accepts at most one expected service", ErrConfig)
	}
	if response.SourceID != response.ServiceID {
		return fmt.Errorf("%w: snapshot source %q does not match service %q", adsync.ErrProtocol, response.SourceID, response.ServiceID)
	}
	if len(expected) == 1 {
		service := expected[0]
		if response.ServiceID != service.ID || response.SourceID != service.ID || response.Epoch != service.Epoch {
			return fmt.Errorf("%w: snapshot identity %q/%q/%q does not match requested service %q epoch %q",
				adsync.ErrProtocol, response.ServiceID, response.SourceID, response.Epoch, service.ID, service.Epoch)
		}
	}
	copied := cloneSnapshotResponse(*response)
	if err := m.verifySnapshot(&copied); err != nil {
		return err
	}
	event, notify, err := m.applySnapshot(&copied)
	if err != nil {
		return err
	}
	if notify {
		m.notify(event)
	}
	return nil
}

func (m *Mirror) applySnapshot(response *adsync.SnapshotResponse) (HeadEvent, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing := m.services[response.ServiceID]
	wasNew := existing == nil
	availableBefore := false
	if wasNew {
		if err := m.reserveServiceLocked(); err != nil {
			return HeadEvent{}, false, err
		}
		existing = &serviceCopy{id: response.ServiceID}
		m.services[response.ServiceID] = existing
	} else {
		availableBefore = m.availableLocked(existing)
	}

	identityChanged := !wasNew && (existing.sourceID != response.SourceID || existing.epoch != response.Epoch)
	if identityChanged && existing.identityRetired(response.SourceID, response.Epoch) {
		// The source already moved away from this identity. A delayed snapshot
		// from it belongs to replaced policy, and its revision cannot be
		// compared with the current identity's, so it must not replace the copy.
		return HeadEvent{}, false, nil
	}
	previousRevision := existing.revision
	previousName := existing.name
	if existing.removed && !identityChanged && response.Revision <= existing.tombstoneRevision {
		// The snapshot reproduces the state the service was removed at or
		// before. It may have been fetched before the removal, so it cannot
		// revive the copy; only newer source state can.
		return HeadEvent{}, false, nil
	}
	if !existing.removed && !identityChanged {
		if response.Revision < existing.revision {
			// A stale snapshot cannot roll back a verified copy.
			return HeadEvent{}, false, nil
		}
		if response.Revision == existing.revision &&
			!existing.stateHash.IsZero() && response.StateHash != existing.stateHash {
			return HeadEvent{}, false, fmt.Errorf(
				"%w: snapshot %q revision %d has two different state hashes",
				adsync.ErrIntegrity, response.ServiceID, response.Revision)
		}
		if response.Revision == existing.revision &&
			!existing.commitHash.IsZero() && response.CommitHash != existing.commitHash {
			// The change log is anchored to the commit hash. Accepting a
			// different history at the same revision would make the next
			// commit fail its previous-hash check while the snapshot endpoint
			// kept answering 304 for the cached anchor.
			return HeadEvent{}, false, fmt.Errorf(
				"%w: snapshot %q revision %d has two different commit hashes",
				adsync.ErrIntegrity, response.ServiceID, response.Revision)
		}
	}

	if identityChanged {
		if len(existing.retired) >= maxRetiredIdentities {
			// Fail safe: stop advertising rather than keep unbounded replay
			// state. The current identity can still be refreshed; only another
			// identity replacement is refused.
			if !existing.overflow {
				existing.overflow = true
				if availableBefore {
					m.version++
				}
			}
			return HeadEvent{}, false, fmt.Errorf("%w: %q", ErrIdentityChurn, response.ServiceID)
		}
		existing.retireIdentity(existing.sourceID, existing.epoch)
	}
	existing.name = response.Snapshot.Name
	existing.sourceID = response.SourceID
	existing.epoch = response.Epoch
	signed := cloneSnapshotResponse(*response)
	existing.signed = &signed
	existing.snapshot = cloneSnapshot(response.Snapshot)
	existing.revision = response.Revision
	existing.stateHash = response.StateHash
	existing.commitHash = response.CommitHash
	existing.log = nil
	existing.logBytes = 0
	existing.overflow = false
	existing.removed = false
	existing.tombstoneRevision = 0

	// A snapshot can make a service visible again (revival after removal or
	// overflow) or remove it from the advertised set. Downstreams compare the
	// version to learn that discovery changed, so any visibility transition
	// must advance it, in addition to identity and name changes.
	if availableBefore != m.availableLocked(existing) {
		m.version++
	}
	if identityChanged {
		m.version++
	}
	if previousName != "" && previousName != existing.name {
		m.version++
	}
	notify := previousName == "" || identityChanged || response.Revision != previousRevision
	return HeadEvent{
		ServiceID: existing.id,
		Epoch:     existing.epoch,
		Revision:  existing.revision,
		StateHash: existing.stateHash,
	}, notify, nil
}

// ApplyChanges records a page of signed commits and applies them to the copy.
// The page must continue exactly at the copy's current revision; a gap, an
// identity change or any failed verification is reported and nothing is
// applied. A signed service delete marks the service removed and reports
// adsync.ErrSourceGone.
//
// A page that is already covered by the copy is ignored, so a duplicate
// delivery is harmless.
func (m *Mirror) ApplyChanges(page *adsync.ChangesResponse) error {
	if page == nil {
		return fmt.Errorf("%w: nil change page", ErrConfig)
	}
	event, notify, err := m.applyChanges(page)
	if err != nil {
		return err
	}
	if notify {
		m.notify(event)
	}
	return nil
}

func (m *Mirror) applyChanges(page *adsync.ChangesResponse) (HeadEvent, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	service := m.services[page.ServiceID]
	if service == nil {
		return HeadEvent{}, false, fmt.Errorf("%w: %q", ErrUnknownService, page.ServiceID)
	}
	if service.removed {
		return HeadEvent{}, false, fmt.Errorf("%w: %q", ErrServiceRemoved, page.ServiceID)
	}
	if service.overflow {
		return HeadEvent{}, false, fmt.Errorf("%w: %q", ErrSnapshotRequired, page.ServiceID)
	}
	if page.SourceID != service.sourceID || page.Epoch != service.epoch {
		return HeadEvent{}, false, fmt.Errorf(
			"%w: page is for source %q epoch %q, copy is source %q epoch %q",
			ErrIdentityChanged, page.SourceID, page.Epoch, service.sourceID, service.epoch)
	}
	if !validRevision(page.From) || !validRevision(page.To) || !validRevision(page.Head) {
		return HeadEvent{}, false, fmt.Errorf("%w: revision out of range", adsync.ErrProtocol)
	}
	if page.To > page.Head {
		return HeadEvent{}, false, fmt.Errorf("%w: page ends after the head", adsync.ErrProtocol)
	}
	if page.To <= service.revision {
		// Already applied. The contract makes repeated delivery safe.
		return HeadEvent{}, false, nil
	}
	if page.From != service.revision+1 {
		return HeadEvent{}, false, fmt.Errorf(
			"%w: page starts at %d, copy is at %d", ErrSnapshotRequired, page.From, service.revision)
	}
	if len(page.Commits) == 0 {
		return HeadEvent{}, false, fmt.Errorf("%w: page has no commits", adsync.ErrProtocol)
	}

	// The copy is advertised at this point; watch whether applying the page
	// takes it out of the advertised set (a signed service delete or a log that
	// outgrew its bound) so the service-set version reflects it.
	availableBefore := m.availableLocked(service)
	previousName := service.name
	working := cloneSnapshot(service.snapshot)
	previousHash := service.commitHash
	for index := range page.Commits {
		commit := &page.Commits[index]
		if commit.Revision != page.From+uint64(index) {
			return HeadEvent{}, false, fmt.Errorf(
				"%w: commit %d is not contiguous", adsync.ErrProtocol, commit.Revision)
		}
		if err := m.verifyCommit(service.sourceID, service.epoch, previousHash, commit); err != nil {
			return HeadEvent{}, false, err
		}
		if err := checkServiceChangeTargets(service.id, commit.Changes); err != nil {
			return HeadEvent{}, false, err
		}
		if err := adsync.ApplyChanges(&working, commit.Changes); err != nil {
			if errors.Is(err, adsync.ErrSourceGone) {
				service.removed = true
				service.name = ""
				service.signed = nil
				service.snapshot = adsync.Snapshot{}
				service.revision = 0
				service.stateHash = adsync.Hash{}
				service.commitHash = adsync.Hash{}
				service.log = nil
				service.logBytes = 0
				service.overflow = false
				service.tombstoneRevision = commit.Revision
				if availableBefore {
					m.version++
				}
				return HeadEvent{}, false, err
			}
			return HeadEvent{}, false, err
		}
		previousHash = commit.Hash
	}

	if last := page.Commits[len(page.Commits)-1]; last.Revision != page.To {
		return HeadEvent{}, false, fmt.Errorf(
			"%w: last commit is at %d, page ends at %d", adsync.ErrProtocol, last.Revision, page.To)
	}
	computed := adsync.StateHash(working)
	if page.To == page.Head && computed != page.HeadStateHash {
		return HeadEvent{}, false, fmt.Errorf(
			"%w: state hash does not match the head at revision %d", adsync.ErrIntegrity, page.Head)
	}

	service.snapshot = working
	service.name = working.Name
	service.revision = page.To
	service.stateHash = computed
	service.commitHash = previousHash
	for index := range page.Commits {
		commit := cloneCommit(page.Commits[index])
		service.log = append(service.log, commit)
		service.logBytes += commitSize(commit)
		if encoded, err := json.Marshal(commit); err == nil && len(encoded) > maxChangesPageBytes {
			// A single commit that cannot fit in one contract page can never
			// be served downstream; only a fresh snapshot can help.
			service.overflow = true
		}
	}
	if len(service.log) > m.maxLogCommits || service.logBytes > m.maxLogBytes {
		// The proxy cannot synthesize a signed snapshot, so a log it can no
		// longer resume from makes the service unserviceable until the process
		// refreshes the snapshot.
		service.overflow = true
	}
	if previousName != service.name {
		m.version++
	}
	if availableBefore != m.availableLocked(service) {
		m.version++
	}
	return HeadEvent{
		ServiceID: service.id,
		Epoch:     service.epoch,
		Revision:  service.revision,
		StateHash: service.stateHash,
	}, true, nil
}

// RemoveService stops serving a service downstream. The tombstone keeps making
// a data request for it report source_gone instead of an unknown service. A
// snapshot fetched before the removal point cannot revive it; only a newer one
// from the same source and epoch can.
func (m *Mirror) RemoveService(serviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	service := m.services[serviceID]
	if service == nil || service.removed {
		return
	}
	service.removed = true
	service.signed = nil
	service.snapshot = adsync.Snapshot{}
	service.tombstoneRevision = service.revision
	service.revision = 0
	service.stateHash = adsync.Hash{}
	service.commitHash = adsync.Hash{}
	service.log = nil
	service.logBytes = 0
	service.overflow = false
	m.version++
}

// MarkUnavailable stops advertising and serving one service while keeping its
// verified copy. The runtime calls it when the source advertises a head for the
// service that the mirror cannot fetch: serving the older copy would let
// downstream instances keep evaluating policy the source has already replaced.
// Snapshots and change pages applied while the flag is set stay withheld; the
// runtime restores availability with MarkAvailable after it has verified that
// the copy reached the advertised identity and revision.
func (m *Mirror) MarkUnavailable(serviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	service := m.services[serviceID]
	if service == nil || service.removed || service.unavailable {
		return
	}
	wasAvailable := m.availableLocked(service)
	service.unavailable = true
	if wasAvailable {
		m.version++
	}
}

// MarkAvailable makes a withheld copy servable again. The runtime calls it only
// after CurrentInfo confirms the advertised identity and revision, so a copy
// applied during a multi-page catch-up cannot be served before the catch-up
// completed.
func (m *Mirror) MarkAvailable(serviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	service := m.services[serviceID]
	if service == nil || service.removed || !service.unavailable {
		return
	}
	service.unavailable = false
	if m.availableLocked(service) {
		m.version++
	}
}

// reserveServiceLocked makes room for one new service. Removed services are
// tombstones, not active copies, so they do not count against MaxServices. A
// tombstone is a deletion guard and is never evicted; if the combined store is
// full, the new service is refused instead.
func (m *Mirror) reserveServiceLocked() error {
	active := 0
	for _, service := range m.services {
		if !service.removed {
			active++
		}
	}
	if active >= MaxServices {
		return fmt.Errorf("%w: %d services", ErrTooManyServices, active)
	}
	if len(m.services) >= MaxServices+maxTombstones {
		return fmt.Errorf("%w: %d services and tombstones", ErrTooManyServices, len(m.services))
	}
	return nil
}

// Services returns every service this proxy currently advertises, sorted by
// service id. A service whose log reached its bound is not advertised: the
// proxy cannot bring a downstream from a signed snapshot to the service's head
// any more, so failing closed is the only correct answer.
func (m *Mirror) Services() []adsync.Service {
	services, _ := m.ServicesVersion()
	return services
}

// ServicesVersion returns the advertised services together with the version of
// the service set they were read from. Both are read under one lock: a caller
// that records the version must never pair it with a list from a different
// version, or it can miss an added or removed service.
func (m *Mirror) ServicesVersion() ([]adsync.Service, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	services := make([]adsync.Service, 0, len(m.services))
	for _, service := range m.services {
		if !m.availableLocked(service) {
			continue
		}
		services = append(services, adsync.Service{
			ID:            service.id,
			Name:          service.name,
			Revision:      service.revision,
			Epoch:         service.epoch,
			HeadStateHash: service.stateHash,
		})
	}
	sort.Slice(services, func(left, right int) bool {
		return services[left].ID < services[right].ID
	})
	return services, m.version
}

// Lookup returns the current identity of one service and whether it can be
// served.
func (m *Mirror) Lookup(serviceID string) (ServiceInfo, ServiceAvailability) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	service := m.services[serviceID]
	if service == nil {
		return ServiceInfo{}, ServiceUnknown
	}
	if service.removed {
		return ServiceInfo{}, ServiceRemoved
	}
	if service.unavailable || service.overflow || service.signed == nil {
		return ServiceInfo{}, ServiceNeedsSnapshot
	}
	return ServiceInfo{
		ID:         service.id,
		Name:       service.name,
		SourceID:   service.sourceID,
		Epoch:      service.epoch,
		Revision:   service.revision,
		StateHash:  service.stateHash,
		CommitHash: service.commitHash,
	}, ServiceAvailable
}

// CurrentInfo returns the identity and revision of the verified copy, whether
// or not the service is currently advertised. The runtime uses it to check that
// a withheld copy reached the advertised head; downstream serving still uses
// Lookup, which reports a withheld service as needing a snapshot.
func (m *Mirror) CurrentInfo(serviceID string) (ServiceInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	service := m.services[serviceID]
	if service == nil || service.removed || service.overflow || service.signed == nil {
		return ServiceInfo{}, false
	}
	return ServiceInfo{
		ID:         service.id,
		Name:       service.name,
		SourceID:   service.sourceID,
		Epoch:      service.epoch,
		Revision:   service.revision,
		StateHash:  service.stateHash,
		CommitHash: service.commitHash,
	}, true
}

// Snapshot returns the signed snapshot the service is anchored to. The
// response is the exact identity Cloud signed; the proxy never re-signs it.
func (m *Mirror) Snapshot(serviceID string) (adsync.SnapshotResponse, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	service := m.services[serviceID]
	if service == nil || !m.availableLocked(service) || service.signed == nil {
		return adsync.SnapshotResponse{}, false
	}
	return cloneSnapshotResponse(*service.signed), true
}

// Changes builds one change page starting after since. A since older than the
// anchor snapshot, or newer than the head, cannot be resumed and reports
// ErrHistoryUnavailable; the downstream then reloads the snapshot.
func (m *Mirror) Changes(serviceID string, since uint64, limit int) (adsync.ChangesResponse, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	service := m.services[serviceID]
	if service == nil {
		return adsync.ChangesResponse{}, fmt.Errorf("%w: %q", ErrUnknownService, serviceID)
	}
	if service.removed {
		return adsync.ChangesResponse{}, fmt.Errorf("%w: %q", ErrServiceRemoved, serviceID)
	}
	if !m.availableLocked(service) || service.signed == nil {
		return adsync.ChangesResponse{}, fmt.Errorf("%w: %q", ErrSnapshotRequired, serviceID)
	}
	if since > service.revision {
		return adsync.ChangesResponse{}, fmt.Errorf(
			"%w: since %d is after head %d", ErrHistoryUnavailable, since, service.revision)
	}
	if since < service.signed.Revision {
		return adsync.ChangesResponse{}, fmt.Errorf(
			"%w: since %d is before the snapshot at %d", ErrHistoryUnavailable, since, service.signed.Revision)
	}

	commits := make([]adsync.Commit, 0, limit)
	for index := range service.log {
		commit := &service.log[index]
		if commit.Revision <= since {
			continue
		}
		commits = append(commits, cloneCommit(*commit))
		if len(commits) == limit {
			break
		}
	}
	to := since
	if len(commits) > 0 {
		to = commits[len(commits)-1].Revision
	}
	return adsync.ChangesResponse{
		ServiceID:     service.id,
		SourceID:      service.sourceID,
		Epoch:         service.epoch,
		From:          since + 1,
		To:            to,
		Head:          service.revision,
		HeadStateHash: service.stateHash,
		Commits:       commits,
		NextSince:     to + 1,
	}, nil
}

// NeedsSnapshot returns the services whose log reached its bound and must be
// refreshed from the source before they can be served downstream again. It is
// advisory: the process should synchronize a fresh snapshot for each of them.
func (m *Mirror) NeedsSnapshot() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var ids []string
	for _, service := range m.services {
		if service.overflow && !service.removed {
			ids = append(ids, service.id)
		}
	}
	sort.Strings(ids)
	return ids
}

// Version is the service-set version reported to downstream instances. It
// advances whenever the advertised set can change: a service is added, removed,
// renamed, re-anchored to a new sourceId or epoch, hidden because its log
// overflowed, or made visible again by a fresh snapshot.
func (m *Mirror) Version() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *Mirror) availableLocked(service *serviceCopy) bool {
	return !service.removed && !service.overflow && !service.unavailable && service.signed != nil
}

func (m *Mirror) notify(event HeadEvent) {
	m.mu.RLock()
	observers := make([]func(HeadEvent), 0, len(m.observers))
	for _, observer := range m.observers {
		observers = append(observers, observer)
	}
	m.mu.RUnlock()
	for _, observer := range observers {
		observer(event)
	}
}

func (m *Mirror) verifySnapshot(response *adsync.SnapshotResponse) error {
	if !validEntityID(response.ServiceID) {
		return fmt.Errorf("%w: invalid serviceId %q", adsync.ErrProtocol, response.ServiceID)
	}
	if !validEntityID(response.SourceID) {
		return fmt.Errorf("%w: invalid sourceId %q", adsync.ErrProtocol, response.SourceID)
	}
	if !validEntityID(response.Epoch) {
		return fmt.Errorf("%w: invalid epoch %q", adsync.ErrProtocol, response.Epoch)
	}
	if !validRevision(response.Revision) {
		return fmt.Errorf("%w: revision out of range", adsync.ErrProtocol)
	}
	if !serviceNamePattern.MatchString(response.Snapshot.Name) {
		return fmt.Errorf("%w: invalid service name %q", adsync.ErrProtocol, response.Snapshot.Name)
	}
	if response.StateHash.IsZero() || response.CommitHash.IsZero() {
		return fmt.Errorf("%w: snapshot hashes are missing", adsync.ErrProtocol)
	}
	if computed := adsync.StateHash(response.Snapshot); computed != response.StateHash {
		return fmt.Errorf("%w: snapshot stateHash does not match its content", adsync.ErrIntegrity)
	}
	return m.keys.Verify(response.Signature, adsync.SnapshotSignedBytes(
		response.SourceID, response.Epoch, response.Revision, response.StateHash, response.CommitHash))
}

func (m *Mirror) verifyCommit(sourceID, epoch string, previous adsync.Hash, commit *adsync.Commit) error {
	if !validRevision(commit.Revision) {
		return fmt.Errorf("%w: commit revision out of range", adsync.ErrProtocol)
	}
	if commit.PrevHash != previous {
		return fmt.Errorf("%w: commit %d does not chain", adsync.ErrIntegrity, commit.Revision)
	}
	computed, err := adsync.CommitHash(commit.PrevHash, commit.Changes)
	if err != nil {
		return err
	}
	if computed != commit.Hash {
		return fmt.Errorf("%w: commit %d hash does not match its changes", adsync.ErrIntegrity, commit.Revision)
	}
	return m.keys.Verify(commit.Signature,
		adsync.CommitSignedBytes(sourceID, epoch, commit.Revision, commit.Hash))
}

func checkServiceChangeTargets(serviceID string, changes []adsync.Change) error {
	for _, change := range changes {
		if change.Kind != "service" {
			continue
		}
		if change.ID != serviceID {
			return fmt.Errorf("%w: service change targets %q, want %q", adsync.ErrProtocol, change.ID, serviceID)
		}
	}
	return nil
}

func validRevision(revision uint64) bool {
	const maxSafeInteger = 1<<53 - 1
	return revision <= maxSafeInteger
}

func cloneSnapshotResponse(response adsync.SnapshotResponse) adsync.SnapshotResponse {
	response.Snapshot = cloneSnapshot(response.Snapshot)
	return response
}

func cloneSnapshot(snapshot adsync.Snapshot) adsync.Snapshot {
	cloned := snapshot
	cloned.Policies = append([]adsync.Policy(nil), snapshot.Policies...)
	cloned.RolePolicies = append([]adsync.Policy(nil), snapshot.RolePolicies...)
	return cloned
}

func cloneCommit(commit adsync.Commit) adsync.Commit {
	cloned := commit
	cloned.Changes = make([]adsync.Change, len(commit.Changes))
	for index, change := range commit.Changes {
		cloned.Changes[index] = change
		if change.Name != nil {
			name := *change.Name
			cloned.Changes[index].Name = &name
		}
		if change.SPDL != nil {
			spdl := *change.SPDL
			cloned.Changes[index].SPDL = &spdl
		}
	}
	return cloned
}

// commitSize estimates the memory a commit occupies in the log. It only has to
// be conservative enough for the byte bound.
func commitSize(commit adsync.Commit) int64 {
	size := int64(256)
	for _, change := range commit.Changes {
		size += int64(len(change.ID) + 128)
		if change.Name != nil {
			size += int64(len(*change.Name))
		}
		if change.SPDL != nil {
			size += int64(len(*change.SPDL))
		}
	}
	return size
}
