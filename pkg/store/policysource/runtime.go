// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/adsproxy"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

const (
	defaultReconcilePeriod = time.Minute
	defaultAgentVersion    = "speedle-ads"
	maxRememberedCommands  = 100
	maxMirrorPages         = 1000
)

// distributionRuntime owns the production channel, reconciliation trigger and
// optional downstream server. It starts when the evaluator installs its watch
// and is joined by Store.Close.
type distributionRuntime struct {
	cancel  context.CancelFunc
	done    chan struct{}
	trigger chan struct{}
	channel *adsync.Channel

	mirror    *adsproxy.Mirror
	proxy     *adsproxy.Server
	mirrorIDs map[string]struct{}

	// mirrorMu serializes the mirror availability decisions of a
	// synchronization pass with the channel callbacks that invalidate a copy.
	// A notification, removal or rename must never be overwritten by a pass
	// that began before it.
	mirrorMu sync.Mutex

	commandMu    sync.Mutex
	seenCommands map[string]struct{}
	commandOrder []string
}

func (s *Store) startRuntime() error {
	if !s.options.StartRuntime {
		return nil
	}
	s.runtimeMu.Lock()
	if s.runtime != nil {
		s.runtimeMu.Unlock()
		return nil
	}
	s.runtimeMu.Unlock()

	instanceID := s.options.InstanceID
	credentialParts := strings.Split(s.options.Credential, "_")
	credentialInstanceID := ""
	if len(credentialParts) == 3 {
		credentialInstanceID = credentialParts[1]
	}
	if instanceID == "" {
		instanceID = credentialInstanceID
	}
	if credentialInstanceID != "" && instanceID != credentialInstanceID {
		return errors.New(errors.ConfigError, "policy source InstanceID does not match the instance credential")
	}
	agentVersion := s.options.AgentVersion
	if agentVersion == "" {
		agentVersion = defaultAgentVersion
	}
	chain := append([]string(nil), s.options.Chain...)
	if len(chain) == 0 {
		chain = []string{instanceID}
	}

	runtime := &distributionRuntime{
		trigger:      make(chan struct{}, 1),
		mirrorIDs:    make(map[string]struct{}),
		seenCommands: make(map[string]struct{}),
	}
	if s.options.ServeDownstream {
		if s.options.DownstreamCredentialsFile == "" {
			return errors.New(errors.ConfigError, "DownstreamCredentialsFile is required when ServeDownstream is true")
		}
		mirror, err := adsproxy.NewMirror(adsproxy.MirrorOptions{PublicKeys: s.options.PublicKeys})
		if err != nil {
			return errors.Wrap(err, errors.ConfigError, "invalid downstream mirror configuration")
		}
		proxy, err := adsproxy.NewServer(adsproxy.Config{
			InstanceID:                  instanceID,
			Chain:                       chain,
			Mirror:                      mirror,
			CredentialFile:              s.options.DownstreamCredentialsFile,
			MaxDownstream:               s.options.MaxDownstream,
			MaxConnectionsPerDownstream: s.options.MaxConnectionsPerDownstream,
			MaxChainDepth:               s.options.MaxChainDepth,
			HeartbeatInterval:           s.options.HeartbeatInterval,
			StaleAfter:                  s.options.StaleAfter,
			StopAfter:                   s.options.StopAfter,
			Paused: func() bool {
				// A stopped instance no longer verifies the copies it
				// distributes, so downstream distribution must stop with it.
				instance := s.machine.Instance()
				return instance.Paused || instance.Liveness == adsync.LivenessStopped
			},
		})
		if err != nil {
			return errors.Wrap(err, errors.ConfigError, "invalid downstream server configuration")
		}
		runtime.mirror = mirror
		runtime.proxy = proxy
	}

	upstreamInstanceID := ""
	if len(chain) > 1 {
		upstreamInstanceID = chain[len(chain)-2]
	}
	channel, err := adsync.NewChannel(s.client, adsync.ChannelOptions{
		InstanceID:         instanceID,
		AgentVersion:       agentVersion,
		Chain:              chain,
		Machine:            s.machine,
		Transport:          s.options.Transport,
		UpstreamInstanceID: upstreamInstanceID,
		Fallback:           s.fallbackState,
		OnNotify: func(notify adsync.Notify) {
			// The source announced a head this mirror has not verified yet.
			// Withhold the copy until the next pass reaches the new head, so a
			// failing or slow discovery cannot leave downstream instances on
			// the previous revision.
			s.withholdMirrorService(notify.ServiceID)
			s.triggerSync()
		},
		OnServiceSetChange: func(change adsync.ServiceSetChange) {
			s.applyServiceSetChange(change)
			s.triggerSync()
		},
		OnCommand: s.handleRuntimeCommand,
	})
	if err != nil {
		if runtime.proxy != nil {
			runtime.proxy.Close()
		}
		return errors.Wrap(err, errors.ConfigError, "invalid policy source channel configuration")
	}
	runtime.channel = channel

	ctx, cancel := context.WithCancel(context.Background())
	runtime.cancel = cancel
	runtime.done = make(chan struct{})
	s.runtimeMu.Lock()
	if s.runtime != nil {
		s.runtimeMu.Unlock()
		cancel()
		if runtime.proxy != nil {
			runtime.proxy.Close()
		}
		return nil
	}
	s.runtime = runtime
	s.runtimeMu.Unlock()

	var running sync.WaitGroup
	running.Add(2)
	go func() {
		defer running.Done()
		if err := channel.Run(ctx); err != nil && ctx.Err() == nil {
			log.Errorf("policy-source notification channel stopped: %v", err)
			cancel()
		}
	}()
	go func() {
		defer running.Done()
		s.reconcile(ctx)
	}()
	go func() {
		running.Wait()
		close(runtime.done)
	}()
	s.triggerSync()
	return nil
}

