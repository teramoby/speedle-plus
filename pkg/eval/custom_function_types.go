// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"reflect"
	"strings"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// checkCustomFunctionArguments checks a call against the function's declared
// parameter types. A function without declarations is not checked.
func checkCustomFunctionArguments(cf *pms.Function, arguments []interface{}) error {
	if len(cf.Params) == 0 {
		return nil
	}
	if matchDeclaredArguments(cf.Params, arguments) {
		return nil
	}
	// Never include an argument value: it may carry request data.
	if len(arguments) != len(cf.Params) && !declaresListParameter(cf.Params) {
		return newCustomFunctionError(cf.Name, errors.Errorf(errors.CustomerFuncError,
			"expects %d parameters, got %d", len(cf.Params), len(arguments)))
	}
	for i, declared := range cf.Params {
		if i < len(arguments) && !customFunctionValueMatches(declared, arguments[i]) {
			return newCustomFunctionError(cf.Name, errors.Errorf(errors.CustomerFuncError,
				"parameter %d is %T, want %s", i+1, arguments[i], declared))
		}
	}
	return newCustomFunctionError(cf.Name, errors.Errorf(errors.CustomerFuncError,
		"arguments (%d) do not match the declared parameter types [%s]", len(arguments), strings.Join(cf.Params, ", ")))
}

// checkCustomFunctionResult checks a returned value against the function's
// declared result type. A function without a declaration is not checked.
func checkCustomFunctionResult(cf *pms.Function, result interface{}) error {
	if cf.Result == "" {
		return nil
	}
	if !customFunctionValueMatches(cf.Result, result) {
		return newCustomFunctionError(cf.Name, errors.Errorf(errors.CustomerFuncError,
			"result is %T, want %s", result, cf.Result))
	}
	return nil
}

func declaresListParameter(declared []string) bool {
	for _, param := range declared {
		if strings.HasSuffix(param, "[]") {
			return true
		}
	}
	return false
}

// matchDeclaredArguments reports whether the arguments match the declared
// parameter types. A scalar parameter matches one argument. A list parameter
// matches either one list value or a run of its element type: a condition
// passes a list value to a function as that value's separate elements, so a
// list parameter stands for a variable number of arguments.
func matchDeclaredArguments(declared []string, arguments []interface{}) bool {
	// reachable[j] reports whether the parameters seen so far can consume
	// arguments[:j]. Each parameter is matched in one pass over the arguments,
	// so the work stays linear in their count however many parameters declare
	// a list.
	reachable := make([]bool, len(arguments)+1)
	reachable[0] = true
	for _, param := range declared {
		next := make([]bool, len(arguments)+1)
		element, list := strings.CutSuffix(param, "[]")
		if !list {
			for j := 0; j < len(arguments); j++ {
				if reachable[j] && customFunctionScalarMatches(param, arguments[j]) {
					next[j+1] = true
				}
			}
			reachable = next
			continue
		}
		// run[j] counts the consecutive arguments from j that match the
		// element type; a list value may also arrive as those elements.
		run := make([]int, len(arguments)+1)
		for j := len(arguments) - 1; j >= 0; j-- {
			if customFunctionScalarMatches(element, arguments[j]) {
				run[j] = run[j+1] + 1
			}
		}
		// The positions a list parameter can consume form one interval per
		// reachable position: [j, j+run[j]], including the empty list at j.
		// Marking the union of those intervals in one sweep avoids rescanning
		// the arguments for every start.
		furthest := -1
		for j := 0; j <= len(arguments); j++ {
			if reachable[j] {
				// One argument that is a list of the element type.
				if j < len(arguments) && customFunctionValueMatches(param, arguments[j]) {
					next[j+1] = true
				}
				if end := j + run[j]; end > furthest {
					furthest = end
				}
			}
			if j <= furthest {
				next[j] = true
			}
		}
		reachable = next
	}
	return reachable[len(arguments)]
}

// customFunctionValueMatches reports whether a single value matches a
// declared type, including the list forms.
func customFunctionValueMatches(declared string, value interface{}) bool {
	if element, list := strings.CutSuffix(declared, "[]"); list {
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				if !customFunctionScalarMatches(element, rv.Index(i).Interface()) {
					return false
				}
			}
			return true
		}
		return false
	}
	return customFunctionScalarMatches(declared, value)
}

func customFunctionScalarMatches(declared string, value interface{}) bool {
	switch declared {
	case pms.FunctionTypeString:
		_, ok := value.(string)
		return ok
	case pms.FunctionTypeNumeric:
		return isNumericValue(value)
	case pms.FunctionTypeBool:
		_, ok := value.(bool)
		return ok
	case pms.FunctionTypeDatetime:
		return isDatetimeValue(value)
	}
	return false
}

// isNumericValue accepts the integer and floating-point types that a
// condition, an attribute or a JSON response can produce.
func isNumericValue(value interface{}) bool {
	switch value.(type) {
	case float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	}
	return false
}

// isDatetimeValue accepts the representations a condition, an attribute or a
// JSON response uses for a datetime: a parsed constant, a Unix time as SPDL
// attributes carry it, or an RFC3339 string.
func isDatetimeValue(value interface{}) bool {
	if _, ok := value.(time.Time); ok {
		return true
	}
	if isNumericValue(value) {
		return true
	}
	if text, ok := value.(string); ok {
		_, err := time.Parse(time.RFC3339Nano, text)
		return err == nil
	}
	return false
}
