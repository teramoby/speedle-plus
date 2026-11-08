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
	syncer    *adsync.Syncer
	client    *adsync.Client
	machine   *adsync.Machine
	selection []string
	options   Options

	// fallback is true when authorization requests for a syncing service may
	// be forwarded to Cloud. The client carries the instance credential and
	// the transport, so no separate configuration is needed.
	fallback bool

	// syncMu serializes synchronization passes.
	syncMu sync.Mutex

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

	watchMu sync.Mutex
	events  pms.StorageChangeChannel

	runtimeMu sync.Mutex
	runtime   *distributionRuntime
	closeOnce sync.Once
}

// serviceCopy is one verified service, parsed and ready to publish.
type serviceCopy struct {
	service   *pms.Service
	revision  uint64
	stateHash adsync.Hash
	state     *adsync.ServiceState
}

var (
	_ pms.PolicyStoreManagerADS = (*Store)(nil)
	_ pms.StoreHealthChecker    = (*Store)(nil)
	_ adsync.StatusProvider     = (*Store)(nil)
)

// Open validates the options, builds the protocol client and the state
// machine, and performs one bootstrap synchronization. A transient bootstrap
// failure does not fail startup: the store stays empty and fail closed, and a
// later Sync can fill it. A configuration error does fail startup.
func Open(ctx context.Context, options Options) (*Store, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	selection, err := normalizeServices(options.Services)
	if err != nil {
		return nil, err
	}
	if options.PublicKeys == nil {
		return nil, errors.New(errors.ConfigError, "policy source public keys are required")
	}
	if options.PolicySourceURL == "" {
		return nil, errors.New(errors.ConfigError, "policy source URL is required")
	}

	client, err := adsync.NewClient(adsync.Config{
		BaseURL:            options.PolicySourceURL,
		Credential:         options.Credential,
		FallbackBaseURL:    options.FallbackURL,
		FallbackCredential: options.FallbackCredential,
		FallbackHTTPClient: options.FallbackHTTPClient,
		PublicKeys:         options.PublicKeys,
		HTTPClient:         options.HTTPClient,
		CAFile:             options.CAFile,
		ClientCertFile:     options.ClientCertFile,
		ClientKeyFile:      options.ClientKeyFile,
		RequestTimeout:     options.RequestTimeout,
		Retry:              options.Retry,
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.ConfigError, "invalid policy source client configuration")
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
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.ConfigError, "invalid policy source thresholds")
	}

	store := &Store{
		syncer:         syncer,
		client:         client,
		machine:        machine,
		selection:      selection,
		options:        options,
		fallback:       !options.DisableFallback,
		copies:         make(map[string]*serviceCopy),
		pendingRemoved: make(map[string]string),
	}

	bootstrapCtx, cancel := context.WithTimeout(ctx, defaultBootstrapWindow)
	defer cancel()
	if err := store.Sync(bootstrapCtx); err != nil {
		if errors.Code(err) == errors.ConfigError {
			return nil, err
		}
		// The source may be unreachable at startup. The instance is then not
		// confirmed live and every decision fails closed until a sync succeeds.
		log.Warnf("policy-source store started without a complete bootstrap: %v", err)
	}
	return store, nil
}

// Type returns the store type name.
func (s *Store) Type() string { return StoreType }

// RequireWatch reports that the evaluator must subscribe to the change
// events: the published assembly is switched only through them.
func (s *Store) RequireWatch() bool { return true }

// Sync performs one synchronization pass: it discovers the visible services
// and advertised heads without confirming a heartbeat, brings every selected
// service to a verified revision, drops services that are no longer visible,
// and publishes the resulting assembly. A pass that changed any service emits
// one FULL_RELOAD event.
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

	discovery, err := s.syncer.Services(ctx)
	if err != nil {
		if stderrors.Is(err, adsync.ErrUnauthorized) || stderrors.Is(err, adsync.ErrForbidden) {
			s.machine.Stop(adsync.ReasonRevoked)
		}
		return err
	}
	selected, err := s.selectServices(discovery.Services)
	if err != nil {
		s.machine.Stop(adsync.ReasonConfigError)
		return err
	}
	// Discovery updates the authoritative set and advertised heads, but a data
	// pull is not a confirmed heartbeat. Only the notification channel's ack
	// refreshes liveness or confirms a head as eligible for in_sync.
	if err := s.machine.Discover(selected); err != nil {
		return err
	}

	selectedIDs := make(map[string]struct{}, len(selected))
	var errs []error
	for _, service := range selected {
		selectedIDs[service.ID] = struct{}{}
		if err := s.syncService(ctx, service); err != nil {
			errs = append(errs, err)
			if stderrors.Is(err, adsync.ErrServiceNotFound) || stderrors.Is(err, adsync.ErrSourceGone) {
				// The source no longer serves the service. The state machine
				// removes it; the local copy must go too.
				s.removeCopy(service.ID)
			}
		}
	}

	for _, serviceID := range s.dropUnselected(selectedIDs) {
		if err := s.syncer.Forget(serviceID); err != nil {
			errs = append(errs, fmt.Errorf("service %q: %w", serviceID, err))
		}
	}
	s.publish()
	s.syncMirror(ctx, selected)
	return joinErrors(errs)
}

