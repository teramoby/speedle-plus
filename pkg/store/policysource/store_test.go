// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	speedleerrors "github.com/teramoby/speedle-plus/pkg/errors"
)

func mustRead(t *testing.T, store *Store) *pms.PolicyStore {
	t.Helper()
	policyStore, err := store.ReadPolicyStore()
	if err != nil {
		t.Fatal(err)
	}
	return policyStore
}

func serviceNamed(t *testing.T, policyStore *pms.PolicyStore, name string) *pms.Service {
	t.Helper()
	for _, service := range policyStore.Services {
		if service.Name == name {
			return service
		}
	}
	t.Fatalf("no service %q in %+v", name, policyStore.Services)
	return nil
}

func waitForReload(t *testing.T, events pms.StorageChangeChannel) {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("change channel was closed")
		}
		if event.Type != pms.FULL_RELOAD {
			t.Fatalf("event type %v, want FULL_RELOAD", event.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no FULL_RELOAD event")
	}
}

func expectNoEvent(t *testing.T, events pms.StorageChangeChannel) {
	t.Helper()
	select {
	case event, ok := <-events:
		if ok {
			t.Fatalf("unexpected event %v", event.Type)
		}
		t.Fatal("change channel was closed")
	case <-time.After(100 * time.Millisecond):
	}
}

func booksService() *testService {
	service := newTestService(testServiceID1, "books")
	service.publish(func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-read-books", "read-books", "grant user alice read books"),
			policy("p-write-books", "write-books", `grant group editors write "book:1"`),
		}
		snapshot.RolePolicies = []adsync.Policy{
			policy("r-editor", "editor", "grant user bill role editor"),
		}
	})
	return service
}

func ordersService() *testService {
	service := newTestService(testServiceID2, "orders")
	service.publish(func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-read-orders", "read-orders", "grant user alice read orders if level >= 3"),
		}
	})
	return service
}

// TestOpenSyncWithoutRuntimeConfirmsLiveness covers the low-level Open path: a
// store that does not own the notification runtime has no channel to produce a
// confirmed ack, so its caller-driven Sync passes are the confirmation.
// Repeated passes must keep the instance live and its services in sync without
// another heartbeat source.
func TestOpenSyncWithoutRuntimeConfirmsLiveness(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store, err := Open(nil, fixture.options)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for pass := 0; pass < 2; pass++ {
		if err := store.Sync(context.Background()); err != nil {
			t.Fatalf("sync pass %d: %v", pass, err)
		}
		if err := store.Confirm(); err != nil {
			t.Fatalf("confirm pass %d: %v", pass, err)
		}
		_, acknowledge, err := store.ReadPolicyStoreForReload()
		if err != nil {
			t.Fatalf("read published assembly: %v", err)
		}
		acknowledge()
	}
	status, known := store.Status("books")
	if !known {
		t.Fatal("books is not tracked")
	}
	if status.Service.State != adsync.SyncStateInSync {
		t.Fatalf("books state after caller-driven sync = %q", status.Service.State)
	}
	if status.Instance.Liveness != adsync.LivenessLive {
		t.Fatalf("instance liveness = %q, want live", status.Instance.Liveness)
	}
	if status.Instance.LastConfirmedAckAt.IsZero() {
		t.Fatal("caller-driven sync did not confirm liveness")
	}
}

// TestRenameGuardSurvivesAReplacementFetch covers the window between a
// successful replacement fetch and the evaluator installing the renamed
// assembly: the previous service name must stay unavailable until the
// installation is acknowledged.
func TestRenameGuardSurvivesAReplacementFetch(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store, err := Open(nil, fixture.options)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	confirmStoreGeneration(t, store)

	// The channel reports an authoritative rename and the next pass fetches the
	// replacement copy from the source.
	if !store.renameCopies([]adsync.Service{{ID: testServiceID1, Name: "books-v2"}}) {
		t.Fatal("the rename was not applied to the installed copy")
	}
	store.publish()
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Name = "books-v2"
	})
	if err := store.Sync(nil); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// The evaluator has not installed the renamed assembly yet, so its
	// previously installed name must not route locally.
	if store.RemovalInstalled("books") {
		t.Fatal("the previous name became routable before the renamed assembly was installed")
	}
	_, acknowledge, err := store.ReadPolicyStoreForReload()
	if err != nil {
		t.Fatalf("read published assembly: %v", err)
	}
	acknowledge()
	if !store.RemovalInstalled("books") {
		t.Fatal("the previous name stayed guarded after the renamed assembly was installed")
	}
}

