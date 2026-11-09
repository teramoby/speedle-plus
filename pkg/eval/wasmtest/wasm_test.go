//go:build js && wasm

// Licensed under the Universal Permissive License (UPL) Version 1.0.
package wasmtest

import (
	"strings"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	spdlerrors "github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/eval"
)

// Compile-check the transport-independent exported type and its fields from
// outside package eval. This must remain available in the js/wasm API.
var _ = eval.Request2Delegator{
	Function: (*pms.Function)(nil),
	Request:  (*ext.CustomerFunctionRequest)(nil),
}

// Watching is disabled, so evaluation needs only the initial snapshot and the
// lifecycle methods. The embedded interface supplies the unused store getters.
type snapshotStore struct {
	pms.PolicyStoreManagerADS
	snapshot *pms.PolicyStore
}

func (s *snapshotStore) Type() string { return "memory" }
func (s *snapshotStore) ReadPolicyStore() (*pms.PolicyStore, error) {
	return s.snapshot, nil
}
func (s *snapshotStore) Watch() (pms.StorageChangeChannel, error) { return nil, nil }
func (s *snapshotStore) StopWatch()                               {}

func newWasmEvaluatorFromStore(t *testing.T, store *snapshotStore) *eval.PolicyEvalImpl {
	t.Helper()
	instance, err := eval.NewWithStore(&cfg.Config{}, store)
	if err != nil {
		t.Fatal(err)
	}
	engine := instance.(*eval.PolicyEvalImpl)
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("close evaluator: %v", err)
		}
	})
	return engine
}

func newWasmEvaluator(t *testing.T, policies []*pms.Policy) *eval.PolicyEvalImpl {
	t.Helper()
	return newWasmEvaluatorFromStore(t, &snapshotStore{snapshot: &pms.PolicyStore{
		Functions: []*pms.Function{{Name: "remote", FuncURL: "https://example.invalid"}},
		Services:  []*pms.Service{{Name: "app", Policies: policies}},
	}})
}

func requireUnsupported(t *testing.T, result interface{}, err error) {
	t.Helper()
	if result != nil || err == nil {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if !strings.Contains(err.Error(), "custom_function_unsupported") || spdlerrors.Code(err) != spdlerrors.EvalEngineError {
		t.Fatalf("result=%v err=%v code=%v", result, err, spdlerrors.Code(err))
	}
}

func TestWasmCustomFunctionsUnsupported(t *testing.T) {
	cf := &pms.Function{Name: "remote", FuncURL: "https://example.invalid"}
	request := &ext.CustomerFunctionRequest{}
	for _, delegated := range []bool{false, true} {
		var result interface{}
		var err error
		if delegated {
			result, err = eval.CallCustomerFunctionViaDelegator("https://example.invalid", cf, request)
		} else {
			result, err = eval.CallCustomerFunction(cf, request)
		}
		requireUnsupported(t, result, err)
	}
}

func TestWasmCustomExpressionRejectsBeforeMarshalling(t *testing.T) {
	engine := newWasmEvaluator(t, nil)
	expression, ok := engine.RuntimePolicyStore.Functions["remote"]
	if !ok {
		t.Fatal("custom expression was not registered")
	}
	// A channel cannot be JSON-marshalled. The unsupported error must take
	// precedence over cache-key generation and argument serialization.
	result, err := expression(make(chan int))
	requireUnsupported(t, result, err)
}

func testPolicy(id, effect, condition string) *pms.Policy {
	return &pms.Policy{
		ID:          id,
		Effect:      effect,
		Principals:  [][]string{{"user:alice"}},
		Permissions: []*pms.Permission{{Resource: "document", Actions: []string{"read"}}},
		Condition:   condition,
	}
}

func testSnapshot(functions []*pms.Function, policies []*pms.Policy) *pms.PolicyStore {
	return &pms.PolicyStore{
		Functions: functions,
		Services:  []*pms.Service{{Name: "app", Policies: policies}},
	}
}

func aliceRequest() adsapi.RequestContext {
	return adsapi.RequestContext{
		Subject:     &adsapi.Subject{Principals: []*adsapi.Principal{{Type: adsapi.PRINCIPAL_TYPE_USER, Name: "alice"}}},
		ServiceName: "app",
		Resource:    "document",
		Action:      "read",
	}
}

func TestWasmRejectsCustomFunctionConditionsAtLoad(t *testing.T) {
	remote := []*pms.Function{{Name: "remote", FuncURL: "https://example.invalid"}}
	tests := []struct {
		name       string
		snapshot   *pms.PolicyStore
		policyID   string
		policyKind string
	}{
		{
			name: "conditional DENY plus GRANT cannot load",
			snapshot: testSnapshot(remote, []*pms.Policy{
				testPolicy("grant", pms.Grant, ""),
				testPolicy("deny", pms.Deny, "remote()"),
			}),
			policyID:   "deny",
			policyKind: "policy",
		},
		{
			name:       "conditional GRANT cannot load",
			snapshot:   testSnapshot(remote, []*pms.Policy{testPolicy("grant", pms.Grant, "remote()")}),
			policyID:   "grant",
			policyKind: "policy",
		},
		{
			name: "role policy condition cannot load",
			snapshot: &pms.PolicyStore{
				Functions: remote,
				Services: []*pms.Service{{
					Name: "app",
					RolePolicies: []*pms.RolePolicy{{
						ID: "role-grant", Effect: pms.Grant, Roles: []string{"reader"},
						Principals: []string{"user:alice"}, Condition: "remote()",
					}},
				}},
			},
			policyID:   "role-grant",
			policyKind: "role policy",
		},
		{
			name: "short-circuited nested call cannot load",
			snapshot: testSnapshot(remote, []*pms.Policy{
				testPolicy("nested", pms.Deny, "false && Sqrt(4) == 2 && remote()"),
			}),
			policyID:   "nested",
			policyKind: "policy",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			instance, err := eval.NewWithStore(&cfg.Config{}, &snapshotStore{snapshot: tc.snapshot})
			if instance != nil {
				t.Fatalf("evaluator loaded with a custom-function condition: %v", instance)
			}
			requireUnsupported(t, nil, err)
			for _, detail := range []string{tc.policyKind, tc.policyID, "remote", "app"} {
				if !strings.Contains(err.Error(), detail) {
					t.Errorf("error %q does not identify %q", err, detail)
				}
			}
		})
	}
}

