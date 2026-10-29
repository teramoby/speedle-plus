// Licensed under the Universal Permissive License (UPL) Version 1.0.
package mysql

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

type watcher struct {
	cancel context.CancelFunc
	done   chan struct{}
	events pms.StorageChangeChannel
}

// Watch has one subscription per Store. Repeated calls return that subscription.
// An immediate snapshot closes the evaluator's initial-read/watch race.
func (s *Store) Watch() (pms.StorageChangeChannel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New(errors.StoreError, "MySQL store is closed")
	}
	if s.watch != nil {
		return s.watch.events, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &watcher{cancel: cancel, done: make(chan struct{}), events: make(pms.StorageChangeChannel, 1)}
	s.watch = w
	go s.poll(ctx, w)
	return w.events, nil
}

func (s *Store) poll(ctx context.Context, w *watcher) {
	defer close(w.done)
	defer close(w.events)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	var seen *int64
	for {
		ps, revision, err := s.snapshot(ctx, seen)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warnf("MySQL watch will retry: %v", err)
		} else if ps != nil {
			select {
			case w.events <- pms.StoreChangeEvent{Type: pms.FULL_RELOAD, ID: revision, Content: ps}:
				seen = &revision
			case <-ctx.Done():
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// StopWatch waits for in-flight reads/sends and is safe to call repeatedly.
func (s *Store) StopWatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopWatch()
}
func (s *Store) stopWatch() {
	if s.watch != nil {
		s.watch.cancel()
		<-s.watch.done
		s.watch = nil
	}
}

// Close stops the watcher and releases the connection pool. It is idempotent.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.stopWatch()
	return s.db.Close()
}