// machineService returns the state machine's current record for one service
// ID. The record carries the authoritative membership, name and head.
func (s *Store) machineService(serviceID string) (adsync.ServiceStatus, bool) {
	for _, status := range s.machine.Services() {
		if status.ID == serviceID {
			return status, true
		}
	}
	return adsync.ServiceStatus{}, false
}

// mirrorRestoreAllowed reports whether a copy a pass verified may be
// advertised again. The service must still be an active member of the
// authoritative set with the name discovery advertised, and a head the
// channel already announced must not be newer than the verified revision. The
// caller holds mirrorMu, so an invalidation callback cannot interleave between
// this check and the restore.
func (s *Store) mirrorRestoreAllowed(service adsync.Service) bool {
	status, known := s.machineService(service.ID)
	if !known || status.State == adsync.SyncStateRemoved || status.Name != service.Name {
		return false
	}
	return status.Head <= service.Revision
}

// withholdMirror stops advertising one service while keeping its verified
// copy. The caller holds no mirror lock.
func (s *Store) withholdMirror(runtime *distributionRuntime, serviceID string) {
	runtime.mirrorMu.Lock()
	runtime.mirror.MarkUnavailable(serviceID)
	runtime.mirrorMu.Unlock()
}

// withholdMirrorService stops advertising a service whose head the source just
// advanced, until a pass verifies the new head. reachMirrorHead marks it
// available again once the advertised identity and head are confirmed.
func (s *Store) withholdMirrorService(serviceID string) {
	s.runtimeMu.Lock()
	runtime := s.runtime
	s.runtimeMu.Unlock()
	if runtime == nil || runtime.mirror == nil {
		return
	}
	s.withholdMirror(runtime, serviceID)
}

// runBeforeMirrorRestore invokes the test hook installed by
// setBeforeMirrorRestore, if any.
func (s *Store) runBeforeMirrorRestore(serviceID string) {
	s.hookMu.Lock()
	hook := s.beforeMirrorRestore
	s.hookMu.Unlock()
	if hook != nil {
		hook(serviceID)
	}
}

