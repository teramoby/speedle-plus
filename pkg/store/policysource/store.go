// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	stderrors "errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/pdl"
)

// ErrInvalidSPDL means a verified snapshot contains a policy body that the
// engine cannot parse. The parser's own message quotes the policy text, so it
// is deliberately dropped: neither errors nor logs may carry policy content.
var ErrInvalidSPDL = stderrors.New("policysource: invalid SPDL")

// Store is the read-only policy store of one ADS instance. It holds the
// verified copies of every synchronized service and publishes them as one
// pms.PolicyStore.
type Store struct {
	syncer  *adsync.Syncer
	client  *adsync.Client
	machine *adsync.Machine
	options Options

	// fallback is true when authorization requests for a syncing or stale
	// service may be forwarded to the upstream. The client carries the API
	// key and the transport, so no separate configuration is needed.
	fallback bool

	// syncMu serializes synchronization passes.
	syncMu sync.Mutex

	// integrity holds the retry backoff of services whose copy failed
	// integrity.
	integrityMu sync.Mutex
	integrity   map[string]integrityRetry

	// mu guards copies and the published assembly. pendingRemoved records the
	// names of removed services until the evaluator has installed the assembly
	// that no longer contains them.
	mu               sync.RWMutex
	copies           map[string]*serviceCopy
	snapshot         *pms.PolicyStore
	published        string
	publishedCopies  map[string]*serviceCopy
	publishedRemoved map[string]string
	pendingRemoved   map[string]string
	// installed is the fingerprint of the assembly the evaluator most
	// recently acknowledged; hasInstalled is false until the first one.
	installed    string
	hasInstalled bool
	// discovered is the service set from the most recent successful discovery.
	// Confirm replays it for a store that does not own a channel.
	discovered      []adsync.Service
	discoveredKnown bool

	watchMu sync.Mutex
	events  pms.StorageChangeChannel

	runtimeMu sync.Mutex
	runtime   *distributionRuntime
	closeOnce sync.Once
	tlsCancel context.CancelFunc
	tlsDone   chan struct{}
	tlsReload chan struct{}

	// afterFetch, when set by a test, runs after a service fetch succeeds and
	// before its result is stored. It is nil in production.
	afterFetch func(serviceID string)

	// hookMu guards the test-only hooks below. Production leaves them nil.
	hookMu         sync.Mutex
	beforeDiscover func()
}

// serviceCopy is one verified service, parsed and ready to publish.
type serviceCopy struct {
	service   *pms.Service
	revision  uint64
	stateHash adsync.Hash
	state     *adsync.ServiceState
	attempt   adsync.SyncAttempt
}

var (
	_ pms.PolicyStoreManagerADS = (*Store)(nil)
	_ pms.StoreHealthChecker    = (*Store)(nil)
	_ adsync.StatusProvider     = (*Store)(nil)
)

