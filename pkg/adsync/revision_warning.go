// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Share the budget across machines and syncers, so an ack, notification and
// snapshot for the same service cannot each emit a warning within a minute.
var revisionWarnings = revisionWarningLimiter{clock: systemClock{}}

// Also bound bursts across services, including a response containing a whole
// restored service set. Every dropped event is counted in the next log entry.
const revisionWarningBurst = 10

type revisionWarningLimiter struct {
	mu          sync.Mutex
	clock       Clock
	last        map[string]time.Time
	lastCleanup time.Time
	windowStart time.Time
	emitted     int
	suppressed  uint64
}

// Capture values under the owner's lock and emit only after releasing it: a
// slow log sink must not block other readers or updates of the owner.
type revisionWarning struct {
	serviceID, epoch, source string
	localSource              string
	upstream, local, head    uint64
}

func (w revisionWarning) log() {
	revisionWarnings.warn(w)
}

// local is the memory or disk-cache copy's revision (zero without a copy);
// head is the separately known upstream head, which may be ahead of that copy.
func (l *revisionWarningLimiter) warn(w revisionWarning) {
	l.mu.Lock()
	now := l.clock.Now()
	if l.windowStart.IsZero() || now.Sub(l.windowStart) >= time.Minute {
		l.windowStart, l.emitted = now, 0
	}
	last, seen := l.last[w.serviceID]
	if (seen && now.Sub(last) < time.Minute) || l.emitted >= revisionWarningBurst {
		l.suppressed++
		l.mu.Unlock()
		return
	}
	// Expired entries are no longer needed. Reclaim them at most once a minute
	// rather than retaining every service the process has ever encountered.
	if l.last == nil {
		l.last = make(map[string]time.Time)
	}
	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= time.Minute {
		for id, last := range l.last {
			if now.Sub(last) >= time.Minute {
				delete(l.last, id)
			}
		}
		l.lastCleanup = now
	}
	l.last[w.serviceID] = now
	l.emitted++
	// Process-wide count, including per-service and global-budget suppression.
	suppressed := l.suppressed
	l.suppressed = 0
	l.mu.Unlock()

	log.WithFields(log.Fields{
		"service_id":            w.serviceID,
		"epoch":                 w.epoch,
		"upstream_revision":     w.upstream,
		"local_revision":        w.local,
		"local_source":          w.localSource,
		"known_head_revision":   w.head,
		"source":                w.source,
		"suppressed_since_last": suppressed,
	}).Warn("policy-source revision is below the local copy or known head in the same epoch; ignoring rollback")
}
