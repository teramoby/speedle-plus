// Licensed under the Universal Permissive License (UPL) Version 1.0.

package eval

import (
	"reflect"
	"sort"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/condexpr"
)

const dropConditionTokensFixture = `{
	"services": [{
		"name": "svc",
		"policies": [
			{"id": "arith", "effect": "grant", "permissions": [{"resource": "/arith", "actions": ["get"]}], "condition": "a + 2 * 3 == 8 && b > 1"},
			{"id": "regex", "effect": "grant", "permissions": [{"resource": "/regex", "actions": ["get"]}], "condition": "s =~ '^ab' && s !~ 'z$'"},
			{"id": "ternary", "effect": "grant", "permissions": [{"resource": "/ternary", "actions": ["get"]}], "condition": "a > 1 ? flag : !flag"},
			{"id": "function", "effect": "grant", "permissions": [{"resource": "/function", "actions": ["get"]}], "condition": "Max(a, 2) == 3 && Sqrt(9) == 3"},
			{"id": "membership", "effect": "grant", "permissions": [{"resource": "/membership", "actions": ["get"]}], "condition": "'x' in arr && a in (1, 2, 3)"},
			{"id": "bitwise", "effect": "grant", "permissions": [{"resource": "/bitwise", "actions": ["get"]}], "condition": "(a & 2) == 2 && (a | 1) == 3 && (a ^ 3) == 1 && (a << 1) == 4 && (a >> 1) == 1 && (~a) == -3"},
			{"id": "missing", "effect": "grant", "permissions": [{"resource": "/missing", "actions": ["get"]}], "condition": "missing == 1"},
			{"id": "type-error", "effect": "grant", "permissions": [{"resource": "/type", "actions": ["get"]}], "condition": "a && flag"},
			{"id": "coalesce", "effect": "grant", "permissions": [{"resource": "/coalesce", "actions": ["get"]}], "condition": "(missing ?? a) == 2"},
			{"id": "elided", "effect": "grant", "permissions": [{"resource": "/elided", "actions": ["get"]}], "condition": "1 + 2 == 3"},
			{"id": "deny", "effect": "deny", "permissions": [{"resource": "/deny", "actions": ["get"]}], "condition": "a < 0"}
		],
		"rolePolicies": [
			{"id": "role", "effect": "grant", "roles": ["reader"], "principals": ["user:alice"], "resources": ["/role"], "condition": "flag"}
		]
	}]
}`

type conditionDecision struct {
	allowed bool
	reason  adsapi.Reason
	err     string
	roles   []string
}

func conditionDecisions(t *testing.T, evaluator InternalEvaluator) []conditionDecision {
	t.Helper()
	subject := &adsapi.Subject{Principals: []*adsapi.Principal{{Type: adsapi.PRINCIPAL_TYPE_USER, Name: "alice"}}}
	base := map[string]interface{}{"a": 2.0, "b": 2.0, "s": "abc", "flag": true, "arr": []interface{}{"x", "y"}}
	with := func(changes map[string]interface{}) map[string]interface{} {
		attributes := make(map[string]interface{}, len(base)+len(changes))
		for name, value := range base {
			attributes[name] = value
		}
		for name, value := range changes {
			attributes[name] = value
		}
		return attributes
	}
	cases := []struct {
		resource   string
		attributes map[string]interface{}
	}{
		{"/arith", base}, {"/arith", with(map[string]interface{}{"a": 0.0})},
		{"/regex", base}, {"/regex", with(map[string]interface{}{"s": "zzz"})},
		{"/ternary", base}, {"/ternary", with(map[string]interface{}{"flag": false})},
		{"/function", base}, {"/function", with(map[string]interface{}{"a": 1.0})},
		{"/membership", base}, {"/membership", with(map[string]interface{}{"a": 4.0})},
		{"/bitwise", base}, {"/bitwise", with(map[string]interface{}{"a": 1.0})},
		{"/missing", map[string]interface{}{"missing": 1.0}}, {"/missing", map[string]interface{}{}},
		{"/type", base},
		{"/coalesce", base}, {"/coalesce", with(map[string]interface{}{"a": 3.0})},
		{"/elided", map[string]interface{}{}},
		{"/deny", with(map[string]interface{}{"a": -1.0})}, {"/deny", base},
	}
	decisions := make([]conditionDecision, 0, len(cases)+1)
	for _, c := range cases {
		allowed, reason, err := evaluator.IsAllowed(adsapi.RequestContext{
			Subject: subject, ServiceName: "svc", Resource: c.resource, Action: "get", Attributes: c.attributes,
		})
		decision := conditionDecision{allowed: allowed, reason: reason}
		if err != nil {
			decision.err = err.Error()
		}
		decisions = append(decisions, decision)
	}
	roles, err := evaluator.GetAllGrantedRoles(adsapi.RequestContext{
		Subject: subject, ServiceName: "svc", Resource: "/role", Action: "get", Attributes: base,
	})
	sort.Strings(roles)
	decision := conditionDecision{roles: roles}
	if err != nil {
		decision.err = err.Error()
	}
	return append(decisions, decision)
}

