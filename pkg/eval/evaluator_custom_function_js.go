//go:build js && wasm

// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"unicode"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// CallCustomerFunction returns custom_function_unsupported on js/wasm.
// Evaluator construction, snapshot reloads and incremental cache mutations
// reject policies that would call this stub before activating them.
func CallCustomerFunction(_ *pms.Function, _ *ext.CustomerFunctionRequest) (interface{}, error) {
	return nil, errors.New(errors.EvalEngineError, "custom_function_unsupported: custom functions are unavailable in js/wasm")
}

// CallCustomerFunctionViaDelegator also returns custom_function_unsupported.
func CallCustomerFunctionViaDelegator(_ string, cf *pms.Function, request *ext.CustomerFunctionRequest) (interface{}, error) {
	return CallCustomerFunction(cf, request)
}

// validatePolicyStoreCustomFunctions rejects a snapshot before any of its
// conditions can be evaluated without the custom functions they reference.
func validatePolicyStoreCustomFunctions(ps *pms.PolicyStore) error {
	customFunctions := getCustomFunctionNames(ps.Functions)
	for _, service := range ps.Services {
		if err := validateServiceCustomFunctions(service, customFunctions); err != nil {
			return err
		}
	}
	return nil
}

func validateServiceCustomFunctions(service *pms.Service, customFunctions map[string]struct{}) error {
	if len(customFunctions) == 0 {
		return nil
	}
	for _, policy := range service.Policies {
		if err := validateConditionCustomFunctions("policy", service.Name, policy.ID, policy.Condition, customFunctions); err != nil {
			return err
		}
	}
	for _, policy := range service.RolePolicies {
		if err := validateConditionCustomFunctions("role policy", service.Name, policy.ID, policy.Condition, customFunctions); err != nil {
			return err
		}
	}
	return nil
}

func validateConditionCustomFunctions(policyKind, serviceName, policyID, expression string, customFunctions map[string]struct{}) error {
	if name := calledCustomFunction(expression, customFunctions); name != "" {
		return errors.Errorf(errors.EvalEngineError, "custom_function_unsupported: %s %q in service %q calls custom function %q on js/wasm", policyKind, policyID, serviceName, name)
	}
	return nil
}

// Adding a function can resolve a previously unknown function in an existing
// condition. Validate the prospective function set before registering it.
func validateFunctionAdditionCustomFunctions(rtps *RuntimePolicyStore, function *pms.Function) error {
	customFunctions := make(map[string]struct{}, len(rtps.customFunctionNames)+1)
	for name := range rtps.customFunctionNames {
		customFunctions[name] = struct{}{}
	}
	customFunctions[function.Name] = struct{}{}
	for _, service := range rtps.RuntimeServices {
		err := func() error {
			service.RLock()
			defer service.RUnlock()
			for _, policy := range service.PoliciesCache.PolicyMap {
				if err := validateConditionCustomFunctions("policy", service.Name, policy.ID, policy.Condition, customFunctions); err != nil {
					return err
				}
			}
			for _, policy := range service.RolePoliciesCache.PolicyMap {
				if err := validateConditionCustomFunctions("role policy", service.Name, policy.ID, policy.Condition, customFunctions); err != nil {
					return err
				}
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// calledCustomFunction follows the expression lexer's identifier, string and
// escaped-variable rules. It detects calls in every condition, including
// branches that could be short-circuited, without executing any function.
func calledCustomFunction(expression string, customFunctions map[string]struct{}) string {
	runes := []rune(expression)
	for i := 0; i < len(runes); {
		switch r := runes[i]; {
		case r == '\'' || r == '"' || r == '[':
			isEnd := func(c rune) bool { return c == '\'' || c == '"' }
			if r == '[' {
				isEnd = func(c rune) bool { return c == ']' }
			}
			for i++; i < len(runes) && !isEnd(runes[i]); i++ {
				if runes[i] == '\\' {
					i++
				}
			}
			i++
		case unicode.IsLetter(r) || r == '_':
			start := i
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '.') {
				i++
			}
			name := string(runes[start:i])
			for i < len(runes) && unicode.IsSpace(runes[i]) {
				i++
			}
			if i < len(runes) && runes[i] == '(' {
				if _, ok := customFunctions[name]; ok {
					return name
				}
			}
		default:
			i++
		}
	}
	return ""
}

// generateCustomerExpressionFunction rejects every call before copying or
// marshalling arguments and before consulting the cache.
func (frc *FuncResultCache) generateCustomerExpressionFunction(_ *string, _ *pms.Function) (govaluate.ExpressionFunction, error) {
	return func(_ ...interface{}) (interface{}, error) {
		return CallCustomerFunction(nil, nil)
	}, nil
}

// No periodic cache cleanup runs on js/wasm. AddToCache remains available to
// embedders; expired entries are reclaimed when ReadFromCache reads the same
// key or when CleanExpiredResult is called explicitly. Entries without a TTL
// remain until they are deleted explicitly or the cache is replaced.
func (p *PolicyEvalImpl) cleanExpiredFunctionResultPeriodically() {}
