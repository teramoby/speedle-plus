package utils

import (
	"reflect"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func TestSnapshotIdentity(t *testing.T) {
	ps := &pms.PolicyStore{Services: []*pms.Service{
		{Name: "app", Policies: []*pms.Policy{{ID: "fixed"}, {}}, RolePolicies: []*pms.RolePolicy{{}}},
		{Name: "global", RolePolicies: []*pms.RolePolicy{{ID: "role"}, {}}},
	}}
	prepared, err := PreparePolicyStore(ps)
	if err != nil {
		t.Fatal(err)
	}
	if ps.Services[0].Policies[1].ID != "" || ps.Services[0].RolePolicies[0].ID != "" || ps.Services[1].RolePolicies[1].ID != "" {
		t.Fatal("import changed caller IDs")
	}
	if prepared.Services[0].Policies[0].ID != "fixed" || prepared.Services[1].RolePolicies[0].ID != "role" {
		t.Fatal("import replaced existing IDs")
	}
	if err := ValidatePolicyIDs(prepared); err != nil {
		t.Fatal(err)
	}
	for _, service := range prepared.Services {
		for _, policy := range service.Policies {
			if policy.ID == "" {
				t.Fatal("missing policy ID")
			}
		}
		for _, policy := range service.RolePolicies {
			if policy.ID == "" {
				t.Fatal("missing role policy ID")
			}
		}
	}
}

func TestSnapshotDuplicateIDs(t *testing.T) {
	for _, roles := range []bool{false, true} {
		for _, sameKind := range []bool{false, true} {
			a, b := &pms.Service{Name: "app"}, &pms.Service{Name: "global"}
			add := func(service *pms.Service, role bool, id string) {
				if role {
					service.RolePolicies = append(service.RolePolicies, &pms.RolePolicy{ID: id})
				} else {
					service.Policies = append(service.Policies, &pms.Policy{ID: id})
				}
			}
			add(a, roles, "same")
			add(b, roles == sameKind, "same")
			_, err := PreparePolicyStore(&pms.PolicyStore{Services: []*pms.Service{a, b}})
			if errors.Code(err) != errors.InvalidRequest || !strings.Contains(err.Error(), `"same"`) || !strings.Contains(err.Error(), `"global"`) {
				t.Fatalf("role=%v sameKind=%v: %v", roles, sameKind, err)
			}
		}
	}
	// IDs remain case-sensitive.
	if err := ValidatePolicyIDs(&pms.PolicyStore{Services: []*pms.Service{{Policies: []*pms.Policy{{ID: "A"}, {ID: "a"}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePolicyIDs(&pms.PolicyStore{Services: []*pms.Service{{Policies: []*pms.Policy{{ID: string([]byte{0xff})}}}}}); errors.Code(err) != errors.InvalidRequest {
		t.Fatalf("invalid UTF-8 ID: %v", err)
	}
}

func TestSnapshotNullEntries(t *testing.T) {
	for _, ps := range []*pms.PolicyStore{nil, {Services: []*pms.Service{nil}}, {Services: []*pms.Service{{Policies: []*pms.Policy{nil}}}}, {Services: []*pms.Service{{RolePolicies: []*pms.RolePolicy{nil}}}}, {Functions: []*pms.Function{nil}}} {
		if _, err := PreparePolicyStore(ps); errors.Code(err) != errors.InvalidRequest {
			t.Fatalf("%#v: %v", ps, err)
		}
	}
	ps := &pms.PolicyStore{Services: []*pms.Service{nil, {Policies: []*pms.Policy{nil}, RolePolicies: []*pms.RolePolicy{nil}}}}
	if err := ValidatePolicyIDs(ps); err != nil {
		t.Fatal(err)
	}
	if got, err := PreparePolicyStore(&pms.PolicyStore{}); err != nil || !reflect.DeepEqual(got.Services, []*pms.Service{}) {
		t.Fatalf("empty store: %v %v", got, err)
	}
}
