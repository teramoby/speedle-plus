package storetest

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// Every backend must reject invalid subjects before any data or IDs change.
func testRolePolicyPrincipals(t *testing.T, s pms.PolicyStoreManager) {
	must(t, s.CreateService(&pms.Service{Name: "app", RolePolicies: []*pms.RolePolicy{{Effect: pms.Grant, Principals: []string{"user:alice"}, Roles: []string{"reader"}}}}))
	before, err := s.ReadPolicyStore()
	must(t, err)
	for _, effect := range []string{pms.Grant, pms.Deny} {
		for i, principals := range [][]string{nil, {}, {""}, {" \t"}, {"user:alice", ""}} {
			t.Run(fmt.Sprintf("%s/%d", effect, i), func(t *testing.T) {
				invalid := &pms.RolePolicy{ID: "unchanged", Effect: effect, Principals: principals, Roles: []string{"reader"}}
				_, err := s.CreateRolePolicy("app", invalid)
				wantCode(t, "role creation", err, errors.InvalidRequest)
				wantCode(t, "nested creation", s.CreateService(&pms.Service{Name: "bad", RolePolicies: []*pms.RolePolicy{invalid}}), errors.InvalidRequest)
				wantCode(t, "snapshot replacement", s.WritePolicyStore(&pms.PolicyStore{Services: []*pms.Service{{Name: "bad", RolePolicies: []*pms.RolePolicy{invalid}}}}), errors.InvalidRequest)
				after, err := s.ReadPolicyStore()
				must(t, err)
				if !reflect.DeepEqual(before, after) || invalid.ID != "unchanged" {
					t.Fatal("invalid role rule changed data or caller IDs")
				}
			})
		}
	}
	_, err = s.CreateRolePolicy("app", nil)
	wantCode(t, "null role creation", err, errors.InvalidRequest)
}
