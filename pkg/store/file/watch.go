// Licensed under the Universal Permissive License (UPL) Version 1.0.
package file

import (
	"context"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

type fileWatcher struct {
	cancel context.CancelFunc
	done   chan struct{}
	events pms.StorageChangeChannel
}

// A lost file is a recoverable store failure, never a reason to terminate ADS.
// Cancellation also interrupts a send to an absent/slow event consumer.
func (s *Store) Watch() (pms.StorageChangeChannel, error) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watch != nil {
		return s.watch.events, nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrap(err, errors.StoreError, "create policy file watcher")
	}
	if err := w.Add(s.FileLocation); err != nil {
		w.Close()
		return nil, errors.Wrap(err, errors.StoreError, "watch policy file")
	}
	ctx, cancel := context.WithCancel(context.Background())
	sub := &fileWatcher{cancel: cancel, done: make(chan struct{}), events: make(pms.StorageChangeChannel, 1)}
	s.watch = sub
	go func() {
		defer close(sub.done)
		defer close(sub.events)
		defer w.Close()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		send := func(kind pms.EventType) bool {
			select {
			case sub.events <- pms.StoreChangeEvent{Type: kind}:
				return true
			case <-ctx.Done():
				return false
			}
		}
		lost := false
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
					_ = w.Remove(s.FileLocation)
					if err := w.Add(s.FileLocation); err != nil {
						if !lost && !send(pms.STORE_UNAVAILABLE) {
							return
						}
						lost = true
					} else {
						lost = false
						if !send(pms.FULL_RELOAD) {
							return
						}
					}
				} else if event.Op&fsnotify.Write != 0 {
					if !send(pms.FULL_RELOAD) {
						return
					}
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
				if !lost && !send(pms.STORE_UNAVAILABLE) {
					return
				}
				lost = true
			case <-ticker.C:
				if err := s.CheckHealth(ctx); err != nil {
					if !lost && !send(pms.STORE_UNAVAILABLE) {
						return
					}
					lost = true
				} else if lost {
					_ = w.Remove(s.FileLocation)
					if err := w.Add(s.FileLocation); err == nil {
						lost = false
						if !send(pms.FULL_RELOAD) {
							return
						}
					}
				}
			}
		}
	}()
	return sub.events, nil
}

func (s *Store) StopWatch() {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watch != nil {
		s.watch.cancel()
		<-s.watch.done
		s.watch = nil
	}
}
