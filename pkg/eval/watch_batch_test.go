// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"sync"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
)

type batchWatchStore struct {
	pms.PolicyStoreManagerADS
	mu               sync.Mutex
	snapshot         *pms.PolicyStore
	reads, blockRead int
	started, release chan struct{}
	events           pms.StorageChangeChannel
}

func (s *batchWatchStore) ReadPolicyStore() (*pms.PolicyStore, error) {
	s.mu.Lock()
	s.reads++
	snapshot, block := s.snapshot, s.reads == s.blockRead
	s.mu.Unlock()
	if block {
		close(s.started)
		<-s.release
	}
	return snapshot, nil
}
func (s *batchWatchStore) Watch() (pms.StorageChangeChannel, error) { return s.events, nil }
func (*batchWatchStore) StopWatch()                                 {}

func TestFeatureWatchBurstCoalescesFreshReloads(t *testing.T) {
	for _, duringRead := range []bool{false, true} {
		name := "queued-burst"
		if duringRead {
			name = "revocation-during-read"
		}
		t.Run(name, func(t *testing.T) {
			initial := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}}
			revoked := &pms.PolicyStore{Services: []*pms.Service{{Name: "app"}}}
			s := &batchWatchStore{snapshot: initial, events: make(pms.StorageChangeChannel, 1024), started: make(chan struct{}), release: make(chan struct{})}
			if duringRead {
				s.blockRead = 3 // First two reads close the startup/subscribe gap.
			}
			engine, err := NewWithStore(&cfg.Config{EnableWatch: true}, s)
			if err != nil {
				t.Fatal(err)
			}
			e := engine.(*PolicyEvalImpl)
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(s.release) }); _ = e.Close(); close(s.events) })
			if duringRead {
				s.events <- pms.StoreChangeEvent{Type: pms.FULL_RELOAD, Content: initial}
				select {
				case <-s.started:
				case <-time.After(time.Second):
					t.Fatal("snapshot read did not start")
				}
			}
			s.mu.Lock()
			s.snapshot = revoked
			s.mu.Unlock()
			for i := 0; i < 1000; i++ {
				// Payloads are deliberately stale; every read must be authoritative.
				s.events <- pms.StoreChangeEvent{Type: pms.FULL_RELOAD, Content: initial}
			}
			release.Do(func() { close(s.release) })
			deadline := time.Now().Add(3 * time.Second)
			for {
				allowed, _, err := e.IsAllowed(featureContext())
				if !allowed && err == nil && len(s.events) == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("final revocation not loaded: %v %v", allowed, err)
				}
				time.Sleep(time.Millisecond)
			}
			s.mu.Lock()
			reads := s.reads
			s.mu.Unlock()
			t.Logf("1000 queued changes: %d complete reads including 2 startup reads", reads)
			if reads > 4 {
				t.Fatalf("1000 queued changes caused %d complete reads (including 2 startup reads)", reads)
			}
			if duringRead && reads < 4 {
				t.Fatal("event during snapshot read was lost")
			}
		})
	}
}
