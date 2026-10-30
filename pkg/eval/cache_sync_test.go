//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

// newFixtureEvaluator returns an evaluator over its configured fixture holding
// content, and the store.
func newFixtureEvaluator(t *testing.T, content string) (*PolicyEvalImpl, pms.PolicyStoreManager) {
	t.Helper()
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	conf := &cfg.Config{}
	ps := storetest.PolicyStore(t, location)
	evaluator, err := NewWithStore(conf, ps)
	if err != nil {
		t.Fatal(err)
	}
	return evaluator.(*PolicyEvalImpl), ps
}

func allowed(t *testing.T, p *PolicyEvalImpl, service, user, resource, action string) bool {
	t.Helper()
	ok, _, _ := p.IsAllowed(adsapi.RequestContext{
		Subject:     &adsapi.Subject{Principals: []*adsapi.Principal{{Type: adsapi.PRINCIPAL_TYPE_USER, Name: user}}},
		ServiceName: service, Resource: resource, Action: action,
	})
	return ok
}

// apply runs the store watch loop over the given events.
func apply(p *PolicyEvalImpl, events ...pms.StoreChangeEvent) {
	ch := make(chan pms.StoreChangeEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	p.updateRuntimeCacheWithStoreChange(ch)
}

func grantPolicy(id, user, resource string) *pms.Policy {
	return &pms.Policy{ID: id, Name: id, Effect: pms.Grant, Principals: [][]string{{"user:" + user}},
		Permissions: []*pms.Permission{{Resource: resource, Actions: []string{"read"}}}}
}

func TestCacheFollowsStoreEvents(t *testing.T) {
	p, _ := newFixtureEvaluator(t, `{}`)

	apply(p, pms.StoreChangeEvent{Type: pms.SERVICE_ADD, Content: &pms.Service{Name: "app", Policies: []*pms.Policy{grantPolicy("1", "alice", "r1")}}})
	if !allowed(t, p, "app", "alice", "r1", "read") {
		t.Fatal("a policy of an added service was not applied")
	}

	apply(p, pms.StoreChangeEvent{Type: pms.POLICY_ADD, Content: []pms.StoreUpdateData{
		{ServiceName: "app", Data: grantPolicy("2", "bob", "r2")},
		{ServiceName: "nope", Data: grantPolicy("3", "bob", "r3")},
	}})
	if !allowed(t, p, "app", "bob", "r2", "read") {
		t.Fatal("an added policy was not applied")
	}

	adminPolicy := &pms.Policy{ID: "4", Effect: pms.Grant, Principals: [][]string{{"role:admin"}},
		Permissions: []*pms.Permission{{Resource: "r4", Actions: []string{"write"}}}}
	rolePolicy := &pms.RolePolicy{ID: "5", Effect: pms.Grant, Roles: []string{"admin"}, Principals: []string{"user:carol"}}
	apply(p,
		pms.StoreChangeEvent{Type: pms.POLICY_ADD, Content: []pms.StoreUpdateData{{ServiceName: "app", Data: adminPolicy}}},
		pms.StoreChangeEvent{Type: pms.ROLEPOLICY_ADD, Content: []pms.StoreUpdateData{
			{ServiceName: "app", Data: rolePolicy}, {ServiceName: "nope", Data: rolePolicy}}},
	)
	if !allowed(t, p, "app", "carol", "r4", "write") {
		t.Fatal("an added role policy was not applied")
	}
	apply(p, pms.StoreChangeEvent{Type: pms.ROLEPOLICY_DELETE, Content: []pms.StoreUpdateData{
		{ServiceName: "app", Data: rolePolicy}, {ServiceName: "nope", Data: rolePolicy}}})
	if allowed(t, p, "app", "carol", "r4", "write") {
		t.Fatal("a deleted role policy still applies")
	}

	apply(p, pms.StoreChangeEvent{Type: pms.POLICY_DELETE, Content: []pms.StoreUpdateData{
		{ServiceName: "app", Data: grantPolicy("2", "bob", "r2")}, {ServiceName: "nope", Data: grantPolicy("2", "", "")}}})
	if allowed(t, p, "app", "bob", "r2", "read") {
		t.Fatal("a deleted policy still applies")
	}

	apply(p, pms.StoreChangeEvent{Type: pms.SERVICE_DELETE, Content: []string{"app"}})
	if allowed(t, p, "app", "alice", "r1", "read") {
		t.Fatal("a deleted service still applies")
	}
	if p.RuntimePolicyStore.RuntimeServices["app"] != nil {
		t.Fatal("the deleted service is still cached")
	}
}

func TestCacheFollowsFunctionEvents(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"result": true}`))
	}))
	defer server.Close()

	p, _ := newFixtureEvaluator(t, `{"services":[{"name":"app","policies":[
		{"id":"1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"r","actions":["read"]}],"condition":"isOpen()"}]}]}`)
	if allowed(t, p, "app", "alice", "r", "read") {
		t.Fatal("a condition with an unknown function must not grant")
	}
	apply(p, pms.StoreChangeEvent{Type: pms.FUNCTION_ADD, Content: &pms.Function{Name: "isOpen", FuncURL: server.URL, ResultCachable: true}})
	// Conditions compiled before the function existed are recompiled.
	if !allowed(t, p, "app", "alice", "r", "read") {
		t.Fatal("the added function was not used")
	}
	if !allowed(t, p, "app", "alice", "r", "read") {
		t.Fatal("the added function was not used")
	}
	if calls != 1 {
		t.Fatalf("the cached result was not reused: %d calls", calls)
	}

	apply(p, pms.StoreChangeEvent{Type: pms.FUNCTION_DELETE, Content: []string{"isOpen"}})
	if _, ok := p.RuntimePolicyStore.Functions["isOpen"]; ok {
		t.Fatal("the deleted function is still loaded")
	}
	if len(p.RuntimePolicyStore.FunctionResultCache.Results) != 0 {
		t.Fatal("results of the deleted function are still cached")
	}
	if allowed(t, p, "app", "alice", "r", "read") {
		t.Fatal("a condition using a deleted function still grants")
	}
}

func TestFullReload(t *testing.T) {
	p, ps := newFixtureEvaluator(t, `{"services":[{"name":"app","policies":[
		{"id":"1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"r","actions":["read"]}]}]}]}`)
	var replaced pms.PolicyStore
	json.Unmarshal([]byte(`{"services":[{"name":"app","policies":[
		{"id":"2","effect":"grant","principals":[["user:bob"]],"permissions":[{"resource":"r","actions":["read"]}]}]}],
		"functions":[{"name":"f","funcURL":"http://127.0.0.1:1"}]}`), &replaced)
	if err := ps.WritePolicyStore(&replaced); err != nil {
		t.Fatal(err)
	}
	apply(p, pms.StoreChangeEvent{Type: pms.FULL_RELOAD})
	if allowed(t, p, "app", "alice", "r", "read") || !allowed(t, p, "app", "bob", "r", "read") {
		t.Fatal("FULL_RELOAD did not replace the cache")
	}
	if _, ok := p.RuntimePolicyStore.Functions["f"]; !ok {
		t.Fatal("FULL_RELOAD did not load the functions")
	}
	if err := p.Refresh(); err != nil {
		t.Fatal(err)
	}

	// A store that cannot be read leaves the cache as it was.
	p.Store = storetest.NewFailingStore()
	p.Refresh()
	if !allowed(t, p, "app", "bob", "r", "read") {
		t.Fatal("a failed reload dropped the cache")
	}
}

func TestSyncReload(t *testing.T) {
	p, ps := newFixtureEvaluator(t, `{"services":[
		{"name":"gone","policies":[{"id":"9","effect":"grant","principals":[["user:x"]]}]},
		{"name":"app","policies":[
			{"id":"1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"r","actions":["read"]}]}],
		 "rolePolicies":[{"id":"11","effect":"grant","roles":["old"],"principals":["user:alice"]}]}],
		"functions":[{"name":"oldf","funcURL":"http://127.0.0.1:1"}]}`)

	var next pms.PolicyStore
	json.Unmarshal([]byte(`{"services":[
		{"name":"app","policies":[
			{"id":"2","effect":"grant","principals":[["role:new"]],"permissions":[{"resource":"r","actions":["read"]}]}],
		 "rolePolicies":[{"id":"12","effect":"grant","roles":["new"],"principals":["user:bob"]}]},
		{"name":"added"}],
		"functions":[{"name":"newf","funcURL":"http://127.0.0.1:1"}]}`), &next)
	if err := ps.WritePolicyStore(&next); err != nil {
		t.Fatal(err)
	}
	apply(p, pms.StoreChangeEvent{Type: pms.SYNC_RELOAD, Content: []interface{}{
		[]string{"app", "added"},
		map[int]string{2: "app"},
		map[int]string{12: "app"},
		[]string{"newf"},
	}})
	if allowed(t, p, "app", "alice", "r", "read") || !allowed(t, p, "app", "bob", "r", "read") {
		t.Fatal("SYNC_RELOAD did not sync policies and role policies")
	}
	if p.RuntimePolicyStore.RuntimeServices["gone"] != nil || p.RuntimePolicyStore.RuntimeServices["added"] == nil {
		t.Fatal("SYNC_RELOAD did not sync services")
	}
	functions := p.RuntimePolicyStore.Functions
	if _, ok := functions["newf"]; !ok {
		t.Fatal("SYNC_RELOAD did not add newf")
	}
	if _, ok := functions["oldf"]; ok {
		t.Fatal("SYNC_RELOAD did not remove oldf")
	}
}

func TestSyncReloadErrors(t *testing.T) {
	p, _ := newFixtureEvaluator(t, `{"services":[{"name":"app","policies":[{"id":"1","effect":"grant"}],
		"rolePolicies":[{"id":"2","effect":"grant"}]}]}`)
	services, policies, rolePolicies, functions := []string{"app"}, map[int]string{1: "app"}, map[int]string{2: "app"}, []string{}
	cases := map[string][]interface{}{
		"short data set":          {services},
		"services":                {1, policies, rolePolicies, functions},
		"policies":                {services, 1, rolePolicies, functions},
		"role policies":           {services, policies, 1, functions},
		"functions":               {services, policies, rolePolicies, 1},
		"missing service":         {[]string{"app", "missing"}, policies, rolePolicies, functions},
		"missing policy":          {services, map[int]string{1: "app", 7: "app"}, rolePolicies, functions},
		"missing role policy":     {services, policies, map[int]string{2: "app", 8: "app"}, functions},
		"missing custom function": {services, policies, rolePolicies, []string{"nofunc"}},
	}
	for name, dataSet := range cases {
		err := p.syncRuntimeCache(dataSet)
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	// SYNC_RELOAD logs the error and keeps watching.
	apply(p, pms.StoreChangeEvent{Type: pms.SYNC_RELOAD, Content: []interface{}{}})

	// Cached IDs must be numbers to be compared with the store's.
	p.RuntimePolicyStore.RuntimeServices["app"].PoliciesCache.PolicyMap["abc"] = &pms.Policy{ID: "abc"}
	if err := p.syncRuntimeCache([]interface{}{services, policies, rolePolicies, functions}); errors.Code(err) != errors.EvalCacheError {
		t.Errorf("non-numeric policy ID: %v", err)
	}
	delete(p.RuntimePolicyStore.RuntimeServices["app"].PoliciesCache.PolicyMap, "abc")
	p.RuntimePolicyStore.RuntimeServices["app"].RolePoliciesCache.PolicyMap["abc"] = &pms.RolePolicy{ID: "abc"}
	if err := p.syncRuntimeCache([]interface{}{services, policies, rolePolicies, functions}); errors.Code(err) != errors.EvalCacheError {
		t.Errorf("non-numeric role policy ID: %v", err)
	}
}

func TestEvaluatorFactories(t *testing.T) {
	location := filepath.Join(t.TempDir(), "policies.json")
	os.WriteFile(location, []byte(`{"services":[{"name":"app","policies":[
		{"id":"1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"r","actions":["read"]}]}]}]}`), 0600)
	evaluator, err := NewFromFile(location, false)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := evaluator.IsAllowed(adsapi.RequestContext{Subject: &adsapi.Subject{Principals: []*adsapi.Principal{{Type: "user", Name: "alice"}}},
		ServiceName: "app", Resource: "r", Action: "read"}); !ok {
		t.Fatal("NewFromFile did not load the policies")
	}

	config := filepath.Join(t.TempDir(), "config.json")
	configuration, err := json.Marshal(&cfg.Config{StoreConfig: storetest.ConfigFromFile(t, location), EnableWatch: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, configuration, 0600); err != nil {
		t.Fatal(err)
	}
	watched, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	watched.(*PolicyEvalImpl).StopWatch()
	t.Cleanup(func() { _ = watched.(*PolicyEvalImpl).Close() })

	if _, err := New(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("New with a missing config must fail")
	}
	if _, err := NewFromConfig(&cfg.Config{StoreConfig: &cfg.StoreConfig{StoreType: "nosuchstore"}}); err == nil {
		t.Fatal("NewFromConfig with an unknown store must fail")
	}
}