func TestOpenAssemblesServicesAndParsesSPDL(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store := fixture.open(nil)

	if store.Type() != StoreType {
		t.Fatalf("Type() = %q", store.Type())
	}
	if !store.RequireWatch() {
		t.Fatal("the policy-source store must require watching")
	}
	policyStore := mustRead(t, store)
	if len(policyStore.Services) != 2 {
		t.Fatalf("%d services, want 2", len(policyStore.Services))
	}
	if policyStore.Services[0].Name != "books" || policyStore.Services[1].Name != "orders" {
		t.Fatalf("services are not sorted: %q %q", policyStore.Services[0].Name, policyStore.Services[1].Name)
	}

	books := serviceNamed(t, policyStore, "books")
	if len(books.Policies) != 2 || len(books.RolePolicies) != 1 {
		t.Fatalf("books has %d policies and %d role policies", len(books.Policies), len(books.RolePolicies))
	}
	readBooks := books.Policies[0]
	if readBooks.ID != "p-read-books" || readBooks.Name != "read-books" || readBooks.Effect != pms.Grant {
		t.Fatalf("parsed policy %+v", readBooks)
	}
	if len(readBooks.Principals) != 1 || readBooks.Principals[0][0] != "user:alice" {
		t.Fatalf("principals %+v", readBooks.Principals)
	}
	if len(readBooks.Permissions) != 1 || readBooks.Permissions[0].Resource != "books" ||
		len(readBooks.Permissions[0].Actions) != 1 || readBooks.Permissions[0].Actions[0] != "read" {
		t.Fatalf("permissions %+v", readBooks.Permissions)
	}
	writeBooks := books.Policies[1]
	if writeBooks.Permissions[0].Resource != "book:1" || writeBooks.Principals[0][0] != "group:editors" {
		t.Fatalf("quoted resource policy %+v", writeBooks)
	}
	editor := books.RolePolicies[0]
	if editor.ID != "r-editor" || len(editor.Roles) != 1 || editor.Roles[0] != "editor" ||
		len(editor.Principals) != 1 || editor.Principals[0] != "user:bill" {
		t.Fatalf("role policy %+v", editor)
	}

	orders := serviceNamed(t, policyStore, "orders")
	if orders.Policies[0].Condition != "level >= 3" {
		t.Fatalf("condition %q", orders.Policies[0].Condition)
	}

	// The adsync machine confirms every selected service as in sync.
	status, known := store.Status("books")
	if !known || status.Service.State != adsync.SyncStateInSync ||
		status.Service.Revision != 2 || status.Service.Head != 2 ||
		status.Instance.Liveness != adsync.LivenessLive {
		t.Fatalf("books status %+v", status)
	}
	if err := store.CheckHealth(nil); err != nil {
		t.Fatalf("healthy store: %v", err)
	}
}

func TestSPDLEdgeCases(t *testing.T) {
	service := newTestService(testServiceID1, "books")
	service.publish(func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-spaces", "spaces", `grant user alice read "book list"`),
			policy("p-unicode", "读-书📚", `grant user 张伟 read "数据📚" if region == "eu"`),
			policy("p-expression", "expression", `grant user alice read expr:resource.name`),
		}
		snapshot.RolePolicies = []adsync.Policy{
			policy("r-multi", "multi", "grant user alice role admin, editor"),
		}
	})
	fixture := newTestFixture(t, service)
	store := fixture.open(nil)

	books := serviceNamed(t, mustRead(t, store), "books")
	if len(books.Policies) != 3 {
		t.Fatalf("%d policies", len(books.Policies))
	}
	byID := make(map[string]*pms.Policy, len(books.Policies))
	for _, parsed := range books.Policies {
		byID[parsed.ID] = parsed
	}
	spaces := byID["p-spaces"]
	if spaces.Permissions[0].Resource != "book list" {
		t.Fatalf("quoted resource %q", spaces.Permissions[0].Resource)
	}
	unicode := byID["p-unicode"]
	if unicode.Name != "读-书📚" || unicode.Principals[0][0] != "user:张伟" ||
		unicode.Permissions[0].Resource != "数据📚" || unicode.Condition != `region == "eu"` {
		t.Fatalf("unicode policy %+v", unicode)
	}
	expression := byID["p-expression"]
	if expression.Permissions[0].Resource != "" || expression.Permissions[0].ResourceExpression != "resource.name" {
		t.Fatalf("resource expression %+v", expression.Permissions[0])
	}
	editor := books.RolePolicies[0]
	if len(editor.Roles) != 2 || editor.Roles[0] != "admin" || editor.Roles[1] != "editor" {
		t.Fatalf("roles %+v", editor.Roles)
	}
}

