//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package condition checks that a policy condition compiles, and says where it
// does not.
package condition

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
)

const (
	undefinedFunction = "Undefined function "
	maxRetries        = 8
)

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
	for _, name := range calledNames(condition) {
		functions[name] = anyFunction
	}
	// calledNames finds the calls, so this loop should not need to retry; the
	// bound keeps the work linear if it does.
	for retries := 0; ; retries++ {
		_, err := govaluate.NewEvaluableExpressionWithFunctions(condition, functions)
		if err == nil {
			return nil
		}
		position := -1
		var expErr *govaluate.ExpressionError
		if errors.As(err, &expErr) {
			if name := strings.TrimPrefix(expErr.Message, undefinedFunction); name != expErr.Message && functions[name] == nil && retries < maxRetries {
				functions[name] = anyFunction
				continue
			}
			// Point an early end at the last character, not past trailing spaces.
			position = min(expErr.Position, len([]rune(strings.TrimRight(condition, " \t"))))
		}
		return &Error{Condition: condition, Position: position, Reason: strings.TrimSpace(err.Error())}
	}
}

// keywords are words the parser reads as operators or values, not names.
var keywords = map[string]bool{"in": true, "IN": true, "true": true, "false": true}

// calledNames returns the names followed by "(" outside string literals and
// escaped [variables], so that one parse is enough to check a condition.
func calledNames(condition string) []string {
	var names []string
	runes := []rune(condition)
	for i := 0; i < len(runes); {
		switch r := runes[i]; {
		case r == '\'' || r == '"' || r == '[':
			// Like the parser: a string ends at either quote, a variable at
			// "]", and a backslash escapes the next character.
			isEnd := func(c rune) bool { return c == '\'' || c == '"' }
			if r == '[' {
				isEnd = func(c rune) bool { return c == ']' }
			}
			for i++; i < len(runes) && !isEnd(runes[i]); i++ {
				if runes[i] == '\\' {
					i++
				}
			}
			i++
		case unicode.IsLetter(r) || r == '_':
			start := i
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '.') {
				i++
			}
			name := string(runes[start:i])
			for i < len(runes) && unicode.IsSpace(runes[i]) {
				i++
			}
			if i < len(runes) && runes[i] == '(' && !keywords[name] {
				names = append(names, name)
			}
		default:
			i++
		}
	}
	return names
}

func anyFunction(args ...interface{}) (interface{}, error) {
	return true, nil
}
