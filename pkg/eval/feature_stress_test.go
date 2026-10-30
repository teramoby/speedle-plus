//go:build feature_stress

package eval

import (
	"fmt"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"os"
	"slices"
	"strconv"
	"testing"
)

func TestFeatureLargePolicyRoleSets(t *testing.T) {
	maximum := 10000
	if raw := os.Getenv("SPEEDLE_STRESS_POLICIES"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 10000 || n > 100000 {
			t.Fatalf("invalid stress size %q", raw)
		}
		maximum = n
	}
	for _, size := range []int{1000, maximum} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			s := &pms.Service{Name: "app"}
			for i := 0; i < size; i++ {
				id := fmt.Sprint(i)
				p := featurePolicy("p"+id, "amount < 100 && risk < 50")
				p.Principals = [][]string{{"role:r" + id}}
				p.Permissions[0].Resource = "book" + id
				s.Policies = append(s.Policies, p)
				s.RolePolicies = append(s.RolePolicies, &pms.RolePolicy{ID: "r" + id, Effect: pms.Grant, Principals: []string{"user:u" + id}, Roles: []string{"r" + id}})
			}
			e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{s}})
			cached := e.RuntimePolicyStore.RuntimeServices["app"]
			if len(cached.PoliciesCache.PolicyMap) != size || len(cached.RolePoliciesCache.PolicyMap) != size {
				t.Fatalf("snapshot truncated: policies=%d roles=%d want=%d", len(cached.PoliciesCache.PolicyMap), len(cached.RolePoliciesCache.PolicyMap), size)
			}
			for _, i := range []int{0, size / 2, size - 1} {
				for _, permitted := range []bool{true, false} {
					ctx := featureContext()
					ctx.Resource = "book" + fmt.Sprint(i)
					ctx.Subject = &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "u" + fmt.Sprint(i)}}}
					amount := 1.0
					if !permitted {
						amount = 1000
					}
					ctx.Attributes = map[string]interface{}{"amount": amount, "risk": 1.0}
					for repeat := 0; repeat < 2; repeat++ {
						got, _, err := e.IsAllowed(ctx)
						if got != permitted || err != nil {
							t.Fatalf("size=%d user=%d permitted=%v got=%v err=%v", size, i, permitted, got, err)
						}
					}
				}
			}
		})
	}
}

func TestFeatureRoleMixedGraphCapacity(t *testing.T) {
	// Every component contains a diamond, a conditional denial of its only
	// entry, and an independent explicit-user grant. Check every role and permission.
	for _, size := range []int{128, 1024} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			service := &pms.Service{Name: "app", RolePolicies: []*pms.RolePolicy{roleRegressionPolicy("blocker", pms.Grant, []string{"user:alice"}, "blocker")}}
			for i := 0; i < size; i++ {
				n := fmt.Sprint(i)
				service.RolePolicies = append(service.RolePolicies,
					roleRegressionPolicy("root-"+n, pms.Grant, []string{"user:alice"}, "root"+n),
					roleRegressionPolicy("independent-"+n, pms.Grant, []string{"user:alice"}, "independent"+n),
					roleRegressionPolicy("diamond-"+n, pms.Grant, []string{"role:root" + n}, "middle"+n, "join"+n),
					roleRegressionPolicy("join-"+n, pms.Grant, []string{"role:middle" + n}, "join"+n),
					roleRegressionPolicy("leaf-"+n, pms.Grant, []string{"role:join" + n}, "leaf"+n),
					&pms.RolePolicy{ID: "deny-" + n, Effect: pms.Deny, Principals: []string{"role:blocker"}, Roles: []string{"root" + n}, Condition: "blocked"})
				for _, kind := range []string{"independent", "leaf"} {
					policy := featurePolicy("p-"+kind+n, "amount < 100")
					policy.Principals = [][]string{{"role:" + kind + n}}
					policy.Permissions[0].Resource = kind + n
					service.Policies = append(service.Policies, policy)
				}
			}
			e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{service}})
			cached := e.RuntimePolicyStore.RuntimeServices["app"]
			if len(cached.PoliciesCache.PolicyMap) != len(service.Policies) || len(cached.RolePoliciesCache.PolicyMap) != len(service.RolePolicies) {
				t.Fatalf("mixed snapshot truncated: policies=%d/%d role policies=%d/%d", len(cached.PoliciesCache.PolicyMap), len(service.Policies), len(cached.RolePoliciesCache.PolicyMap), len(service.RolePolicies))
			}
			for _, blocked := range []bool{false, true} {
				ctx := featureContext()
				ctx.Attributes = map[string]interface{}{"blocked": blocked, "amount": 1.0}
				wantRoles := []string{"blocker"}
				wantResources := map[string]bool{}
				for i := 0; i < size; i++ {
					n := fmt.Sprint(i)
					wantRoles = append(wantRoles, "independent"+n)
					wantResources["independent"+n] = true
					if !blocked {
						wantRoles = append(wantRoles, "root"+n, "middle"+n, "join"+n, "leaf"+n)
						wantResources["leaf"+n] = true
					}
				}
				roles, err := e.GetAllGrantedRoles(ctx)
				slices.Sort(roles)
				slices.Sort(wantRoles)
				if err != nil || !slices.Equal(roles, wantRoles) {
					t.Fatalf("size=%d blocked=%v roles=%d want=%d err=%v", size, blocked, len(roles), len(wantRoles), err)
				}
				permissions, err := e.GetAllGrantedPermissions(ctx)
				if err != nil || len(permissions) != len(wantResources) {
					t.Fatalf("size=%d blocked=%v permissions=%d want=%d err=%v", size, blocked, len(permissions), len(wantResources), err)
				}
				for _, permission := range permissions {
					if !wantResources[permission.Resource] || !slices.Equal(permission.Actions, []string{"read"}) {
						t.Fatalf("unexpected permission %+v", permission)
					}
				}
				for _, i := range []int{0, size / 2, size - 1} {
					for _, kind := range []string{"independent", "leaf"} {
						ctx.Resource = kind + fmt.Sprint(i)
						assertRoleRegression(t, e, ctx, wantRoles, wantResources[ctx.Resource])
					}
				}
			}
		})
	}
}