// setBeforeMirrorRestore installs a hook that runs after a pass verified a
// mirror copy and before the pass decides whether to restore it. Tests use it
// to interleave a channel callback deterministically; production leaves it
// nil.
func (s *Store) setBeforeMirrorRestore(hook func(serviceID string)) {
	s.hookMu.Lock()
	s.beforeMirrorRestore = hook
	s.hookMu.Unlock()
}

// runBeforeDiscover invokes the test hook installed by setBeforeDiscover, if
// any.
func (s *Store) runBeforeDiscover() {
	s.hookMu.Lock()
	hook := s.beforeDiscover
	s.hookMu.Unlock()
	if hook != nil {
		hook()
	}
}

// setBeforeDiscover installs a hook that runs in a pass after discovery
// removals and rename guards were applied and before the state machine learns
// the discovered set. Tests use it to observe that window deterministically;
// production leaves it nil.
func (s *Store) setBeforeDiscover(hook func()) {
	s.hookMu.Lock()
	s.beforeDiscover = hook
	s.hookMu.Unlock()
}

// applyServiceSetChange applies the authoritative removals and renames from a
// channel ack to the local copies and the downstream mirror immediately. A
// successful discovery would do the same, but it can fail; the ack alone
// already proves the service is gone or renamed, so the evaluator must stop
// serving the old copy before the next discovery attempt.
func (s *Store) applyServiceSetChange(change adsync.ServiceSetChange) {
	if len(change.Removed) == 0 && len(change.Renamed) == 0 {
		return
	}
	// The mirror changes come first: discarding the local copy below can block
	// behind an in-flight fetch of the same service, and downstream instances
	// must stop serving the removed or renamed copy without waiting for it.
	// The mirror changes are also serialized with a pass's final restore
	// decision, so an older pass cannot bring the copy back.
	s.runtimeMu.Lock()
	runtime := s.runtime
	s.runtimeMu.Unlock()
	if runtime != nil && runtime.mirror != nil {
		runtime.mirrorMu.Lock()
		for _, service := range change.Removed {
			runtime.mirror.RemoveService(service.ID)
		}
		for _, service := range change.Renamed {
			// The mirror serves signed snapshots, which still carry the old
			// name. Withhold the service until a snapshot or change page
			// verifies the new name at the advertised head.
			runtime.mirror.MarkUnavailable(service.ID)
		}
		runtime.mirrorMu.Unlock()
	}
	changed := false
	for _, service := range change.Removed {
		s.removeCopy(service.ID)
		if err := s.syncer.Forget(service.ID); err != nil {
			log.Warnf("policy-source could not discard removed service %q: %v", service.ID, err)
		}
		changed = true
	}
	if s.renameCopies(change.Renamed) {
		changed = true
	}
	if changed {
		s.publish()
	}
}

// renameCopies applies authoritative service renames to the installed copies.
// The old name is recorded like a removal until the evaluator installs the
// assembly carrying the new name, so the request gate cannot evaluate the
// renamed service under its previous name in the meantime.
func (s *Store) renameCopies(renames []adsync.Service) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, service := range renames {
		copied := s.copies[service.ID]
		if copied == nil || copied.service.Name == service.Name {
			continue
		}
		s.pendingRemoved[service.ID] = copied.service.Name
		copied.service.Name = service.Name
		changed = true
	}
	return changed
}

func (s *Store) reconcile(ctx context.Context) {
	period := s.options.ReconcilePeriod
	if period == 0 {
		period = defaultReconcilePeriod
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.currentRuntimeTrigger():
		}
		if err := s.Sync(ctx); err != nil && ctx.Err() == nil {
			log.Warnf("policy-source reconciliation did not complete: %v", err)
		}
	}
}

func (s *Store) currentRuntimeTrigger() <-chan struct{} {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.runtime == nil {
		return nil
	}
	return s.runtime.trigger
}

func (s *Store) triggerSync() {
	s.runtimeMu.Lock()
	runtime := s.runtime
	s.runtimeMu.Unlock()
	if runtime == nil {
		return
	}
	select {
	case runtime.trigger <- struct{}{}:
	default:
	}
}