// Open validates the options and builds the protocol client and the state
// machine. The instance authenticates with its API key or mTLS and identifies itself
// with the instance ID the client creates on its first request and persists in
// CacheDir.
//
// With the owned runtime (StartRuntime), the service set and heads come only
// from channel acks, and the source registers the instance on its first
// channel request, so nothing is fetched here. Cached services are tracked as
// syncing: their requests are forwarded until the first ack confirms the
// cached revision and state hash. Without the runtime, Open performs one
// bootstrap pass through discovery; a transient failure leaves the store empty
// and fail closed, while a configuration error fails startup.
func Open(ctx context.Context, options Options) (_ *Store, err error) {
	defer func() { err = redactConfigError(err, options.APIKey) }()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateUpstream(&options); err != nil {
		return nil, err
	}
	if options.APIKey == "" {
		if options.ClientCertFile == "" || options.ClientKeyFile == "" || options.CAFile == "" {
			return nil, errors.New(errors.ConfigError, "cascade requires ClientCertFile, ClientKeyFile and CAFile")
		}
		if options.CacheDir == "" {
			return nil, errors.New(errors.ConfigError, "CacheDir is required in cascade mode")
		}
		if options.InstanceID != "" {
			return nil, errors.New(errors.ConfigError, "cascade generates InstanceID in CacheDir")
		}
	}
	if options.ServeDownstream {
		// Serving downstream instances needs the relay identity of the
		// cascade design; until then a relay cannot be configured.
		return nil, errors.New(errors.ConfigError, "ServeDownstream is not supported yet")
	}
	if options.InstanceID != "" && !adsync.ValidInstanceID(options.InstanceID) {
		return nil, errors.New(errors.ConfigError, "policy source InstanceID must be a UUIDv7")
	}
	if options.CacheDir == "" && options.InstanceID == "" {
		log.Warn("policy-source store has no CacheDir: the instance ID is not persisted, so every restart registers a new instance")
	}

	client, err := adsync.NewClientContext(ctx, adsync.Config{
		BaseURL:            options.UpstreamURL,
		APIKey:             string(options.APIKey),
		InstanceID:         options.InstanceID,
		InstanceIDDir:      options.CacheDir,
		HTTPClient:         options.HTTPClient,
		CAFile:             options.CAFile,
		ClientCertFile:     options.ClientCertFile,
		ClientKeyFile:      options.ClientKeyFile,
		TLSReloadInterval:  options.TLSReloadInterval,
		CRLFiles:           options.CRLFiles,
		CRLURLs:            options.CRLURLs,
		CRLRefreshInterval: options.CRLRefreshInterval,
		CRLMaxAge:          options.CRLMaxAge,
		RequestTimeout:     options.RequestTimeout,
		Retry:              options.Retry,
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.ConfigError, "invalid policy source client configuration")
	}
	defer func() {
		if err != nil {
			client.Close()
		}
	}()
	if options.APIKey == "" {
		// Validate retained identity at startup, before any channel can send it.
		if _, err := client.InstanceID(); err != nil {
			return nil, errors.Wrap(err, errors.ConfigError, "invalid cascade CacheDir identity")
		}
	}
	syncer, err := adsync.NewSyncer(client, options.CacheDir)
	if err != nil {
		return nil, errors.Wrap(err, errors.ConfigError, "invalid policy source cache directory")
	}
	machine, err := adsync.NewMachine(adsync.MachineOptions{
		Clock:             options.Clock,
		HeartbeatInterval: options.HeartbeatInterval,
		StaleAfter:        options.StaleAfter,
		StopAfter:         options.StopAfter,
		ReconcileOnAck:    true,
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.ConfigError, "invalid policy source thresholds")
	}

	store := &Store{
		syncer:         syncer,
		client:         client,
		machine:        machine,
		options:        options,
		fallback:       !options.DisableFallback,
		copies:         make(map[string]*serviceCopy),
		pendingRemoved: make(map[string]string),
		integrity:      make(map[string]integrityRetry),
	}
	for _, service := range syncer.CachedServices() {
		if err := machine.Track(service.ID, service.Name); err != nil {
			log.Warnf("policy-source store ignores cached service %q: %v", service.ID, err)
		}
	}
	if options.StartRuntime {
		store.startTLSWatch()
		return store, nil
	}

	bootstrapCtx, cancel := context.WithTimeout(ctx, defaultBootstrapWindow)
	defer cancel()
	if err := store.Sync(bootstrapCtx); err != nil {
		if errors.Code(err) == errors.ConfigError {
			return nil, err
		}
		// The source may be unreachable at startup. The instance is then not
		// confirmed live and every decision fails closed until a sync succeeds.
		log.Warnf("policy-source store started without a complete bootstrap: %v", redactConfigError(err, options.APIKey))
	}
	store.startTLSWatch()
	return store, nil
}

// StaleIsUntrusted reports that a stale instance never evaluates locally,
// whatever forwarding allows: it forwards every request to the upstream, or
// denies it when forwarding is disabled or unavailable.
func (s *Store) StaleIsUntrusted() bool { return true }

// Type returns the store type name.
func (s *Store) Type() string { return StoreType }

// RequireWatch reports that the evaluator must subscribe to the change
// events: the published assembly is switched only through them.
func (s *Store) RequireWatch() bool { return true }

