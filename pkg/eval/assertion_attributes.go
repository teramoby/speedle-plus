// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"reflect"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

// Copy mutable JSON-shaped containers before passing them to an asserter.
// Preserve concrete scalar/array types (including time values) for embedded
// callers. Bound nesting so malformed cyclic/deep containers cannot exhaust
// the stack while preparing the failure boundary.
func copyAssertionAttributes(attributes map[string]interface{}) (map[string]interface{}, error) {
	value, err := copyAssertionValue(reflect.ValueOf(attributes), 0)
	if err != nil {
		return nil, err
	}
	return value.Interface().(map[string]interface{}), nil
}

func copyAssertionValue(value reflect.Value, depth int) (reflect.Value, error) {
	if depth > 128 {
		return reflect.Value{}, errors.New(errors.EvalEngineError, "identity assertion attributes exceed the nesting limit")
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value, nil
		}
		copied, err := copyAssertionValue(value.Elem(), depth+1)
		if err != nil {
			return reflect.Value{}, err
		}
		result := reflect.New(value.Type()).Elem()
		result.Set(copied)
		return result, nil
	case reflect.Map:
		if value.IsNil() {
			return value, nil
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		entries := value.MapRange()
		for entries.Next() {
			copied, err := copyAssertionValue(entries.Value(), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			result.SetMapIndex(entries.Key(), copied)
		}
		return result, nil
	case reflect.Slice, reflect.Array:
		var result reflect.Value
		if value.Kind() == reflect.Slice {
			if value.IsNil() {
				return value, nil
			}
			result = reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		} else {
			result = reflect.New(value.Type()).Elem()
		}
		for i := 0; i < value.Len(); i++ {
			copied, err := copyAssertionValue(value.Index(i), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			result.Index(i).Set(copied)
		}
		return result, nil
	default:
		return value, nil
	}
}

func commitAssertionAttributes(original, private map[string]interface{}, ctxAttributes *map[string]interface{}) {
	if original != nil {
		clear(original)
		for key, value := range private {
			original[key] = value
		}
	}
	// Preserve map identity after ordinary mutation, while retaining a
	// callback's explicit replacement map. No pointed-to memory is accessed.
	if reflect.ValueOf(*ctxAttributes).UnsafePointer() == reflect.ValueOf(private).UnsafePointer() {
		*ctxAttributes = original
	}
}
