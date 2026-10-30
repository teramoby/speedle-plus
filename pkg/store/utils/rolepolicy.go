package utils

import (
	"strings"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// ValidateRolePolicyPrincipals rejects role rules without an explicit subject.
// An omitted subject is not a grant or denial applying to everyone.
func ValidateRolePolicyPrincipals(policy *pms.RolePolicy) error {
	if policy == nil {
		return errors.New(errors.InvalidRequest, "role policy is required")
	}
	if len(policy.Principals) == 0 {
		return errors.New(errors.InvalidRequest, "role policy requires principals")
	}
	for _, principal := range policy.Principals {
		if strings.TrimSpace(principal) == "" {
			return errors.New(errors.InvalidRequest, "role policy principal must not be empty")
		}
	}
	return nil
}

// ValidateServiceRolePolicies validates all nested role rules before mutation.
func ValidateServiceRolePolicies(service *pms.Service) error {
	if service == nil {
		return errors.New(errors.InvalidRequest, "service is required")
	}
	for i, policy := range service.RolePolicies {
		if err := ValidateRolePolicyPrincipals(policy); err != nil {
			return errors.Errorf(errors.InvalidRequest, "service %q role policy %d: %v", service.Name, i+1, err)
		}
	}
	return nil
}
