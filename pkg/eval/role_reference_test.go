package eval

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
)

func TestFeatureRoleMixedGraphReference(t *testing.T) {
	// Both grant and deny dependencies point to higher-numbered roles. This
	// gives an independent topological oracle, without copying the evaluator's
	// graph pruning or its handling of mutually dependent denies.
	type rule struct {
		from, to, gate int
		deny           bool
	}
	const nodes = 24
	for _, seed := range []int64{18, 120, 214, 421, 532, 919, 2026, 7319} {
		rng := rand.New(rand.NewSource(seed))
		rules := []rule{{from: -1, to: 0, gate: -1}, {from: -1, to: 1, gate: 0}}
		for to := 1; to < nodes; to++ {
			if rng.Intn(4) == 0 {
				rules = append(rules, rule{from: -1, to: to, gate: rng.Intn(4), deny: rng.Intn(3) == 0})
			}
			for from := 0; from < to; from++ {
				if rng.Intn(4) == 0 {
					rules = append(rules, rule{from: from, to: to, gate: rng.Intn(4), deny: rng.Intn(3) == 0})
				}
			}
		}
		local, global := &pms.Service{Name: "app"}, &pms.Service{Name: "global"}
		for i, rule := range rules {
			principals := []string{"user:alice"}
			if rule.from >= 0 {
				principals = []string{fmt.Sprintf("role:r%d", rule.from)}
			}
			effect := pms.Grant
			if rule.deny {
				effect = pms.Deny
			}
			policy := roleRegressionPolicy(fmt.Sprintf("rule%d", i), effect, principals, fmt.Sprintf("r%d", rule.to))
			if rule.gate >= 0 {
				policy.Condition = fmt.Sprintf("gate%d", rule.gate)
			}
			service := local
			if i%2 == 0 {
				service = global
			}
			service.RolePolicies = append(service.RolePolicies, policy)
		}
		for i := 0; i < nodes; i++ {
			policy := featurePolicy(fmt.Sprintf("p%d", i), "amount < 100")
			policy.Principals = [][]string{{fmt.Sprintf("role:r%d", i)}}
			policy.Permissions[0].Resource = fmt.Sprintf("book%d", i)
			local.Policies = append(local.Policies, policy)
		}
		for order := 0; order < 2; order++ {
			e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{local, global}})
			for mask := 0; mask < 16; mask++ {
				expected := make([]bool, nodes)
				for to := range expected {
					grant, deny := false, false
					for _, rule := range rules {
						if rule.to != to || (rule.gate >= 0 && mask&(1<<rule.gate) == 0) {
							continue
						}
						if rule.from < 0 || expected[rule.from] {
							grant = grant || !rule.deny
							deny = deny || rule.deny
						}
					}
					expected[to] = grant && !deny
				}
				ctx := featureContext()
				ctx.Attributes = map[string]interface{}{"amount": 1.0}
				for gate := 0; gate < 4; gate++ {
					ctx.Attributes[fmt.Sprintf("gate%d", gate)] = mask&(1<<gate) != 0
				}
				if mask&8 != 0 {
					ctx.Attributes["amount"] = 1000.0
				}
				wantRoles := []string{}
				wantPermissions := map[string]bool{}
				for i, granted := range expected {
					if granted {
						wantRoles = append(wantRoles, fmt.Sprintf("r%d", i))
						if mask&8 == 0 {
							wantPermissions[fmt.Sprintf("book%d", i)] = true
						}
					}
				}
				roles, err := e.GetAllGrantedRoles(ctx)
				slices.Sort(roles)
				slices.Sort(wantRoles)
				if err != nil || !slices.Equal(roles, wantRoles) {
					t.Fatalf("seed=%d order=%d mask=%04b roles=%v want=%v err=%v", seed, order, mask, roles, wantRoles, err)
				}
				permissions, err := e.GetAllGrantedPermissions(ctx)
				if err != nil || len(permissions) != len(wantPermissions) {
					t.Fatalf("seed=%d mask=%04b permissions=%v want=%v err=%v", seed, mask, permissions, wantPermissions, err)
				}
				for _, permission := range permissions {
					if !wantPermissions[permission.Resource] || !slices.Equal(permission.Actions, []string{"read"}) {
						t.Fatalf("unexpected permission %+v", permission)
					}
				}
				for _, i := range []int{0, nodes / 2, nodes - 1} {
					ctx.Resource = fmt.Sprintf("book%d", i)
					assertRoleRegression(t, e, ctx, wantRoles, wantPermissions[ctx.Resource])
				}
			}
			slices.Reverse(local.Policies)
			slices.Reverse(local.RolePolicies)
			slices.Reverse(global.RolePolicies)
		}
	}
}
