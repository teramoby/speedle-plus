// Licensed under the Universal Permissive License (UPL) Version 1.0.
package condition

import (
	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"reflect"
	"testing"
)

func TestConformanceEscapedFunctionDiscovery(t *testing.T) {
	// A backslash in a bare name makes calledNames miss the complete name;
	// Unknown functions are intentionally allowed at policy validation time: custom
	// functions can be registered later. Check must recover it from the parser's undefined-function diagnostic.
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
