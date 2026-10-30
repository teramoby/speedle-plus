//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package etcd

import (
	"fmt"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func TestGetServiceItselfMatchesWholeNames(t *testing.T) {
	s := newPrefixedStore(t).(*Store)
	if err := s.CreateService(&pms.Service{Name: "app2", Type: pms.TypeK8SCluster}); err != nil {
		t.Fatal(err)
	}
	// "app" is a prefix of "app2" but not a service.
	if service, err := s.GetServiceItself("app"); errors.Code(err) != errors.EntityNotFound {
		t.Fatalf("GetServiceItself(app) = %+v, %v; want not found", service, err)
	}
	service, err := s.GetServiceItself("app2")
	if err != nil || service.Type != pms.TypeK8SCluster {
		t.Fatalf("GetServiceItself(app2) = %+v, %v", service, err)
	}
}

func TestGetServicesPages(t *testing.T) {
	s := newPrefixedStore(t).(*Store)
	for _, name := range []string{"a", "b", "c"} {
		if err := s.CreateService(&pms.Service{Name: name, Policies: []*pms.Policy{{Name: "p", Effect: pms.Grant}}}); err != nil {
			t.Fatal(err)
		}
	}
	page, next, err := s.GetServices("", 2, false)
	if err != nil || len(page) != 2 || next != "c" || len(page[0].Policies) != 0 {
		t.Fatalf("first page %+v, next %q, %v", page, next, err)
	}
	page, next, err = s.GetServices(next, -1, true)
	if err != nil || len(page) != 1 || page[0].Name != "c" || next != "" || len(page[0].Policies) != 1 {
		t.Fatalf("last page %+v, next %q, %v", page, next, err)
	}
}

func TestGetPoliciesAndRolePoliciesPages(t *testing.T) {
	s := newPrefixedStore(t).(*Store)
	if err := s.CreateService(&pms.Service{Name: "app"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.CreatePolicy("app", &pms.Policy{Name: fmt.Sprintf("p%d", i), Effect: pms.Grant}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateRolePolicy("app", &pms.RolePolicy{Name: fmt.Sprintf("rp%d", i), Effect: pms.Grant}); err != nil {
			t.Fatal(err)
		}
	}

	policies, next, err := s.GetPolicies("app", "", 2)
	if err != nil || len(policies) != 2 || next == "" {
		t.Fatalf("first policy page %d, next %q, %v", len(policies), next, err)
	}
	policies, last, err := s.GetPolicies("app", next, 2)
	if err != nil || len(policies) != 1 || last != "" {
		t.Fatalf("last policy page %d, next %q, %v", len(policies), last, err)
	}
	if _, _, err := s.GetPolicies("app", "", 0); errors.Code(err) != errors.InvalidRequest {
		t.Fatalf("amount 0: %v", err)
	}

	rolePolicies, next, err := s.GetRolePolicies("app", "", 2)
	if err != nil || len(rolePolicies) != 2 || next == "" {
		t.Fatalf("first role policy page %d, next %q, %v", len(rolePolicies), next, err)
	}
	rolePolicies, last, err = s.GetRolePolicies("app", next, 3)
	if err != nil || len(rolePolicies) != 1 || last != "" {
		t.Fatalf("last role policy page %d, next %q, %v", len(rolePolicies), last, err)
	}
	if _, _, err := s.GetRolePolicies("app", "", -1); errors.Code(err) != errors.InvalidRequest {
		t.Fatalf("amount -1: %v", err)
	}
}
