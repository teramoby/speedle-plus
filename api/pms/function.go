// Licensed under the Universal Permissive License (UPL) Version 1.0.
package pms

import (
	"fmt"
	"strings"
)

// Custom function parameter and result type names. They use the SPDL attribute
// type names; a trailing [] declares a list of that type.
const (
	FunctionTypeString   = "string"
	FunctionTypeNumeric  = "numeric"
	FunctionTypeBool     = "bool"
	FunctionTypeDatetime = "datetime"
)

// IsFunctionType reports whether name is a supported custom function parameter
// or result type: one of the scalar type names, optionally followed by [].
func IsFunctionType(name string) bool {
	scalar, list := strings.CutSuffix(name, "[]")
	if list && strings.HasSuffix(scalar, "[]") {
		// Lists of lists are not supported.
		return false
	}
	switch scalar {
	case FunctionTypeString, FunctionTypeNumeric, FunctionTypeBool, FunctionTypeDatetime:
		return true
	}
	return false
}

// ValidateTypeDeclarations checks the optional type declarations of a custom
// function. An empty Params or Result declares nothing and is valid.
func (f *Function) ValidateTypeDeclarations() error {
	for i, param := range f.Params {
		if !IsFunctionType(param) {
			return fmt.Errorf("params[%d] %q is not a supported type", i, param)
		}
	}
	if f.Result != "" && !IsFunctionType(f.Result) {
		return fmt.Errorf("result %q is not a supported type", f.Result)
	}
	return nil
}
