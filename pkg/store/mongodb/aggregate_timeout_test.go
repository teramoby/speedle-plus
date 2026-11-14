package mongodb

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestAggregationPreservesServerAndClientBudgets(t *testing.T) {
	s := isolatedStore(t)
	if err := s.WritePolicyStore(legacySnapshot(t)); err != nil {
		t.Fatal(err)
	}
	type command struct {
		maxTime     int64
		deadline    time.Time
		hasDeadline bool
	}
	var mu sync.Mutex
	var commands []command
	monitor := &event.CommandMonitor{Started: func(ctx context.Context, e *event.CommandStartedEvent) {
		if e.CommandName != "aggregate" {
			return
		}
		deadline, ok := ctx.Deadline()
		maxTime, _ := e.Command.Lookup("maxTimeMS").AsInt64OK()
		mu.Lock()
		commands = append(commands, command{maxTime, deadline, ok})
		mu.Unlock()
	}}
	client, err := mongo.Connect(options.Client().ApplyURI(storeConfig.StoreProps[MongoURIKey].(string)).SetMonitor(monitor))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.client.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.client = client // isolatedStore's cleanup now owns the monitored replacement.
	for _, test := range []struct {
		name    string
		maxTime int64
		run     func() error
	}{
		{"service names", 2000, func() error { _, err := s.GetServiceNames(); return err }},
		{"policy list", 2000, func() error { _, err := s.ListAllPolicies("books", ""); return err }},
		{"role policy list", 2000, func() error { _, err := s.ListAllRolePolicies("books", ""); return err }},
		{"combined counts", 5000, func() error { _, err := s.GetPolicyAndRolePolicyCounts(); return err }},
		{"policy count", 5000, func() error { _, err := s.GetPolicyCount("books"); return err }},
		{"role policy count", 5000, func() error { _, err := s.GetRolePolicyCount("books"); return err }},
		{"policy", 5000, func() error { _, err := s.GetPolicy("books", "policy-id"); return err }},
		{"role policy", 5000, func() error { _, err := s.GetRolePolicy("books", "role-id"); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			mu.Lock()
			commands = nil
			mu.Unlock()
			started := time.Now()
			if err := test.run(); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			observed := append([]command(nil), commands...)
			mu.Unlock()
			if len(observed) != 1 {
				t.Fatalf("got %d aggregate commands, want 1", len(observed))
			}
			got := observed[0]
			if got.maxTime != test.maxTime {
				t.Fatalf("server maxTimeMS=%d, want %d", got.maxTime, test.maxTime)
			}
			budget := got.deadline.Sub(started)
			if !got.hasDeadline || budget < 4500*time.Millisecond || budget > 5500*time.Millisecond {
				t.Fatalf("client budget=%s, want 5s", budget)
			}
		})
	}
}
