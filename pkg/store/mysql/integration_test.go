package mysql_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/store"
	mysqlstore "github.com/teramoby/speedle-plus/pkg/store/mysql"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsrest"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SPEEDLE_MYSQL_TEST_DSN")
	if dsn == "" {
		if os.Getenv("SPEEDLE_REQUIRE_MYSQL") == "1" {
			t.Fatal("SPEEDLE_MYSQL_TEST_DSN is required")
		}
		t.Skip("set SPEEDLE_MYSQL_TEST_DSN to a disposable MySQL 8.4 database")
	}
	c, err := driver.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(c.DBName, "speedle_test") {
		t.Fatal("MySQL tests require a disposable database whose name starts with speedle_test")
	}
	return dsn
}
func connect(t *testing.T, reset bool) *mysqlstore.Store {
	t.Helper()
	manager, err := store.NewStore("mysql", map[string]interface{}{mysqlstore.DSNKey: testDSN(t), mysqlstore.PollIntervalKey: "50ms", mysqlstore.QueryTimeoutKey: "10s"})
	if err != nil {
		t.Fatal(err)
	}
	s := manager.(*mysqlstore.Store)
	t.Cleanup(func() { _ = s.Close() })
	if reset {
		must(t, s.WritePolicyStore(&pms.PolicyStore{}))
	}
	return s
}
func rawDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func revision(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var r int64
	must(t, db.QueryRow("SELECT revision FROM speedle_store_meta WHERE id=1").Scan(&r))
	return r
}

func TestMySQLConformance(t *testing.T) {
	testDSN(t)
	storetest.RunConformance(t, func(t *testing.T) pms.PolicyStoreManager { return connect(t, true) })
}

func TestMySQLRoundTripAndIdentity(t *testing.T) {
	s := connect(t, true)
	input := &pms.PolicyStore{
		Services: []*pms.Service{{Name: "应用 ' App", Type: pms.TypeApplication, Metadata: map[string]string{"团队": "玩具"},
			Policies:     []*pms.Policy{{ID: "imported-id", Name: "one", Effect: pms.Grant, Principals: [][]string{{"user:alice", "group:staff"}, {"role:admin"}}, Permissions: []*pms.Permission{{ResourceExpression: "/书/.*", Actions: []string{"read", "write"}}}, Condition: "age > 18", Metadata: map[string]string{"m": "v"}}},
			RolePolicies: []*pms.RolePolicy{{ID: "role-id", Name: "role", Effect: pms.Deny, Roles: []string{"admin"}, Principals: []string{"user:bob"}, Resources: []string{"r"}, ResourceExpressions: []string{"r.*"}, Condition: "x == 1", Metadata: map[string]string{"m": "v"}}}}},
		Functions: []*pms.Function{{Name: "函数", Description: "desc", FuncURL: "https://example.com/f", LocalFuncURL: "local", CA: "certificate", ResultCachable: true, ResultTTL: 9223372036854775807, Metadata: map[string]string{"k": "v"}}},
	}
	must(t, s.WritePolicyStore(input))
	got, err := s.ReadPolicyStore()
	must(t, err)
	if !reflect.DeepEqual(input, got) {
		t.Fatalf("round trip differs:\n%+v\n%+v", input, got)
	}
	// Names are case-sensitive and preserve trailing spaces, regardless of DB defaults.
	for _, name := range []string{"App", "app", "app "} {
		must(t, s.CreateService(&pms.Service{Name: name}))
	}
	for _, name := range []string{"App", "app", "app "} {
		if service, err := s.GetService(name); err != nil || service.Name != name {
			t.Fatalf("identity %q: %+v %v", name, service, err)
		}
	}
	must(t, s.DeleteService(input.Services[0].Name))
	if n, err := s.GetPolicyCount(""); err != nil || n != 0 {
		t.Fatalf("cascade policies: %d %v", n, err)
	}
	if n, err := s.GetRolePolicyCount(""); err != nil || n != 0 {
		t.Fatalf("cascade roles: %d %v", n, err)
	}
}

