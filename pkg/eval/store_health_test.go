// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

type availabilityStore struct {
	pms.PolicyStoreManagerADS
	mu                 sync.Mutex
	snapshot           *pms.PolicyStore
	healthErr, readErr error
}

func (s *availabilityStore) ReadPolicyStore() (*pms.PolicyStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot, s.readErr
}
func (s *availabilityStore) CheckHealth(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthErr
}
func (*availabilityStore) StopWatch() {}

func forceHealthProbe(e *PolicyEvalImpl) {
	e.healthMu.Lock()
	e.lastHealthCheck = time.Time{}
	e.healthMu.Unlock()
}

func requireUnavailable(t *testing.T, e *PolicyEvalImpl) {
	t.Helper()
	if allowed, reason, err := e.IsAllowed(featureContext()); allowed || reason != ads.ERROR_IN_EVALUATION || errors.Code(err) != errors.EvalCacheError {
		t.Fatalf("unavailable decision: %v %v %v", allowed, reason, err)
	}
	if roles, err := e.GetAllGrantedRoles(featureContext()); len(roles) != 0 || err == nil {
		t.Fatalf("unavailable roles: %v %v", roles, err)
	}
	if permissions, err := e.GetAllGrantedPermissions(featureContext()); len(permissions) != 0 || err == nil {
		t.Fatalf("unavailable permissions: %v %v", permissions, err)
	}
	if result, err := e.Diagnose(featureContext()); result.Allowed || len(result.GrantedRoles)+len(result.Policies)+len(result.RolePolicies) != 0 || err == nil {
		t.Fatalf("unavailable diagnostic: %+v %v", result, err)
	}
	if allowed, reason, err := e.Discover(featureContext()); allowed || reason != ads.ERROR_IN_EVALUATION || err == nil {
		t.Fatalf("unavailable discover: %v %v %v", allowed, reason, err)
	}
}

func TestFeatureStoreFailureGracePeriod(t *testing.T) {
	for _, grace := range []string{"0s", "100ms"} {
		t.Run(grace, func(t *testing.T) {
			initial := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}}
			s := &availabilityStore{snapshot: initial}
			engine, err := NewWithStore(&cfg.Config{StoreFailureGracePeriod: grace}, s)
			if err != nil {
				t.Fatal(err)
			}
			e := engine.(*PolicyEvalImpl)
			t.Cleanup(func() { _ = e.Close() })
			if allowed, _, err := e.IsAllowed(featureContext()); !allowed || err != nil {
				t.Fatalf("initial: %v %v", allowed, err)
			}
			s.healthErr = errors.New(errors.StoreError, "offline")
			s.readErr = s.healthErr
			forceHealthProbe(e)
			allowed, _, err := e.IsAllowed(featureContext())
			if grace == "0s" {
				if allowed || err == nil {
					t.Fatal("default did not fail closed")
				}
			} else if !allowed || err != nil {
				t.Fatal("complete snapshot unavailable within explicit grace")
			}
			e.cacheGate.Lock()
			first := e.storeFailureSince
			e.storeFailureSince = first.Add(-time.Second)
			expired := e.storeFailureSince
			e.cacheGate.Unlock()
			requireUnavailable(t, e)
			forceHealthProbe(e)
			e.ensureStoreHealth()
			if !e.storeFailureSince.Equal(expired) {
				t.Fatal("retries reset the grace deadline")
			}
			// Connectivity alone is insufficient: a complete read must succeed.
			s.healthErr = nil
			forceHealthProbe(e)
			e.ensureStoreHealth()
			requireUnavailable(t, e)
			if !e.storeFailureSince.Equal(expired) {
				t.Fatal("failed recovery read reset grace")
			}
			// Revocation during the outage must replace the old grant.
			s.snapshot = &pms.PolicyStore{Services: []*pms.Service{{Name: "app"}}}
			s.readErr = nil
			forceHealthProbe(e)
			if allowed, _, err := e.IsAllowed(featureContext()); allowed || err != nil {
				t.Fatalf("recovery ignored revocation: %v %v", allowed, err)
			}
			s.snapshot = initial
			if err := e.Refresh(); err != nil {
				t.Fatal(err)
			}
			if allowed, _, err := e.IsAllowed(featureContext()); !allowed || err != nil {
				t.Fatal("healthy full reload did not restore service")
			}
			apply(e, pms.StoreChangeEvent{Type: pms.POLICY_ADD, Content: "broken partial update"})
			requireUnavailable(t, e)
		})
	}
}

