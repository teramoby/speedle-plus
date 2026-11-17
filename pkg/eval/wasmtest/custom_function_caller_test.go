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

// registerCaller installs a host caller for one test and removes it afterwards.
func registerCaller(t *testing.T, caller eval.CustomFunctionCaller) {
	t.Helper()
	eval.RegisterCustomFunctionCaller(caller)
	t.Cleanup(func() { eval.RegisterCustomFunctionCaller(nil) })
}

func levelRequest(level float64) adsapi.RequestContext {
	ctx := aliceRequest()
	ctx.Attributes = map[string]interface{}{"level": level}
	return ctx
}

// TestWasmRegisteredCallerEvaluatesConditions checks that a registered caller
// makes custom-function snapshots loadable and serves their calls.
func TestWasmRegisteredCallerEvaluatesConditions(t *testing.T) {
	var calls int
	registerCaller(t, func(cf *pms.Function, request *ext.CustomerFunctionRequest) (interface{}, error) {
		calls++
		level, ok := request.Params[0].(float64)
		return ok && level > 0, nil
	})
	remote := []*pms.Function{{Name: "remote", FuncURL: "https://example.invalid",
		Params: []string{"numeric"}, Result: "bool"}}
	engine := newWasmEvaluatorFromStore(t, &snapshotStore{snapshot: testSnapshot(remote, []*pms.Policy{
		testPolicy("grant", pms.Grant, "remote(level)"),
	})})

	allowed, reason, err := engine.IsAllowed(levelRequest(1))
	if err != nil || !allowed || reason != adsapi.GRANT_POLICY_FOUND {
		t.Fatalf("allowed=%v reason=%v err=%v", allowed, reason, err)
	}
	if allowed, _, err := engine.IsAllowed(levelRequest(0)); err != nil || allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if calls != 2 {
		t.Errorf("%d caller invocations, want 2", calls)
	}
}

// TestWasmCallerFailureFailsDecision checks that a caller error and a declared
// type mismatch fail the decision instead of not matching.
func TestWasmCallerFailureFailsDecision(t *testing.T) {
	failure := &pms.Function{Name: "remote", FuncURL: "https://example.invalid",
		Params: []string{"numeric"}, Result: "bool"}
	invalid := &pms.Function{Name: "remote", FuncURL: "https://example.invalid", Result: "bool"}
	tests := []struct {
		name      string
		function  *pms.Function
		condition string
		caller    eval.CustomFunctionCaller
	}{
		{
			name:      "caller error",
			function:  failure,
			condition: "remote(level)",
			caller: func(*pms.Function, *ext.CustomerFunctionRequest) (interface{}, error) {
				return nil, spdlerrors.New(spdlerrors.CustomerFuncError, "function unavailable")
			},
		},
		{
			name:      "wrong argument type",
			function:  failure,
			condition: "remote('high')",
			caller: func(*pms.Function, *ext.CustomerFunctionRequest) (interface{}, error) {
				return true, nil
			},
		},
		{
			name:      "wrong result type",
			function:  invalid,
			condition: "remote() == true",
			caller: func(*pms.Function, *ext.CustomerFunctionRequest) (interface{}, error) {
				return "yes", nil
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			registerCaller(t, tc.caller)
			engine := newWasmEvaluatorFromStore(t, &snapshotStore{snapshot: testSnapshot(
				[]*pms.Function{tc.function}, []*pms.Policy{testPolicy("grant", pms.Grant, tc.condition)})})

			allowed, reason, err := engine.IsAllowed(levelRequest(1))
			if err == nil || allowed {
				t.Fatalf("allowed=%v err=%v", allowed, err)
			}
			if !eval.IsCustomFunctionError(err) || spdlerrors.Code(err) != spdlerrors.CustomerFuncError {
				t.Errorf("error %v is not a customer function error", err)
			}
			if reason != adsapi.ERROR_IN_EVALUATION {
				t.Errorf("reason = %v", reason)
			}
			result, err := engine.Diagnose(levelRequest(1))
			if err == nil || result == nil || !strings.Contains(err.Error(), "remote") {
				t.Errorf("Diagnose result=%v err=%v", result, err)
			}
		})
	}
}

// TestWasmUnregisteredCallerStillRejects checks that removing the caller
// restores the js/wasm rejection of custom-function snapshots.
func TestWasmUnregisteredCallerStillRejects(t *testing.T) {
	registerCaller(t, func(*pms.Function, *ext.CustomerFunctionRequest) (interface{}, error) {
		return true, nil
	})
	remote := []*pms.Function{{Name: "remote", FuncURL: "https://example.invalid"}}
	newWasmEvaluatorFromStore(t, &snapshotStore{snapshot: testSnapshot(remote, []*pms.Policy{
		testPolicy("grant", pms.Grant, "remote()"),
	})})

	eval.RegisterCustomFunctionCaller(nil)
	instance, err := eval.NewWithStore(&cfg.Config{}, &snapshotStore{snapshot: testSnapshot(remote, []*pms.Policy{
		testPolicy("grant", pms.Grant, "remote()"),
	})})
	requireUnsupported(t, instance, err)
}
