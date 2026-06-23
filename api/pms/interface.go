//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pms

import "context"

// StoreManager handles loading and saving the entire policy store.
// It abstracts over storage backends, allowing the system to read
// and write the full set of services, policies, and functions as a unit.
type StoreManager interface {
	ReadPolicyStore() (*PolicyStore, error)
	WritePolicyStore(*PolicyStore) error
	Type() string
}

// FunctionManager provides CRUD operations for custom functions.
// It manages the lifecycle of registered functions that can be
// invoked from policy condition expressions during evaluation.
type FunctionManager interface {
	CreateFunction(function *Function) (*Function, error)
	DeleteFunction(funcName string) error
	DeleteFunctions() error
	GetFunction(funcName string) (*Function, error)
	ListAllFunctions(filter string) ([]*Function, error)
	GetFunctionCount() (int64, error)
}

// ServiceManager manages service-level entities in the policy store.
// It provides operations to create, read, delete, and list services
// along with aggregate statistics for their policies and role policies.
type ServiceManager interface {
	CreateService(service *Service) error
	DeleteService(serviceName string) error
	DeleteServices() error
	GetService(serviceName string) (*Service, error)
	ListAllServices() ([]*Service, error)
	GetServiceCount() (int64, error)
	GetServiceNames() ([]string, error)
	GetPolicyAndRolePolicyCounts() (map[string]*PolicyAndRolePolicyCount, error)
}

// PolicyManager provides CRUD operations for policies within a service.
// It manages the lifecycle of individual authorization policies,
// scoped by service name, and supports filtering when listing policies.
type PolicyManager interface {
	CreatePolicy(serviceName string, policy *Policy) (*Policy, error)
	DeletePolicy(serviceName string, id string) error
	DeletePolicies(serviceName string) error
	GetPolicy(serviceName string, id string) (*Policy, error)
	ListAllPolicies(serviceName string, filter string) ([]*Policy, error)
	GetPolicyCount(serviceName string) (int64, error)
}

// RolePolicyManager provides CRUD operations for role policies within a service.
// It manages the lifecycle of role-based authorization rules,
// scoped by service name, and supports filtering when listing role policies.
type RolePolicyManager interface {
	CreateRolePolicy(serviceName string, policy *RolePolicy) (*RolePolicy, error)
	DeleteRolePolicy(serviceName string, id string) error
	DeleteRolePolicies(serviceName string) error
	GetRolePolicy(serviceName string, id string) (*RolePolicy, error)
	ListAllRolePolicies(serviceName string, filter string) ([]*RolePolicy, error)
	GetRolePolicyCount(serviceName string) (int64, error)
}

// PolicyStoreWatcher enables real-time observation of policy store changes.
// Implementations watch the underlying store for events such as policy
// additions and deletions, delivering them on a channel for live updates.
type PolicyStoreWatcher interface {
	Watch() (StorageChangeChannel, error)
	StopWatch()
}

// PolicyStoreManager is the composite interface for full policy store control.
// It combines store-level operations with service, policy, role policy,
// and function management, plus the ability to watch for live changes.
type PolicyStoreManager interface {
	ServiceManager
	StoreManager
	PolicyManager
	RolePolicyManager
	FunctionManager
	PolicyStoreWatcher
	Health(ctx context.Context) error
	Close() error
}

// PolicyStoreManagerADS is the read-only store interface used by the ADS.
// It provides read access to services, policies, role policies, and
// functions needed for authorization decisions, plus store change watching.
type PolicyStoreManagerADS interface {
	Type() string
	ReadPolicyStore() (*PolicyStore, error)
	GetService(serviceName string) (*Service, error)
	GetPolicy(serviceName string, id string) (*Policy, error)
	GetRolePolicy(serviceName string, id string) (*RolePolicy, error)
	GetFunction(funcName string) (*Function, error)
	PolicyStoreWatcher
	Health(ctx context.Context) error
	Close() error
}