// syncService brings one service to a verified revision and records the result
// in the state machine. A parse failure keeps the previously published copy
// and leaves the service syncing, so the request gate does not serve it
// locally.
func (s *Store) syncService(ctx context.Context, service adsync.Service) error {
	state, err := s.syncer.Sync(ctx, service)
	if err != nil {
		s.machine.SyncFailed(service.ID, err)
		if stderrors.Is(err, adsync.ErrIntegrity) {
			// Syncer already retried through a signed snapshot. A final
			// integrity failure stops the whole instance, not just this service.
			s.machine.Stop(adsync.ReasonIntegrityFailure)
		}
		return fmt.Errorf("service %q: %w", service.Name, err)
	}
	compiled, err := compileService(state)
	if err != nil {
		return fmt.Errorf("service %q: %w", service.Name, err)
	}
	s.mu.Lock()
	s.copies[service.ID] = compiled
	delete(s.pendingRemoved, service.ID)
	s.mu.Unlock()
	// A parsed copy is not yet visible to the evaluator. Applied is deferred
	// until the exact published generation has completed its FULL_RELOAD.
	return nil
}

// selectServices applies the configured service selection. Every explicit
// entry must match a visible service by name or by ULID; a stale selection is
// a configuration error.
func (s *Store) selectServices(discovered []adsync.Service) ([]adsync.Service, error) {
	if len(s.selection) == 1 && s.selection[0] == DefaultServicesSelection {
		return discovered, nil
	}
	found := make(map[string]bool, len(s.selection))
	matched := make([]adsync.Service, 0, len(s.selection))
	seen := make(map[string]struct{}, len(s.selection))
	for _, service := range discovered {
		for _, entry := range s.selection {
			if entry != service.ID && entry != service.Name {
				continue
			}
			found[entry] = true
			if _, duplicate := seen[service.ID]; !duplicate {
				seen[service.ID] = struct{}{}
				matched = append(matched, service)
			}
			break
		}
	}
	for _, entry := range s.selection {
		if !found[entry] {
			return nil, errors.Errorf(errors.ConfigError,
				"configured service %q is not visible to the policy source credential", entry)
		}
	}
	return matched, nil
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
		s.mu.Unlock()
		return
	}
	s.snapshot = assembleStore(s.copies)
	s.published = fingerprint
	s.publishedCopies = make(map[string]*serviceCopy, len(s.copies))
	for serviceID, copied := range s.copies {
		s.publishedCopies[serviceID] = copied
	}
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
	copies := s.publishedCopies
	removed := s.publishedRemoved
	s.mu.RUnlock()
	var once sync.Once
	return snapshot, func() {
		once.Do(func() { s.acknowledgeReload(fingerprint, copies, removed) })
	}, nil
}

func (s *Store) acknowledgeReload(fingerprint string, copies map[string]*serviceCopy, removed map[string]string) {
	s.mu.Lock()
	if fingerprint != s.published {
		s.mu.Unlock()
		return
	}
	states := make([]*adsync.ServiceState, 0, len(copies))
	for _, copied := range copies {
		states = append(states, copied.state)
	}
	for serviceID, name := range removed {
		if s.pendingRemoved[serviceID] == name {
			delete(s.pendingRemoved, serviceID)
		}
	}
	s.mu.Unlock()

	for _, state := range states {
		if err := s.machine.Applied(state); err != nil {
			log.Errorf("policy-source store could not record installed service %q: %v", state.ServiceID, err)
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
