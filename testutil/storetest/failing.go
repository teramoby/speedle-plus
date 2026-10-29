//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package storetest holds policy store doubles for handler tests.
package storetest

import (
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// FailingStore fails every call with Err, so tests can reach the error
// handling of every handler. Counts succeed with zero when CountsSucceed is
// set, so the limit checks pass and the write itself fails.
type FailingStore struct {
	Err           error
	CountsSucceed bool
}

// NewFailingStore returns a store whose calls fail with a store error.
func NewFailingStore() *FailingStore {
	return &FailingStore{Err: errors.New(errors.StoreError, "store unavailable")}
}

func (s *FailingStore) count() (int64, error) {
	if s.CountsSucceed {
		return 0, nil
	}
	return 0, s.Err
}

func (s *FailingStore) ReadPolicyStore() (*pms.PolicyStore, error) { return nil, s.Err }
func (s *FailingStore) WritePolicyStore(*pms.PolicyStore) error    { return s.Err }
func (s *FailingStore) Type() string                               { return "failing" }

func (s *FailingStore) CreateService(*pms.Service) error         { return s.Err }
func (s *FailingStore) DeleteService(string) error               { return s.Err }
func (s *FailingStore) DeleteServices() error                    { return s.Err }
func (s *FailingStore) GetService(string) (*pms.Service, error)  { return nil, s.Err }
func (s *FailingStore) ListAllServices() ([]*pms.Service, error) { return nil, s.Err }
func (s *FailingStore) GetServiceCount() (int64, error)          { return s.count() }
func (s *FailingStore) GetServiceNames() ([]string, error)       { return nil, s.Err }
func (s *FailingStore) GetPolicyAndRolePolicyCounts() (map[string]*pms.PolicyAndRolePolicyCount, error) {
	return nil, s.Err
}

func (s *FailingStore) CreatePolicy(string, *pms.Policy) (*pms.Policy, error) { return nil, s.Err }
func (s *FailingStore) UpdatePolicy(string, *pms.Policy) (*pms.Policy, error) { return nil, s.Err }
func (s *FailingStore) DeletePolicy(string, string) error                     { return s.Err }
func (s *FailingStore) DeletePolicies(string) error                           { return s.Err }
func (s *FailingStore) GetPolicy(string, string) (*pms.Policy, error)         { return nil, s.Err }
func (s *FailingStore) ListAllPolicies(string, string) ([]*pms.Policy, error) { return nil, s.Err }
func (s *FailingStore) GetPolicyCount(string) (int64, error)                  { return s.count() }

func (s *FailingStore) CreateRolePolicy(string, *pms.RolePolicy) (*pms.RolePolicy, error) {
	return nil, s.Err
}
func (s *FailingStore) UpdateRolePolicy(string, *pms.RolePolicy) (*pms.RolePolicy, error) {
	return nil, s.Err
}
func (s *FailingStore) DeleteRolePolicy(string, string) error                 { return s.Err }
func (s *FailingStore) DeleteRolePolicies(string) error                       { return s.Err }
func (s *FailingStore) GetRolePolicy(string, string) (*pms.RolePolicy, error) { return nil, s.Err }
func (s *FailingStore) ListAllRolePolicies(string, string) ([]*pms.RolePolicy, error) {
	return nil, s.Err
}
func (s *FailingStore) GetRolePolicyCount(string) (int64, error) { return s.count() }

func (s *FailingStore) CreateFunction(*pms.Function) (*pms.Function, error) { return nil, s.Err }
func (s *FailingStore) DeleteFunction(string) error                         { return s.Err }
func (s *FailingStore) DeleteFunctions() error                              { return s.Err }
func (s *FailingStore) GetFunction(string) (*pms.Function, error)           { return nil, s.Err }
func (s *FailingStore) ListAllFunctions(string) ([]*pms.Function, error)    { return nil, s.Err }
func (s *FailingStore) GetFunctionCount() (int64, error)                    { return s.count() }

func (s *FailingStore) Watch() (pms.StorageChangeChannel, error) { return nil, s.Err }
func (s *FailingStore) StopWatch()                               {}

func (s *FailingStore) SaveDiscoverRequest(*ads.RequestContext) error { return s.Err }
func (s *FailingStore) GetLastDiscoverRequest(string) (*ads.RequestContext, int64, error) {
	return nil, 0, s.Err
}
func (s *FailingStore) GetDiscoverRequestsSinceRevision(string, int64) ([]*ads.RequestContext, int64, error) {
	return nil, 0, s.Err
}
func (s *FailingStore) GetDiscoverRequests(string) ([]*ads.RequestContext, int64, error) {
	return nil, 0, s.Err
}
func (s *FailingStore) ResetDiscoverRequests(string) error { return s.Err }
func (s *FailingStore) GeneratePolicies(string, string, string, string) (map[string]*pms.Service, int64, error) {
	return nil, 0, s.Err
}
