// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	stderrors "errors"
	"fmt"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

// CustomFunctionError reports a failed custom function call: a timeout or
// transport error, an error status or response, or a value that does not match
// the declared parameter or result types. A condition that reaches such a
// failure fails the whole decision instead of not matching, so an
// authorization request cannot silently ignore a function that could not run.
type CustomFunctionError struct {
	// Function is the name of the custom function that failed.
	Function string
	// Err is the failure reported by the function call or a type check.
	Err error
}

func (e *CustomFunctionError) Error() string {
	return fmt.Sprintf("custom function %q: %v", e.Function, e.Err)
}

// Unwrap exposes the underlying failure to the standard errors.Is and
// errors.As functions.
func (e *CustomFunctionError) Unwrap() error {
	return e.Err
}

// Code keeps the underlying error code when it has one; a type mismatch, which
// has no transport error below it, is a customer function error.
func (e *CustomFunctionError) Code() errors.ErrorCode {
	if code := errors.Code(e.Err); code != errors.UnknownError {
		return code
	}
	return errors.CustomerFuncError
}

// IsCustomFunctionError reports whether err is or wraps a
// CustomFunctionError. Only these errors fail a decision; every other
// condition error keeps its existing "does not match" behavior.
func IsCustomFunctionError(err error) bool {
	var target *CustomFunctionError
	return stderrors.As(err, &target)
}

// newCustomFunctionError wraps a call failure for the named function.
func newCustomFunctionError(function string, err error) error {
	return &CustomFunctionError{Function: function, Err: err}
}
