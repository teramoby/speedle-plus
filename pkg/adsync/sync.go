// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

const (
	// maxSyncPages bounds the catch-up work of one Sync call. A source moving
	// faster than 100 commits per page for this many pages is not a useful
	// synchronization partner; the caller retries later.
	maxSyncPages    = 1000
	syncLockStripes = 64
)

// ServiceState is a verified local copy of one service. Every value returned
// by Syncer is an independent copy, so callers may retain and read it without
// locking.
type ServiceState struct {
	ServiceID  string
	SourceID   string
	Epoch      string
	Revision   uint64
	StateHash  Hash
	CommitHash Hash
	Snapshot   Snapshot

	// signature is set only while the state is still exactly a signed
	// snapshot, so a cache reload can re-verify it.
	signature Signature
}

func (s *ServiceState) clone() *ServiceState {
	if s == nil {
		return nil
	}
	cloned := *s
	cloned.Snapshot = cloneSnapshot(s.Snapshot)
	return &cloned
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	cloned := snapshot
	cloned.Policies = append([]Policy(nil), snapshot.Policies...)
	cloned.RolePolicies = append([]Policy(nil), snapshot.RolePolicies...)
	return cloned
}

// Syncer owns the per-service resume position and the optional disk cache. It
// does not track liveness or evaluate policy.
type Syncer struct {
	client *Client
	cache  *Cache

	mu     sync.Mutex
	states map[string]*ServiceState
	locks  [syncLockStripes]sync.Mutex
}

// NewSyncer returns a Syncer. cacheDir may be empty to keep copies in memory
// only; otherwise each service is cached in its own 0600 file.
func NewSyncer(client *Client, cacheDir string) (*Syncer, error) {
	if client == nil {
		return nil, fmt.Errorf("adsync: a client is required")
	}
	var cache *Cache
	if cacheDir != "" {
		var err error
		cache, err = NewCache(cacheDir, client.keys)
		if err != nil {
			return nil, err
		}
	}
	return &Syncer{
		client: client,
		cache:  cache,
		states: make(map[string]*ServiceState),
	}, nil
}

// Services discovers the services visible to this instance.
func (s *Syncer) Services(ctx context.Context) (*ServicesResponse, error) {
	return s.client.Services(ctx)
}

// State returns a copy of the current verified state of one service.
func (s *Syncer) State(serviceID string) (*ServiceState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, found := s.states[serviceID]
	if !found {
		return nil, false
	}
	return state.clone(), true
}

// Forget drops the in-memory copy and the cache file for one service. It is
// used when the source reports the service removed. Forget is serialized with
// Sync so an in-flight sync cannot recreate a forgotten service afterwards.
func (s *Syncer) Forget(serviceID string) error {
	lock := s.serviceLock(serviceID)
	lock.Lock()
	defer lock.Unlock()
	return s.forget(serviceID)
}

// serviceLock returns the lock that serializes a service's complete sync and
// forget operation. Fixed striping bounds memory regardless of service churn;
// a collision only serializes two otherwise independent operations.
func (s *Syncer) serviceLock(serviceID string) *sync.Mutex {
	digest := hashOf([]byte(serviceID))
	return &s.locks[int(digest[0])%len(s.locks)]
}

// SyncByName discovers the current service set and syncs the named service.
func (s *Syncer) SyncByName(ctx context.Context, name string) (*ServiceState, error) {
	services, err := s.client.Services(ctx)
	if err != nil {
		return nil, err
	}
	for _, service := range services.Services {
		if service.Name == name {
			return s.Sync(ctx, service)
		}
	}
	return nil, fmt.Errorf("%w: no service named %q", ErrServiceNotFound, name)
}

// Sync brings one service to a verified state at or after service.Revision.
//
// A missing, stale or ahead local copy reloads the snapshot. Otherwise changes
// are applied page by page. A 410 (except source_gone) or any page that fails
// validation reloads the snapshot, as the protocol requires. A source_gone
// service deletes its local copy and is not retried.
func (s *Syncer) Sync(ctx context.Context, service Service) (*ServiceState, error) {
	if err := service.validate(); err != nil {
		return nil, err
	}
	lock := s.serviceLock(service.ID)
	lock.Lock()
	defer lock.Unlock()

	local, fromCache, err := s.localState(service.ID)
	if err != nil {
		return nil, err
	}
	if local == nil {
		return s.loadSnapshot(ctx, service, local)
	}
	if local.Epoch != service.Epoch || local.Revision > service.Revision {
		return s.loadSnapshot(ctx, service, local)
	}
	if local.Revision == service.Revision {
		if local.StateHash == service.HeadStateHash {
			if fromCache {
				if err := s.store(local); err != nil {
					return nil, err
				}
			}
			return local, nil
		}
		return s.loadSnapshot(ctx, service, local)
	}

	updated, err := s.catchUp(ctx, local, service)
	switch {
	case err == nil:
		return updated, nil
	case errors.Is(err, ErrSourceGone), errors.Is(err, ErrServiceNotFound):
		if forgetErr := s.forget(service.ID); forgetErr != nil {
			return nil, errors.Join(err, forgetErr)
		}
		return nil, err
	case errors.Is(err, ErrSnapshotRequired):
		return s.loadSnapshot(ctx, service, local)
	case errors.Is(err, ErrIntegrity), errors.Is(err, ErrProtocol):
		return s.loadSnapshot(ctx, service, local)
	default:
		return nil, err
	}
}

