// Licensed under the Universal Permissive License (UPL) Version 1.0.
package condition

import (
	"errors"
	"reflect"
	"testing"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
)

func TestConformanceEscapedFunctionDiscoveryKnownLimitation(t *testing.T) {
	// known difference, behaviour pinned: the private calledNames pre-scan
	// truncates backslash-escaped names. It is used only to seed Check's parser;
	// it is not the authoritative name resolver or custom-function registry.
	// Unknown functions may be registered later. The separate diagnostic test
	// below pins the full names used by Check's fallback, rather than claiming
	// the truncated scan is correct discovery.
	for _, expression := range []string{`f\oo(x)`, `f\oo(x) && b\ar(y)`} {
		if err := Check(expression); err != nil {
			t.Fatalf("%q: %v", expression, err)
		}
		wantNames := []string{"oo"}
		if expression != `f\oo(x)` {
			wantNames = append(wantNames, "ar")
		}
		if got := calledNames(expression); !reflect.DeepEqual(got, wantNames) {
			t.Fatalf("discovered %v, want %v", got, wantNames)
		}
		// The parser resolves the escaped spelling when the function exists.
		// Unknown calls are pinned as explicit evaluation errors on both authorization paths
		// in TestConditionConformance, while these registered calls evaluate.
		identity := func(args ...interface{}) (interface{}, error) { return args[0], nil }
		compiled, err := govaluate.NewEvaluableExpressionWithFunctions(expression, map[string]govaluate.ExpressionFunction{"foo": identity, "bar": identity})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := compiled.Evaluate(map[string]interface{}{"x": true, "y": true}); got != true || err != nil {
			t.Fatalf("registered call: %v %v", got, err)
		}
	}
	if got, err := anyFunction(); got != true || err != nil {
		t.Fatalf("placeholder=%v %v", got, err)
	}
}

func TestConformanceEscapedFunctionDiagnosticRecovery(t *testing.T) {
	for _, c := range []struct {
		expression string
		resolved   []string
	}{
		{`f\oo(x)`, []string{"foo"}},
		{`f\oo(x) && b\ar(y)`, []string{"foo", "bar"}},
	} {
		t.Run(c.expression, func(t *testing.T) {
			functions := map[string]govaluate.ExpressionFunction{}
			for _, name := range calledNames(c.expression) {
				functions[name] = anyFunction
			}
			// Start with exactly Check's incomplete scan. Each parser diagnostic
			// must supply the full escaped name, in order; only registering that
			// full name may advance to the next unknown call or a successful parse.
			for _, name := range c.resolved {
				_, err := govaluate.NewEvaluableExpressionWithFunctions(c.expression, functions)
				var diagnostic *govaluate.ExpressionError
				if !errors.As(err, &diagnostic) || diagnostic.Message != "Undefined function "+name {
					t.Fatalf("parser diagnostic = %v, want full name %q", err, name)
				}
				functions[name] = anyFunction
			}
			if _, err := govaluate.NewEvaluableExpressionWithFunctions(c.expression, functions); err != nil {
				t.Fatalf("resolved names %v did not compile: %v", c.resolved, err)
			}
			if err := Check(c.expression); err != nil {
				t.Fatalf("Check did not recover the parser-resolved names: %v", err)
			}
		})
	}
}
