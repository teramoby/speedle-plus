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
		{Name: "app", Policies: []*pms.Policy{{Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}, ID: "same", Effect: pms.Deny}}},
		{Name: "global", RolePolicies: []*pms.RolePolicy{{Principals: []string{"user:alice"}, ID: "same", Effect: pms.Grant, Roles: []string{"reader"}}}},
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

func TestFeatureRoleMissingPrincipalsRejected(t *testing.T) {
	for _, effect := range []string{pms.Grant, pms.Deny} {
		for _, principals := range [][]string{nil, {}, {""}, {"user:alice", ""}} {
			invalid := roleRegressionPolicy("invalid", effect, principals, "reader")
			bad := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", RolePolicies: []*pms.RolePolicy{invalid}}}}
			if _, err := NewWithStore(&cfg.Config{EnableWatch: true}, identitySnapshotStore{snapshot: bad}); errors.Code(err) != errors.InvalidRequest {
				t.Fatalf("effect=%s principals=%q startup: %v", effect, principals, err)
			}
			e := roleRegressionEvaluator(t, "reader", roleRegressionPolicy("valid", pms.Grant, []string{"user:alice"}, "reader"))
			assertRoleRegression(t, e, featureContext(), []string{"reader"}, true)
			for _, event := range []pms.StoreChangeEvent{
				{Type: pms.FULL_RELOAD, Content: bad},
				{Type: pms.SERVICE_ADD, Content: bad.Services[0]},
				{Type: pms.ROLEPOLICY_ADD, Content: []pms.StoreUpdateData{{ServiceName: "app", Data: invalid}}},
			} {
				apply(e, event)
				assertRoleRegression(t, e, featureContext(), []string{"reader"}, true)
				if _, ok := e.RuntimePolicyStore.RuntimeServices["app"].RolePoliciesCache.PolicyMap["invalid"]; ok {
					t.Fatal("invalid incremental role rule entered cache")
				}
			}
			// Low-level cache callers also cannot introduce an implicit universal rule.
			e.RuntimePolicyStore.RuntimeServices["app"].RolePoliciesCache.AddRolePolicyToCache(invalid, nil)
			assertRoleRegression(t, e, featureContext(), []string{"reader"}, true)
		}
	}
}

// The formerly reported fatal probe had an invalid root. Reject that exact
// malformed graph at startup instead of assigning it public-role semantics.
func TestFeatureRoleInvalidCycleRootRejected(t *testing.T) {
	bad := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", RolePolicies: []*pms.RolePolicy{
		roleRegressionPolicy("missing-subject", pms.Grant, nil, "cycleA"),
		roleRegressionPolicy("root", pms.Grant, []string{"user:alice"}, "victim"),
		roleRegressionPolicy("cycle1", pms.Grant, []string{"role:cycleA"}, "cycleB"),
		roleRegressionPolicy("cycle2", pms.Grant, []string{"role:cycleB"}, "cycleA"),
		roleRegressionPolicy("deny", pms.Deny, []string{"role:cycleB"}, "victim"),
	}}}}
	if _, err := NewWithStore(&cfg.Config{EnableWatch: true}, identitySnapshotStore{snapshot: bad}); errors.Code(err) != errors.InvalidRequest {
		t.Fatalf("invalid cycle root accepted: %v", err)
	}
}