func (s *Syncer) localState(serviceID string) (*ServiceState, bool, error) {
	s.mu.Lock()
	stored, found := s.states[serviceID]
	s.mu.Unlock()
	if found {
		return stored.clone(), false, nil
	}
	if s.cache == nil {
		return nil, false, nil
	}
	state, err := s.cache.Load(serviceID)
	if err != nil {
		// A corrupt or unsafe cache is a cache miss. The next successful sync
		// atomically replaces the file.
		return nil, false, nil
	}
	if state == nil {
		return nil, false, nil
	}
	// A disk cache entry is only a tentative resume position. In particular,
	// incremental entries have no snapshot signature. Keep it out of s.states
	// until discovery or a verified change chain confirms the resulting head.
	return state.clone(), true, nil
}

func (s *Syncer) loadSnapshot(ctx context.Context, service Service, local *ServiceState) (*ServiceState, error) {
	response, _, err := s.client.Snapshot(ctx, service.ID, "")
	if err != nil {
		if errors.Is(err, ErrServiceNotFound) {
			if forgetErr := s.forget(service.ID); forgetErr != nil {
				return nil, errors.Join(err, forgetErr)
			}
		}
		return nil, err
	}
	if response.SourceID != service.ID || response.Epoch != service.Epoch {
		return nil, fmt.Errorf("%w: snapshot identity %q/%q does not match requested service %q epoch %q",
			ErrProtocol, response.SourceID, response.Epoch, service.ID, service.Epoch)
	}
	if response.Revision == service.Revision && response.StateHash != service.HeadStateHash {
		return nil, fmt.Errorf("%w: snapshot hash does not match the discovered head at revision %d",
			ErrIntegrity, service.Revision)
	}
	if local != nil && local.Epoch == response.Epoch {
		if response.Revision < local.Revision {
			return nil, fmt.Errorf("%w: snapshot revision %d would roll back local revision %d",
				ErrProtocol, response.Revision, local.Revision)
		}
	}
	state := &ServiceState{
		ServiceID:  response.ServiceID,
		SourceID:   response.SourceID,
		Epoch:      response.Epoch,
		Revision:   response.Revision,
		StateHash:  response.StateHash,
		CommitHash: response.CommitHash,
		Snapshot:   cloneSnapshot(response.Snapshot),
		signature:  response.Signature,
	}
	if state.Revision < service.Revision {
		// A lagging replica may legitimately anchor its signed snapshot behind
		// discovery. Catch up before publishing anything, and do not return a
		// successful sync until the advertised revision has been reached.
		caughtUp, err := s.catchUp(ctx, state, service)
		if err != nil {
			if errors.Is(err, ErrSourceGone) || errors.Is(err, ErrServiceNotFound) {
				if forgetErr := s.forget(service.ID); forgetErr != nil {
					return nil, errors.Join(err, forgetErr)
				}
			}
			return nil, err
		}
		return caughtUp, nil
	}
	if err := s.store(state); err != nil {
		return nil, err
	}
	return state.clone(), nil
}

