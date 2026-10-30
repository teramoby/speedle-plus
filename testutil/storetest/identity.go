package storetest

import (
	"reflect"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func testGlobalPolicyIDs(t *testing.T, s pms.PolicyStoreManager) {
	seed := &pms.PolicyStore{Services: []*pms.Service{
		{Name: "app", Policies: []*pms.Policy{{Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "p1", Effect: pms.Grant}}},
		{Name: "global", RolePolicies: []*pms.RolePolicy{{Principals: []string{"user:alice"}, ID: "rp1", Effect: pms.Deny, Roles: []string{"reader"}}}},
	}}
	must(t, s.WritePolicyStore(seed))
	before, err := s.ReadPolicyStore()
	must(t, err)
	for _, bad := range []*pms.PolicyStore{
		{Services: []*pms.Service{{Name: "a", Policies: []*pms.Policy{{Effect: pms.Grant, Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "same"}, {Effect: pms.Grant, Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "same"}}}}},
		{Services: []*pms.Service{{Name: "a", Policies: []*pms.Policy{{Effect: pms.Grant, Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "same"}}}, {Name: "b", Policies: []*pms.Policy{{Effect: pms.Grant, Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "same"}}}}},
		{Services: []*pms.Service{{Name: "app", RolePolicies: []*pms.RolePolicy{{Effect: pms.Grant, Roles: []string{"reader"}, Principals: []string{"user:alice"}, ID: "same"}}}, {Name: "global", RolePolicies: []*pms.RolePolicy{{Effect: pms.Grant, Roles: []string{"reader"}, Principals: []string{"user:alice"}, ID: "same"}}}}},
		{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{{Effect: pms.Grant, Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "same"}}}, {Name: "global", RolePolicies: []*pms.RolePolicy{{Effect: pms.Grant, Roles: []string{"reader"}, Principals: []string{"user:alice"}, ID: "same"}}}}},
	} {
		wantCode(t, "duplicate snapshot IDs", s.WritePolicyStore(bad), errors.InvalidRequest)
		after, err := s.ReadPolicyStore()
		must(t, err)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("invalid snapshot changed stored data")
		}
	}
	missing := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{{Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "fixed", Effect: pms.Grant}, {Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, Effect: pms.Grant}}, RolePolicies: []*pms.RolePolicy{{Principals: []string{"user:alice"}, Effect: pms.Grant, Roles: []string{"reader"}}}}}}
	must(t, s.WritePolicyStore(missing))
	if missing.Services[0].Policies[1].ID != "" || missing.Services[0].RolePolicies[0].ID != "" {
		t.Fatal("snapshot import mutated caller IDs")
	}
	seen := map[string]bool{}
	check := func(id string) {
		t.Helper()
		if id == "" || seen[id] {
			t.Fatalf("missing or repeated ID %q", id)
		}
		seen[id] = true
	}
	ps, err := s.ReadPolicyStore()
	must(t, err)
	fixed := false
	for _, service := range ps.Services {
		for _, policy := range service.Policies {
			check(policy.ID)
			fixed = fixed || policy.ID == "fixed"
		}
		for _, policy := range service.RolePolicies {
			check(policy.ID)
		}
	}
	if !fixed {
		t.Fatal("snapshot import replaced a supplied ID")
	}
	// Creation always generates fresh IDs, including when callers supply one.
	for _, name := range []string{"a", "b"} {
		must(t, s.CreateService(&pms.Service{Name: name, Policies: []*pms.Policy{{Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "caller", Effect: pms.Grant}}, RolePolicies: []*pms.RolePolicy{{Principals: []string{"user:alice"}, ID: "caller", Effect: pms.Grant, Roles: []string{"r"}}}}))
		service, err := s.GetService(name)
		must(t, err)
		check(service.Policies[0].ID)
		check(service.RolePolicies[0].ID)
		policy, err := s.CreatePolicy(name, &pms.Policy{Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "caller", Effect: pms.Grant})
		must(t, err)
		check(policy.ID)
		role, err := s.CreateRolePolicy(name, &pms.RolePolicy{Principals: []string{"user:alice"}, ID: "caller", Effect: pms.Grant, Roles: []string{"r"}})
		must(t, err)
		check(role.ID)
	}
	if seen["caller"] {
		t.Fatal("creation retained a caller-supplied ID")
	}
}
