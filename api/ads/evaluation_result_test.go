//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package ads

import (
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
)

func TestAddRolePolicy(t *testing.T) {
	var result EvaluationResult
	rolePolicy := &pms.RolePolicy{ID: "rp", Name: "n", Effect: "grant", Roles: []string{"r"},
		Principals: []string{"user:a"}, Resources: []string{"x"}, ResourceExpressions: []string{"y.*"}, Condition: "a > 1"}
	result.AddRolePolicy(rolePolicy, true)
	result.AddRolePolicy(&pms.RolePolicy{ID: "plain"}, false)
	first, second := result.RolePolicies[0], result.RolePolicies[1]
	if first.Status != Evaluation_TakeEffect || first.ResourceExpressions[0] != "y.*" ||
		first.Condition.ConditionExpression != "a > 1" || first.Condition.EvaluationResult != "true" {
		t.Fatalf("role policy %+v", first)
	}
	if second.Status != Evaluation_ConditionFailed || second.Condition != nil {
		t.Fatalf("role policy without condition %+v", second)
	}
	convertMetaRolePolicy2ApiEvaluatedRolePolicy(nil, nil, true)
}

func TestAddPolicyAndPolicies(t *testing.T) {
	var result EvaluationResult
	policy := &pms.Policy{ID: "p", Effect: "grant", Condition: "c",
		Permissions: []*pms.Permission{{Resource: "r", Actions: []string{"get"}}}}
	result.AddPolicy(policy, Evaluation_ConditionFailed, false)
	if got := result.Policies[0]; got.Status != Evaluation_ConditionFailed || got.Condition.EvaluationResult != "false" ||
		got.Permissions[0].Resource != "r" {
		t.Fatalf("policy %+v", got)
	}

	result = EvaluationResult{}
	deny1, deny2, grant := &pms.Policy{ID: "d1"}, &pms.Policy{ID: "d2"}, &pms.Policy{ID: "g"}
	result.AddPolicies([]*pms.Policy{grant}, []*pms.Policy{deny1, deny2})
	statuses := []string{}
	for _, p := range result.Policies {
		statuses = append(statuses, p.ID+":"+p.Status)
	}
	// The first deny takes effect and every other policy is ignored.
	want := []string{"d1:takeEffect", "d2:ignored", "g:ignored"}
	for i := range want {
		if statuses[i] != want[i] {
			t.Fatalf("statuses %v, want %v", statuses, want)
		}
	}

	result = EvaluationResult{}
	result.AddPolicies([]*pms.Policy{grant, deny1}, nil)
	if result.Policies[0].Status != Evaluation_TakeEffect || result.Policies[1].Status != Evaluation_Ignored {
		t.Fatalf("grant only: %+v %+v", result.Policies[0], result.Policies[1])
	}
	convertMetaPolicy2ApiEvaluatedPolicy(nil, nil, "", "")
}

func TestReasonString(t *testing.T) {
	if GRANT_POLICY_FOUND.String() != "GRANT_POLICY_FOUND" || REASON_NOT_AVAILABLE.String() != "REASON_NOT_AVAILABLE" {
		t.Fatal("unexpected reason names")
	}
}
