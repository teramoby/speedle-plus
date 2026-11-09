// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import "github.com/teramoby/speedle-plus/api/pms"

// Watch returns the channel on which the store reports that a new assembly is
// available. Repeated calls return the same channel until StopWatch.
func (s *Store) Watch() (pms.StorageChangeChannel, error) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.events == nil {
		s.events = make(pms.StorageChangeChannel, 1)
	}
	return s.events, nil
}

// StopWatch closes the change channel. A later Watch creates a new one.
func (s *Store) StopWatch() {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.events != nil {
		close(s.events)
		s.events = nil
	}
}

// notifyReload announces a new published assembly. A pending event is enough:
// the evaluator always reads the whole current assembly, never a delta, so a
// second event adds nothing. This never blocks a synchronization pass.
func (s *Store) notifyReload() {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.events == nil {
		return
	}
	select {
	case s.events <- pms.StoreChangeEvent{Type: pms.FULL_RELOAD}:
	default:
	}
}
