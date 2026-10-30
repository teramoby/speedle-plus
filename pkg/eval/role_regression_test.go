package eval

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"slices"
	"testing"
	"time"

	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
)

func roleRegressionPolicy(id, effect string, principals []string, roles ...string) *pms.RolePolicy {
	return &pms.RolePolicy{ID: id, Name: id, Effect: effect, Principals: principals, Roles: roles}
}
func roleRegressionPermission(role string) *pms.Policy {
	p := featurePolicy("permission", "")
	p.Principals = [][]string{{"role:" + role}}
	return p
}
func roleRegressionEvaluator(t *testing.T, target string, rp ...*pms.RolePolicy) *PolicyEvalImpl {
	return featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{roleRegressionPermission(target)}, RolePolicies: rp}}})
}

// Check the same role outcome through every embedded decision API.
func assertRoleRegression(t *testing.T, e *PolicyEvalImpl, ctx ads.RequestContext, roles []string, want bool) {
	t.Helper()
	got, err := e.GetAllGrantedRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	expected := slices.Clone(roles)
	slices.Sort(expected)
	if !slices.Equal(got, expected) {
		t.Fatalf("roles %v want %v", got, expected)
	}
	allowed, reason, err := e.IsAllowed(ctx)
	if err != nil || allowed != want {
		t.Fatalf("decision %v %v %v want %v", allowed, reason, err, want)
	}
	diagnostic, err := e.Diagnose(ctx)
	if err != nil || diagnostic.Allowed != want || diagnostic.Reason != reason {
		t.Fatalf("diagnose %+v %v", diagnostic, err)
	}
	slices.Sort(diagnostic.GrantedRoles)
	if !slices.Equal(diagnostic.GrantedRoles, expected) {
		t.Fatalf("diagnostic roles %v want %v", diagnostic.GrantedRoles, expected)
	}
	permissions, err := e.GetAllGrantedPermissions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, permission := range permissions {
		listed = listed || (permission.Resource == ctx.Resource && slices.Contains(permission.Actions, ctx.Action))
	}
	if listed != want {
		t.Fatalf("permissions %+v cover request=%v want %v", permissions, listed, want)
	}
}

func TestFeatureRoleUniversalDeny(t *testing.T) {
	for _, public := range []bool{false, true} {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprintf("public=%v/blocked=%v", public, blocked), func(t *testing.T) {
				principals := []string{"user:alice"}
				if public {
					principals = nil
				}
				d := roleRegressionPolicy("deny", pms.Deny, nil, "reader")
				d.Condition = "blocked"
				e := roleRegressionEvaluator(t, "reader", roleRegressionPolicy("grant", pms.Grant, principals, "reader"), d)
				ctx := featureContext()
				if public {
					ctx.Subject = nil
				}
				ctx.Attributes = map[string]interface{}{"blocked": blocked}
				roles := []string{}
				if !blocked {
					roles = []string{"reader"}
				}
				assertRoleRegression(t, e, ctx, roles, !blocked)
			})
		}
	}
}

func TestFeatureRoleUniversalGrantSurvivesUnrelatedDeny(t *testing.T) {
	control := roleRegressionEvaluator(t, "public", roleRegressionPolicy("public", pms.Grant, nil, "public"))
	if ok, _, err := control.IsAllowed(featureContext()); !ok || err != nil {
		t.Fatalf("control public grant failed: %v %v", ok, err)
	}
	e := roleRegressionEvaluator(t, "public",
		roleRegressionPolicy("public", pms.Grant, nil, "public"),
		roleRegressionPolicy("roots", pms.Grant, []string{"user:alice"}, "a", "b"),
		roleRegressionPolicy("deny", pms.Deny, []string{"role:a"}, "b"))
	assertRoleRegression(t, e, featureContext(), []string{"a", "public"}, true)
}

func TestFeatureRoleDiamondDeny(t *testing.T) {
	e := roleRegressionEvaluator(t, "leaf",
		roleRegressionPolicy("roots", pms.Grant, []string{"user:alice"}, "root", "blocker"),
		roleRegressionPolicy("a", pms.Grant, []string{"role:root"}, "middle", "join"),
		roleRegressionPolicy("b", pms.Grant, []string{"role:middle"}, "join"),
		roleRegressionPolicy("c", pms.Grant, []string{"role:join"}, "leaf"),
		roleRegressionPolicy("d", pms.Deny, []string{"role:blocker"}, "root"))
	for i := 0; i < 200; i++ {
		assertRoleRegression(t, e, featureContext(), []string{"blocker"}, false)
	}
}

