// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import "github.com/teramoby/speedle-plus/api/pms"

// The published assembly is immutable. Every read returns a copy so a caller
// can never change the store through a returned pointer.

func clonePolicyStore(policyStore *pms.PolicyStore) *pms.PolicyStore {
	if policyStore == nil {
		return &pms.PolicyStore{}
	}
	cloned := &pms.PolicyStore{}
	if policyStore.Functions != nil {
		cloned.Functions = make([]*pms.Function, 0, len(policyStore.Functions))
		for _, function := range policyStore.Functions {
			cloned.Functions = append(cloned.Functions, cloneFunction(function))
		}
	}
	if policyStore.Services != nil {
		cloned.Services = make([]*pms.Service, 0, len(policyStore.Services))
		for _, service := range policyStore.Services {
			cloned.Services = append(cloned.Services, cloneService(service))
		}
	}
	return cloned
}

func cloneService(service *pms.Service) *pms.Service {
	if service == nil {
		return nil
	}
	cloned := &pms.Service{
		Name:     service.Name,
		Type:     service.Type,
		Metadata: cloneMetadata(service.Metadata),
	}
	if service.Policies != nil {
		cloned.Policies = make([]*pms.Policy, 0, len(service.Policies))
		for _, policy := range service.Policies {
			cloned.Policies = append(cloned.Policies, clonePolicy(policy))
		}
	}
	if service.RolePolicies != nil {
		cloned.RolePolicies = make([]*pms.RolePolicy, 0, len(service.RolePolicies))
		for _, rolePolicy := range service.RolePolicies {
			cloned.RolePolicies = append(cloned.RolePolicies, cloneRolePolicy(rolePolicy))
		}
	}
	return cloned
}

func clonePolicy(policy *pms.Policy) *pms.Policy {
	if policy == nil {
		return nil
	}
	cloned := &pms.Policy{
		ID:        policy.ID,
		Name:      policy.Name,
		Effect:    policy.Effect,
		Condition: policy.Condition,
		Metadata:  cloneMetadata(policy.Metadata),
	}
	if policy.Permissions != nil {
		cloned.Permissions = make([]*pms.Permission, 0, len(policy.Permissions))
		for _, permission := range policy.Permissions {
			if permission == nil {
				cloned.Permissions = append(cloned.Permissions, nil)
				continue
			}
			cloned.Permissions = append(cloned.Permissions, &pms.Permission{
				Resource:           permission.Resource,
				ResourceExpression: permission.ResourceExpression,
				Actions:            append([]string(nil), permission.Actions...),
			})
		}
	}
	if policy.Principals != nil {
		cloned.Principals = make([][]string, 0, len(policy.Principals))
		for _, principals := range policy.Principals {
			cloned.Principals = append(cloned.Principals, append([]string(nil), principals...))
		}
	}
	return cloned
}

func cloneRolePolicy(rolePolicy *pms.RolePolicy) *pms.RolePolicy {
	if rolePolicy == nil {
		return nil
	}
	return &pms.RolePolicy{
		ID:                  rolePolicy.ID,
		Name:                rolePolicy.Name,
		Effect:              rolePolicy.Effect,
		Roles:               append([]string(nil), rolePolicy.Roles...),
		Principals:          append([]string(nil), rolePolicy.Principals...),
		Resources:           append([]string(nil), rolePolicy.Resources...),
		ResourceExpressions: append([]string(nil), rolePolicy.ResourceExpressions...),
		Condition:           rolePolicy.Condition,
		Metadata:            cloneMetadata(rolePolicy.Metadata),
	}
}

func cloneFunction(function *pms.Function) *pms.Function {
	if function == nil {
		return nil
	}
	return &pms.Function{
		Name:           function.Name,
		Description:    function.Description,
		FuncURL:        function.FuncURL,
		LocalFuncURL:   function.LocalFuncURL,
		CA:             function.CA,
		ResultCachable: function.ResultCachable,
		ResultTTL:      function.ResultTTL,
		Metadata:       cloneMetadata(function.Metadata),
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
