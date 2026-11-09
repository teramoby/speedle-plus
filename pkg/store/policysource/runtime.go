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
		OnNotify:           func(adsync.Notify) { s.triggerSync() },
		OnServiceSetChange: func(adsync.ServiceSetChange) { s.triggerSync() },
		OnCommand:          s.handleRuntimeCommand,
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
		if command.CredentialProfile == "" || command.SourceURL != s.options.PolicySourceURL {
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
		live[service.ID] = struct{}{}
		if _, syncFailed := failed[service.ID]; syncFailed && s.mirrorBehind(runtime.mirror, service) {
			// The local copy could not be verified at the advertised head. A
			// downstream copy at an older revision is just as stale, so withhold
			// it before the (possibly retried) mirror fetch below.
			runtime.mirror.MarkUnavailable(service.ID)
		}
		if err := s.reachMirrorHead(ctx, runtime.mirror, service); err != nil {
			// The source advertises a head this mirror could not verify.
			// Serving the older copy would let downstream instances keep
			// evaluating policy the source has already replaced, so the
			// service is withheld until a later fetch reaches the head.
			log.Warnf("policy-source mirror cannot serve service %q at the advertised head: %v", service.ID, err)
			runtime.mirror.MarkUnavailable(service.ID)
			continue
		}
		// The copy verifiably matches the advertised identity and head. It may
		// have been withheld during an earlier pass; restore serving now, after
		// the whole catch-up has completed.
		runtime.mirror.MarkAvailable(service.ID)
	}
	for id := range runtime.mirrorIDs {
		if _, found := live[id]; !found {
			runtime.mirror.RemoveService(id)
			delete(runtime.mirrorIDs, id)
		}
	}
	for id := range live {
		runtime.mirrorIDs[id] = struct{}{}
	}
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
			if info.StateHash == service.HeadStateHash {
				return nil
			}
			return fmt.Errorf("%w: mirror copy of service %q at revision %d has a different state hash than the advertised head",
				adsync.ErrIntegrity, service.ID, service.Revision)
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
	if info.SourceID != service.ID || info.Epoch != service.Epoch {
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