func assertConditionTokens(t *testing.T, evaluator InternalEvaluator, dropped bool) {
	t.Helper()
	impl, ok := evaluator.(*PolicyEvalImpl)
	if !ok {
		t.Fatalf("evaluator type %T", evaluator)
	}
	service := impl.RuntimePolicyStore.RuntimeServices["svc"]
	if service == nil {
		t.Fatal("service not loaded")
	}
	conditions := map[string]*condexpr.EvaluableExpression{}
	for id, condition := range service.PoliciesCache.Conditions {
		conditions[id] = condition
	}
	for id, condition := range service.RolePoliciesCache.Conditions {
		conditions["role:"+id] = condition
	}
	if len(conditions) == 0 {
		t.Fatal("no compiled conditions")
	}
	for id, condition := range conditions {
		if condition == nil {
			t.Fatalf("condition %s is nil", id)
		}
		if condition.String() == "" {
			t.Fatalf("condition %s lost its source text", id)
		}
		if dropped && condition.Tokens() != nil {
			t.Fatalf("condition %s kept %d tokens", id, len(condition.Tokens()))
		}
		if !dropped && len(condition.Tokens()) == 0 {
			t.Fatalf("condition %s kept no tokens", id)
		}
	}
}

func TestDropConditionTokensOption(t *testing.T) {
	preparePolicyDataInStore([]byte(dropConditionTokensFixture), t)

	results := make(map[bool][]conditionDecision, 2)
	for _, dropped := range []bool{false, true} {
		evaluator, err := NewWithStore(&cfg.Config{DropConditionTokens: dropped}, testPS)
		if err != nil {
			t.Fatalf("drop=%t: %v", dropped, err)
		}
		assertConditionTokens(t, evaluator, dropped)
		results[dropped] = conditionDecisions(t, evaluator)

		// A policy added after construction is compiled with the same option.
		evaluator.(*PolicyEvalImpl).AddPolicyInRuntimeCache("svc", &pms.Policy{
			ID: "added", Effect: pms.Grant,
			Permissions: []*pms.Permission{{Resource: "/added", Actions: []string{"get"}}},
			Condition:   "a > 1",
		})
		added := evaluator.(*PolicyEvalImpl).RuntimePolicyStore.RuntimeServices["svc"].PoliciesCache.Conditions["added"]
		if added == nil || added.String() != "a > 1" {
			t.Fatalf("drop=%t: added condition %+v", dropped, added)
		}
		if dropped && added.Tokens() != nil {
			t.Fatalf("drop=true: added condition kept %d tokens", len(added.Tokens()))
		}
		if !dropped && len(added.Tokens()) == 0 {
			t.Fatal("drop=false: added condition kept no tokens")
		}

		// A reload recompiles every condition through the same option.
		if err := evaluator.Refresh(); err != nil {
			t.Fatalf("drop=%t: refresh: %v", dropped, err)
		}
		assertConditionTokens(t, evaluator, dropped)
	}
	for i := range results[false] {
		if !reflect.DeepEqual(results[false][i], results[true][i]) {
			t.Errorf("decision %d differs: with tokens %+v, without tokens %+v", i, results[false][i], results[true][i])
		}
	}
}
