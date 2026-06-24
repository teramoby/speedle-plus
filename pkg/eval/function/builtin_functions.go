//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package function

import (
	"math"
	"reflect"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

// Add all built-in functions in this file

func Sqrt(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: Sqrt(x)")
	if len(args) != 1 {
		return nil, err
	}
	x, ok := args[0].(float64)
	if !ok {
		return nil, err
	}
	return math.Sqrt(x), nil
}

func Max(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: Max(x1, x2, ...), xi must be numeric")
	if len(args) < 1 {
		return nil, err
	}
	max, ok := args[0].(float64)
	if !ok {
		return nil, err
	}
	for i := 1; i < len(args); i++ {
		v, ok := args[i].(float64)
		if !ok {
			return nil, err
		}
		max = math.Max(max, v)
	}
	return max, nil
}

func Min(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: Min(x1, x2, ...), xi must be numeric")
	if len(args) < 1 {
		return nil, err
	}
	min, ok := args[0].(float64)
	if !ok {
		return nil, err
	}
	for i := 1; i < len(args); i++ {
		v, ok := args[i].(float64)
		if !ok {
			return nil, err
		}
		min = math.Min(min, v)
	}
	return min, nil
}

func Sum(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: Sum(x1, x2, ...), xi must be numeric")
	var sum float64 = 0
	for i := range args {
		v, ok := args[i].(float64)
		if !ok {
			return nil, err
		}
		sum += v
	}
	return sum, nil
}

func Avg(args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return float64(0), nil
	}
	sum, err := Sum(args...)
	if err != nil {
		return nil, err
	}
	s, ok := sum.(float64)
	if !ok {
		return nil, errors.New(errors.BuiltInFuncError, "Sum did not return a float64")
	}
	return s / float64(len(args)), nil
}

// IsSubSet(S1, S2) means "is S1 a subset of S2"
func IsSubSet(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: IsSubSet(S1, S2) - S1 and S2 are both slice, and test if S1 is a subset of S2")
	n := len(args)
	if n < 2 {
		return nil, err
	}

	var s1, s2 interface{}
	if n == 2 {
		// The expression evaluator (govaluate) converts typed slices to
		// []interface{} via MapParameters.Get. When s1 has a single
		// element, the separatorStage places it as a bare value instead of
		// wrapping it in a slice. Detect and repair that case.
		s1, s2 = args[0], args[1]
		// Guard against nil arguments (e.g. a referenced attribute that
		// resolved to JSON null). reflect.TypeOf(nil) returns nil, and
		// calling .Kind() on a nil Type panics.
		t1, t2 := reflect.TypeOf(s1), reflect.TypeOf(s2)
		if t1 == nil || t2 == nil {
			return false, nil
		}
		if t1.Kind() != reflect.Slice && t2.Kind() == reflect.Slice {
			s1 = []interface{}{args[0]}
		}
	} else {
		// More than 2 args: the evaluator expanded both parameters into
		// individual elements. Reconstruct: first n-1 args form s1, last
		// arg is s2 (which itself is an []interface{} from the evaluator).
		buf := make([]interface{}, n-1)
		copy(buf, args[:n-1])
		s1 = buf
		s2 = args[n-1]
	}

	// Both s1 and s2 must be slices.
	t1, t2 := reflect.TypeOf(s1), reflect.TypeOf(s2)
	if t1 == nil || t2 == nil ||
		t1.Kind() != reflect.Slice || t2.Kind() != reflect.Slice {
		return nil, err
	}

	v1 := reflect.ValueOf(s1)
	v2 := reflect.ValueOf(s2)
	n1 := v1.Len()
	n2 := v2.Len()
	if n1 == 0 || n2 == 0 || n1 > n2 {
		return false, nil
	}

outer:
	for i := 0; i < n1; i++ {
		for j := 0; j < n2; j++ {
			if v1.Index(i).Interface() == v2.Index(j).Interface() {
				continue outer
			}
		}
		return false, nil
	}
	return true, nil
}
