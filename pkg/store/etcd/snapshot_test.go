// Licensed under the Universal Permissive License (UPL) Version 1.0.
package etcd

import (
	"context"
	"fmt"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func TestRecoverySnapshotIncludesPrefixRelatedIDsAcrossPages(t *testing.T) {
	s := newPrefixedStore(t).(*Store)
	service := &pms.Service{Name: "app", Type: pms.TypeApplication}
	// A continuation key can itself prefix another policy ID. All policies,
	// including deny rules on later pages, must survive a recovery reload.
	for i := 0; i < 1200; i++ {
		id := fmt.Sprintf("%04d", i)
		service.Policies = append(service.Policies, &pms.Policy{ID: id, Effect: pms.Deny, Principals: [][]string{{"user:alice"}}})
		service.Policies = append(service.Policies, &pms.Policy{ID: id + "0", Effect: pms.Grant, Principals: [][]string{{"user:bob"}}})
	}
	service.RolePolicies = []*pms.RolePolicy{{ID: "reader", Effect: pms.Grant, Principals: []string{"user:alice"}, Roles: []string{"reader"}}}
	if err := s.CreateService(service); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFunction(&pms.Function{Name: "f", FuncURL: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.ReadPolicyStore()
	if err != nil || len(snapshot.Services) != 1 || len(snapshot.Services[0].Policies) != len(service.Policies) || len(snapshot.Services[0].RolePolicies) != 1 || len(snapshot.Functions) != 1 {
		t.Fatalf("incomplete recovery snapshot: %v", err)
	}
	seen := map[string]bool{}
	for _, policy := range snapshot.Services[0].Policies {
		seen[policy.ID] = true
	}
	for _, policy := range service.Policies {
		if !seen[policy.ID] {
			t.Fatalf("lost policy %s", policy.ID)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.readSnapshot(ctx); errors.Code(err) != errors.StoreError {
		t.Fatalf("canceled snapshot: %v", err)
	}
}