func TestReadAPIs(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store := fixture.open(nil)

	service, err := store.GetService("books")
	if err != nil || service.Name != "books" || len(service.Policies) != 2 {
		t.Fatalf("GetService: %+v %v", service, err)
	}
	if _, err := store.GetService("missing"); speedleerrors.Code(err) != speedleerrors.EntityNotFound {
		t.Fatalf("missing service: %v", err)
	}

	policy, err := store.GetPolicy("books", "p-read-books")
	if err != nil || policy.Name != "read-books" {
		t.Fatalf("GetPolicy: %+v %v", policy, err)
	}
	if _, err := store.GetPolicy("books", "missing"); speedleerrors.Code(err) != speedleerrors.EntityNotFound {
		t.Fatalf("missing policy: %v", err)
	}
	if _, err := store.GetPolicy("missing", "p-read-books"); speedleerrors.Code(err) != speedleerrors.EntityNotFound {
		t.Fatalf("policy of a missing service: %v", err)
	}

	rolePolicy, err := store.GetRolePolicy("books", "r-editor")
	if err != nil || rolePolicy.Name != "editor" {
		t.Fatalf("GetRolePolicy: %+v %v", rolePolicy, err)
	}
	if _, err := store.GetRolePolicy("books", "missing"); speedleerrors.Code(err) != speedleerrors.EntityNotFound {
		t.Fatalf("missing role policy: %v", err)
	}
	if _, err := store.GetFunction("anything"); speedleerrors.Code(err) != speedleerrors.EntityNotFound {
		t.Fatalf("GetFunction: %v", err)
	}

	services, err := store.ListAllServices()
	if err != nil || len(services) != 2 {
		t.Fatalf("ListAllServices: %d %v", len(services), err)
	}
	names, err := store.GetServiceNames()
	if err != nil || len(names) != 2 || names[0] != "books" {
		t.Fatalf("GetServiceNames: %v %v", names, err)
	}
	if count, _ := store.GetServiceCount(); count != 2 {
		t.Fatalf("GetServiceCount = %d", count)
	}
	counts, err := store.GetPolicyAndRolePolicyCounts()
	if err != nil {
		t.Fatal(err)
	}
	if counts["books"].PolicyCount != 2 || counts["books"].RolePolicyCount != 1 ||
		counts["orders"].PolicyCount != 1 || counts["orders"].RolePolicyCount != 0 {
		t.Fatalf("counts %+v", counts)
	}

	for filter, want := range map[string]int{
		"": 2, "name eq read-books": 1, "name co books": 2, "name sw read": 1,
		"name pr": 2, "name gt read-books": 1, "name ge read-books": 2,
		"name lt read-books": 0, "name le read-books": 1, "name xx books": 2,
	} {
		policies, err := store.ListAllPolicies("books", filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(policies) != want {
			t.Errorf("filter %q: %d policies, want %d", filter, len(policies), want)
		}
	}
	if count, err := store.GetPolicyCount("books"); err != nil || count != 2 {
		t.Fatalf("GetPolicyCount(books) = %d %v", count, err)
	}
	if count, err := store.GetPolicyCount(""); err != nil || count != 3 {
		t.Fatalf("GetPolicyCount(all) = %d %v", count, err)
	}
	if _, err := store.GetPolicyCount("missing"); speedleerrors.Code(err) != speedleerrors.EntityNotFound {
		t.Fatalf("policy count of a missing service: %v", err)
	}

	for filter, want := range map[string]int{"": 1, "name eq editor": 1, "name sw ed": 1} {
		rolePolicies, err := store.ListAllRolePolicies("books", filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(rolePolicies) != want {
			t.Errorf("filter %q: %d role policies, want %d", filter, len(rolePolicies), want)
		}
	}
	if count, _ := store.GetRolePolicyCount("books"); count != 1 {
		t.Fatalf("GetRolePolicyCount(books) = %d", count)
	}
	if count, _ := store.GetRolePolicyCount(""); count != 1 {
		t.Fatalf("GetRolePolicyCount(all) = %d", count)
	}
	functions, err := store.ListAllFunctions("")
	if err != nil || len(functions) != 0 {
		t.Fatalf("ListAllFunctions: %d %v", len(functions), err)
	}
	if count, err := store.GetFunctionCount(); err != nil || count != 0 {
		t.Fatalf("GetFunctionCount = %d %v", count, err)
	}
}

// A returned value is a copy: changing it must not change the store.
func TestReadsReturnCopies(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)

	policyStore := mustRead(t, store)
	policyStore.Services[0].Name = "changed"
	policyStore.Services[0].Policies[0].Name = "changed"
	policyStore.Services[0].Policies[0].Permissions[0].Actions[0] = "changed"

	service, err := store.GetService("books")
	if err != nil {
		t.Fatal(err)
	}
	if service.Name != "books" || service.Policies[0].Name != "read-books" ||
		service.Policies[0].Permissions[0].Actions[0] != "read" {
		t.Fatalf("the store was changed through a read: %+v", service)
	}
}

func TestReadOnlyWrites(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)

	calls := map[string]func() error{
		"WritePolicyStore": func() error { return store.WritePolicyStore(&pms.PolicyStore{}) },
		"CreateService":    func() error { return store.CreateService(&pms.Service{Name: "x"}) },
		"DeleteService":    func() error { return store.DeleteService("books") },
		"DeleteServices":   func() error { return store.DeleteServices() },
		"CreatePolicy": func() error {
			_, err := store.CreatePolicy("books", &pms.Policy{})
			return err
		},
		"DeletePolicy":   func() error { return store.DeletePolicy("books", "p-read-books") },
		"DeletePolicies": func() error { return store.DeletePolicies("books") },
		"CreateRolePolicy": func() error {
			_, err := store.CreateRolePolicy("books", &pms.RolePolicy{})
			return err
		},
		"DeleteRolePolicy":   func() error { return store.DeleteRolePolicy("books", "r-editor") },
		"DeleteRolePolicies": func() error { return store.DeleteRolePolicies("books") },
		"CreateFunction": func() error {
			_, err := store.CreateFunction(&pms.Function{Name: "f", FuncURL: "https://f"})
			return err
		},
		"DeleteFunction":  func() error { return store.DeleteFunction("f") },
		"DeleteFunctions": func() error { return store.DeleteFunctions() },
	}
	for name, call := range calls {
		err := call()
		if err == nil {
			t.Fatalf("%s succeeded on a read-only store", name)
		}
		if speedleerrors.Code(err) != speedleerrors.StoreError || !strings.Contains(err.Error(), "read-only") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestReloadIsOneAtomicSwap(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store := fixture.open(nil)
	events, err := store.Watch()
	if err != nil {
		t.Fatal(err)
	}

	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-read-books", "read-books", "grant user carol read books"),
		}
	})
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	waitForReload(t, events)

	policyStore := mustRead(t, store)
	books := serviceNamed(t, policyStore, "books")
	if len(books.Policies) != 1 || books.Policies[0].Principals[0][0] != "user:carol" {
		t.Fatalf("books after reload %+v", books.Policies)
	}
	// The service the pass re-verified unchanged is still there.
	serviceNamed(t, policyStore, "orders")

	// A pass that applies nothing new must not emit a reload.
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	expectNoEvent(t, events)
}