// Sync performs one synchronization pass: it takes the assigned services and
// heads from the most recent channel ack (or, for a store without the owned
// runtime, from discovery), drops services that are no longer visible, brings every
// selected service to a verified revision, and publishes the resulting
// assembly. A pass that changed any service emits one FULL_RELOAD event.
//
// Discovery is data only: it does not confirm a heartbeat. The notification
// channel's acks confirm liveness and heads. A store opened without the owned
// notification runtime has no channel; its owner calls Confirm after a
// successful pass.
//
// Sync is safe for concurrent use; passes are serialized. The notification
// channel can call it whenever a revision is announced and after a confirmed
// ack.
func (s *Store) Sync(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	var selected []adsync.Service
	if s.runtimeChannel() {
		// The owned runtime never pulls discovery: every ack carries the
		// authoritative set and heads.
		services, acked := s.ackedServices()
		if !acked {
			return nil
		}
		selected = services
	} else {
		discovery, err := s.syncer.Services(ctx)
		if err != nil {
			if stderrors.Is(err, adsync.ErrUnauthorized) || stderrors.Is(err, adsync.ErrForbidden) {
				s.machine.Stop(adsync.ReasonRevoked)
			}
			return err
		}
		selected = discovery.Services
	}
	selectedIDs := make(map[string]struct{}, len(selected))
	for _, service := range selected {
		selectedIDs[service.ID] = struct{}{}
	}
	var errs []error
	// Apply discovery removals before fetching anything: the request gate must
	// stop evaluating a removed service as soon as the machine reports it
	// removed, not after a possibly slow fetch of another service finishes.
	removed := s.dropUnselected(selectedIDs)
	s.keepIntegrityRetries(selectedIDs)
	for _, serviceID := range removed {
		if err := s.syncer.Forget(serviceID); err != nil {
			errs = append(errs, fmt.Errorf("service %q: %w", serviceID, err))
		}
	}
	if len(removed) > 0 {
		s.publish()
	}

	// Discovery can observe a rename before the channel reports it. Record the
	// previous name as pending removal before the machine forgets it, so the
	// request gate fails closed while the evaluator still holds the old
	// assembly.
	s.guardRenames(selected)
	s.runBeforeDiscover()

	// Discovery updates the authoritative set and advertised heads, but a data
	// pull is not a confirmed heartbeat. Only a channel ack or an explicit
	// Confirm from a store without the owned channel can refresh liveness or
	// confirm a head as eligible for in_sync.
	if err := s.machine.Discover(selected); err != nil {
		return joinErrors(errs)
	}
	s.mu.Lock()
	s.discovered = append([]adsync.Service(nil), selected...)
	s.discoveredKnown = true
	s.mu.Unlock()

	failed := make(map[string]struct{})
	for _, service := range selected {
		// A copy that failed integrity at this head is retried with
		// backoff, so a persistent mismatch does not pull a snapshot on
		// every ack.
		if s.integrityWaiting(service) {
			continue
		}
		if err := s.syncService(ctx, service); err != nil {
			errs = append(errs, err)
			failed[service.ID] = struct{}{}
			if stderrors.Is(err, adsync.ErrServiceNotFound) || stderrors.Is(err, adsync.ErrSourceGone) {
				// The source no longer serves the service. The state machine
				// removes it; the local copy must go too.
				s.removeCopy(service.ID)
			}
		}
	}

	s.publish()
	return joinErrors(errs)
}

// Confirm applies the service set of the most recent successful discovery as a
// confirming channel ack. A store opened without the owned notification runtime
// has no channel to produce acks; its owner calls Confirm after a successful
// Sync when its own channel or synchronization loop observes the source. The
// owned runtime receives the same confirmation from its channel and never needs
// this method.
func (s *Store) Confirm() error {
	s.mu.RLock()
	services := append([]adsync.Service{}, s.discovered...)
	known := s.discoveredKnown
	s.mu.RUnlock()
	if !known {
		return nil
	}
	return s.machine.Ack(adsync.Ack{Services: services})
}