func TestFeatureInvalidStoreFailureGracePeriod(t *testing.T) {
	for _, value := range []string{"-1s", "forever", "1h2x", "999999999999999999999h"} {
		if e, err := NewWithStore(&cfg.Config{StoreFailureGracePeriod: value}, storetest.NewFailingStore()); e != nil || errors.Code(err) != errors.ConfigError {
			t.Fatalf("%q: %v %v", value, e, err)
		}
	}
}

func TestFeatureRecoveryIgnoresQueuedStaleGrant(t *testing.T) {
	old := &pms.Service{Name: "app", Policies: []*pms.Policy{recoveryGrant("old-grant")}}
	s := &availabilityStore{snapshot: &pms.PolicyStore{Services: []*pms.Service{old}}}
	engine, err := NewWithStore(&cfg.Config{}, s)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.(*PolicyEvalImpl)
	t.Cleanup(func() { _ = e.Close() })
	apply(e, pms.StoreChangeEvent{Type: pms.STORE_UNAVAILABLE})
	s.snapshot = &pms.PolicyStore{Services: []*pms.Service{{Name: "app"}}}
	changes := make(pms.StorageChangeChannel, 1)
	// The payload was created before revocation but delivered after reconnect.
	changes <- pms.StoreChangeEvent{Type: pms.SERVICE_ADD, Content: old}
	done := make(chan struct{})
	go func() { e.followStoreChanges(changes); close(done) }()
	t.Cleanup(func() { e.watchStopped.Store(true); close(changes); <-done })
	deadline := time.Now().Add(time.Second)
	for {
		allowed, _, err := e.IsAllowed(featureContext())
		if err == nil {
			if allowed {
				t.Fatal("queued pre-outage grant overrode the recovery snapshot")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovery event was not processed")
		}
		time.Sleep(time.Millisecond)
	}
}

type restartingWatchStore struct {
	*availabilityStore
	mu       sync.Mutex
	next     pms.StorageChangeChannel
	watchErr error
	attempts int
}

func (s *restartingWatchStore) Watch() (pms.StorageChangeChannel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	return s.next, s.watchErr
}

func TestFeatureClosedWatchRequiresResubscription(t *testing.T) {
	initial := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}}
	first, second := make(pms.StorageChangeChannel), make(pms.StorageChangeChannel)
	s := &restartingWatchStore{availabilityStore: &availabilityStore{snapshot: initial}, next: first}
	engine, err := NewWithStore(&cfg.Config{EnableWatch: true}, s)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.(*PolicyEvalImpl)
	t.Cleanup(func() { _ = e.Close(); close(second) })
	s.mu.Lock()
	s.next = second
	s.mu.Unlock()
	s.availabilityStore.mu.Lock()
	s.snapshot = &pms.PolicyStore{Services: []*pms.Service{{Name: "app"}}}
	s.availabilityStore.mu.Unlock()
	close(first)
	deadline := time.Now().Add(3 * time.Second)
	for {
		allowed, _, err := e.IsAllowed(featureContext())
		if !e.watchNeedsRestart.Load() && !allowed && err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watch did not resubscribe with a fresh snapshot: %v %v", allowed, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFeatureReadableStoreCannotClearBrokenWatch(t *testing.T) {
	for _, grace := range []string{"0s", "100ms"} {
		t.Run(grace, func(t *testing.T) {
			initial := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}}
			first, second := make(pms.StorageChangeChannel, 1), make(pms.StorageChangeChannel)
			s := &restartingWatchStore{availabilityStore: &availabilityStore{snapshot: initial}, next: first}
			engine, err := NewWithStore(&cfg.Config{EnableWatch: true, StoreFailureGracePeriod: grace}, s)
			if err != nil {
				t.Fatal(err)
			}
			e := engine.(*PolicyEvalImpl)
			t.Cleanup(func() { _ = e.Close(); close(first); close(second) })
			s.mu.Lock()
			s.next = nil
			s.watchErr = errors.New(errors.StoreError, "subscription unavailable")
			s.mu.Unlock()
			// The event channel stays open, as it does during a MongoDB stream retry.
			first <- pms.StoreChangeEvent{Type: pms.STORE_UNAVAILABLE}
			deadline := time.Now().Add(time.Second)
			for {
				e.cacheGate.RLock()
				failed := !e.storeFailureSince.IsZero()
				e.cacheGate.RUnlock()
				if failed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("watch failure not recorded")
				}
				time.Sleep(time.Millisecond)
			}
			e.cacheGate.Lock()
			e.storeFailureSince = e.storeFailureSince.Add(-time.Second)
			firstFailure := e.storeFailureSince
			e.cacheGate.Unlock()
			forceHealthProbe(e)
			requireUnavailable(t, e)
			// Even repeated successful data reads must not extend grace or validate a
			// cache that has no working subscription.
			if err := e.Refresh(); err != nil {
				t.Fatal(err)
			}
			requireUnavailable(t, e)
			e.cacheGate.RLock()
			sameDeadline := e.storeFailureSince.Equal(firstFailure)
			e.cacheGate.RUnlock()
			if !sameDeadline {
				t.Fatal("successful data read reset a broken watch's grace deadline")
			}
			deadline = time.Now().Add(3 * time.Second)
			for {
				s.mu.Lock()
				attempts := s.attempts
				s.mu.Unlock()
				if attempts >= 2 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("failed subscription was not retried")
				}
				time.Sleep(time.Millisecond)
			}
			requireUnavailable(t, e)
			s.availabilityStore.mu.Lock()
			s.snapshot = &pms.PolicyStore{Services: []*pms.Service{{Name: "app"}}}
			s.availabilityStore.mu.Unlock()
			s.mu.Lock()
			s.next = second
			s.watchErr = nil
			s.mu.Unlock()
			deadline = time.Now().Add(3 * time.Second)
			for {
				allowed, _, err := e.IsAllowed(featureContext())
				if err == nil {
					if allowed {
						t.Fatal("revoked grant restored")
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("working subscription and snapshot did not recover")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

// Runs against the selected real backend in both root and feature-stress stages.
func TestFeatureStoreOutageRecovery(t *testing.T) {
	for _, watching := range []bool{false, true} {
		t.Run(fmt.Sprintf("watch=%v", watching), func(t *testing.T) {
			grant := recoveryGrant("grant")
			grant.Principals = [][]string{{"role:reader"}}
			grant.Condition = "level >= 3 && (region == 'eu' || emergency == true)"
			initial := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{grant}, RolePolicies: []*pms.RolePolicy{{ID: "role", Effect: pms.Grant, Principals: []string{"user:alice"}, Roles: []string{"reader"}}}}}}
			fixture, err := storetest.NewFixture(initial)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(fixture.Close)
			configuration, setOnline := storetest.OutageConfiguration(t, fixture.Config)
			reader, err := store.NewStore(configuration.StoreType, configuration.StoreProps)
			if err != nil {
				t.Fatal(err)
			}
			engine, err := NewWithStore(&cfg.Config{EnableWatch: watching}, reader)
			if err != nil {
				t.Fatal(err)
			}
			e := engine.(*PolicyEvalImpl)
			t.Cleanup(func() {
				_ = e.Close()
				if closer, ok := reader.(io.Closer); ok {
					_ = closer.Close()
				}
			})
			ctx := featureContext()
			ctx.Attributes = map[string]interface{}{"level": 3.0, "region": "eu", "emergency": false}
			if allowed, _, err := e.IsAllowed(ctx); !allowed || err != nil {
				t.Fatalf("initial grant: %v %v", allowed, err)
			}
			setOnline(false)
			deadline := time.Now().Add(20 * time.Second)
			for {
				allowed, reason, err := e.IsAllowed(ctx)
				if !allowed && reason == ads.ERROR_IN_EVALUATION && err != nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("store outage not detected: %v %v", allowed, err)
				}
				time.Sleep(20 * time.Millisecond)
			}
			// More than the old file watcher's fatal timeout; retries must not
			// exit ADS or make stale grants valid again.
			until := time.Now().Add(2200 * time.Millisecond)
			for time.Now().Before(until) {
				if allowed, _, err := e.IsAllowed(ctx); allowed || err == nil {
					t.Fatal("stale grant survived outage")
				}
				time.Sleep(50 * time.Millisecond)
			}
			revoked := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: initial.Services[0].Policies, RolePolicies: []*pms.RolePolicy{}}}}
			if fixture.Config.StoreType == "file" {
				raw, err := json.Marshal(revoked)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fixture.Config.StoreProps["FileLocation"].(string)+".offline", raw, 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := fixture.Store.WritePolicyStore(revoked); err != nil {
				t.Fatal(err)
			}
			setOnline(true)
			deadline = time.Now().Add(20 * time.Second)
			for {
				allowed, _, err := e.IsAllowed(ctx)
				if err == nil {
					if allowed {
						t.Fatal("recovery restored a revoked role")
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("store did not recover: %v", err)
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err := fixture.Store.WritePolicyStore(initial); err != nil {
				t.Fatal(err)
			}
			if !watching {
				if err := e.Refresh(); err != nil {
					t.Fatal(err)
				}
			}
			deadline = time.Now().Add(10 * time.Second)
			for {
				allowed, _, err := e.IsAllowed(ctx)
				if allowed && err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("healthy follow-up grant: %v %v", allowed, err)
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Logf("real backend %s: outage denied, process survived, revoked role absent after recovery, healthy grant restored", fixture.Config.StoreType)
		})
	}
}
