package utils

import (
	"fmt"
	"unicode/utf8"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/suid"
)

// ValidatePolicyIDs checks the store-wide identity namespace shared by policies
// and role policies. Missing IDs can be assigned when importing a snapshot.
func ValidatePolicyIDs(ps *pms.PolicyStore) error {
	if ps == nil {
		return errors.New(errors.InvalidRequest, "policy store is required")
	}
	seen := make(map[string]string)
	check := func(id, location string) error {
		if id == "" {
			return nil
		}
		if !utf8.ValidString(id) {
			return errors.Errorf(errors.InvalidRequest, "invalid UTF-8 policy ID in %s", location)
		}
		if previous, ok := seen[id]; ok {
			return errors.Errorf(errors.InvalidRequest, "duplicate policy ID %q in %s and %s", id, previous, location)
		}
		seen[id] = location
		return nil
	}
	for _, service := range ps.Services {
		if service == nil {
			continue
		}
		for i, policy := range service.Policies {
			if policy != nil {
				if err := check(policy.ID, fmt.Sprintf("service %q policy %d", service.Name, i+1)); err != nil {
					return err
				}
			}
		}
		for i, policy := range service.RolePolicies {
			if policy != nil {
				if err := check(policy.ID, fmt.Sprintf("service %q role policy %d", service.Name, i+1)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// PreparePolicyStore preserves existing IDs, assigns missing IDs, and rejects
// duplicates before a backend can mutate its data. It never changes its caller.
func PreparePolicyStore(ps *pms.PolicyStore) (*pms.PolicyStore, error) {
	if err := ValidatePolicyIDs(ps); err != nil {
		return nil, err
	}
	prepared := *ps
	prepared.Services = make([]*pms.Service, len(ps.Services))
	for i, service := range ps.Services {
		if service == nil {
			return nil, errors.New(errors.InvalidRequest, "service is required")
		}
		copy := *service
		copy.Policies = make([]*pms.Policy, len(service.Policies))
		for j, policy := range service.Policies {
			if policy == nil {
				return nil, errors.New(errors.InvalidRequest, "policy is required")
			}
			value := *policy
			if value.ID == "" {
				value.ID = suid.New().String()
			}
			copy.Policies[j] = &value
		}
		copy.RolePolicies = make([]*pms.RolePolicy, len(service.RolePolicies))
		for j, policy := range service.RolePolicies {
			if policy == nil {
				return nil, errors.New(errors.InvalidRequest, "role policy is required")
			}
			value := *policy
			if value.ID == "" {
				value.ID = suid.New().String()
			}
			copy.RolePolicies[j] = &value
		}
		prepared.Services[i] = &copy
	}
	for _, function := range prepared.Functions {
		if function == nil {
			return nil, errors.New(errors.InvalidRequest, "function is required")
		}
	}
	if err := ValidatePolicyIDs(&prepared); err != nil {
		return nil, err
	}
	return &prepared, nil
}