// syncService brings one service to a verified revision and records the result
// in the state machine. A parse failure keeps the previously published copy
// and leaves the service syncing, so the request gate does not serve it
// locally.
//
// The attempt is taken before the fetch. A resync, removal or re-add while the
// fetch is in flight makes it stale: its result is neither published nor
// reported, and the next pass fetches again.
func (s *Store) syncService(ctx context.Context, service adsync.Service) error {
	attempt := s.machine.BeginSync(service.ID)
	// A notified head newer than the ack is fetched right away. A copy that
	// still does not match the acknowledged head after a snapshot reload
	// fails only this service closed. An authentication failure on a data
	// endpoint is not a revocation here: the channel, which uses the same key,
	// decides that.
	minRevision := uint64(0)
	if status, known := s.machineService(service.ID); known {
		minRevision = status.Head
	}
	state, err := s.syncer.SyncAtLeast(ctx, service, minRevision)
	if err != nil {
		if !stderrors.Is(err, adsync.ErrUnauthorized) && !stderrors.Is(err, adsync.ErrForbidden) {
			s.machine.SyncFailed(service.ID, err)
		}
		// A resync, removal or re-add while the fetch was in flight makes
		// its integrity failure obsolete: the next pass fetches again.
		if stderrors.Is(err, adsync.ErrIntegrity) && s.machine.FailAttempt(attempt) {
			log.Errorf("policy-source service %q fails closed: its copy does not match the acknowledged head: %v", service.Name, err)
			s.recordIntegrityFailure(service, attempt)
		}
		return fmt.Errorf("service %q: %w", service.Name, err)
	}
	s.clearIntegrityRetry(service.ID)
	return s.installFetched(service, state, attempt)
}

// installFetched parses a fetched copy and stores it for the next publish,
// unless the attempt became stale while the fetch was in flight.
func (s *Store) installFetched(service adsync.Service, state *adsync.ServiceState, attempt adsync.SyncAttempt) error {
	if s.afterFetch != nil {
		s.afterFetch(service.ID)
	}
	compiled, err := compileService(state)
	if err != nil {
		return fmt.Errorf("service %q: %w", service.Name, err)
	}
	compiled.attempt = attempt
	s.mu.Lock()
	// Checked under s.mu so a concurrent resync either sees this copy and
	// drops it, or invalidates the attempt before the copy is stored.
	if !s.machine.AttemptCurrent(attempt) {
		s.mu.Unlock()
		return nil
	}
	// Defense in depth: a rename may have landed while the fetch was in
	// flight. A copy that still carries the previous name must not replace
	// the renamed one, even if its attempt was taken after the rename.
	if status, known := s.machineService(service.ID); !known || status.Name != compiled.service.Name {
		s.mu.Unlock()
		return nil
	}
	s.copies[service.ID] = compiled
	// A pending removal or rename entry stays until the evaluator acknowledges
	// the assembly that replaces the old copy. Deleting it here would let the
	// previous name route locally again while the old assembly is still
	// installed.
	s.mu.Unlock()
	// A parsed copy is not yet visible to the evaluator. ApplyAttempt is
	// deferred until the exact published generation has completed its
	// FULL_RELOAD.
	return nil
}

// dropCopy deletes one local copy while the service remains tracked, for
// example during a forced resync.
func (s *Store) dropCopy(serviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.copies, serviceID)
}

// removeCopy deletes a removed service and keeps its request path unavailable
// until the evaluator acknowledges the assembly without that service.
func (s *Store) removeCopy(serviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if copied := s.copies[serviceID]; copied != nil {
		s.pendingRemoved[serviceID] = copied.service.Name
	}
	delete(s.copies, serviceID)
}

// guardRenames records the previous name of every selected service whose
// advertised name changed, before discovery updates the state machine. The
// request gate resolves a service by name, so the previous name must fail
// closed until the evaluator installs the assembly that carries the new name;
// the entry is cleared by that installation's acknowledgement.
func (s *Store) guardRenames(selected []adsync.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, service := range selected {
		copied := s.copies[service.ID]
		if copied == nil || copied.service.Name == service.Name {
			continue
		}
		if _, pending := s.pendingRemoved[service.ID]; !pending {
			s.pendingRemoved[service.ID] = copied.service.Name
		}
	}
}

