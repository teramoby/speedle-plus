// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

const storeHealthInterval = time.Second

// These helpers change availability only while holding cacheGate. A failed
// read preserves data for a configured grace period, never its validity forever.
func (p *PolicyEvalImpl) storeUnavailableLocked() {
	if p.watchManaged && !p.watchStopped.Load() {
		p.watchNeedsRestart.Store(true)
	}
	if p.storeFailureSince.IsZero() {
		p.storeFailureSince = time.Now()
		log.Error("Policy store unavailable; a complete reload is required")
	}
}

func (p *PolicyEvalImpl) storeAvailableLocked() {
	// A fresh data read does not repair a failed subscription. Keep the
	// original failure/grace deadline until a new watch has been installed.
	if !p.watchNeedsRestart.Load() {
		p.storeFailureSince = time.Time{}
	}
	p.cacheFailed.Store(false)
}

func (p *PolicyEvalImpl) cacheErrorLocked() error {
	if p.closed.Load() {
		return errors.New(errors.EvalCacheError, "authorization engine is closed")
	}
	if p.cacheFailed.Load() {
		return errors.New(errors.EvalCacheError, "authorization cache requires a successful full reload")
	}
	if !p.storeFailureSince.IsZero() && time.Since(p.storeFailureSince) >= p.storeFailureGracePeriod {
		return errors.New(errors.EvalCacheError, "policy store unavailable; a successful full reload is required")
	}
	return nil
}

func (p *PolicyEvalImpl) probeStoreHealth() (err error) {
	defer recoverEvaluation("StoreHealth", &err)
	ctx, cancel := context.WithTimeout(context.Background(), storeHealthInterval)
	defer cancel()
	return p.healthChecker.CheckHealth(ctx)
}

// Also used by requests when policy watching is explicitly disabled. This
// avoids a background goroutine for static evaluators, while checking liveness
// at most once per second. A successful probe alone never validates old data.
func (p *PolicyEvalImpl) ensureStoreHealth() {
	var recovered error
	defer func() {
		if recovered != nil {
			p.cacheGate.Lock()
			p.cacheFailed.Store(true)
			p.cacheGate.Unlock()
		}
	}()
	defer recoverEvaluation("StoreHealthMonitor", &recovered)
	if p.healthChecker == nil || p.closed.Load() {
		return
	}
	p.healthMu.Lock()
	defer p.healthMu.Unlock()
	if time.Since(p.lastHealthCheck) < storeHealthInterval {
		return
	}
	err := p.probeStoreHealth()
	p.lastHealthCheck = time.Now()
	if err != nil {
		p.cacheGate.Lock()
		defer p.cacheGate.Unlock()
		p.storeUnavailableLocked()
		return
	}
	p.cacheGate.RLock()
	reload := !p.storeFailureSince.IsZero() || p.cacheFailed.Load()
	p.cacheGate.RUnlock()
	if reload && !p.watchNeedsRestart.Load() {
		_ = p.Refresh()
	}
}

// Watch events are invalidation signals, including for extensions without the
// optional health checker. Read a fresh complete snapshot
// rather than replaying stale deltas queued before/during an outage. This also
// prevents an old payload from restoring a revoked grant after recovery.
func (p *PolicyEvalImpl) followStoreChanges(ch pms.StorageChangeChannel) {
	ticker := time.NewTicker(storeHealthInterval)
	defer ticker.Stop()
	for {
		if p.watchStopped.Load() {
			return
		}
		if p.watchNeedsRestart.Load() {
			ch = nil
		}
		select {
		case event, ok := <-ch:
			if p.collectStoreInvalidations(ch, event, ok) {
				_ = p.applyStoreChange(pms.StoreChangeEvent{Type: pms.FULL_RELOAD})
			}
		case <-ticker.C:
			if ch == nil {
				var err error
				ch, err = p.reopenStoreWatch()
				if err != nil || ch == nil {
					continue
				}
				p.watchNeedsRestart.Store(false)
				_ = p.Refresh()
			}
			p.ensureStoreHealth()
		}
	}
}

// Collect a short burst before reading. The fixed window cannot be extended
// by continuous writers. Stop collecting before the snapshot read: any events
// arriving during that read remain queued and require another fresh read.
// Unavailability/closure takes effect immediately, without waiting for a timer.
func (p *PolicyEvalImpl) collectStoreInvalidations(ch pms.StorageChangeChannel, event pms.StoreChangeEvent, ok bool) bool {
	window := time.NewTimer(5 * time.Millisecond)
	defer window.Stop()
	for {
		if !ok || event.Type == pms.STORE_UNAVAILABLE {
			p.watchNeedsRestart.Store(true)
			if !p.watchStopped.Load() {
				_ = p.applyStoreChange(pms.StoreChangeEvent{Type: pms.STORE_UNAVAILABLE})
			}
			return false
		}
		select {
		case event, ok = <-ch:
		case <-window.C:
			return !p.watchStopped.Load()
		}
	}
}

func (p *PolicyEvalImpl) reopenStoreWatch() (ch pms.StorageChangeChannel, err error) {
	defer recoverEvaluation("RestartStoreWatch", &err)
	p.watchLifecycleMu.Lock()
	defer p.watchLifecycleMu.Unlock()
	if p.watchStopped.Load() || p.closed.Load() {
		return nil, nil
	}
	p.Store.StopWatch()
	return p.Store.Watch()
}