func TestConcurrentReadsSeeAWholeAssembly(t *testing.T) {
	service := newTestService(testServiceID1, "books")
	service.publish(func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-a", "alpha", "grant user alice read alpha"),
			policy("p-b", "beta", "grant user alice read beta"),
		}
	})
	fixture := newTestFixture(t, service)
	store := fixture.open(nil)
	events, err := store.Watch()
	if err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool
	var oldSeen, newSeen atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	for reader := 0; reader < 8; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				policyStore, err := store.ReadPolicyStore()
				if err != nil || len(policyStore.Services) != 1 {
					failures.Add(1)
					continue
				}
				policies := policyStore.Services[0].Policies
				if len(policies) != 2 {
					failures.Add(1)
					continue
				}
				switch {
				case policies[0].ID == "p-a" && policies[1].ID == "p-b":
					oldSeen.Add(1)
				case policies[0].ID == "p-c" && policies[1].ID == "p-d":
					newSeen.Add(1)
				default:
					// A mixture means the assembly was not swapped atomically.
					failures.Add(1)
				}
				if _, err := store.GetService("books"); err != nil {
					failures.Add(1)
				}
			}
		}()
	}

	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-c", "gamma", "grant user alice read gamma"),
			policy("p-d", "delta", "grant user alice read delta"),
		}
	})
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	waitForReload(t, events)
	time.Sleep(50 * time.Millisecond)
	stop.Store(true)
	wg.Wait()

	if failures.Load() != 0 {
		t.Fatalf("%d readers saw a partial assembly", failures.Load())
	}
	if oldSeen.Load() == 0 || newSeen.Load() == 0 {
		t.Fatalf("readers saw old %d times and new %d times", oldSeen.Load(), newSeen.Load())
	}
}