// dropUnselected deletes the copies of services that are no longer part of the
// authoritative set and returns their IDs.
func (s *Store) dropUnselected(selected map[string]struct{}) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var removed []string
	for serviceID := range s.copies {
		if _, keep := selected[serviceID]; keep {
			continue
		}
		s.pendingRemoved[serviceID] = s.copies[serviceID].service.Name
		delete(s.copies, serviceID)
		removed = append(removed, serviceID)
	}
	sort.Strings(removed)
	return removed
}

// publish swaps in a freshly assembled store when the verified copies changed,
// then emits one FULL_RELOAD. The swap is atomic: a reader sees either the old
// assembly or the new one.
func (s *Store) publish() {
	s.mu.Lock()
	fingerprint := fingerprintCopies(s.copies)
	if s.snapshot != nil && fingerprint == s.published {
		// The content is unchanged, so there is nothing to reload. The copies
		// still carry this pass's attempts: keep them for a pending
		// acknowledgment, and when the evaluator already serves exactly this
		// content, report the re-verified copies now. Without this, a service
		// whose state was reset (a stop, or a removal followed by a re-add)
		// while its content stayed the same would never return to in_sync.
		s.publishedCopies = cloneCopies(s.copies)
		var states []appliedCopy
		if s.hasInstalled && s.installed == fingerprint {
			states = appliedCopies(s.publishedCopies)
		}
		s.mu.Unlock()
		s.reportApplied(states)
		return
	}
	s.snapshot = assembleStore(s.copies)
	s.published = fingerprint
	s.publishedCopies = cloneCopies(s.copies)
	s.publishedRemoved = make(map[string]string, len(s.pendingRemoved))
	for serviceID, name := range s.pendingRemoved {
		s.publishedRemoved[serviceID] = name
	}
	s.mu.Unlock()
	s.notifyReload()
}

// compileService parses one verified service. Policy identifiers and names
// come from the snapshot; the SPDL text is only the parse input.
func compileService(state *adsync.ServiceState) (*serviceCopy, error) {
	if state == nil {
		return nil, ErrInvalidSPDL
	}
	service := &pms.Service{Name: state.Snapshot.Name}
	for _, policy := range state.Snapshot.Policies {
		parsed, _, err := pdl.ParsePolicy(policy.SPDL, policy.Name)
		if err != nil {
			return nil, fmt.Errorf("%w: policy %q", ErrInvalidSPDL, policy.ID)
		}
		parsed.ID = policy.ID
		service.Policies = append(service.Policies, parsed)
	}
	for _, rolePolicy := range state.Snapshot.RolePolicies {
		parsed, _, err := pdl.ParseRolePolicy(rolePolicy.SPDL, rolePolicy.Name)
		if err != nil {
			return nil, fmt.Errorf("%w: role policy %q", ErrInvalidSPDL, rolePolicy.ID)
		}
		parsed.ID = rolePolicy.ID
		service.RolePolicies = append(service.RolePolicies, parsed)
	}
	sort.Slice(service.Policies, func(left, right int) bool {
		return service.Policies[left].ID < service.Policies[right].ID
	})
	sort.Slice(service.RolePolicies, func(left, right int) bool {
		return service.RolePolicies[left].ID < service.RolePolicies[right].ID
	})
	return &serviceCopy{service: service, revision: state.Revision, stateHash: state.StateHash, state: state}, nil
}

func assembleStore(copies map[string]*serviceCopy) *pms.PolicyStore {
	ids := make([]string, 0, len(copies))
	for serviceID := range copies {
		ids = append(ids, serviceID)
	}
	sort.Slice(ids, func(left, right int) bool {
		leftName, rightName := copies[ids[left]].service.Name, copies[ids[right]].service.Name
		if leftName != rightName {
			return leftName < rightName
		}
		return ids[left] < ids[right]
	})
	policyStore := &pms.PolicyStore{}
	for _, serviceID := range ids {
		policyStore.Services = append(policyStore.Services, copies[serviceID].service)
	}
	return policyStore
}

