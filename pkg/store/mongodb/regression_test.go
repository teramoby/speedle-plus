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

func TestWatchReopensAndReloadsAfterTheStreamEnds(t *testing.T) {
	s := isolatedStore(t)
	if !supportsTransactions(t, s) {
		t.Skip("change streams require a replica set or sharded cluster")
	}
	defer func(previous time.Duration) { watchRetryMin = previous }(watchRetryMin)
	watchRetryMin = 2 * time.Second
	if err := s.CreateService(&pms.Service{Name: "before"}); err != nil {
		t.Fatal(err)
	}
	changes, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	receive := func(within time.Duration) (pms.StoreChangeEvent, bool) {
		select {
		case event, ok := <-changes:
			if !ok {
				t.Fatal("watch closed after the change stream ended")
			}
			return event, true
		case <-time.After(within):
			return pms.StoreChangeEvent{}, false
		}
	}
	// Dropping the database invalidates the stream, as a terminal error would end it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.client.Database(s.Database).Drop(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := receive(10 * time.Second); !ok {
		t.Fatal("no event for the dropped database")
	}
	for {
		if _, ok := receive(500 * time.Millisecond); !ok {
			break
		}
	}
	// No stream is open now: this change reaches ADS only through the reload
	// that follows the reopened stream.
	if err := s.CreateService(&pms.Service{Name: "between"}); err != nil {
		t.Fatal(err)
	}
	if event, ok := receive(10 * time.Second); !ok || event.Type != pms.FULL_RELOAD {
		t.Fatalf("event after reopening: %+v (received %v), want FULL_RELOAD", event, ok)
	}
	if err := s.CreateService(&pms.Service{Name: "after"}); err != nil {
		t.Fatal(err)
	}
	if event, ok := receive(10 * time.Second); !ok || event.Type != pms.SERVICE_ADD || event.Content.(*pms.Service).Name != "after" {
		t.Fatalf("event on the reopened stream: %+v (received %v), want SERVICE_ADD after", event, ok)
	}
}

func TestReadPolicyStoreSeesOneGeneration(t *testing.T) {
	s := isolatedStore(t)
	if !supportsTransactions(t, s) {
		t.Skip("snapshot reads require a replica set or sharded cluster")
	}
	generation := func(name string) *pms.PolicyStore {
		return &pms.PolicyStore{
			Services:  []*pms.Service{{Name: name}},
			Functions: []*pms.Function{{Name: name, FuncURL: "http://" + name}},
		}
	}
	if err := s.WritePolicyStore(generation("a")); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	written := make(chan error, 1)
	go func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				written <- nil
				return
			default:
			}
			if err := s.WritePolicyStore(generation([]string{"a", "b"}[i%2])); err != nil {
				written <- err
				return
			}
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ps, err := s.ReadPolicyStore()
		if err != nil {
			t.Fatal(err)
		}
		if len(ps.Services) != 1 || len(ps.Functions) != 1 || ps.Services[0].Name != ps.Functions[0].Name {
			names := []string{}
			for _, service := range ps.Services {
				names = append(names, "service "+service.Name)
			}
			for _, function := range ps.Functions {
				names = append(names, "function "+function.Name)
			}
			close(stop)
			<-written
			t.Fatalf("read a store that was never written: %v", names)
		}
	}
	close(stop)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotReplacementIsOneReload(t *testing.T) {
	s := isolatedStore(t)
	if !supportsTransactions(t, s) {
		t.Skip("change streams require a replica set or sharded cluster")
	}
	snapshot := func(names ...string) *pms.PolicyStore {
		ps := &pms.PolicyStore{}
		for _, name := range names {
			ps.Services = append(ps.Services, &pms.Service{Name: name})
		}
		return ps
	}
	if err := s.WritePolicyStore(snapshot("a", "b")); err != nil {
		t.Fatal(err)
	}
	changes, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	next := func() pms.StoreChangeEvent {
		t.Helper()
		select {
		case event, ok := <-changes:
			if !ok {
				t.Fatal("watch closed")
			}
			return event
		case <-time.After(10 * time.Second):
			t.Fatal("no change event")
		}
		return pms.StoreChangeEvent{}
	}
	// The transaction deletes a and b and inserts a, b and c. A watcher must not
	// see those one by one, with the services missing in between.
	if err := s.WritePolicyStore(snapshot("a", "b", "c")); err != nil {
		t.Fatal(err)
	}
	if event := next(); event.Type != pms.FULL_RELOAD {
		t.Fatalf("first event after snapshot replacement: %+v, want FULL_RELOAD", event)
	}
	// The next event is the next write, not the rest of the transaction.
	if err := s.CreateService(&pms.Service{Name: "d"}); err != nil {
		t.Fatal(err)
	}
	if event := next(); event.Type != pms.SERVICE_ADD || event.Content.(*pms.Service).Name != "d" {
		t.Fatalf("event after the reload: %+v, want SERVICE_ADD d", event)
	}
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
