//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"regexp"
	"sync"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	log "github.com/sirupsen/logrus"
)

var (
	compiledRegexCache sync.Map
)

// matchRegexCompiled compiles the pattern once and caches the result.
// It returns true if pattern matches s.
func matchRegexCompiled(pattern, s string) bool {
	re, ok := compiledRegexCache.Load(pattern)
	if !ok {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return false
		}
		re, _ = compiledRegexCache.LoadOrStore(pattern, re)
	}
	return re.(*regexp.Regexp).MatchString(s)
}

func matchResource(requestRes string, resources, resExpressions []string) bool {
	//in role policy, resources/resExpressions could be empty, which means any resource
	if (resources == nil || len(resources) == 0) && (resExpressions == nil || len(resExpressions) == 0) {
		return true
	}
	for _, res := range resources {
		if requestRes == res {
			return true
		}
	}
	// NOTE: Resource expressions are user-supplied and compiled as regex patterns.
	// If untrusted users can define resource expressions, they may be able to craft
	// patterns that cause ReDoS (catastrophic backtracking). Consider adding a timeout
	// or complexity limit on user-supplied patterns if this becomes a concern.
	for _, resExp := range resExpressions {
		if matchRegexCompiled(resExp, requestRes) {
			return true
		}
	}
	return false
}

// Returns if policy is matched
func matchResourceAction(policy *pms.Policy, ctx *internalRequestContext) bool {
	//we interpret nil or empty resource/permission/action/principal etc as ANY resource/permission/action/principal
	if policy.Permissions == nil || len(policy.Permissions) == 0 { //any permissions
		return true
	}
	for _, perm := range policy.Permissions {
		resExpMatch := false
		if len(perm.ResourceExpression) != 0 {
			resExpMatch = matchRegexCompiled(perm.ResourceExpression, ctx.Resource)
			//TODO log error
		}
		resNameMatch := perm.Resource == ctx.Resource
		if (len(perm.Resource) == 0 && len(perm.ResourceExpression) == 0) || resExpMatch || resNameMatch {
			if perm.Actions == nil || len(perm.Actions) == 0 { //any action
				return true
			}
			for _, act := range perm.Actions {
				if act == ctx.Action {
					return true
				}
			}
		}

	}
	return false
}

func denyOverwriteCombiner(grantedPolicies []*pms.Policy, deniedPolicies []*pms.Policy,
	context *internalRequestContext, evaluationResult *adsapi.EvaluationResult) (bool, adsapi.Reason) {

	if evaluationResult != nil {
		evaluationResult.AddPolicies(grantedPolicies, deniedPolicies)
	}

	// Evaluate denied policies first
	if len(deniedPolicies) > 0 {
		// If the number of matched denied policies is bigger than 0, then return false directly
		return false, adsapi.DENY_POLICY_FOUND
	}

	if len(grantedPolicies) == 0 {
		// No granted policy defined, return false directly
		// No need to check deny policies
		return false, adsapi.NO_APPLICABLE_POLICIES
	}

	if len(grantedPolicies) > 0 {
		// If the number of matched granted policies is bigger than 0, then return true directly
		return true, adsapi.GRANT_POLICY_FOUND
	}

	//should not go here
	return false, adsapi.REASON_NOT_AVAILABLE
}

func updateSubjectWithBuiltInRoles(s *subject) {
	principals := []string{"role:" + adsapi.BuiltIn_Role_Everyone}
	if s == nil {
		principals = append(principals, "role:"+adsapi.BuiltIn_Role_Anonymous)
	} else {
		if len(s.Users) == 0 && len(s.Groups) == 0 && len(s.Entities) == 0 {
			principals = append(principals, "role:"+adsapi.BuiltIn_Role_Anonymous)
		} else {
			if len(s.Users) != 0 {
				principals = append(principals, s.Users...)
			}
			if len(s.Entities) != 0 {
				principals = append(principals, s.Entities...)
			}

			principals = append(principals, "role:"+adsapi.BuiltIn_Role_Authenticated)
		}

		principals = append(principals, s.Groups...)
	}

	s.Principals = principals
}

