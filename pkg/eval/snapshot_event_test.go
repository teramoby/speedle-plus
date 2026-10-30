package eval

import (
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/suid"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

type closableStore struct {
	*storetest.FailingStore
	closes, stops int
}

func (s *closableStore) Close() error { s.closes++; return nil }
func (s *closableStore) StopWatch()   { s.stops++ }

type closeBuilder struct{ s *closableStore }

func (b closeBuilder) NewStore(map[string]interface{}) (pms.PolicyStoreManager, error) {
	return b.s, nil
}
func (b closeBuilder) GetStoreParams() map[string]string { return nil }

func TestSnapshotEventDoesNotReadDatabaseAgain(t *testing.T) {
	s := &storetest.FailingStore{Err: errors.New(errors.StoreError, "database is now offline")}
	p := &PolicyEvalImpl{Store: s, RuntimePolicyStore: NewRuntimePolicyStore()}
	events := make(pms.StorageChangeChannel, 1)
	events <- pms.StoreChangeEvent{Type: pms.FULL_RELOAD, Content: &pms.PolicyStore{Services: []*pms.Service{{Name: "from-snapshot"}}}}
	close(events)
	p.updateRuntimeCacheWithStoreChange(events)
	if p.RuntimePolicyStore.RuntimeServices["from-snapshot"] == nil {
		t.Fatal("snapshot event tried to read the offline database")
	}
	// A legacy event still attempts a DB read; a failure preserves the cache.
	legacy := make(pms.StorageChangeChannel, 1)
	legacy <- pms.StoreChangeEvent{Type: pms.FULL_RELOAD}
	close(legacy)
	p.updateRuntimeCacheWithStoreChange(legacy)
	if p.RuntimePolicyStore.RuntimeServices["from-snapshot"] == nil {
		t.Fatal("failed legacy reload cleared cache")
	}
}

func TestEvaluatorCloseAndFactoryFailure(t *testing.T) {
	s := &closableStore{FailingStore: &storetest.FailingStore{Err: errors.New(errors.StoreError, "unavailable")}}
	p := &PolicyEvalImpl{Store: s}
	p.cleanExpiredFunctionResultPeriodically()
	p.StopWatch()
	p.StopWatch()
	// Close twice: the second call must succeed without closing the store again.
	first, second := p.Close(), p.Close()
	if first != nil || second != nil || s.closes != 1 || s.stops != 1 {
		t.Fatalf("close=%d stop=%d", s.closes, s.stops)
	}
	name := "close-on-evaluator-failure-" + suid.New().String()
	store.Register(name, closeBuilder{s})
	if _, err := NewFromConfig(&cfg.Config{StoreConfig: &cfg.StoreConfig{StoreType: name}}); err == nil {
		t.Fatal("expected load failure")
	}
	if s.closes != 2 {
		t.Fatal("factory did not close newly created store")
	}
}
