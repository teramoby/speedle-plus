// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"net/http"
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
)

// distributionRuntime owns the production channel and reconciliation trigger.
// It starts when the evaluator installs its watch and is joined by
// Store.Close.
type distributionRuntime struct {
	cancel  context.CancelFunc
	done    chan struct{}
	trigger chan struct{}
	channel *adsync.Channel

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

	agentVersion := s.options.AgentVersion
	if agentVersion == "" {
		agentVersion = defaultAgentVersion
	}
	runtime := &distributionRuntime{
		trigger:      make(chan struct{}, 1),
		seenCommands: make(map[string]struct{}),
	}
	channel, err := adsync.NewChannel(s.client, adsync.ChannelOptions{
		AgentVersion:    agentVersion,
		Machine:         s.machine,
		Transport:       s.options.Transport,
		Fallback:        s.fallbackState,
		DownstreamCount: s.downstreamSessionCount,
		OnNotify: func(adsync.Notify) {
			s.triggerSync()
		},
		OnServiceSetChange: func(change adsync.ServiceSetChange) {
			s.applyServiceSetChange(change)
			s.triggerSync()
		},
		OnAck: func() {
			// Reconcile on every ack: any service the ack does not confirm
			// as in sync (behind, different hash or epoch, or not yet
			// loaded) starts a pass. An in-sync instance pulls nothing.
			for _, service := range s.machine.Services() {
				if service.State == adsync.SyncStateSyncing {
					s.triggerSync()
					return
				}
			}
		},
		OnCommand: s.handleRuntimeCommand,
	})
	if err != nil {
		return errors.Wrap(err, errors.ConfigError, "invalid policy source channel configuration")
	}
	runtime.channel = channel
	return s.runRuntime(runtime)
}

// runRuntime installs runtime and starts its channel and reconciliation
// loops. A runtime installed concurrently wins and this one is discarded.
func (s *Store) runRuntime(runtime *distributionRuntime) error {
	channel := runtime.channel
	ctx, cancel := context.WithCancel(context.Background())
	runtime.cancel = cancel
	runtime.done = make(chan struct{})
	s.runtimeMu.Lock()
	if s.runtime != nil {
		s.runtimeMu.Unlock()
		cancel()
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

// runtimeChannel reports whether the store is configured for the owned
// runtime, whose channel acks are the only source of the service set.
func (s *Store) runtimeChannel() bool { return s.options.StartRuntime }

// ackedServices returns the service set and heads of the most recent
// confirming ack. The boolean is false before the first ack.
func (s *Store) ackedServices() ([]adsync.Service, bool) {
	s.runtimeMu.Lock()
	runtime := s.runtime
	s.runtimeMu.Unlock()
	if runtime == nil || runtime.channel == nil {
		return nil, false
	}
	return runtime.channel.AckedServices()
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
// channel ack to the local copies immediately. A
// successful discovery would do the same, but it can fail; the ack alone
// already proves the service is gone or renamed, so the evaluator must stop
// serving the old copy before the next discovery attempt.
func (s *Store) applyServiceSetChange(change adsync.ServiceSetChange) {
	if len(change.Removed) == 0 && len(change.Renamed) == 0 {
		return
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

// newDownstreamServing builds the mirror and protocol server of a relay. The
// protocol server is served only by the dedicated downstream TLS listener,
// never by the application listener. Downstream distribution is tied to the
// state machine: it stops while the instance is paused, and while it is not
// live it can no longer vouch for the copies it holds.
func newDownstreamServing(client *adsync.Client, machine *adsync.Machine, options Options) (*downstreamServing, error) {
	mirror, err := adsproxy.NewMirror(adsproxy.MirrorOptions{})
	if err != nil {
		return nil, errors.Wrap(err, errors.ConfigError, "invalid downstream mirror configuration")
	}
	instanceID, err := client.InstanceID()
	if err != nil {
		return nil, errors.Wrap(err, errors.ConfigError,
			"a persisted instance ID is required to serve downstream instances")
	}
	server, err := adsproxy.NewServer(adsproxy.Config{
		InstanceID:        instanceID,
		Mirror:            mirror,
		HeartbeatInterval: options.HeartbeatInterval,
		StaleAfter:        options.StaleAfter,
		StopAfter:         options.StopAfter,
		Paused: func() bool {
			instance := machine.Instance()
			return instance.Paused || instance.Liveness != adsync.LivenessLive
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.ConfigError, "invalid downstream server configuration")
	}
	return &downstreamServing{mirror: mirror, server: server}, nil
}

// DownstreamHandler returns the policy-source protocol handler a relay serves
// on its dedicated downstream TLS listener. It is nil for a store that does
// not serve downstream instances.
func (s *Store) DownstreamHandler() http.Handler {
	if s.downstream == nil {
		return nil
	}
	return s.downstream.server
}

// DownstreamCount returns the number of distinct direct downstream sessions
// currently connected. The relay reports it in its own heartbeat.
func (s *Store) DownstreamCount() int {
	if s.downstream == nil {
		return 0
	}
	return s.downstream.server.DownstreamCount()
}

// downstreamSessionCount is the heartbeat callback of the owned channel.
func (s *Store) downstreamSessionCount() uint64 {
	return uint64(s.DownstreamCount())
}

// Close stops and joins the channel and reconciliation loops before closing
// downstream sessions and the evaluator watch. It is safe to call repeatedly.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.StopWatch()
		if s.downstream != nil {
			s.downstream.server.Close()
		}
		s.runtimeMu.Lock()
		runtime := s.runtime
		s.runtimeMu.Unlock()
		if runtime == nil {
			return
		}
		runtime.cancel()
		<-runtime.done
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
		s.changeDistribution(s.machine.Pause)
		return &adsync.CommandResult{Status: "acked"}
	case "resume":
		s.changeDistribution(s.machine.Resume)
		s.triggerSync()
		return &adsync.CommandResult{Status: "acked"}
	case "resync":
		s.resync(command.ServiceID)
		return &adsync.CommandResult{Status: "acked"}
	case "stop":
		s.changeDistribution(func() { s.machine.Stop(adsync.ReasonStoppedByCommand) })
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
		s.clearIntegrityRetry(id)
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