func TestInvalidSPDLKeepsPreviousCopy(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)
	events, err := store.Watch()
	if err != nil {
		t.Fatal(err)
	}

	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-broken", "broken", "grant user alice"),
		}
	})
	syncErr := store.Sync(nil)
	if syncErr == nil {
		t.Fatal("a snapshot with invalid SPDL was accepted")
	}
	if !strings.Contains(syncErr.Error(), ErrInvalidSPDL.Error()) {
		t.Fatalf("error does not name the parse failure: %v", syncErr)
	}
	if strings.Contains(syncErr.Error(), "grant user alice") {
		t.Fatalf("error leaks policy content: %v", syncErr)
	}
	expectNoEvent(t, events)

	// The previous, successfully parsed copy is still what readers see.
	policyStore := mustRead(t, store)
	books := serviceNamed(t, policyStore, "books")
	if len(books.Policies) != 2 || books.Policies[0].ID != "p-read-books" {
		t.Fatalf("previous copy was replaced: %+v", books.Policies)
	}
	status, known := store.Status("books")
	if !known || status.Service.State != adsync.SyncStateSyncing || status.Service.Reason != adsync.SyncReasonCatchUp {
		t.Fatalf("a service with an unparsable head must stay syncing: %+v", status)
	}
	// A syncing service does not fail store health; the request gate routes it.
	if err := store.CheckHealth(nil); err != nil {
		t.Fatalf("syncing service failed the health check: %v", err)
	}

	// A later good revision is published and announced.
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{
			policy("p-fixed", "fixed", "grant user alice read books"),
		}
	})
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	waitForReload(t, events)
	confirmStoreGeneration(t, store)
	books = serviceNamed(t, mustRead(t, store), "books")
	if len(books.Policies) != 1 || books.Policies[0].ID != "p-fixed" {
		t.Fatalf("books after recovery %+v", books.Policies)
	}
	if status, _ := store.Status("books"); status.Service.State != adsync.SyncStateInSync {
		t.Fatalf("books did not return to in_sync: %+v", status)
	}
}

func TestRemovedServiceIsDropped(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store := fixture.open(nil)
	events, err := store.Watch()
	if err != nil {
		t.Fatal(err)
	}

	fixture.source.removeService("books")
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	waitForReload(t, events)

	policyStore := mustRead(t, store)
	if len(policyStore.Services) != 1 || policyStore.Services[0].Name != "orders" {
		t.Fatalf("services after removal %+v", policyStore.Services)
	}
	if _, err := store.GetService("books"); speedleerrors.Code(err) != speedleerrors.EntityNotFound {
		t.Fatalf("removed service is still readable: %v", err)
	}
	status, known := store.Status("books")
	if !known || status.Service.State != adsync.SyncStateRemoved {
		t.Fatalf("removed service status %+v, known %v", status, known)
	}
}

func TestSelectedServicesOnly(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store := fixture.open(func(options *Options) {
		options.Services = []string{"books"}
	})

	policyStore := mustRead(t, store)
	if len(policyStore.Services) != 1 || policyStore.Services[0].Name != "books" {
		t.Fatalf("services %+v", policyStore.Services)
	}
	if _, known := store.Status("orders"); known {
		t.Fatal("an unselected service is tracked")
	}
	if count := fixture.source.requestsForSuffix("/services/" + testServiceID2 + "/snapshot"); count != 0 {
		t.Fatalf("an unselected service was synchronized %d times", count)
	}

	// A selection by service ULID works too and survives a rename.
	renamed := fixture.open(func(options *Options) {
		options.Services = []string{testServiceID2}
	})
	policyStore = mustRead(t, renamed)
	if len(policyStore.Services) != 1 || policyStore.Services[0].Name != "orders" {
		t.Fatalf("services selected by id %+v", policyStore.Services)
	}
}

func TestWatchStopAndRestart(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)

	first, err := store.Watch()
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Watch()
	if err != nil || again != first {
		t.Fatalf("Watch is not idempotent: %v", err)
	}
	store.StopWatch()
	if _, ok := <-first; ok {
		t.Fatal("the change channel is still open")
	}

	second, err := store.Watch()
	if err != nil {
		t.Fatal(err)
	}
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{policy("p-new", "new", "grant user alice read books")}
	})
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
	waitForReload(t, second)

	// A store without a watcher must not block: notification is a no-op.
	store.StopWatch()
	fixture.source.publishService("books", func(snapshot *adsync.Snapshot) {
		snapshot.Policies = []adsync.Policy{policy("p-newer", "newer", "grant user alice read books")}
	})
	if err := store.Sync(nil); err != nil {
		t.Fatal(err)
	}
}
