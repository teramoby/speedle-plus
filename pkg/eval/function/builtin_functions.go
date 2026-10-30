//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package function

import (
	"math"
	"math/big"
	"reflect"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

// Add all built-in functions in this file

// numbers returns the arguments as float64, or false when any is not numeric
// (including nil).
func numbers(args []interface{}) ([]float64, bool) {
	values := make([]float64, len(args))
	for i, arg := range args {
		value, ok := arg.(float64)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, false
		}
		values[i] = value
	}
	return values, true
}

func Sqrt(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: Sqrt(x)")
	values, ok := numbers(args)
	if !ok || len(values) != 1 || values[0] < 0 {
		return nil, err
	}
	return math.Sqrt(values[0]), nil
}

func Max(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: Max(x1, x2, ...), xi must be numeric")
	values, ok := numbers(args)
	if !ok || len(values) < 1 {
		return nil, err
	}
	max := values[0]
	for _, value := range values[1:] {
		max = math.Max(max, value)
	}
	return max, nil
}

func Min(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: Min(x1, x2, ...), xi must be numeric")
	values, ok := numbers(args)
	if !ok || len(values) < 1 {
		return nil, err
	}
	min := values[0]
	for _, value := range values[1:] {
		min = math.Min(min, value)
	}
	return min, nil
}

func Sum(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: Sum(x1, x2, ...), xi must be numeric")
	values, ok := numbers(args)
	if !ok {
		return nil, err
	}
	var sum float64 = 0
	for _, value := range values {
		sum += value
	}
	if math.IsNaN(sum) || math.IsInf(sum, 0) {
		return nil, errors.New(errors.BuiltInFuncError, "Sum result is not finite")
	}
	return sum, nil
}

func Avg(args ...interface{}) (interface{}, error) {
	values, ok := numbers(args)
	if !ok {
		return nil, errors.New(errors.BuiltInFuncError, "Usage: Avg(x1, x2, ...), xi must be finite numeric")
	}
	// Keep ordinary arithmetic stable (including cancellation), scaling only
	// when the intermediate sum overflows but the mean is representable.
	if len(values) == 0 {
		return float64(0), nil
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	if !math.IsNaN(sum) && !math.IsInf(sum, 0) {
		return sum / float64(len(values)), nil
	}
	var avg float64
	for _, value := range values {
		avg += value / float64(len(values))
	}
	if math.IsNaN(avg) || math.IsInf(avg, 0) {
		return nil, errors.New(errors.BuiltInFuncError, "Avg result is not finite")
	}

	return avg, nil
}

// IsSubSet accepts two scalar slices, or the flattened first slice supplied by
// govaluate. Empty sets retain the historical false result. Duplicates do not
// change membership. Unsupported/mixed element types return BuiltInFuncError.
func IsSubSet(args ...interface{}) (interface{}, error) {
	err := errors.New(errors.BuiltInFuncError, "Usage: IsSubSet(S1, S2) - homogeneous scalar sets required")
	if len(args) < 2 {
		return nil, err
	}
	last := reflect.ValueOf(args[len(args)-1])
	if !last.IsValid() || last.Kind() != reflect.Slice {
		return nil, err
	}
	first := reflect.ValueOf(args[:len(args)-1])
	if len(args) == 2 && args[0] != nil && reflect.TypeOf(args[0]).Kind() == reflect.Slice {
		first = reflect.ValueOf(args[0])
	}
	kind := ""
	convert := func(v interface{}) (interface{}, bool) {
		if v == nil {
			return nil, false
		}
		r := reflect.ValueOf(v)
		var k string
		switch r.Kind() {
		case reflect.String:
			k = "string"
			v = r.String()
		case reflect.Bool:
			k = "bool"
			v = r.Bool()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			k = "number"
			v = new(big.Rat).SetInt64(r.Int()).String()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			k = "number"
			v = new(big.Rat).SetUint64(r.Uint()).String()
		case reflect.Float32, reflect.Float64:
			k = "number"
			number := r.Float()
			if math.IsNaN(number) || math.IsInf(number, 0) {
				return nil, false
			}
			v = new(big.Rat).SetFloat64(number).String()
		default:
			return nil, false
		}
		if kind != "" && kind != k {
			return nil, false
		}
		kind = k
		return v, true
	}
	set := make(map[interface{}]bool, last.Len())
	for i := 0; i < last.Len(); i++ {
		v, ok := convert(last.Index(i).Interface())
		if !ok {
			return nil, err
		}
		set[v] = true
	}
	matched := true
	for i := 0; i < first.Len(); i++ {
		v, ok := convert(first.Index(i).Interface())
		if !ok {
			return nil, err
		}
		matched = matched && set[v]
	}
	return first.Len() > 0 && last.Len() > 0 && matched, nil
}