// fingerprintCopies identifies the published content without comparing the
// parsed policies themselves.
func fingerprintCopies(copies map[string]*serviceCopy) string {
	ids := make([]string, 0, len(copies))
	for serviceID := range copies {
		ids = append(ids, serviceID)
	}
	sort.Strings(ids)
	var builder strings.Builder
	for _, serviceID := range ids {
		copied := copies[serviceID]
		fmt.Fprintf(&builder, "%s\x00%s\x00%d\x00%s\x1e",
			serviceID, copied.service.Name, copied.revision, copied.stateHash.String())
	}
	return builder.String()
}

// joinErrors returns nil for an empty list, the error itself for one entry,
// and a joined error otherwise.
func joinErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return stderrors.Join(errs...)
	}
}

// Status returns the instance and service state for one service by name. It
// implements adsync.StatusProvider for the request gate.
func (s *Store) Status(serviceName string) (adsync.Status, bool) {
	return s.machine.Status(serviceName)
}

// ReadPolicyStore returns a consistent copy of the published assembly. It
// returns an empty store when nothing has been synchronized yet.
func (s *Store) ReadPolicyStore() (*pms.PolicyStore, error) {
	s.mu.RLock()
	snapshot := s.snapshot
	s.mu.RUnlock()
	return clonePolicyStore(snapshot), nil
}

// ReadPolicyStoreForReload returns the assembly and an acknowledgment bound to
// its exact generation. The evaluator calls acknowledge only after installing
// that assembly. A stale acknowledgment does nothing when a newer generation
// has since been published.
func (s *Store) ReadPolicyStoreForReload() (*pms.PolicyStore, func(), error) {
	s.mu.RLock()
	fingerprint := s.published
	snapshot := clonePolicyStore(s.snapshot)
	removed := s.publishedRemoved
	s.mu.RUnlock()
	var once sync.Once
	return snapshot, func() {
		once.Do(func() { s.acknowledgeReload(fingerprint, removed) })
	}, nil
}

func (s *Store) acknowledgeReload(fingerprint string, removed map[string]string) {
	s.mu.Lock()
	// The evaluator installs reloads one at a time, so the latest
	// acknowledgment describes what it serves now, even an outdated one.
	s.installed = fingerprint
	s.hasInstalled = true
	if fingerprint != s.published {
		s.mu.Unlock()
		return
	}
	// The published copies may have been refreshed by a later pass with the
	// same content; their attempts are the most recent ones.
	states := appliedCopies(s.publishedCopies)
	for serviceID, name := range removed {
		if s.pendingRemoved[serviceID] == name {
			delete(s.pendingRemoved, serviceID)
		}
	}
	s.mu.Unlock()
	s.reportApplied(states)
}

// appliedCopy is one installed copy to report to the state machine.
type appliedCopy struct {
	attempt adsync.SyncAttempt
	state   *adsync.ServiceState
}

func appliedCopies(copies map[string]*serviceCopy) []appliedCopy {
	states := make([]appliedCopy, 0, len(copies))
	for _, copied := range copies {
		states = append(states, appliedCopy{attempt: copied.attempt, state: copied.state})
	}
	return states
}

func cloneCopies(copies map[string]*serviceCopy) map[string]*serviceCopy {
	cloned := make(map[string]*serviceCopy, len(copies))
	for serviceID, copied := range copies {
		cloned[serviceID] = copied
	}
	return cloned
}

// reportApplied records installed copies. A copy whose attempt became stale
// after a resync, removal or re-add is ignored by the state machine.
func (s *Store) reportApplied(states []appliedCopy) {
	for _, applied := range states {
		if err := s.machine.ApplyAttempt(applied.attempt, applied.state); err != nil {
			log.Errorf("policy-source store could not record installed service %q: %v", applied.state.ServiceID, err)
		}
	}
}

