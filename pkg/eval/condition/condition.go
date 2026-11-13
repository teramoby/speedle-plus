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

// Name is an identifier in a condition: an attribute (plain or escaped as
// [name]) or a function name.
type Name struct {
	Name string
	// Position is where the name starts, in characters from the start of the
	// condition (at "[" for an escaped name).
	Position int
	// Operator is true for a bare word the parser reads as an operator or a
	// value (in, IN, true, false) rather than a name.
	Operator bool
}

// Names returns the identifiers in condition, outside string literals, in
// order. Escaped [names] are returned without their brackets or escapes.
func Names(condition string) []Name {
	var names []Name
	runes := []rune(condition)
	for i := 0; i < len(runes); {
		switch r := runes[i]; {
		case r == '\'' || r == '"':
			for i++; i < len(runes) && runes[i] != '\'' && runes[i] != '"'; i++ {
				if runes[i] == '\\' {
					i++
				}
			}
			i++
		case r == '[':
			start := i
			var b strings.Builder
			for i++; i < len(runes) && runes[i] != ']'; i++ {
				if runes[i] == '\\' && i+1 < len(runes) {
					i++
				}
				b.WriteRune(runes[i])
			}
			i++
			names = append(names, Name{Name: b.String(), Position: start})
		case unicode.IsLetter(r):
			// Like the parser: a name starts with a letter and continues
			// with letters, digits, "_" and "."; a backslash adds the next
			// character to the name, whatever it is.
			start := i
			var name string
			name, i = readToken(runes, i, isNameRune)
			names = append(names, Name{Name: name, Position: start, Operator: keywords[name]})
		case unicode.IsDigit(r) || r == '.':
			// A number is not a name; the parser reads it the same way.
			_, i = readToken(runes, i, isNumberRune)
		default:
			i++
		}
	}
	return names
}

func isNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
}

func isNumberRune(r rune) bool {
	return unicode.IsDigit(r) || r == '.'
}

// readToken reads runes from i while ok holds, as the parser does: a
// backslash adds the character after it to the token.
func readToken(runes []rune, i int, ok func(rune) bool) (string, int) {
	var b strings.Builder
	for i < len(runes) {
		if runes[i] == '\\' && i+1 < len(runes) {
			b.WriteRune(runes[i+1])
			i += 2
			continue
		}
		if !ok(runes[i]) {
			break
		}
		b.WriteRune(runes[i])
		i++
	}
	return b.String(), i
}

func anyFunction(args ...interface{}) (interface{}, error) {
	return true, nil
}
