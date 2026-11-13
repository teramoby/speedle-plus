// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

// Bounds of the retry delay of a service whose copy failed integrity: the
// first retry waits integrityRetryInitial, each further failure at the same
// head doubles the delay up to integrityRetryMax.
const (
	integrityRetryInitial = time.Minute
	integrityRetryMax     = time.Hour
)

// integrityRetry is the backoff of one integrity-failed service. head is the
// head the failure was observed at; a different head resets the backoff.
// attempt is the sync attempt that failed: once a resync, removal or re-add
// invalidates it, the backoff no longer applies, even if it was written after
// the resync cleared the previous one.
type integrityRetry struct {
	head    adsync.Service
	attempt adsync.SyncAttempt
	delay   time.Duration
	next    time.Time
}

func (s *Store) now() time.Time {
	if s.options.Clock != nil {
		return s.options.Clock.Now()
	}
	return time.Now()
}

func sameHead(a, b adsync.Service) bool {
	return a.Revision == b.Revision && a.Epoch == b.Epoch && a.HeadStateHash == b.HeadStateHash
}

// integrityWaiting reports whether service is still inside its integrity
// retry delay. A new head drops the backoff, so it is fetched right away.
func (s *Store) integrityWaiting(service adsync.Service) bool {
	s.integrityMu.Lock()
	defer s.integrityMu.Unlock()
	retry, found := s.integrity[service.ID]
	if !found {
		return false
	}
	if !sameHead(retry.head, service) || !s.machine.AttemptCurrent(retry.attempt) {
		delete(s.integrity, service.ID)
		return false
	}
	return s.now().Before(retry.next)
}

// recordIntegrityFailure starts or extends the backoff of service.
func (s *Store) recordIntegrityFailure(service adsync.Service, attempt adsync.SyncAttempt) {
	s.integrityMu.Lock()
	defer s.integrityMu.Unlock()
	delay := integrityRetryInitial
	if retry, found := s.integrity[service.ID]; found && sameHead(retry.head, service) && retry.attempt == attempt {
		delay = min(retry.delay*2, integrityRetryMax)
	}
	s.integrity[service.ID] = integrityRetry{head: service, attempt: attempt, delay: delay, next: s.now().Add(delay)}
}

// clearIntegrityRetry drops the backoff of serviceID: its copy verified, or
// a resync command asked for a fetch now.
func (s *Store) clearIntegrityRetry(serviceID string) {
	s.integrityMu.Lock()
	defer s.integrityMu.Unlock()
	delete(s.integrity, serviceID)
}

// keepIntegrityRetries drops the backoff of every service outside selected.
func (s *Store) keepIntegrityRetries(selected map[string]struct{}) {
	s.integrityMu.Lock()
	defer s.integrityMu.Unlock()
	for serviceID := range s.integrity {
		if _, kept := selected[serviceID]; !kept {
			delete(s.integrity, serviceID)
		}
	}
}