func (s *Store) fallbackState() adsync.FallbackState {
	state := adsync.FallbackState{}
	for _, service := range s.Services() {
		if service.State == adsync.SyncStateSyncing && s.FallbackAvailable() {
			state.Active = true
			state.Services = append(state.Services, service.ID)
		}
	}
	return state
}

// DownstreamHandler returns the mounted proxy protocol handler, or nil when
// this ADS is not configured to serve downstream instances.
func (s *Store) DownstreamHandler() http.Handler {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.runtime == nil || s.runtime.proxy == nil {
		return nil
	}
	return s.runtime.proxy
}

// ReloadDownstreamCredentials applies a SIGHUP credential reload. Existing
// channels are closed by the proxy so every downstream re-authenticates.
func (s *Store) ReloadDownstreamCredentials() error {
	s.runtimeMu.Lock()
	runtime := s.runtime
	s.runtimeMu.Unlock()
	if runtime == nil || runtime.proxy == nil {
		return nil
	}
	return runtime.proxy.ReloadCredentials()
}

// Close stops and joins the channel and reconciliation loops before closing
// downstream sessions and the evaluator watch. It is safe to call repeatedly.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.StopWatch()
		s.runtimeMu.Lock()
		runtime := s.runtime
		s.runtimeMu.Unlock()
		if runtime == nil {
			return
		}
		runtime.cancel()
		<-runtime.done
		if runtime.proxy != nil {
			runtime.proxy.Close()
		}
	})
	return nil
}

func (s *Store) handleRuntimeCommand(command adsync.Command) *adsync.CommandResult {
	s.runtimeMu.Lock()
	runtime := s.runtime
	s.runtimeMu.Unlock()
	if runtime == nil {
		return &adsync.CommandResult{Status: "rejected", Result: "runtime is not running"}
	}
	runtime.commandMu.Lock()
	if _, seen := runtime.seenCommands[command.ID]; seen {
		runtime.commandMu.Unlock()
		return nil
	}
	runtime.seenCommands[command.ID] = struct{}{}
	runtime.commandOrder = append(runtime.commandOrder, command.ID)
	if len(runtime.commandOrder) > maxRememberedCommands {
		oldest := runtime.commandOrder[0]
		runtime.commandOrder = runtime.commandOrder[1:]
		delete(runtime.seenCommands, oldest)
	}
	runtime.commandMu.Unlock()

	switch command.Action {
	case "pause":
		s.machine.Pause()
		return &adsync.CommandResult{Status: "acked"}
	case "resume":
		s.machine.Resume()
		s.triggerSync()
		return &adsync.CommandResult{Status: "acked"}
	case "resync":
		s.resync(command.ServiceID)
		return &adsync.CommandResult{Status: "acked"}
	case "set_source":
		if command.CredentialProfile == "" || command.SourceURL != s.options.UpstreamURL {
			return &adsync.CommandResult{Status: "rejected", Result: "unknown credential profile"}
		}
		s.triggerSync()
		return &adsync.CommandResult{Status: "acked"}
	case "stop":
		s.machine.Stop(adsync.ReasonStoppedByCommand)
		return &adsync.CommandResult{Status: "acked"}
	default:
		return &adsync.CommandResult{Status: "rejected", Result: "unknown action"}
	}
}

func (s *Store) resync(serviceID string) {
	var targets []string
	if serviceID != "" {
		targets = append(targets, serviceID)
	} else {
		for _, service := range s.Services() {
			if service.State != adsync.SyncStateRemoved {
				targets = append(targets, service.ID)
			}
		}
	}
	changed := false
	for _, id := range targets {
		s.machine.Resync(id)
		if err := s.syncer.Forget(id); err != nil {
			log.Warnf("policy-source resync could not discard service %q: %v", id, err)
			continue
		}
		s.dropCopy(id)
		changed = true
	}
	if changed {
		s.publish()
	}
	s.triggerSync()
}