func TestWasmLoadsAndEvaluatesWithoutCustomFunctionCalls(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *pms.PolicyStore
	}{
		{
			name:     "no custom functions",
			snapshot: testSnapshot(nil, []*pms.Policy{testPolicy("grant", pms.Grant, "")}),
		},
		{
			name: "unused definition and built-in condition",
			snapshot: testSnapshot(
				[]*pms.Function{{Name: "remote", FuncURL: "https://example.invalid"}},
				[]*pms.Policy{testPolicy("builtin", pms.Grant, "Sqrt(64) == 8 && 'remote()' == 'remote()'")},
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine := newWasmEvaluatorFromStore(t, &snapshotStore{snapshot: tc.snapshot})
			allowed, reason, err := engine.IsAllowed(aliceRequest())
			if err != nil || !allowed || reason != adsapi.GRANT_POLICY_FOUND {
				t.Fatalf("allowed=%v reason=%v err=%v", allowed, reason, err)
			}
		})
	}
}

func TestWasmRejectsCustomFunctionSnapshotOnRefresh(t *testing.T) {
	valid := testSnapshot(nil, []*pms.Policy{testPolicy("grant", pms.Grant, "")})
	store := &snapshotStore{snapshot: valid}
	engine := newWasmEvaluatorFromStore(t, store)

	store.snapshot = testSnapshot(
		[]*pms.Function{{Name: "remote", FuncURL: "https://example.invalid"}},
		[]*pms.Policy{
			testPolicy("grant", pms.Grant, ""),
			testPolicy("deny", pms.Deny, "remote()"),
		},
	)
	requireUnsupported(t, nil, engine.Refresh())
	if allowed, _, err := engine.IsAllowed(aliceRequest()); allowed || err == nil {
		t.Fatalf("rejected snapshot left authorization available: allowed=%v err=%v", allowed, err)
	}

	store.snapshot = valid
	if err := engine.Refresh(); err != nil {
		t.Fatalf("valid snapshot did not recover evaluator: %v", err)
	}
	if allowed, _, err := engine.IsAllowed(aliceRequest()); !allowed || err != nil {
		t.Fatalf("valid snapshot did not restore authorization: allowed=%v err=%v", allowed, err)
	}
}