// RemovalInstalled reports whether a removed service name has disappeared from
// the evaluator. Until then the request gate must fail closed instead of
// evaluating the old, still-installed service.
func (s *Store) RemovalInstalled(serviceName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, removedName := range s.pendingRemoved {
		if removedName == serviceName {
			return false
		}
	}
	return true
}

// GetService returns one service by name.
func (s *Store) GetService(serviceName string) (*pms.Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	service := findService(s.publishedLocked(), serviceName)
	if service == nil {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	return cloneService(service), nil
}

// GetPolicy returns one policy of a service by ID.
func (s *Store) GetPolicy(serviceName string, id string) (*pms.Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	service := findService(s.publishedLocked(), serviceName)
	if service == nil {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	for _, policy := range service.Policies {
		if policy.ID == id {
			return clonePolicy(policy), nil
		}
	}
	return nil, errors.Errorf(errors.EntityNotFound, "unable to find policy %q in service %q", id, serviceName)
}

// GetRolePolicy returns one role policy of a service by ID.
func (s *Store) GetRolePolicy(serviceName string, id string) (*pms.RolePolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	service := findService(s.publishedLocked(), serviceName)
	if service == nil {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	for _, rolePolicy := range service.RolePolicies {
		if rolePolicy.ID == id {
			return cloneRolePolicy(rolePolicy), nil
		}
	}
	return nil, errors.Errorf(errors.EntityNotFound, "unable to find role policy %q in service %q", id, serviceName)
}

// GetFunction always reports that the function does not exist: the policy
// source protocol carries no functions.
func (s *Store) GetFunction(funcName string) (*pms.Function, error) {
	return nil, errors.Errorf(errors.EntityNotFound, "function %q is not found", funcName)
}

// ListAllServices returns every synchronized service.
func (s *Store) ListAllServices() ([]*pms.Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	published := s.publishedLocked()
	services := make([]*pms.Service, 0, len(published.Services))
	for _, service := range published.Services {
		services = append(services, cloneService(service))
	}
	return services, nil
}

// GetServiceNames returns the names of every synchronized service.
func (s *Store) GetServiceNames() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	published := s.publishedLocked()
	names := make([]string, 0, len(published.Services))
	for _, service := range published.Services {
		names = append(names, service.Name)
	}
	return names, nil
}

// GetServiceCount returns the number of synchronized services.
func (s *Store) GetServiceCount() (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.publishedLocked().Services)), nil
}

// GetPolicyAndRolePolicyCounts returns both counts per service.
func (s *Store) GetPolicyAndRolePolicyCounts() (map[string]*pms.PolicyAndRolePolicyCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	published := s.publishedLocked()
	counts := make(map[string]*pms.PolicyAndRolePolicyCount, len(published.Services))
	for _, service := range published.Services {
		counts[service.Name] = &pms.PolicyAndRolePolicyCount{
			PolicyCount:     int64(len(service.Policies)),
			RolePolicyCount: int64(len(service.RolePolicies)),
		}
	}
	return counts, nil
}