func (s *Syncer) catchUp(ctx context.Context, local *ServiceState, service Service) (*ServiceState, error) {
	working := local.clone()
	anchor := working.CommitHash
	revision := working.Revision

	for pages := 0; ; pages++ {
		if pages >= maxSyncPages {
			return nil, fmt.Errorf("%w: too many change pages while syncing %q", ErrProtocol, service.ID)
		}
		page, err := s.client.Changes(ctx, service.ID, working.SourceID, working.Epoch, revision, anchor, changePageLimit)
		if err != nil {
			return nil, err
		}

		if len(page.Commits) == 0 {
			if hash := StateHash(working.Snapshot); hash != page.HeadStateHash {
				return nil, fmt.Errorf("%w: state hash does not match the source head at revision %d", ErrIntegrity, page.Head)
			}
			working.StateHash = page.HeadStateHash
			break
		}

		for index := range page.Commits {
			commit := &page.Commits[index]
			if err := checkServiceChangeTargets(working.ServiceID, commit.Changes); err != nil {
				return nil, err
			}
			if err := applyChanges(&working.Snapshot, commit.Changes); err != nil {
				return nil, err
			}
			anchor = commit.Hash
			revision = commit.Revision
		}
		working.CommitHash = anchor
		working.Revision = revision
		// The applied state is no longer exactly the signed snapshot, so its
		// signature can no longer anchor a cache reload.
		working.signature = Signature{}

		if page.To == page.Head {
			if hash := StateHash(working.Snapshot); hash != page.HeadStateHash {
				return nil, fmt.Errorf("%w: state hash does not match the source head at revision %d", ErrIntegrity, page.Head)
			}
			working.StateHash = page.HeadStateHash
			break
		}
		if page.NextSince != revision+1 {
			return nil, fmt.Errorf("%w: next page does not follow revision %d", ErrProtocol, revision)
		}
	}

	if working.Revision < service.Revision {
		return nil, fmt.Errorf("%w: source head revision %d is behind discovered revision %d",
			ErrUnavailable, working.Revision, service.Revision)
	}
	if working.Revision == service.Revision && working.StateHash != service.HeadStateHash {
		return nil, fmt.Errorf("%w: change head does not match discovery at revision %d",
			ErrIntegrity, service.Revision)
	}
	// The source may have advanced beyond discovery during catch-up. Only the
	// final verified state is published; a failed or lagging pass leaves the
	// previous published state untouched.
	if err := s.store(working); err != nil {
		return nil, err
	}
	return working.clone(), nil
}

func checkServiceChangeTargets(serviceID string, changes []Change) error {
	for _, change := range changes {
		if isServiceChange(change) && change.ID != serviceID {
			return fmt.Errorf("%w: service change targets %q, want %q", ErrProtocol, change.ID, serviceID)
		}
	}
	return nil
}

// ApplyChanges applies a commit's changes in order to snapshot. Upserts replace
// by id, deletes of unknown ids are no-ops and a signed service delete reports
// ErrSourceGone. Policy lists are sorted by id afterwards, which is the order
// the canonical state hash uses. A proxy serving a synced copy uses this to
// apply the same commits the syncer applies, so both derive the same state.
func ApplyChanges(snapshot *Snapshot, changes []Change) error {
	return applyChanges(snapshot, changes)
}

// applyChanges applies a commit's changes in order to a working snapshot.
// Upserts replace by id, deletes of unknown ids are no-ops and a signed
// service delete means the source is gone.
func applyChanges(snapshot *Snapshot, changes []Change) error {
	for _, change := range changes {
		switch change.Kind {
		case changeKindAuthzName:
			policies, err := applyPolicyChange(snapshot.Policies, change)
			if err != nil {
				return err
			}
			snapshot.Policies = policies
		case changeKindRoleName:
			policies, err := applyPolicyChange(snapshot.RolePolicies, change)
			if err != nil {
				return err
			}
			snapshot.RolePolicies = policies
		case changeKindServiceName:
			if change.Op == changeOpDeleteName {
				return fmt.Errorf("%w: the source deleted the service", ErrSourceGone)
			}
			if change.Name == nil {
				return fmt.Errorf("%w: service change has no name", ErrProtocol)
			}
			snapshot.Name = *change.Name
		default:
			return fmt.Errorf("%w: unknown change kind %q", ErrProtocol, change.Kind)
		}
	}
	sort.Slice(snapshot.Policies, func(left, right int) bool {
		return snapshot.Policies[left].ID < snapshot.Policies[right].ID
	})
	sort.Slice(snapshot.RolePolicies, func(left, right int) bool {
		return snapshot.RolePolicies[left].ID < snapshot.RolePolicies[right].ID
	})
	return nil
}

func applyPolicyChange(policies []Policy, change Change) ([]Policy, error) {
	switch change.Op {
	case changeOpDeleteName:
		for index := range policies {
			if policies[index].ID == change.ID {
				return append(policies[:index], policies[index+1:]...), nil
			}
		}
		return policies, nil
	case changeOpUpsertName:
		if change.Name == nil || change.SPDL == nil {
			return nil, fmt.Errorf("%w: upsert has no name or SPDL", ErrProtocol)
		}
		updated := Policy{ID: change.ID, Name: *change.Name, SPDL: *change.SPDL}
		for index := range policies {
			if policies[index].ID == change.ID {
				policies[index] = updated
				return policies, nil
			}
		}
		return append(policies, updated), nil
	default:
		return nil, fmt.Errorf("%w: unknown change op %q", ErrProtocol, change.Op)
	}
}

func (s *Syncer) store(state *ServiceState) error {
	if s.cache != nil {
		if err := s.cache.Save(state); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.states[state.ServiceID] = state.clone()
	s.mu.Unlock()
	return nil
}

func (s *Syncer) forget(serviceID string) error {
	if s.cache != nil {
		if err := s.cache.Remove(serviceID); err != nil {
			return err
		}
	}
	s.mu.Lock()
	delete(s.states, serviceID)
	s.mu.Unlock()
	return nil
}
