//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pmsimpl

import (
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// countingStore reports fixed counts, or fails every count.
type countingStore struct {
	pms.PolicyStoreManager
	services, policies, rolePolicies, functions int64
	err                                         error
	failRolePolicyCount                         bool
}

func (s *countingStore) GetServiceCount() (int64, error) { return s.services, s.err }
func (s *countingStore) GetPolicyCount(string) (int64, error) {
	return s.policies, s.err
}
func (s *countingStore) GetRolePolicyCount(string) (int64, error) {
	if s.failRolePolicyCount {
		return 0, errors.New(errors.StoreError, "count failed")
	}
	return s.rolePolicies, s.err
}
func (s *countingStore) GetFunctionCount() (int64, error) { return s.functions, s.err }

func setLimits(t *testing.T, services, policies, functions, size int64) {
	t.Helper()
	old := []int64{MaxServiceNum, MaxPolicyNum, MaxFunctionNum, MaxPolicySize}
	MaxServiceNum, MaxPolicyNum, MaxFunctionNum, MaxPolicySize = services, policies, functions, size
	t.Cleanup(func() { MaxServiceNum, MaxPolicyNum, MaxFunctionNum, MaxPolicySize = old[0], old[1], old[2], old[3] })
}

func wantCode(t *testing.T, name string, err error, want errors.ErrorCode) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
		return
	}
	if errors.Code(err) != want {
		t.Errorf("%s: error %v, want code %s", name, err, want)
	}
}

func TestCheckService(t *testing.T) {
	setLimits(t, 2, 3, -1, 100)
	small := &pms.Policy{ID: "p", Effect: "grant"}
	big := &pms.Policy{ID: "p", Effect: "grant", Condition: string(make([]byte, 200))}
	cases := []struct {
		name    string
		store   *countingStore
		service *pms.Service
		want    errors.ErrorCode
	}{
		{"fits", &countingStore{services: 1, policies: 1}, &pms.Service{Policies: []*pms.Policy{small}}, ""},
		{"too many services", &countingStore{services: 2}, &pms.Service{}, errors.ExceedLimit},
		{"too many policies", &countingStore{policies: 2, rolePolicies: 1}, &pms.Service{Policies: []*pms.Policy{small}}, errors.ExceedLimit},
		{"role policies count too", &countingStore{}, &pms.Service{RolePolicies: make([]*pms.RolePolicy, 4)}, errors.ExceedLimit},
		{"policy too big", &countingStore{}, &pms.Service{Policies: []*pms.Policy{big}}, errors.ExceedLimit},
		{"role policy too big", &countingStore{}, &pms.Service{RolePolicies: []*pms.RolePolicy{{Condition: string(make([]byte, 200))}}}, errors.ExceedLimit},
		{"role policy fits", &countingStore{}, &pms.Service{RolePolicies: []*pms.RolePolicy{{ID: "rp"}}}, ""},
		{"service count fails", &countingStore{err: errors.New(errors.StoreError, "x")}, &pms.Service{}, errors.StoreError},
		{"policy count fails", &countingStore{failRolePolicyCount: true}, &pms.Service{}, errors.StoreError},
	}
	for _, c := range cases {
		wantCode(t, c.name, CheckService(c.service, c.store), c.want)
	}
}

func TestCheckServiceWithoutLimits(t *testing.T) {
	setLimits(t, -1, -1, -1, -1)
	service := &pms.Service{Policies: []*pms.Policy{{Condition: string(make([]byte, 5000))}}}
	wantCode(t, "unlimited", CheckService(service, &countingStore{services: 1000, policies: 1000}), "")
}

func TestCheckPolicy(t *testing.T) {
	setLimits(t, -1, 2, -1, 100)
	policy := &pms.Policy{ID: "p", Effect: "grant"}
	cases := []struct {
		name    string
		service string
		policy  *pms.Policy
		store   *countingStore
		want    errors.ErrorCode
	}{
		{"fits", "app", policy, &countingStore{policies: 1}, ""},
		{"global service", pms.GlobalService, policy, &countingStore{}, errors.InvalidRequest},
		{"no effect", "app", &pms.Policy{ID: "p"}, &countingStore{}, errors.InvalidRequest},
		{"too many", "app", policy, &countingStore{policies: 1, rolePolicies: 1}, errors.ExceedLimit},
		{"too big", "app", &pms.Policy{Effect: "deny", Condition: string(make([]byte, 200))}, &countingStore{}, errors.ExceedLimit},
		{"count fails", "app", policy, &countingStore{err: errors.New(errors.StoreError, "x")}, errors.StoreError},
	}
	for _, c := range cases {
		wantCode(t, c.name, CheckPolicy(c.service, c.policy, c.store), c.want)
	}
}

func TestCheckRolePolicy(t *testing.T) {
	setLimits(t, -1, 2, -1, 100)
	rolePolicy := &pms.RolePolicy{ID: "rp", Effect: "grant"}
	cases := []struct {
		name       string
		rolePolicy *pms.RolePolicy
		store      *countingStore
		want       errors.ErrorCode
	}{
		{"fits", rolePolicy, &countingStore{rolePolicies: 1}, ""},
		{"no effect", &pms.RolePolicy{ID: "rp"}, &countingStore{}, errors.InvalidRequest},
		{"too many", rolePolicy, &countingStore{policies: 2}, errors.ExceedLimit},
		{"too big", &pms.RolePolicy{Effect: "deny", Condition: string(make([]byte, 200))}, &countingStore{}, errors.ExceedLimit},
		{"count fails", rolePolicy, &countingStore{failRolePolicyCount: true}, errors.StoreError},
	}
	for _, c := range cases {
		wantCode(t, c.name, CheckRolePolicy("app", c.rolePolicy, c.store), c.want)
	}
}

func TestCheckFunction(t *testing.T) {
	setLimits(t, -1, -1, 2, -1)
	wantCode(t, "fits", CheckFunction(&pms.Function{}, &countingStore{functions: 1}), "")
	wantCode(t, "too many", CheckFunction(&pms.Function{}, &countingStore{functions: 2}), errors.ExceedLimit)
	wantCode(t, "count fails", CheckFunction(&pms.Function{}, &countingStore{err: errors.New(errors.StoreError, "x")}), errors.StoreError)
}

func TestCheckMaxSizeUnmarshalable(t *testing.T) {
	valid, err := checkMaxSize(make(chan int), 10)
	if valid || errors.Code(err) != errors.SerializationError {
		t.Fatalf("checkMaxSize(chan) = %v, %v", valid, err)
	}
}