// ListAllPolicies returns the policies of one service, applying the shared
// name filter.
func (s *Store) ListAllPolicies(serviceName string, filter string) ([]*pms.Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	service := findService(s.publishedLocked(), serviceName)
	if service == nil {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	parsed := parseFilter(filter)
	policies := make([]*pms.Policy, 0, len(service.Policies))
	for _, policy := range service.Policies {
		if parsed != nil && !nameFilter(policy.Name, parsed) {
			continue
		}
		policies = append(policies, clonePolicy(policy))
	}
	return policies, nil
}

// GetPolicyCount returns the policy count of one service, or of every service
// when the name is empty.
func (s *Store) GetPolicyCount(serviceName string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if serviceName != "" {
		service := findService(s.publishedLocked(), serviceName)
		if service == nil {
			return 0, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
		}
		return int64(len(service.Policies)), nil
	}
	var count int64
	for _, service := range s.publishedLocked().Services {
		count += int64(len(service.Policies))
	}
	return count, nil
}

// ListAllRolePolicies returns the role policies of one service, applying the
// shared name filter.
func (s *Store) ListAllRolePolicies(serviceName string, filter string) ([]*pms.RolePolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	service := findService(s.publishedLocked(), serviceName)
	if service == nil {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	parsed := parseFilter(filter)
	rolePolicies := make([]*pms.RolePolicy, 0, len(service.RolePolicies))
	for _, rolePolicy := range service.RolePolicies {
		if parsed != nil && !nameFilter(rolePolicy.Name, parsed) {
			continue
		}
		rolePolicies = append(rolePolicies, cloneRolePolicy(rolePolicy))
	}
	return rolePolicies, nil
}

// GetRolePolicyCount returns the role policy count of one service, or of every
// service when the name is empty.
func (s *Store) GetRolePolicyCount(serviceName string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if serviceName != "" {
		service := findService(s.publishedLocked(), serviceName)
		if service == nil {
			return 0, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
		}
		return int64(len(service.RolePolicies)), nil
	}
	var count int64
	for _, service := range s.publishedLocked().Services {
		count += int64(len(service.RolePolicies))
	}
	return count, nil
}

// ListAllFunctions always returns an empty list.
func (s *Store) ListAllFunctions(filter string) ([]*pms.Function, error) {
	return []*pms.Function{}, nil
}

// GetFunctionCount is always zero.
func (s *Store) GetFunctionCount() (int64, error) { return 0, nil }

// readOnly reports every write attempt. The policy source is authoritative for
// this store.
func readOnly(operation string) error {
	return errors.Errorf(errors.StoreError, "policy-source store is read-only: %s is not supported", operation)
}

// WritePolicyStore is not supported.
func (s *Store) WritePolicyStore(*pms.PolicyStore) error { return readOnly("WritePolicyStore") }

// CreateService is not supported.
func (s *Store) CreateService(*pms.Service) error { return readOnly("CreateService") }

// DeleteService is not supported.
func (s *Store) DeleteService(string) error { return readOnly("DeleteService") }

// DeleteServices is not supported.
func (s *Store) DeleteServices() error { return readOnly("DeleteServices") }

// CreatePolicy is not supported.
func (s *Store) CreatePolicy(string, *pms.Policy) (*pms.Policy, error) {
	return nil, readOnly("CreatePolicy")
}

// DeletePolicy is not supported.
func (s *Store) DeletePolicy(string, string) error { return readOnly("DeletePolicy") }

// DeletePolicies is not supported.
func (s *Store) DeletePolicies(string) error { return readOnly("DeletePolicies") }

// CreateRolePolicy is not supported.
func (s *Store) CreateRolePolicy(string, *pms.RolePolicy) (*pms.RolePolicy, error) {
	return nil, readOnly("CreateRolePolicy")
}

// DeleteRolePolicy is not supported.
func (s *Store) DeleteRolePolicy(string, string) error { return readOnly("DeleteRolePolicy") }

// DeleteRolePolicies is not supported.
func (s *Store) DeleteRolePolicies(string) error { return readOnly("DeleteRolePolicies") }

// CreateFunction is not supported.
func (s *Store) CreateFunction(*pms.Function) (*pms.Function, error) {
	return nil, readOnly("CreateFunction")
}

// DeleteFunction is not supported.
func (s *Store) DeleteFunction(string) error { return readOnly("DeleteFunction") }

// DeleteFunctions is not supported.
func (s *Store) DeleteFunctions() error { return readOnly("DeleteFunctions") }

// emptyPolicyStore is the immutable assembly of a store that has not
// synchronized anything yet.
var emptyPolicyStore = &pms.PolicyStore{}

// publishedLocked returns the current assembly. The caller holds at least a
// read lock.
func (s *Store) publishedLocked() *pms.PolicyStore {
	if s.snapshot == nil {
		return emptyPolicyStore
	}
	return s.snapshot
}

// findService returns the service with the given name from a published
// assembly. The assembly and everything it references are immutable.
func findService(policyStore *pms.PolicyStore, serviceName string) *pms.Service {
	if policyStore == nil {
		return nil
	}
	for _, service := range policyStore.Services {
		if service.Name == serviceName {
			return service
		}
	}
	return nil
}
