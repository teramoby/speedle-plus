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

type revisionWarningLimiter struct {
	mu          sync.Mutex
	clock       Clock
	last        map[string]time.Time
	lastCleanup time.Time
}

func (l *revisionWarningLimiter) warn(serviceID, epoch string, upstream, local uint64, source string) {
	l.mu.Lock()
	now := l.clock.Now()
	if last, ok := l.last[serviceID]; ok && now.Sub(last) < time.Minute {
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
	l.last[serviceID] = now
	l.mu.Unlock()

	log.WithFields(log.Fields{
		"service_id":        serviceID,
		"epoch":             epoch,
		"upstream_revision": upstream,
		"local_revision":    local,
		"source":            source,
	}).Warn("policy-source revision is lower than local revision in the same epoch; ignoring rollback")
}
