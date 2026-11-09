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

func newWasmEvaluator(t *testing.T, policies []*pms.Policy) *eval.PolicyEvalImpl {
	t.Helper()
	store := &snapshotStore{snapshot: &pms.PolicyStore{
		Functions: []*pms.Function{{Name: "remote", FuncURL: "https://example.invalid"}},
		Services:  []*pms.Service{{Name: "app", Policies: policies}},
	}}
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

func TestWasmCustomFunctionConditionsDoNotMatch(t *testing.T) {
	tests := []struct {
		name       string
		policies   []*pms.Policy
		wantAllow  bool
		wantReason adsapi.Reason
	}{
		{
			name: "conditional DENY is skipped and another GRANT applies",
			policies: []*pms.Policy{
				testPolicy("grant", pms.Grant, ""),
				testPolicy("deny", pms.Deny, "remote()"),
			},
			wantAllow:  true,
			wantReason: adsapi.GRANT_POLICY_FOUND,
		},
		{
			name:       "conditional GRANT does not match",
			policies:   []*pms.Policy{testPolicy("grant", pms.Grant, "remote()")},
			wantAllow:  false,
			wantReason: adsapi.NO_APPLICABLE_POLICIES,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine := newWasmEvaluator(t, tc.policies)
			allowed, reason, err := engine.IsAllowed(adsapi.RequestContext{
				Subject:     &adsapi.Subject{Principals: []*adsapi.Principal{{Type: adsapi.PRINCIPAL_TYPE_USER, Name: "alice"}}},
				ServiceName: "app",
				Resource:    "document",
				Action:      "read",
			})
			if err != nil || allowed != tc.wantAllow || reason != tc.wantReason {
				t.Fatalf("allowed=%v reason=%v err=%v; want allowed=%v reason=%v", allowed, reason, err, tc.wantAllow, tc.wantReason)
			}
		})
	}
}
