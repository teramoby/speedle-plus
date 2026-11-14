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
	// Aggregation works on standalone MongoDB too; seed without a transaction.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.client.Database(s.Database).Collection("services").InsertOne(ctx, legacySnapshot(t).Services[0]); err != nil {
		t.Fatal(err)
	}
	type command struct {
		maxTime       int64
		maxTimeFields int
		parseErr      error
		deadline      time.Time
		hasDeadline   bool
	}
	var mu sync.Mutex
	var commands []command
	monitor := &event.CommandMonitor{Started: func(ctx context.Context, e *event.CommandStartedEvent) {
		if e.CommandName != "aggregate" {
			return
		}
		deadline, ok := ctx.Deadline()
		maxTime, _ := e.Command.Lookup("maxTimeMS").AsInt64OK()
		elements, parseErr := e.Command.Elements()
		fields := 0
		for _, element := range elements {
			if element.Key() == "maxTimeMS" {
				fields++
			}
		}
		mu.Lock()
		commands = append(commands, command{maxTime: maxTime, maxTimeFields: fields, parseErr: parseErr, deadline: deadline, hasDeadline: ok})
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
			if len(observed) == 0 {
				t.Fatal("no aggregate command observed")
			}
			// A retry must preserve the same command and original deadline.
			for _, got := range observed {
				if got.parseErr != nil || got.maxTimeFields != 1 {
					t.Fatalf("maxTimeMS fields=%d, parse error=%v; want exactly one", got.maxTimeFields, got.parseErr)
				}
				if got.maxTime != test.maxTime {
					t.Fatalf("server maxTimeMS=%d, want %d", got.maxTime, test.maxTime)
				}
				budget := got.deadline.Sub(started)
				if !got.hasDeadline || budget < 4500*time.Millisecond || budget > 5500*time.Millisecond {
					t.Fatalf("client budget=%s, want 5s", budget)
				}
			}
		})
	}
}
