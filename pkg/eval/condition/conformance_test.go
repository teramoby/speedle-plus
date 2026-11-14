// Licensed under the Universal Permissive License (UPL) Version 1.0.
package condition

import "testing"

func TestConformanceEscapedFunctionDiscovery(t *testing.T) {
	// A backslash in a bare name makes calledNames miss the complete name;
	// Check must recover it from the parser's undefined-function diagnostic.
	for _, expression := range []string{`f\oo(x)`, `f\oo(x) && b\ar(y)`} {
		if err := Check(expression); err != nil {
			t.Fatalf("%q: %v", expression, err)
		}
	}
	if got, err := anyFunction(); got != true || err != nil {
		t.Fatalf("placeholder=%v %v", got, err)
	}
}