// syncMirror feeds the downstream server from the same verified protocol
// client and selected service set as the local store. The mirror verifies every
// signed snapshot and commit again before making it available downstream.
func (s *Store) syncMirror(ctx context.Context, selected []adsync.Service, failed map[string]struct{}) {
	s.runtimeMu.Lock()
	runtime := s.runtime
	s.runtimeMu.Unlock()
	if runtime == nil || runtime.mirror == nil {
		return
	}
	live := make(map[string]struct{}, len(selected))
	for _, service := range selected {
		// A channel ack can remove or rename a service while this pass is in
		// flight. A signed snapshot from a lagging replica must not revive a
		// service the authoritative set dropped, so check the set again before
		// fetching and again before the copy becomes servable.
		runtime.mirrorMu.Lock()
		status, known := s.machineService(service.ID)
		inactive := !known || status.State == adsync.SyncStateRemoved
		renamed := !inactive && status.Name != service.Name
		if inactive {
			runtime.mirror.RemoveService(service.ID)
		}
		runtime.mirrorMu.Unlock()
		if inactive {
			continue
		}
		live[service.ID] = struct{}{}
		if renamed {
			// The discovery response is older than the authoritative rename,
			// and the signed copy still carries the previous name. Keep it
			// withheld until a pass verifies the new name.
			s.withholdMirror(runtime, service.ID)
			continue
		}
		if _, syncFailed := failed[service.ID]; syncFailed && s.mirrorBehind(runtime.mirror, service) {
			// The local copy could not be verified at the advertised head. A
			// downstream copy at an older revision is just as stale, so withhold
			// it before the (possibly retried) mirror fetch below.
			s.withholdMirror(runtime, service.ID)
		}
		if err := s.reachMirrorHead(ctx, runtime.mirror, service); err != nil {
			// The source advertises a head this mirror could not verify.
			// Serving the older copy would let downstream instances keep
			// evaluating policy the source has already replaced, so the
			// service is withheld until a later fetch reaches the head.
			log.Warnf("policy-source mirror cannot serve service %q at the advertised head: %v", service.ID, err)
			s.withholdMirror(runtime, service.ID)
			continue
		}
		s.runBeforeMirrorRestore(service.ID)
		// The copy verifiably matches the advertised identity and head. It may
		// have been withheld during an earlier pass; restore serving now, after
		// the whole catch-up has completed, unless the authoritative set or the
		// announced head changed while the pass was in flight. The check runs
		// under mirrorMu so a notification or ack cannot withhold the copy
		// between the check and the restore.
		runtime.mirrorMu.Lock()
		if !s.mirrorRestoreAllowed(service) {
			runtime.mirror.MarkUnavailable(service.ID)
		} else {
			runtime.mirror.MarkAvailable(service.ID)
		}
		runtime.mirrorMu.Unlock()
	}
	runtime.mirrorMu.Lock()
	for id := range runtime.mirrorIDs {
		if _, found := live[id]; !found {
			runtime.mirror.RemoveService(id)
			delete(runtime.mirrorIDs, id)
		}
	}
	for id := range live {
		runtime.mirrorIDs[id] = struct{}{}
	}
	runtime.mirrorMu.Unlock()
}

