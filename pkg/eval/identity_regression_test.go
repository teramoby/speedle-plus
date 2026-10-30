package eval

import (
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

type identitySnapshotStore struct {
	pms.PolicyStoreManagerADS
	snapshot *pms.PolicyStore
}

func (s identitySnapshotStore) ReadPolicyStore() (*pms.PolicyStore, error) { return s.snapshot, nil }

func TestFeatureRoleDuplicateSnapshotIDs(t *testing.T) {
	bad := &pms.PolicyStore{Services: []*pms.Service{
		{Name: "app", Policies: []*pms.Policy{{ID: "same", Effect: pms.Deny}}},
		{Name: "global", RolePolicies: []*pms.RolePolicy{{ID: "same", Effect: pms.Grant, Roles: []string{"reader"}}}},
	}}
	// Reject before starting a watch (the stub has no Watch implementation).
	if _, err := NewWithStore(&cfg.Config{EnableWatch: true}, identitySnapshotStore{snapshot: bad}); errors.Code(err) != errors.InvalidRequest {
		t.Fatalf("duplicate startup snapshot: %v", err)
	}
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{featurePolicy("old", "")}}}})
	apply(e, pms.StoreChangeEvent{Type: pms.FULL_RELOAD, Content: bad})
	if ok, _, err := e.IsAllowed(featureContext()); !ok || err != nil {
		t.Fatalf("invalid full reload changed cache: %v %v", ok, err)
	}
}
