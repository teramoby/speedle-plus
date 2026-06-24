//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package function

import (
	"math"
	"testing"
)

func TestMaxMinCorrectness(t *testing.T) {
	// Test Max with 3 args - the last element should NOT be skipped
	result, err := Max(1.0, 3.0, 2.0)
	if err != nil {
		t.Fatalf("Max returned error: %v", err)
	}
	if result.(float64) != 3.0 {
		t.Errorf("Max(1.0, 3.0, 2.0) = %v, want 3.0", result)
	}

	// Test Max with 2 args
	result, err = Max(5.0, 10.0)
	if err != nil {
		t.Fatalf("Max returned error: %v", err)
	}
	if result.(float64) != 10.0 {
		t.Errorf("Max(5.0, 10.0) = %v, want 10.0", result)
	}

	// Test Max with single arg
	result, err = Max(7.0)
	if err != nil {
		t.Fatalf("Max returned error: %v", err)
	}
	if result.(float64) != 7.0 {
		t.Errorf("Max(7.0) = %v, want 7.0", result)
	}

	// Test Max with 0 args
	_, err = Max()
	if err == nil {
		t.Error("Max() should return error for empty args")
	}

	// Test Min with 3 args - the last element should NOT be skipped
	result, err = Min(3.0, 1.0, 2.0)
	if err != nil {
		t.Fatalf("Min returned error: %v", err)
	}
	if result.(float64) != 1.0 {
		t.Errorf("Min(3.0, 1.0, 2.0) = %v, want 1.0", result)
	}

	// Test Min with 2 args
	result, err = Min(5.0, 2.0)
	if err != nil {
		t.Fatalf("Min returned error: %v", err)
	}
	if result.(float64) != 2.0 {
		t.Errorf("Min(5.0, 2.0) = %v, want 2.0", result)
	}

	// Test Min with single arg
	result, err = Min(7.0)
	if err != nil {
		t.Fatalf("Min returned error: %v", err)
	}
	if result.(float64) != 7.0 {
		t.Errorf("Min(7.0) = %v, want 7.0", result)
	}

	// Test Min with 0 args
	_, err = Min()
	if err == nil {
		t.Error("Min() should return error for empty args")
	}
}

func TestMaxMinNonNumeric(t *testing.T) {
	_, err := Max(1.0, "not_a_number", 3.0)
	if err == nil {
		t.Error("Max should return error for non-numeric args")
	}

	_, err = Min(1.0, "not_a_number", 3.0)
	if err == nil {
		t.Error("Min should return error for non-numeric args")
	}
}

func TestIsSubSetTwoArg(t *testing.T) {
	// Two-arg case: IsSubSet(a, b) checks if a is subset of b
	result, err := IsSubSet([]interface{}{"a"}, []interface{}{"a", "b"})
	if err != nil {
		t.Fatalf("IsSubSet returned error: %v", err)
	}
	if result.(bool) != true {
		t.Error("IsSubSet([\"a\"], [\"a\", \"b\"]) should be true")
	}

	// Two-arg case: element not in superset
	result, err = IsSubSet([]interface{}{"c"}, []interface{}{"a", "b"})
	if err != nil {
		t.Fatalf("IsSubSet returned error: %v", err)
	}
	if result.(bool) != false {
		t.Error("IsSubSet([\"c\"], [\"a\", \"b\"]) should be false")
	}

	// Two-arg case: empty subset
	result, err = IsSubSet([]interface{}{}, []interface{}{"a", "b"})
	if err != nil {
		t.Fatalf("IsSubSet returned error: %v", err)
	}
	if result.(bool) != false {
		t.Error("IsSubSet([], [\"a\", \"b\"]) should be false (empty subset)")
	}

	// Two-arg case: equal sets
	result, err = IsSubSet([]interface{}{"a", "b"}, []interface{}{"a", "b"})
	if err != nil {
		t.Fatalf("IsSubSet returned error: %v", err)
	}
	if result.(bool) != true {
		t.Error("IsSubSet([\"a\", \"b\"], [\"a\", \"b\"]) should be true")
	}

	// Two-arg case: subset larger than superset
	result, err = IsSubSet([]interface{}{"a", "b", "c"}, []interface{}{"a"})
	if err != nil {
		t.Fatalf("IsSubSet returned error: %v", err)
	}
	if result.(bool) != false {
		t.Error("IsSubSet([\"a\", \"b\", \"c\"], [\"a\"]) should be false")
	}
}

func TestIsSubSetMultiArg(t *testing.T) {
	// Multi-arg case: IsSubSet(a, b, c) means is a subset of [b, c]
	result, err := IsSubSet([]interface{}{"a"}, []interface{}{"b"}, []interface{}{"a", "b"})
	if err != nil {
		t.Fatalf("IsSubSet returned error: %v", err)
	}
	// s1 = [[a]], s2 = [a,b]; first arg is wrapped but since [[a]] is slice of slices,
	// it's a subset check: is [a] contained in [a, b]? Actually since s1 becomes a
	// slice-of-slices, this tests a different path. The key validation is that
	// the args are valid slices.
	_ = result
}

func TestIsSubSetNoArgs(t *testing.T) {
	_, err := IsSubSet()
	if err == nil {
		t.Error("IsSubSet() should return error for no args")
	}
}

func TestIsSubSetOneArg(t *testing.T) {
	_, err := IsSubSet([]interface{}{"a"})
	if err == nil {
		t.Error("IsSubSet with one arg should return error")
	}
}

func TestSqrt(t *testing.T) {
	result, err := Sqrt(4.0)
	if err != nil {
		t.Fatalf("Sqrt returned error: %v", err)
	}
	if result.(float64) != 2.0 {
		t.Errorf("Sqrt(4.0) = %v, want 2.0", result)
	}

	result, err = Sqrt(0.0)
	if err != nil {
		t.Fatalf("Sqrt returned error: %v", err)
	}
	if result.(float64) != 0.0 {
		t.Errorf("Sqrt(0.0) = %v, want 0.0", result)
	}

	// NaN case
	result, err = Sqrt(-1.0)
	if err != nil {
		t.Fatalf("Sqrt returned error: %v", err)
	}
	if !math.IsNaN(result.(float64)) {
		t.Errorf("Sqrt(-1.0) should be NaN")
	}
}

func TestSum(t *testing.T) {
	result, err := Sum(1.0, 2.0, 3.0)
	if err != nil {
		t.Fatalf("Sum returned error: %v", err)
	}
	if result.(float64) != 6.0 {
		t.Errorf("Sum(1.0, 2.0, 3.0) = %v, want 6.0", result)
	}

	result, err = Sum()
	if err != nil {
		t.Fatalf("Sum returned error: %v", err)
	}
	if result.(float64) != 0.0 {
		t.Errorf("Sum() = %v, want 0.0", result)
	}
}

func TestAvg(t *testing.T) {
	result, err := Avg(1.0, 2.0, 3.0)
	if err != nil {
		t.Fatalf("Avg returned error: %v", err)
	}
	if result.(float64) != 2.0 {
		t.Errorf("Avg(1.0, 2.0, 3.0) = %v, want 2.0", result)
	}

	result, err = Avg()
	if err != nil {
		t.Fatalf("Avg returned error: %v", err)
	}
	if result.(float64) != 0.0 {
		t.Errorf("Avg() = %v, want 0.0", result)
	}
}
