//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package condition checks that a policy condition compiles, and says where it
// does not.
package condition

import (
	"errors"
	"fmt"
	"strings"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
)

const undefinedFunction = "Undefined function "

// Error is a condition that does not compile.
type Error struct {
	Condition string
	// Position is where the error is, in characters from the start of the
	// condition; -1 when the parser does not say.
	Position int
	Reason   string
}

func (e *Error) Error() string {
	if e.Position < 0 {
		return fmt.Sprintf("invalid condition %q: %s", e.Condition, e.Reason)
	}
	return fmt.Sprintf("invalid condition at character %d: %s\n%s\n%s^",
		e.Position+1, e.Reason, e.Condition, strings.Repeat(" ", e.Position))
}

// Check returns an *Error when condition does not compile. Calls to functions
// it does not know are allowed: the evaluator adds the built-in functions, and
// custom functions can be created after the policies that use them.
func Check(condition string) error {
	if strings.TrimSpace(condition) == "" {
		return nil
	}
	functions := map[string]govaluate.ExpressionFunction{}
	for {
		_, err := govaluate.NewEvaluableExpressionWithFunctions(condition, functions)
		if err == nil {
			return nil
		}
		position := -1
		var expErr *govaluate.ExpressionError
		if errors.As(err, &expErr) {
			if name := strings.TrimPrefix(expErr.Message, undefinedFunction); name != expErr.Message && functions[name] == nil {
				functions[name] = anyFunction
				continue
			}
			// Point an early end at the last character, not past trailing spaces.
			position = min(expErr.Position, len([]rune(strings.TrimRight(condition, " \t"))))
		}
		return &Error{Condition: condition, Position: position, Reason: strings.TrimSpace(err.Error())}
	}
}

func anyFunction(args ...interface{}) (interface{}, error) {
	return true, nil
}