// reachMirrorHead brings the mirror copy of one service to the revision the
// source advertises. A successful snapshot or change page clears an earlier
// unavailability, so a recovered source restores downstream serving without a
// restart.
func (s *Store) reachMirrorHead(ctx context.Context, mirror *adsproxy.Mirror, service adsync.Service) error {
	info, current := mirror.CurrentInfo(service.ID)
	sameIdentity := current && info.SourceID == service.ID && info.Epoch == service.Epoch
	if sameIdentity {
		switch {
		case info.Revision > service.Revision:
			// The mirror is ahead of the advertised head, which can only mean
			// discovery is stale; the verified copy is not older policy.
			return nil
		case info.Revision == service.Revision:
			if info.StateHash != service.HeadStateHash {
				return fmt.Errorf("%w: mirror copy of service %q at revision %d has a different state hash than the advertised head",
					adsync.ErrIntegrity, service.ID, service.Revision)
			}
			if info.Name != service.Name {
				return fmt.Errorf("%w: mirror copy of service %q still carries name %q, advertised name is %q",
					adsync.ErrProtocol, service.ID, info.Name, service.Name)
			}
			return nil
		}
		if err := s.catchUpMirror(ctx, mirror, service, info); err == nil && s.mirrorReached(mirror, service) {
			return nil
		}
	}
	if err := s.loadMirrorSnapshot(ctx, mirror, service); err != nil {
		return err
	}
	if s.mirrorReached(mirror, service) {
		return nil
	}
	// A lagging replica can serve a signed snapshot behind its advertised head.
	// Resume from the snapshot and verify the missing revisions before serving.
	info, current = mirror.CurrentInfo(service.ID)
	if !current {
		return fmt.Errorf("mirror holds no verified copy of service %q", service.ID)
	}
	if err := s.catchUpMirror(ctx, mirror, service, info); err != nil {
		return err
	}
	if !s.mirrorReached(mirror, service) {
		return fmt.Errorf("mirror copy of service %q is behind the advertised revision %d", service.ID, service.Revision)
	}
	return nil
}

// mirrorMatchesHead reports whether the verified copy matches the advertised
// identity and head. A copy ahead of discovery is accepted as verified; at the
// advertised revision the state hash must match, so an inconsistent source
// cannot make the runtime advertise a different policy generation.
func mirrorMatchesHead(info adsproxy.ServiceInfo, service adsync.Service) bool {
	if info.SourceID != service.ID || info.Epoch != service.Epoch || info.Name != service.Name {
		return false
	}
	if info.Revision > service.Revision {
		return true
	}
	return info.Revision == service.Revision && info.StateHash == service.HeadStateHash
}

// mirrorReached reports whether the verified copy matches the advertised
// identity and has reached its head. It deliberately ignores the withheld flag:
// the caller decides when the copy becomes servable again.
func (s *Store) mirrorReached(mirror *adsproxy.Mirror, service adsync.Service) bool {
	info, current := mirror.CurrentInfo(service.ID)
	return current && mirrorMatchesHead(info, service)
}

// mirrorBehind reports whether the mirror holds a copy of the service that does
// not match the identity and head the source advertises. A service the mirror
// never held needs no withholding: no downstream instance has seen it.
func (s *Store) mirrorBehind(mirror *adsproxy.Mirror, service adsync.Service) bool {
	info, availability := mirror.Lookup(service.ID)
	switch availability {
	case adsproxy.ServiceUnknown:
		return false
	case adsproxy.ServiceAvailable:
		return info.SourceID != service.ID || info.Epoch != service.Epoch || info.Revision < service.Revision
	default:
		return true
	}
}

func (s *Store) catchUpMirror(ctx context.Context, mirror *adsproxy.Mirror, service adsync.Service, info adsproxy.ServiceInfo) error {
	since := info.Revision
	anchor := info.CommitHash
	for pages := 0; pages < maxMirrorPages; pages++ {
		page, err := s.client.Changes(ctx, service.ID, info.SourceID, info.Epoch, since, anchor, 100)
		if err != nil {
			return err
		}
		if err := mirror.ApplyChanges(page); err != nil {
			return err
		}
		if page.To >= page.Head || len(page.Commits) == 0 {
			return nil
		}
		since = page.To
		anchor = page.Commits[len(page.Commits)-1].Hash
	}
	return fmt.Errorf("policy-source mirror exceeded %d change pages", maxMirrorPages)
}

func (s *Store) loadMirrorSnapshot(ctx context.Context, mirror *adsproxy.Mirror, service adsync.Service) error {
	snapshot, _, err := s.client.Snapshot(ctx, service.ID, "")
	if err != nil {
		return err
	}
	return mirror.ApplySnapshot(snapshot, service)
}
