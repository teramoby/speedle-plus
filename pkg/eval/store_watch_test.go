// Licensed under the Universal Permissive License (UPL) Version 1.0.

package eval

import (
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// watchRequiringStore stands in for a store that publishes through events
// only.
type watchRequiringStore struct {
	pms.PolicyStoreManagerADS
	required bool
	events   pms.StorageChangeChannel
	watched  bool
	stopped  bool
}

func (s *watchRequiringStore) RequireWatch() bool { return s.required }

func (s *watchRequiringStore) ReadPolicyStore() (*pms.PolicyStore, error) {
	return &pms.PolicyStore{}, nil
}

func (s *watchRequiringStore) Watch() (pms.StorageChangeChannel, error) {
	s.watched = true
	return s.events, nil
}

func (s *watchRequiringStore) StopWatch() { s.stopped = true }

func TestWatchRequirementForcesWatching(t *testing.T) {
	store := &watchRequiringStore{required: true, events: make(pms.StorageChangeChannel)}
	engine, err := NewWithStore(&cfg.Config{EnableWatch: false}, store)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := engine.(*PolicyEvalImpl)
	if !store.watched || !evaluator.watchManaged {
		t.Fatal("a store that requires watching was not watched")
	}
	if err := evaluator.Close(); err != nil {
		t.Fatal(err)
	}
	close(store.events)
	if !store.stopped {
		t.Fatal("the store watch was not stopped")
	}
}

func TestWatchRequiringStoreRejectsAGracePeriod(t *testing.T) {
	store := &watchRequiringStore{required: true, events: make(pms.StorageChangeChannel)}
	if _, err := NewWithStore(&cfg.Config{StoreFailureGracePeriod: "1s"}, store); errors.Code(err) != errors.ConfigError {
		t.Fatalf("error %v, code %s, want %s", err, errors.Code(err), errors.ConfigError)
	}
}

func TestStoreWithoutWatchRequirementKeepsConfiguration(t *testing.T) {
	store := &watchRequiringStore{events: make(pms.StorageChangeChannel)}
	engine, err := NewWithStore(&cfg.Config{EnableWatch: false, StoreFailureGracePeriod: "1s"}, store)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := engine.(*PolicyEvalImpl)
	t.Cleanup(func() { _ = evaluator.Close() })
	if store.watched || evaluator.watchManaged {
		t.Fatal("watching was forced on a store that does not require it")
	}
}
