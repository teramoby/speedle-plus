// Licensed under the Universal Permissive License (UPL) Version 1.0.
package etcd

import (
	"context"
	"strings"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type etcdWatcher struct {
	cancel context.CancelFunc
	done   chan struct{}
	events pms.StorageChangeChannel
}

func (s *Store) Watch() (pms.StorageChangeChannel, error) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watch != nil {
		return s.watch.events, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &etcdWatcher{cancel: cancel, done: make(chan struct{}), events: make(pms.StorageChangeChannel)}
	ready := make(chan error, 1)
	s.watch = w
	go func() {
		defer close(w.done)
		defer close(w.events)
		send := func(e pms.StoreChangeEvent) bool {
			select {
			case w.events <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		first := true
		for {
			err := s.followEtcd(ctx, send, func() {
				if first {
					ready <- nil
					first = false
				} else {
					send(pms.StoreChangeEvent{Type: pms.FULL_RELOAD})
				}
			})
			if ctx.Err() != nil {
				return
			}
			if first {
				ready <- err
				return
			}
			if !send(pms.StoreChangeEvent{Type: pms.STORE_UNAVAILABLE}) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	select {
	case err := <-ready:
		if err == nil {
			return w.events, nil
		}
		cancel()
		<-w.done
		s.watch = nil
		return nil, err
	case <-time.After(requestTimeout):
		cancel()
		<-w.done
		s.watch = nil
		return nil, errors.New(errors.StoreError, "etcd watch subscription timed out")
	}
}

func (s *Store) followEtcd(ctx context.Context, send func(pms.StoreChangeEvent) bool, ready func()) error {
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	changes := s.client.Watch(watchCtx, s.KeyPrefix, clientv3.WithPrefix(), clientv3.WithCreatedNotify())
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			check, done := context.WithTimeout(ctx, time.Second)
			err := s.CheckHealth(check)
			done()
			if err != nil {
				return err
			}
		case response, ok := <-changes:
			if !ok {
				return errors.New(errors.StoreError, "etcd watch closed")
			}
			if err := response.Err(); err != nil {
				return errors.New(errors.StoreError, "etcd watch failed")
			}
			if response.Created {
				ready()
			}
			for _, event := range response.Events {
				key := string(event.Kv.Key)
				var change pms.StoreChangeEvent
				if strings.HasPrefix(key, s.KeyPrefix+ServicesKey+KeySeparator) {
					name := strings.TrimSuffix(strings.TrimPrefix(key, s.KeyPrefix+ServicesKey+KeySeparator), KeySeparator)
					if strings.Contains(name, KeySeparator) {
						continue
					}
					if event.Type == clientv3.EventTypeDelete {
						change = pms.StoreChangeEvent{Type: pms.SERVICE_DELETE, Content: []string{name}}
					} else {
						service, err := s.GetService(name)
						if errors.Code(err) == errors.EntityNotFound {
							continue
						}
						if err != nil {
							return err
						}
						change = pms.StoreChangeEvent{Type: pms.SERVICE_ADD, Content: service}
					}
				} else if strings.HasPrefix(key, s.KeyPrefix+FunctionsKey+KeySeparator) {
					name := strings.TrimPrefix(key, s.KeyPrefix+FunctionsKey+KeySeparator)
					if event.Type == clientv3.EventTypeDelete {
						change = pms.StoreChangeEvent{Type: pms.FUNCTION_DELETE, Content: []string{name}}
					} else {
						function, err := s.GetFunction(name)
						if errors.Code(err) == errors.EntityNotFound {
							continue
						}
						if err != nil {
							return err
						}
						change = pms.StoreChangeEvent{Type: pms.FUNCTION_ADD, Content: function}
					}
				} else {
					continue
				}
				change.ID = response.Header.Revision
				if !send(change) {
					return ctx.Err()
				}
			}
		}
	}
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