func TestMySQLAtomicWritesAndConcurrentWriters(t *testing.T) {
	s := connect(t, true)
	other := connect(t, false)
	db := rawDB(t)
	old := &pms.PolicyStore{Services: []*pms.Service{{Name: "old"}}, Functions: []*pms.Function{{Name: "old", FuncURL: "https://old"}}}
	must(t, s.WritePolicyStore(old))
	before := revision(t, db)
	err := s.WritePolicyStore(&pms.PolicyStore{Services: []*pms.Service{{Name: "new"}, {Name: "new"}}})
	if errors.Code(err) != errors.EntityAlreadyExists {
		t.Fatalf("duplicate batch: %v", err)
	}
	got, err := s.ReadPolicyStore()
	must(t, err)
	if got.Services[0].Name != "old" || got.Functions[0].Name != "old" || revision(t, db) != before {
		t.Fatal("partial replacement or revision escaped rollback")
	}

	// Force failure after the parent and first policy have already been inserted.
	_, err = db.Exec(`CREATE TRIGGER speedle_test_fail_role BEFORE INSERT ON speedle_role_policies FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected failure'`)
	must(t, err)
	t.Cleanup(func() { db.Exec("DROP TRIGGER IF EXISTS speedle_test_fail_role") })
	service := &pms.Service{Name: "partial", Policies: []*pms.Policy{{Name: "p"}}, RolePolicies: []*pms.RolePolicy{{Name: "rp"}}}
	if s.CreateService(service) == nil {
		t.Fatal("trigger failure ignored")
	}
	if _, err := s.GetService("partial"); errors.Code(err) != errors.EntityNotFound {
		t.Fatal("parent escaped rollback")
	}
	if service.Policies[0].ID != "" || revision(t, db) != before {
		t.Fatal("failed write changed caller or revision")
	}
	_, err = db.Exec("DROP TRIGGER speedle_test_fail_role")
	must(t, err)

	must(t, s.CreateService(&pms.Service{Name: "app"}))
	var wg sync.WaitGroup
	failures := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			writer := s
			if i%2 == 0 {
				writer = other
			}
			_, err := writer.CreatePolicy("app", &pms.Policy{Name: fmt.Sprint(i), Effect: pms.Grant})
			if err != nil {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if n, err := s.GetPolicyCount("app"); err != nil || n != 32 {
		t.Fatalf("lost writes: %d %v", n, err)
	}
}

func TestMySQLConsistentSnapshots(t *testing.T) {
	s := connect(t, true)
	writer := connect(t, false)
	makeSnapshot := func(name string) *pms.PolicyStore {
		return &pms.PolicyStore{Services: []*pms.Service{{Name: name}}, Functions: []*pms.Function{{Name: name, FuncURL: "https://example.com"}}}
	}
	must(t, s.WritePolicyStore(makeSnapshot("a")))
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 30; i++ {
			if err := writer.WritePolicyStore(makeSnapshot([]string{"a", "b"}[i%2])); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 30; i++ {
		ps, err := s.ReadPolicyStore()
		must(t, err)
		if len(ps.Services) != 1 || len(ps.Functions) != 1 || ps.Services[0].Name != ps.Functions[0].Name {
			t.Fatalf("mixed snapshot: %+v", ps)
		}
	}
	must(t, <-done)
}

func TestMySQLPrebuiltSchemaWithReadOnlyAccount(t *testing.T) {
	connect(t, true)
	db := rawDB(t)
	configuration, _ := driver.ParseDSN(testDSN(t))
	if configuration.User != "root" {
		t.Skip("read-only account fixture requires a disposable root account")
	}
	user := fmt.Sprintf("speedle_ro_%x", time.Now().UnixNano())
	_, err := db.Exec("CREATE USER '" + user + "'@'%' IDENTIFIED BY 'readonly-test-only'")
	must(t, err)
	t.Cleanup(func() { db.Exec("DROP USER '" + user + "'@'%'") })
	_, err = db.Exec("GRANT SELECT ON `" + strings.ReplaceAll(configuration.DBName, "`", "``") + "`.* TO '" + user + "'@'%'")
	must(t, err)
	configuration.User = user
	configuration.Passwd = "readonly-test-only"
	properties := map[string]interface{}{mysqlstore.DSNKey: configuration.FormatDSN(), mysqlstore.InitSchemaKey: false}
	manager, err := (mysqlstore.MySQLStoreBuilder{}).NewStore(properties)
	must(t, err)
	defer manager.(*mysqlstore.Store).Close()
	_, err = manager.ReadPolicyStore()
	must(t, err)
	if manager.CreateService(&pms.Service{Name: "denied"}) == nil {
		t.Fatal("read-only account wrote data")
	}
	properties[mysqlstore.InitSchemaKey] = true
	if _, err := (mysqlstore.MySQLStoreBuilder{}).NewStore(properties); err == nil {
		t.Fatal("DDL attempted with read-only account unexpectedly succeeded")
	}
}

func TestMySQLWatchRecoversAfterReadTimeout(t *testing.T) {
	writer := connect(t, true)
	readerManager, err := (mysqlstore.MySQLStoreBuilder{}).NewStore(map[string]interface{}{mysqlstore.DSNKey: testDSN(t), mysqlstore.InitSchemaKey: false, mysqlstore.PollIntervalKey: "1s", mysqlstore.QueryTimeoutKey: "1s"})
	must(t, err)
	reader := readerManager.(*mysqlstore.Store)
	defer reader.Close()
	events, err := reader.Watch()
	must(t, err)
	select {
	case <-events:
	case <-time.After(3 * time.Second):
		t.Fatal("no initial snapshot")
	}
	// Leave a revision pending, then make the revision query unavailable long
	// enough to exceed the reader's timeout. No change may be discarded.
	must(t, writer.CreateService(&pms.Service{Name: "after-outage"}))
	db := rawDB(t)
	connection, err := db.Conn(context.Background())
	must(t, err)
	defer connection.Close()
	_, err = connection.ExecContext(context.Background(), "LOCK TABLES speedle_store_meta WRITE")
	must(t, err)
	time.Sleep(2300 * time.Millisecond)
	_, err = connection.ExecContext(context.Background(), "UNLOCK TABLES")
	must(t, err)
	select {
	case event := <-events:
		if event.Content.(*pms.PolicyStore).Services[0].Name != "after-outage" {
			t.Fatal("lost pending revision")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not recover")
	}
}

func TestMySQLPMSAndADS(t *testing.T) {
	writer := connect(t, true)
	reader := connect(t, false)
	service := &pms.Service{Name: "app", Policies: []*pms.Policy{{Name: "initial", Effect: pms.Grant, Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "r", Actions: []string{"read"}}}}}}
	router, err := pmsrest.NewRouter(writer)
	must(t, err)
	server := httptest.NewServer(router)
	defer server.Close()
	// Use the real PMS REST endpoint and confirm it returns persisted IDs.
	response, err := server.Client().Post(server.URL+"/policy-mgmt/v1/service", "application/json", strings.NewReader(jsonTextForTest(service)))
	must(t, err)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 201 || !strings.Contains(string(body), `"id"`) {
		t.Fatalf("PMS create: %d %s", response.StatusCode, body)
	}
	loaded, err := writer.GetService("app")
	must(t, err)
	// Mutate after the first read but before Watch, reproducing the startup gap.
	_, err = reader.ReadPolicyStore()
	must(t, err)
	must(t, writer.DeletePolicy("app", loaded.Policies[0].ID))
	events, err := reader.Watch()
	must(t, err)
	select {
	case event := <-events:
		if len(event.Content.(*pms.PolicyStore).Services[0].Policies) != 0 {
			t.Fatal("initial watch missed deletion")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no initial snapshot")
	}
	reader.StopWatch()
	evaluator, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, reader)
	must(t, err)
	defer evaluator.(io.Closer).Close()
	request := ads.RequestContext{ServiceName: "app", Resource: "r", Action: "read", Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}}}}
	waitDecision := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			allowed, _, err := evaluator.IsAllowed(request)
			if allowed == want && (err == nil || (!want && errors.Code(err) == errors.EvalEngineError)) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("ADS did not converge to %v", want)
	}
	waitDecision(false)
	created, err := writer.CreatePolicy("app", service.Policies[0])
	must(t, err)
	waitDecision(true)
	must(t, writer.DeletePolicy("app", created.ID))
	waitDecision(false)
	must(t, writer.WritePolicyStore(&pms.PolicyStore{Services: []*pms.Service{service}}))
	waitDecision(true)
	must(t, writer.DeleteServices())
	waitDecision(false)
}

func jsonTextForTest(v interface{}) string { raw, _ := json.Marshal(v); return string(raw) }