func TestFeatureRoleOrphanCycleAfterDeny(t *testing.T) {
	for _, independent := range []bool{false, true} {
		t.Run(fmt.Sprint(independent), func(t *testing.T) {
			policies := []*pms.RolePolicy{
				roleRegressionPolicy("roots", pms.Grant, []string{"user:alice"}, "entry", "blocker"),
				roleRegressionPolicy("start", pms.Grant, []string{"role:entry"}, "cycleA"),
				roleRegressionPolicy("cycle1", pms.Grant, []string{"role:cycleA"}, "cycleB"),
				roleRegressionPolicy("cycle2", pms.Grant, []string{"role:cycleB"}, "cycleA"),
				roleRegressionPolicy("deny", pms.Deny, []string{"role:blocker"}, "entry"),
			}
			want := []string{"blocker"}
			if independent {
				policies = append(policies, roleRegressionPolicy("independent", pms.Grant, []string{"user:alice"}, "cycleB"))
				want = append(want, "cycleA", "cycleB")
			}
			e := roleRegressionEvaluator(t, "cycleB", policies...)
			assertRoleRegression(t, e, featureContext(), want, independent)
		})
	}
}

func TestFeatureRoleCyclicDenier(t *testing.T) {
	e := roleRegressionEvaluator(t, "victim",
		roleRegressionPolicy("roots", pms.Grant, []string{"user:alice"}, "entry", "victim"),
		roleRegressionPolicy("start", pms.Grant, []string{"role:entry"}, "cycleA"),
		roleRegressionPolicy("cycle1", pms.Grant, []string{"role:cycleA"}, "cycleB"),
		roleRegressionPolicy("cycle2", pms.Grant, []string{"role:cycleB"}, "cycleA"),
		roleRegressionPolicy("deny", pms.Deny, []string{"role:cycleB"}, "victim"))
	for i := 0; i < 100; i++ {
		allowed, _, err := e.IsAllowed(featureContext())
		if err != nil || allowed {
			t.Fatalf("iteration=%d allowed=%v err=%v", i, allowed, err)
		}
	}
}

func TestFeatureRolePublicCyclicDenier(t *testing.T) {
	if os.Getenv("SPEEDLE_ROLE_CYCLE_CHILD") != "1" {
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, "-test.run=^TestFeatureRolePublicCyclicDenier$", "-test.timeout=25s")
		command.Env = append(os.Environ(), "SPEEDLE_ROLE_CYCLE_CHILD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("cycle subprocess: %v: %s", err, output)
		}
		return
	}
	// A regression must fail this child quickly instead of exhausting the
	// runner's memory. Stack overflow cannot be recovered in the parent.
	previous := debug.SetMaxStack(512 << 10)
	defer debug.SetMaxStack(previous)
	e := roleRegressionEvaluator(t, "victim",
		roleRegressionPolicy("public", pms.Grant, nil, "cycleA"),
		roleRegressionPolicy("root", pms.Grant, []string{"user:alice"}, "victim"),
		roleRegressionPolicy("cycle1", pms.Grant, []string{"role:cycleA"}, "cycleB"),
		roleRegressionPolicy("cycle2", pms.Grant, []string{"role:cycleB"}, "cycleA"),
		roleRegressionPolicy("deny", pms.Deny, []string{"role:cycleB"}, "victim"))
	assertRoleRegression(t, e, featureContext(), []string{"cycleA", "cycleB"}, false)
}

func TestFeatureRoleConditionINNegative(t *testing.T) {
	for _, expression := range []string{"x in (x,x)", "x in ys"} {
		t.Run(expression, func(t *testing.T) {

			e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{featurePolicy("p", expression)}}}})
			ctx := featureContext()
			ctx.Attributes = map[string]interface{}{"x": []interface{}{"a", "b"}, "ys": []interface{}{[]interface{}{"a", "b"}}}
			allowed, reason, err := e.IsAllowed(ctx)
			if allowed || reason != ads.NO_APPLICABLE_POLICIES || err != nil {
				t.Fatalf("invalid IN operand: %v %v %v", allowed, reason, err)
			}
		})
	}
}

