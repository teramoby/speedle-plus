// Licensed under the Universal Permissive License (UPL) Version 1.0.
package function

import (
	"github.com/teramoby/speedle-plus/pkg/errors"
	"math"
	"testing"
)

func TestConformanceSubsetArityAndNonfinite(t *testing.T) {
	for _, args := range [][]interface{}{nil, {1.0}, {[]interface{}{math.NaN()}, []interface{}{1.0}}, {[]interface{}{1.0}, []interface{}{math.Inf(1)}}} {
		if value, err := IsSubSet(args...); value != nil || errors.Code(err) != errors.BuiltInFuncError {
			t.Fatalf("%v: %v %v", args, value, err)
		}
	}
}

func TestConformanceAverageScaledRoundingOverflow(t *testing.T) {
	// Eleven finite maxima overflow even after scaling because the running
	// floating-point sum rounds upward. Pin the error, rather than changing Avg.
	args := make([]interface{}, 11)
	for i := range args {
		args[i] = math.MaxFloat64
	}
	if value, err := Avg(args...); value != nil || errors.Code(err) != errors.BuiltInFuncError {
		t.Fatalf("Avg(11 maxima)=%v %v", value, err)
	}
}
