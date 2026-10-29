package mongodb

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"go.mongodb.org/mongo-driver/bson"
)

func isolatedStore(t *testing.T) *Store {
	t.Helper()
	if !mongoAvailable {
		t.Skip("MongoDB not available")
	}
	manager, err := (MongoStoreBuilder{}).NewStore(storeConfig.StoreProps)
	if err != nil {
		t.Fatal(err)
	}
	s := manager.(*Store)
	s.Database = fmt.Sprintf("speedle_regression_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		s.StopWatch()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.client.Database(s.Database).Drop(ctx); err != nil {
			t.Error(err)
		}
		if err := s.client.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	})
	return s
}

func supportsTransactions(t *testing.T, s *Store) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var hello struct {
		SetName string `bson:"setName"`
		Message string `bson:"msg"`
	}
	if err := s.client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		t.Fatal(err)
	}
	return hello.SetName != "" || hello.Message == "isdbgrid"
}

func TestSnapshotReplacementRollback(t *testing.T) {
	s := isolatedStore(t)
	if err := s.CreateService(&pms.Service{Name: "original"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFunction(&pms.Function{Name: "original", FuncURL: "http://original"}); err != nil {
		t.Fatal(err)
	}
	before, err := s.ReadPolicyStore()
	if err != nil {
		t.Fatal(err)
	}
	// Duplicate function keys fail after the transaction has deleted the old
	// snapshot and inserted the replacement services.
	invalid := &pms.PolicyStore{
		Services: []*pms.Service{{Name: "replacement"}},
		Functions: []*pms.Function{
			{Name: "duplicate", FuncURL: "http://one"},
			{Name: "duplicate", FuncURL: "http://two"},
		},
	}
	if err := s.WritePolicyStore(invalid); err == nil {
		t.Fatal("expected replacement to fail")
	}
	after, err := s.ReadPolicyStore()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("failed replacement changed snapshot: before=%+v after=%+v", before, after)
	}
	if !supportsTransactions(t, s) {
		// A standalone MongoDB must reject even valid replacements safely.
		if err := s.WritePolicyStore(&pms.PolicyStore{}); err == nil {
			t.Fatal("standalone MongoDB unexpectedly accepted a transaction")
		}
		if _, err := s.GetService("original"); err != nil {
			t.Fatal("unsupported transaction removed original service:", err)
		}
		return
	}
	valid := &pms.PolicyStore{Services: []*pms.Service{{Name: "replacement", Policies: []*pms.Policy{{ID: "stable-id", Name: "grant"}}}}}
	if err := s.WritePolicyStore(valid); err != nil {
		t.Fatal(err)
	}
	service, err := s.GetService("replacement")
	if err != nil || len(service.Policies) != 1 || service.Policies[0].ID != "stable-id" {
		t.Fatalf("snapshot IDs were not preserved: service=%+v err=%v", service, err)
	}
	if err := s.WritePolicyStore(&pms.PolicyStore{}); err != nil {
		t.Fatal(err)
	}
	names, err := s.GetServiceNames()
	if err != nil || len(names) != 0 {
		t.Fatalf("empty replacement: names=%v err=%v", names, err)
	}
}

func TestBulkDeleteWatchAndStop(t *testing.T) {
	s := isolatedStore(t)
	if !supportsTransactions(t, s) {
		t.Skip("change streams require a replica set or sharded cluster")
	}
	if err := s.CreateService(&pms.Service{Name: "watched"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFunction(&pms.Function{Name: "watched", FuncURL: "http://function"}); err != nil {
		t.Fatal(err)
	}
	names, err := s.GetServiceNames()
	if err != nil || !reflect.DeepEqual(names, []string{"watched"}) {
		t.Fatalf("service names=%v err=%v", names, err)
	}
	changes, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteServices(); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFunctions(); err != nil {
		t.Fatal(err)
	}
	seen := map[pms.EventType]bool{}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for len(seen) < 2 {
		select {
		case event, ok := <-changes:
			if !ok {
				t.Fatal("watch closed before deletion events")
			}
			if event.Type != pms.SERVICE_DELETE && event.Type != pms.FUNCTION_DELETE {
				t.Fatalf("unexpected event: %+v", event)
			}
			if !reflect.DeepEqual(event.Content, []string{"watched"}) {
				t.Fatalf("unexpected deletion content: %+v", event)
			}
			seen[event.Type] = true
		case <-timer.C:
			t.Fatal("bulk deletion did not reach watcher")
		}
	}
	stop := func() {
		t.Helper()
		done := make(chan struct{})
		go func() { s.StopWatch(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("StopWatch blocked")
		}
		if _, ok := <-changes; ok {
			t.Fatal("watch channel remained open after StopWatch")
		}
	}
	stop()        // idle cursor
	s.StopWatch() // idempotent
	changes, err = s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateService(&pms.Service{Name: "unconsumed"}); err != nil {
		t.Fatal(err)
	}
	// Do not receive from changes: stopping must cancel both cursor reads and sends.
	stop()
}

func TestChangeEventReload(t *testing.T) {
	for _, operation := range []string{"drop", "rename", "dropDatabase", "invalidate", "update"} {
		t.Run(operation, func(t *testing.T) {
			event := mongoChangeEvent{Operation: operation}
			event.Namespace.Collection = "services"
			events, err := event.storeEvents()
			if err != nil || len(events) != 1 || events[0].Type != pms.FULL_RELOAD {
				t.Fatalf("events=%+v err=%v", events, err)
			}
		})
	}
}

func TestSnapshotRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	for _, snapshot := range []*pms.PolicyStore{
		nil,
		{Services: []*pms.Service{nil}},
		{Services: []*pms.Service{{Name: "invalid", Policies: []*pms.Policy{nil}}}},
		{Services: []*pms.Service{{Name: "invalid", RolePolicies: []*pms.RolePolicy{nil}}}},
		{Functions: []*pms.Function{nil}},
		{Functions: []*pms.Function{{Name: "missing-url"}}},
	} {
		// No client: invalid input must return before opening a session.
		if err := (&Store{}).WritePolicyStore(snapshot); err == nil {
			t.Fatalf("accepted invalid snapshot: %+v", snapshot)
		}
	}
}

func TestChangeEventDocuments(t *testing.T) {
	for _, collection := range []string{"services", "functions"} {
		for _, operation := range []string{"insert", "update", "replace", "delete"} {
			t.Run(collection+"/"+operation, func(t *testing.T) {
				event := mongoChangeEvent{Operation: operation}
				event.Namespace.Collection = collection
				event.DocumentKey.ID = "example"
				var document interface{} = &pms.Service{Name: "example"}
				add, remove := pms.SERVICE_ADD, pms.SERVICE_DELETE
				if collection == "functions" {
					document = &pms.Function{Name: "example", FuncURL: "http://function"}
					add, remove = pms.FUNCTION_ADD, pms.FUNCTION_DELETE
				}
				data, err := bson.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
				event.FullDocument = bson.Raw(data)
				events, err := event.storeEvents()
				if err != nil {
					t.Fatal(err)
				}
				if operation == "delete" {
					if len(events) != 1 || events[0].Type != remove || !reflect.DeepEqual(events[0].Content, []string{"example"}) {
						t.Fatalf("unexpected deletion events: %+v", events)
					}
					return
				}
				expected := 1
				if collection == "services" && operation != "insert" {
					expected = 2
				}
				if len(events) != expected || events[len(events)-1].Type != add || !reflect.DeepEqual(events[len(events)-1].Content, document) {
					t.Fatalf("unexpected document events: %+v", events)
				}
				if expected == 2 && events[0].Type != remove {
					t.Fatalf("service update did not remove old cache entry: %+v", events)
				}
			})
		}
	}
}

func TestNameFilterValuesAreLiterals(t *testing.T) {
	s := isolatedStore(t)
	if err := s.CreateService(&pms.Service{Name: "app", Policies: []*pms.Policy{{Name: "$special"}, {Name: "ordinary"}}}); err != nil {
		t.Fatal(err)
	}
	policies, err := s.ListAllPolicies("app", "name eq $special")
	if err != nil || len(policies) != 1 || policies[0].Name != "$special" {
		t.Fatalf("literal policy filter: %+v, %v", policies, err)
	}
	if _, err := s.CreateFunction(&pms.Function{Name: "$function", FuncURL: "http://function"}); err != nil {
		t.Fatal(err)
	}
	functions, err := s.ListAllFunctions("name eq $function")
	if err != nil || len(functions) != 1 || functions[0].Name != "$function" {
		t.Fatalf("literal function filter: %+v, %v", functions, err)
	}
}