func TestFeatureRoleConditionalChainMatrix(t *testing.T) {
	first := roleRegressionPolicy("r1", pms.Grant, []string{"user:alice"}, "entry")
	first.Condition = "eligible && score > 0"
	second := roleRegressionPolicy("r2", pms.Grant, []string{"role:entry"}, "reader")
	second.Condition = "region == 'cn'"
	deny := roleRegressionPolicy("r3", pms.Deny, []string{"user:alice"}, "reader")
	deny.Condition = "blocked"
	policy := roleRegressionPermission("reader")
	policy.Condition = "amount < 100 && (approved || emergency)"
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{policy}, RolePolicies: []*pms.RolePolicy{first, second, deny}}}})
	for mask := 0; mask < 128; mask++ {
		bit := func(i int) bool { return mask&(1<<i) != 0 }
		region := "us"
		if bit(2) {
			region = "cn"
		}
		score := -1.0
		if bit(1) {
			score = 1
		}
		amount := 1000.0
		if bit(4) {
			amount = 1
		}
		ctx := featureContext()
		ctx.Attributes = map[string]interface{}{"eligible": bit(0), "score": score, "region": region, "blocked": bit(3), "amount": amount, "approved": bit(5), "emergency": bit(6)}
		want := bit(0) && bit(1) && bit(2) && !bit(3) && bit(4) && (bit(5) || bit(6))
		got, _, err := e.IsAllowed(ctx)
		if got != want || err != nil {
			t.Fatalf("mask %07b got=%v want=%v err=%v", mask, got, want, err)
		}
	}
	// Condition failures on a role grant are nonmatching, as documented.
	for _, attrs := range []map[string]interface{}{nil, {"eligible": true}, {"eligible": true, "score": "bad"}, {"eligible": true, "score": 1.0, "region": []interface{}{"cn"}}} {
		ctx := featureContext()
		ctx.Attributes = attrs
		if got, _, err := e.IsAllowed(ctx); got || err != nil {
			t.Errorf("attrs=%v got=%v err=%v", attrs, got, err)
		}
	}
	// Alternate ordinary grant survives a false or erroneous deny condition.
	grant := featurePolicy("grant", "")
	d := featurePolicy("deny", "blocked")
	d.Effect = pms.Deny
	fallback := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{grant, d}}}})
	for _, attrs := range []map[string]interface{}{nil, {"blocked": "bad"}, {"blocked": false}, {"blocked": true}} {
		ctx := featureContext()
		ctx.Attributes = attrs
		want := attrs == nil || attrs["blocked"] != true
		got, _, err := fallback.IsAllowed(ctx)
		if got != want || err != nil {
			t.Errorf("deny attrs=%v got=%v want=%v err=%v", attrs, got, want, err)
		}
	}
}

func TestFeatureRoleGlobalIDDConditionMatrix(t *testing.T) {
	root := roleRegressionPolicy("global-root", pms.Grant, []string{"idd=corp:user:alice"}, "entry")
	root.Condition = "eligible"
	d := roleRegressionPolicy("global-deny", pms.Deny, []string{"user:alice"}, "reader")
	d.Condition = "blocked"
	next := roleRegressionPolicy("local-chain", pms.Grant, []string{"role:entry"}, "reader")
	next.Condition = "localgate"
	next.ResourceExpressions = []string{"^book/[0-9]+$"}
	grant := featurePolicy("role-grant", "amount < 100")
	grant.Principals = [][]string{{"role:reader", "group:dev", "group:prod"}}
	grant.Permissions[0].Resource = "book/1"
	alternative := featurePolicy("alternative", "emergency")
	alternative.Principals = [][]string{{"user:alice"}}
	alternative.Permissions[0].Resource = "book/1"
	revoke := featurePolicy("revoke", "revoked")
	revoke.Effect = pms.Deny
	revoke.Permissions[0].Resource = "book/1"
	local := &pms.Service{Name: "app", Policies: []*pms.Policy{grant, alternative, revoke}, RolePolicies: []*pms.RolePolicy{next}}
	global := &pms.Service{Name: "global", RolePolicies: []*pms.RolePolicy{root, d}}
	for order := 0; order < 2; order++ {
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{local, global}})
		for mask := 0; mask < 1024; mask++ {
			bit := func(i int) bool { return mask&(1<<i) != 0 }
			ctx := featureContext()
			ctx.Resource = "other"
			if bit(3) {
				ctx.Resource = "book/1"
			}
			ctx.Subject.Principals[0].IDD = "other"
			if bit(0) {
				ctx.Subject.Principals[0].IDD = "corp"
			}
			if bit(4) {
				ctx.Subject.Principals = append(ctx.Subject.Principals, &ads.Principal{Type: "group", Name: "dev"})
			}
			if bit(5) {
				ctx.Subject.Principals = append(ctx.Subject.Principals, &ads.Principal{Type: "group", Name: "prod"})
			}
			amount := 1000.0
			if bit(6) {
				amount = 1
			}
			ctx.Attributes = map[string]interface{}{"eligible": bit(1), "localgate": bit(2), "amount": amount, "blocked": bit(7), "revoked": bit(8), "emergency": bit(9)}
			want := bit(3) && !bit(8) && (bit(9) || (bit(0) && bit(1) && bit(2) && bit(4) && bit(5) && bit(6) && !bit(7)))
			got, _, err := e.IsAllowed(ctx)
			diagnostic, diagErr := e.Diagnose(ctx)
			if got != want || err != nil || diagErr != nil || diagnostic.Allowed != want {
				t.Fatalf("order %d mask %010b got=%v diag=%v want=%v err=%v/%v", order, mask, got, diagnostic.Allowed, want, err, diagErr)
			}
		}
		slices.Reverse(local.Policies)
		slices.Reverse(global.RolePolicies)
	}
}
