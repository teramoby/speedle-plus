// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// resultServer serves one fixed function response for every call.
func resultServer(t *testing.T, body string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// newDeclaredFunctionEvaluator builds an evaluator whose only policy is a
// grant to alice with the given condition.
func newDeclaredFunctionEvaluator(t *testing.T, function, condition string) *PolicyEvalImpl {
	t.Helper()
	p, _ := newFixtureEvaluator(t, fmt.Sprintf(`{
		"functions":[%s],
		"services":[{"name":"app","policies":[{"id":"grant","effect":"grant",%s,"condition":%q}]}]}`,
		function, aliceReadPrincipals, condition))
	return p
}

func functionJSON(url string, extra string) string {
	return fmt.Sprintf(`{"name":"f","funcURL":%q,"resultCachable":false%s}`, url, extra)
}

func TestFunctionTypeDeclarationsAcceptMatchingValues(t *testing.T) {
	cases := []struct {
		name       string
		function   string
		response   string
		condition  string
		attributes map[string]interface{}
	}{
		{
			name:      "scalars",
			function:  `,"params":["numeric","string"],"result":"numeric"`,
			response:  `{"result":7}`,
			condition: "f(1, 'a') == 7",
		},
		{
			name:      "bool result",
			function:  `,"params":["bool"],"result":"bool"`,
			response:  `{"result":true}`,
			condition: "f(true)",
		},
		{
			name:      "string result",
			function:  `,"result":"string"`,
			response:  `{"result":"open"}`,
			condition: "f() == 'open'",
		},
		{
			name:      "numeric list argument",
			function:  `,"params":["numeric[]"],"result":"bool"`,
			response:  `{"result":true}`,
			condition: "f((1, 2, 3))",
		},
		{
			name:      "single element list argument",
			function:  `,"params":["numeric[]"],"result":"bool"`,
			response:  `{"result":true}`,
			condition: "f(1)",
		},
		{
			name:      "empty list argument",
			function:  `,"params":["string[]"],"result":"bool"`,
			response:  `{"result":true}`,
			condition: "f(groups)",
			attributes: map[string]interface{}{
				"groups": []interface{}{},
			},
		},
		{
			name:      "string list argument from attributes",
			function:  `,"params":["string[]"],"result":"bool"`,
			response:  `{"result":true}`,
			condition: "f(groups)",
			attributes: map[string]interface{}{
				"groups": []interface{}{"a", "b"},
			},
		},
		{
			name:      "datetime argument constant",
			function:  `,"params":["datetime"],"result":"bool"`,
			response:  `{"result":true}`,
			condition: "f('2020-01-02T03:04:05Z')",
		},
		{
			name:      "datetime argument attribute",
			function:  `,"params":["datetime"],"result":"bool"`,
			response:  `{"result":true}`,
			condition: "f(attr_time)",
			attributes: map[string]interface{}{
				"attr_time": float64(1577934245),
			},
		},
		{
			name:      "datetime result",
			function:  `,"result":"datetime"`,
			response:  `{"result":"2020-01-02T03:04:05Z"}`,
			condition: "f() == f()",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newDeclaredFunctionEvaluator(t, functionJSON(resultServer(t, tc.response), tc.function), tc.condition)
			ctx := aliceContext("app", "r", "read")
			ctx.Attributes = tc.attributes
			allowed, _, err := p.IsAllowed(ctx)
			if err != nil || !allowed {
				t.Fatalf("allowed=%v err=%v", allowed, err)
			}
		})
	}
}

func TestFunctionTypeDeclarationMismatchesFailTheDecision(t *testing.T) {
	cases := []struct {
		name      string
		function  string
		condition string
		message   string
	}{
		{
			name:      "argument count",
			function:  `,"params":["numeric","string"]`,
			condition: "f(1) == 1",
			message:   "expects 2 parameters, got 1",
		},
		{
			name:      "argument type",
			function:  `,"params":["numeric"]`,
			condition: "f('secret-value') == 1",
			message:   "parameter 1 is string, want numeric",
		},
		{
			name:      "scalar for a list",
			function:  `,"params":["numeric[]"]`,
			condition: "f(true) == 1",
			message:   "parameter 1 is bool, want numeric[]",
		},
		{
			name:      "list element type",
			function:  `,"params":["numeric[]"]`,
			condition: "f(('a', 'b')) == 1",
			message:   "parameter 1 is string, want numeric[]",
		},
		{
			name:      "result type",
			function:  `,"result":"numeric"`,
			condition: "f() == 1",
			message:   "result is string, want numeric",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newDeclaredFunctionEvaluator(t, functionJSON(resultServer(t, `{"result":"not-a-number"}`), tc.function), tc.condition)
			allowed, reason, err := p.IsAllowed(aliceContext("app", "r", "read"))
			if err == nil || allowed {
				t.Fatalf("allowed=%v err=%v", allowed, err)
			}
			if !IsCustomFunctionError(err) || errors.Code(err) != errors.CustomerFuncError {
				t.Errorf("error %v is not a customer function error", err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Errorf("error %q does not contain %q", err, tc.message)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Errorf("error %q exposes an argument value", err)
			}
			if reason != adsapi.ERROR_IN_EVALUATION {
				t.Errorf("reason = %v", reason)
			}
		})
	}
}

// TestUndeclaredFunctionUnchanged checks that a function without declarations
// is called exactly as before, including a result type the condition rejects.
func TestUndeclaredFunctionUnchanged(t *testing.T) {
	p := newDeclaredFunctionEvaluator(t, functionJSON(resultServer(t, `{"result":"text"}`), ""), "f() == 'text'")
	if allowed, _, err := p.IsAllowed(aliceContext("app", "r", "read")); err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	p = newDeclaredFunctionEvaluator(t, functionJSON(resultServer(t, `{"result":5}`), ""), "f() == 'text'")
	allowed, _, err := p.IsAllowed(aliceContext("app", "r", "read"))
	if err != nil || allowed {
		t.Fatalf("allowed=%v err=%v; an undeclared result must keep the old nonmatch", allowed, err)
	}
}

// TestFunctionTypeMismatchNotCached checks that a result rejected by its
// declaration is not stored in the result cache.
func TestFunctionTypeMismatchNotCached(t *testing.T) {
	var calls int32
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `{"result":"text"}`)
	}))
	t.Cleanup(counting.Close)
	p, _ := newFixtureEvaluator(t, fmt.Sprintf(`{
		"functions":[{"name":"f","funcURL":%q,"resultCachable":true,"resultTTL":60,"result":"numeric"}],
		"services":[{"name":"app","policies":[{"id":"grant","effect":"grant",%s,"condition":"f() == 1"}]}]}`,
		counting.URL, aliceReadPrincipals))
	for i := 0; i < 2; i++ {
		if _, _, err := p.IsAllowed(aliceContext("app", "r", "read")); !IsCustomFunctionError(err) {
			t.Fatalf("call %d: error %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("%d calls, want 2: a rejected result was cached", got)
	}
}