func convertRoleToPrincipal(name string) string {
	return "role:" + name
}

func calculatePermissions(grantedPermissions, deniedPermissions []pms.Permission) []pms.Permission {
	if len(deniedPermissions) == 0 {
		return grantedPermissions
	}

	// Pre-build maps for O(1) lookups instead of O(G*D) nested scans.
	// deniedByResource maps an exact resource name to a set of denied actions.
	deniedByResource := make(map[string]map[string]bool, len(deniedPermissions))
	// deniedByExpr stores denied permissions that use resource expressions.
	type deniedExprEntry struct {
		expr    string
		actions map[string]bool
	}
	var deniedByExpr []deniedExprEntry
	// denyAllActions is true when a denied permission has no resource and no expression (matches everything).
	var denyAllActions map[string]bool

	for _, dp := range deniedPermissions {
		actions := make(map[string]bool, len(dp.Actions))
		for _, a := range dp.Actions {
			actions[a] = true
		}
		if len(dp.Resource) == 0 && len(dp.ResourceExpression) == 0 {
			denyAllActions = actions
		} else if len(dp.ResourceExpression) > 0 {
			deniedByExpr = append(deniedByExpr, deniedExprEntry{expr: dp.ResourceExpression, actions: actions})
		} else {
			deniedByResource[dp.Resource] = actions
		}
	}

	var finalPermissions []pms.Permission
	for _, permission := range grantedPermissions {
		grantActions := make([]string, 0, len(permission.Actions))
		isDenied := false

		for _, grantedAction := range permission.Actions {
			actionDenied := false

			// Check deny-all (matches any resource).
			if denyAllActions != nil && denyAllActions[grantedAction] {
				actionDenied = true
			}

			// Check exact resource match.
			if !actionDenied {
				if deniedActions, ok := deniedByResource[permission.Resource]; ok {
					actionDenied = deniedActions[grantedAction]
				}
			}

			// Check resource expression match.
			if !actionDenied {
				for _, de := range deniedByExpr {
					if matchRegexCompiled(de.expr, permission.Resource) {
						if de.actions[grantedAction] {
							actionDenied = true
							break
						}
					}
				}
			}

			if !actionDenied {
				grantActions = append(grantActions, grantedAction)
			}
		}

		if len(grantActions) == 0 {
			isDenied = true
		}

		if !isDenied {
			finalPermissions = append(finalPermissions, pms.Permission{
				Resource: permission.Resource,
				Actions:  grantActions,
			})
		}
	}
	return finalPermissions

}

func evaluateCondition(condition *govaluate.EvaluableExpression, attributes map[string]interface{}) (bool, error) {
	res, err := condition.Evaluate(attributes)
	if err != nil {
		log.Errorf("Error happens in evaluating condition (%s): %v", condition.String(), err)
		return false, err
	}
	b, ok := res.(bool)
	if !ok || !b {
		return false, nil
	}
	return true, nil
}

func matchRolePolicyPrincipals(subjectSet map[string]bool, rolePolicyPrincipalList []string) bool {
	if subjectSet == nil || len(subjectSet) == 0 {
		return false
	}

	if rolePolicyPrincipalList == nil || len(rolePolicyPrincipalList) == 0 {
		return true
	}

	for _, policyPrincipal := range rolePolicyPrincipalList {
		if subjectSet[policyPrincipal] {
			return true
		}
	}
	return false

}

/**
It's regarded as matched only if all items in princs2 are included in princs1
*/
func matchPrincipals(subjectSet map[string]bool, policyPrincipalList [][]string) bool {
	if subjectSet == nil || len(subjectSet) == 0 {
		return false
	}

	if policyPrincipalList == nil || len(policyPrincipalList) == 0 {
		return true
	}

	for _, andPrincipals := range policyPrincipalList {
		// one of item in policy principals matched, returns true
		matched := true
		for _, policyPrincipal := range andPrincipals {
			if !subjectSet[policyPrincipal] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}

	return false
}
