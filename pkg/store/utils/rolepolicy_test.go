package utils

import (
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func TestRolePolicyRequiresPrincipals(t *testing.T) {
	if err := ValidateRolePolicyPrincipals(nil); errors.Code(err) != errors.InvalidRequest {
		t.Fatalf("null rule: %v", err)
	}
	for _, effect := range []string{pms.Grant, pms.Deny} {
		for _, principals := range [][]string{nil, {}, {""}, {" \t"}, {"user:alice", ""}} {
			policy := &pms.RolePolicy{Effect: effect, Principals: principals, Roles: []string{"reader"}}
			ps := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", RolePolicies: []*pms.RolePolicy{policy}}}}
			if _, err := PreparePolicyStore(ps); errors.Code(err) != errors.InvalidRequest || !strings.Contains(err.Error(), "principal") {
				t.Fatalf("effect=%s principals=%q: %v", effect, principals, err)
			}
			if policy.ID != "" {
				t.Fatal("invalid import mutated caller")
			}
		}
		for _, principals := range [][]string{{"user:alice"}, {"group:dev"}, {"entity:app"}, {"role:manager"}, {"idd=corp:user:alice"}} {
			if err := ValidateRolePolicyPrincipals(&pms.RolePolicy{Effect: effect, Principals: principals, Roles: []string{"reader"}}); err != nil {
				t.Fatalf("explicit principals %q: %v", principals, err)
			}
		}
	}
}
