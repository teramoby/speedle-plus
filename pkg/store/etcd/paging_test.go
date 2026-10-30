//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package etcd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestPrefixGetIncludesKeysExtendingPageBoundary(t *testing.T) {
	s := newPrefixedStore(t).(*Store)
	prefix := s.KeyPrefix + "/page-boundary/"
	want := make(map[string]bool)
	ops := make([]clientv3.Op, 0, pageSize+2)
	for i := 0; i < pageSize; i++ {
		key := fmt.Sprintf("%s%04d", prefix, i)
		want[key] = true
		ops = append(ops, clientv3.OpPut(key, "value"))
	}
	for _, key := range []string{fmt.Sprintf("%s%04d-child", prefix, pageSize-1), prefix + "tail"} {
		want[key] = true
		ops = append(ops, clientv3.OpPut(key, "value"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for start := 0; start < len(ops); start += 64 {
		if _, err := s.client.Txn(ctx).Then(ops[start:min(start+64, len(ops))]...).Commit(); err != nil {
			t.Fatal(err)
		}
	}
	for _, keysOnly := range []bool{false, true} {
		var options []clientv3.OpOption
		if keysOnly {
			options = append(options, clientv3.WithKeysOnly())
		}
		pages, err := s.prefixGet(prefix, options...)
		if err != nil {
			t.Fatal(err)
		}
		seen := make(map[string]bool)
		for _, page := range pages {
			for _, kv := range page.Kvs {
				key := string(kv.Key)
				if !want[key] || seen[key] || (!keysOnly && string(kv.Value) != "value") {
					t.Fatalf("keysOnly=%v unexpected key/value %q %q", keysOnly, key, kv.Value)
				}
				seen[key] = true
			}
		}
		if len(seen) != len(want) {
			t.Fatalf("keysOnly=%v received %d keys, want %d", keysOnly, len(seen), len(want))
		}
	}
}

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
