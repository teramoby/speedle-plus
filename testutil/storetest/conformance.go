//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package storetest

import (
	"sort"
	"testing"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
)

// RunConformance checks the behaviour every policy store shares. newStore
// returns an empty store; each subtest gets its own.
//
// Stores differ on listing or deleting the policies of a missing service
// (the file store fails, etcd returns nothing), so that is not checked.
func RunConformance(t *testing.T, newStore func(t *testing.T) pms.PolicyStoreManager) {
	t.Run("Services", func(t *testing.T) { testServices(t, newStore(t)) })
	t.Run("Policies", func(t *testing.T) { testPolicies(t, newStore(t)) })
	t.Run("RolePolicies", func(t *testing.T) { testRolePolicies(t, newStore(t)) })
	RunUpdateConformance(t, newStore)
	t.Run("Functions", func(t *testing.T) { testFunctions(t, newStore(t)) })
	t.Run("PolicyStore", func(t *testing.T) { testPolicyStore(t, newStore(t)) })
	t.Run("Discover", func(t *testing.T) {
		manager, ok := newStore(t).(store.DiscoverRequestManager)
		if !ok {
			t.Skip("the store does not manage discover requests")
		}
		testDiscover(t, manager)
	})
}

func wantCode(t *testing.T, what string, err error, code errors.ErrorCode) {
	t.Helper()
	if errors.Code(err) != code {
		t.Errorf("%s: error %v, want code %s", what, err, code)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func testServices(t *testing.T, s pms.PolicyStoreManager) {
	if s.Type() == "" {
		t.Error("empty store type")
	}
	for _, name := range []string{"app1", "app2"} {
		must(t, s.CreateService(&pms.Service{Name: name, Type: pms.TypeApplication,
			Policies:     []*pms.Policy{{Name: "p", Effect: pms.Grant}},
			RolePolicies: []*pms.RolePolicy{{Name: "rp", Effect: pms.Grant, Roles: []string{"r"}}}}))
	}
	wantCode(t, "duplicate service", s.CreateService(&pms.Service{Name: "app1"}), errors.EntityAlreadyExists)

	service, err := s.GetService("app1")
	must(t, err)
	if service.Name != "app1" || len(service.Policies) != 1 || service.Policies[0].ID == "" || service.RolePolicies[0].ID == "" {
		t.Errorf("service %+v; created policies need IDs", service)
	}
	_, err = s.GetService("missing")
	wantCode(t, "missing service", err, errors.EntityNotFound)

	services, err := s.ListAllServices()
	must(t, err)
	names, err := s.GetServiceNames()
	must(t, err)
	sort.Strings(names)
	count, err := s.GetServiceCount()
	must(t, err)
	if len(services) != 2 || count != 2 || len(names) != 2 || names[0] != "app1" || names[1] != "app2" {
		t.Errorf("services %d, names %v, count %d", len(services), names, count)
	}
	counts, err := s.GetPolicyAndRolePolicyCounts()
	must(t, err)
	if c := counts["app2"]; c == nil || c.PolicyCount != 1 || c.RolePolicyCount != 1 {
		t.Errorf("counts %+v", counts)
	}

	must(t, s.DeleteService("app1"))
	wantCode(t, "deleting a missing service", s.DeleteService("app1"), errors.EntityNotFound)
	must(t, s.DeleteServices())
	if count, _ := s.GetServiceCount(); count != 0 {
		t.Errorf("%d services left", count)
	}
}

func policyNamed(name string) *pms.Policy {
	return &pms.Policy{Name: name, Effect: pms.Grant, Principals: [][]string{{"user:alice"}},
		Permissions: []*pms.Permission{{Resource: "r", Actions: []string{"get"}}}}
}

func testPolicies(t *testing.T, s pms.PolicyStoreManager) {
	must(t, s.CreateService(&pms.Service{Name: "app"}))
	must(t, s.CreateService(&pms.Service{Name: "other"}))
	ids := map[string]string{}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		created, err := s.CreatePolicy("app", policyNamed(name))
		must(t, err)
		if created.ID == "" || created.Name != name {
			t.Fatalf("created %+v", created)
		}
		ids[name] = created.ID
	}
	_, err := s.CreatePolicy("other", policyNamed("delta"))
	must(t, err)
	if _, err := s.CreatePolicy("missing", policyNamed("x")); err == nil {
		t.Error("a policy in a missing service must fail")
	}

	got, err := s.GetPolicy("app", ids["beta"])
	must(t, err)
	if got.Name != "beta" || got.Permissions[0].Resource != "r" {
		t.Errorf("policy %+v", got)
	}
	_, err = s.GetPolicy("app", "nope")
	wantCode(t, "missing policy", err, errors.EntityNotFound)
	if _, err := s.GetPolicy("missing", ids["beta"]); err == nil {
		t.Error("a policy of a missing service must fail")
	}

	for filter, want := range map[string]int{
		"": 3, "name eq beta": 1, "name co mm": 1, "name sw al": 1, "name pr": 3,
		"name gt beta": 1, "name ge beta": 2, "name lt beta": 1, "name le beta": 2,
		"name xx beta": 3, "id eq 1": 3, "name": 3,
	} {
		policies, err := s.ListAllPolicies("app", filter)
		must(t, err)
		if len(policies) != want {
			t.Errorf("filter %q: %d policies, want %d", filter, len(policies), want)
		}
	}

	if count, _ := s.GetPolicyCount("app"); count != 3 {
		t.Errorf("app has %d policies", count)
	}
	if count, _ := s.GetPolicyCount(""); count != 4 {
		t.Errorf("all services have %d policies", count)
	}

	must(t, s.DeletePolicy("app", ids["alpha"]))
	wantCode(t, "deleting a deleted policy", s.DeletePolicy("app", ids["alpha"]), errors.EntityNotFound)
	if err := s.DeletePolicy("missing", ids["beta"]); err == nil {
		t.Error("deleting a policy of a missing service must fail")
	}
	must(t, s.DeletePolicies("app"))
	if count, _ := s.GetPolicyCount("app"); count != 0 {
		t.Errorf("%d policies left", count)
	}
}

func rolePolicyNamed(name string) *pms.RolePolicy {
	return &pms.RolePolicy{Name: name, Effect: pms.Grant, Roles: []string{"admin"}, Principals: []string{"user:alice"}}
}

func testRolePolicies(t *testing.T, s pms.PolicyStoreManager) {
	must(t, s.CreateService(&pms.Service{Name: "app"}))
	ids := map[string]string{}
	for _, name := range []string{"alpha", "beta"} {
		created, err := s.CreateRolePolicy("app", rolePolicyNamed(name))
		must(t, err)
		ids[name] = created.ID
	}
	if _, err := s.CreateRolePolicy("missing", rolePolicyNamed("x")); err == nil {
		t.Error("a role policy in a missing service must fail")
	}
	got, err := s.GetRolePolicy("app", ids["alpha"])
	must(t, err)
	if got.Name != "alpha" || got.Roles[0] != "admin" {
		t.Errorf("role policy %+v", got)
	}
	_, err = s.GetRolePolicy("app", "nope")
	wantCode(t, "missing role policy", err, errors.EntityNotFound)
	if _, err := s.GetRolePolicy("missing", ids["alpha"]); err == nil {
		t.Error("a role policy of a missing service must fail")
	}
	for filter, want := range map[string]int{"": 2, "name eq alpha": 1, "name sw b": 1} {
		rolePolicies, err := s.ListAllRolePolicies("app", filter)
		must(t, err)
		if len(rolePolicies) != want {
			t.Errorf("filter %q: %d role policies, want %d", filter, len(rolePolicies), want)
		}
	}
	if count, _ := s.GetRolePolicyCount("app"); count != 2 {
		t.Errorf("app has %d role policies", count)
	}
	if count, _ := s.GetRolePolicyCount(""); count != 2 {
		t.Errorf("all services have %d role policies", count)
	}
	must(t, s.DeleteRolePolicy("app", ids["alpha"]))
	wantCode(t, "deleting a deleted role policy", s.DeleteRolePolicy("app", ids["alpha"]), errors.EntityNotFound)
	if err := s.DeleteRolePolicy("missing", ids["beta"]); err == nil {
		t.Error("deleting a role policy of a missing service must fail")
	}
	must(t, s.DeleteRolePolicies("app"))
	if count, _ := s.GetRolePolicyCount("app"); count != 0 {
		t.Errorf("%d role policies left", count)
	}
}

func testFunctions(t *testing.T, s pms.PolicyStoreManager) {
	for _, name := range []string{"f1", "f2"} {
		created, err := s.CreateFunction(&pms.Function{Name: name, FuncURL: "https://f/" + name, ResultCachable: true, ResultTTL: 5})
		must(t, err)
		if created.Name != name {
			t.Errorf("created %+v", created)
		}
	}
	_, err := s.CreateFunction(&pms.Function{Name: "f1", FuncURL: "https://f/1"})
	wantCode(t, "duplicate function", err, errors.EntityAlreadyExists)
	_, err = s.CreateFunction(&pms.Function{Name: "nourl"})
	wantCode(t, "function without URL", err, errors.InvalidRequest)

	got, err := s.GetFunction("f2")
	must(t, err)
	if got.FuncURL != "https://f/f2" || !got.ResultCachable || got.ResultTTL != 5 {
		t.Errorf("function %+v", got)
	}
	_, err = s.GetFunction("nope")
	wantCode(t, "missing function", err, errors.EntityNotFound)
	for filter, want := range map[string]int{"": 2, "name eq f1": 1} {
		functions, err := s.ListAllFunctions(filter)
		must(t, err)
		if len(functions) != want {
			t.Errorf("filter %q: %d functions, want %d", filter, len(functions), want)
		}
	}
	if count, _ := s.GetFunctionCount(); count != 2 {
		t.Errorf("%d functions", count)
	}
	must(t, s.DeleteFunction("f1"))
	wantCode(t, "deleting a deleted function", s.DeleteFunction("f1"), errors.EntityNotFound)
	must(t, s.DeleteFunctions())
	if count, _ := s.GetFunctionCount(); count != 0 {
		t.Errorf("%d functions left", count)
	}
}

func testPolicyStore(t *testing.T, s pms.PolicyStoreManager) {
	in := &pms.PolicyStore{
		Services: []*pms.Service{{Name: "app", Type: pms.TypeApplication,
			Policies:     []*pms.Policy{{ID: "p1", Name: "p1", Effect: pms.Deny}},
			RolePolicies: []*pms.RolePolicy{{ID: "rp1", Name: "rp1", Effect: pms.Grant}}}},
		Functions: []*pms.Function{{Name: "f", FuncURL: "https://f"}},
	}
	must(t, s.WritePolicyStore(in))
	out, err := s.ReadPolicyStore()
	must(t, err)
	if len(out.Services) != 1 || out.Services[0].Policies[0].ID != "p1" || out.Services[0].RolePolicies[0].ID != "rp1" ||
		len(out.Functions) != 1 || out.Functions[0].Name != "f" {
		t.Errorf("read back %+v", out)
	}
	if policy, err := s.GetPolicy("app", "p1"); err != nil || policy.Effect != pms.Deny {
		t.Errorf("written policy %+v, %v", policy, err)
	}
}

func request(service, resource, user string) *ads.RequestContext {
	return &ads.RequestContext{
		Subject:     &ads.Subject{Principals: []*ads.Principal{{Type: ads.PRINCIPAL_TYPE_USER, Name: user}}},
		ServiceName: service, Resource: resource, Action: "get",
	}
}

func testDiscover(t *testing.T, m store.DiscoverRequestManager) {
	must(t, m.SaveDiscoverRequest(request("app", "r1", "alice")))
	must(t, m.SaveDiscoverRequest(request("app", "r2", "bob")))
	must(t, m.SaveDiscoverRequest(request("other", "r3", "alice")))

	all, revision, err := m.GetDiscoverRequests("")
	must(t, err)
	if len(all) != 3 || revision == 0 {
		t.Errorf("%d requests, revision %d", len(all), revision)
	}
	app, _, err := m.GetDiscoverRequests("app")
	must(t, err)
	if len(app) != 2 {
		t.Errorf("%d app requests", len(app))
	}
	last, lastRevision, err := m.GetLastDiscoverRequest("app")
	must(t, err)
	if last.Resource != "r2" || lastRevision == 0 {
		t.Errorf("last request %+v at %d", last, lastRevision)
	}
	if last, _, err := m.GetLastDiscoverRequest(""); err != nil || last.Resource != "r3" {
		t.Errorf("last request of all services %+v, %v", last, err)
	}

	must(t, m.SaveDiscoverRequest(request("app", "r4", "carol")))
	since, newRevision, err := m.GetDiscoverRequestsSinceRevision("app", lastRevision)
	must(t, err)
	if len(since) != 1 || since[0].Resource != "r4" || newRevision <= lastRevision {
		t.Errorf("since %d: %d requests, revision %d", lastRevision, len(since), newRevision)
	}

	services, _, err := m.GeneratePolicies("app", "", "", "")
	must(t, err)
	if services["app"] == nil || len(services["app"].Policies) == 0 {
		t.Errorf("generated %+v", services)
	}
	services, _, err = m.GeneratePolicies("app", ads.PRINCIPAL_TYPE_USER, "alice", "")
	must(t, err)
	if app := services["app"]; app == nil || len(app.Policies) != 1 {
		t.Errorf("generated for alice %+v", services)
	}

	must(t, m.ResetDiscoverRequests("app"))
	if _, _, err := m.GetLastDiscoverRequest("app"); err == nil {
		t.Error("app requests survived a reset")
	}
	if other, _, err := m.GetDiscoverRequests("other"); err != nil || len(other) != 1 {
		t.Errorf("other requests after resetting app: %d, %v", len(other), err)
	}
	must(t, m.ResetDiscoverRequests(""))
	if all, _, _ := m.GetDiscoverRequests(""); len(all) != 0 {
		t.Errorf("%d requests survived a full reset", len(all))
	}
}

// RunUpdateConformance checks updating policies and role policies in place.
// RunConformance runs it too; stores whose other behaviour differs from the
// shared suite can run it alone.
func RunUpdateConformance(t *testing.T, newStore func(t *testing.T) pms.PolicyStoreManager) {
	t.Run("UpdatePolicies", func(t *testing.T) { testUpdatePolicies(t, newStore(t)) })
	t.Run("UpdateRolePolicies", func(t *testing.T) { testUpdateRolePolicies(t, newStore(t)) })
}

func testUpdatePolicies(t *testing.T, s pms.PolicyStoreManager) {
	must(t, s.CreateService(&pms.Service{Name: "app"}))
	alpha, err := s.CreatePolicy("app", policyNamed("alpha"))
	must(t, err)
	beta, err := s.CreatePolicy("app", policyNamed("beta"))
	must(t, err)

	changed := policyNamed("alpha2")
	changed.ID = alpha.ID
	changed.Effect = pms.Deny
	changed.Permissions[0].Actions = []string{"get", "put"}
	updated, err := s.UpdatePolicy("app", changed)
	must(t, err)
	if updated.ID != alpha.ID || updated.Name != "alpha2" {
		t.Errorf("updated %+v", updated)
	}
	got, err := s.GetPolicy("app", alpha.ID)
	must(t, err)
	if got.Name != "alpha2" || got.Effect != pms.Deny || len(got.Permissions[0].Actions) != 2 {
		t.Errorf("after update got %+v", got)
	}
	other, err := s.GetPolicy("app", beta.ID)
	must(t, err)
	if other.Name != "beta" {
		t.Errorf("the other policy changed: %+v", other)
	}
	if count, _ := s.GetPolicyCount("app"); count != 2 {
		t.Errorf("app has %d policies after an update", count)
	}

	missing := policyNamed("x")
	missing.ID = "nope"
	_, err = s.UpdatePolicy("app", missing)
	wantCode(t, "updating a missing policy", err, errors.EntityNotFound)
	changed.ID = alpha.ID
	_, err = s.UpdatePolicy("missing", changed)
	wantCode(t, "updating a policy of a missing service", err, errors.EntityNotFound)
}

func testUpdateRolePolicies(t *testing.T, s pms.PolicyStoreManager) {
	must(t, s.CreateService(&pms.Service{Name: "app"}))
	alpha, err := s.CreateRolePolicy("app", rolePolicyNamed("alpha"))
	must(t, err)
	beta, err := s.CreateRolePolicy("app", rolePolicyNamed("beta"))
	must(t, err)

	changed := rolePolicyNamed("alpha2")
	changed.ID = alpha.ID
	changed.Roles = []string{"auditor"}
	changed.Principals = []string{"group:ops"}
	updated, err := s.UpdateRolePolicy("app", changed)
	must(t, err)
	if updated.ID != alpha.ID || updated.Name != "alpha2" {
		t.Errorf("updated %+v", updated)
	}
	got, err := s.GetRolePolicy("app", alpha.ID)
	must(t, err)
	if got.Name != "alpha2" || got.Roles[0] != "auditor" || got.Principals[0] != "group:ops" {
		t.Errorf("after update got %+v", got)
	}
	other, err := s.GetRolePolicy("app", beta.ID)
	must(t, err)
	if other.Name != "beta" {
		t.Errorf("the other role policy changed: %+v", other)
	}
	if count, _ := s.GetRolePolicyCount("app"); count != 2 {
		t.Errorf("app has %d role policies after an update", count)
	}

	missing := rolePolicyNamed("x")
	missing.ID = "nope"
	_, err = s.UpdateRolePolicy("app", missing)
	wantCode(t, "updating a missing role policy", err, errors.EntityNotFound)
	changed.ID = alpha.ID
	_, err = s.UpdateRolePolicy("missing", changed)
	wantCode(t, "updating a role policy of a missing service", err, errors.EntityNotFound)
}
