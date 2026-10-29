//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package condition

import (
	"errors"
	"testing"
)

func TestCheckAcceptsValidConditions(t *testing.T) {
	for _, c := range []string{
		"",
		"   ",
		"amount < 5",
		"request_time > '2017-09-04 12:00:00'",
		"a in (1, 2) && !(b == 'x')",
		// Functions are resolved by the evaluator, so unknown names are fine.
		"IsSubSet(groups, ('a', 'b')) && myFunc(x) > 1",
	} {
		if err := Check(c); err != nil {
			t.Errorf("Check(%q) = %v", c, err)
		}
	}
}

func TestCheckReportsWhere(t *testing.T) {
	cases := []struct {
		condition, reason string
		position          int
	}{
		{"amount <=", "Unexpected end of expression", 9},
		{"amount <=   ", "Unexpected end of expression", 9},
		{"(a > 1", "Unbalanced parenthesis", 0},
		{"a > (b + 1))", "Unbalanced parenthesis", 11},
		{"a b", "Cannot transition token types from VARIABLE [a] to VARIABLE [b]", 2},
		{"a == 'x", "Unclosed string literal", 5},
		{"a === b", "Invalid token: '==='", 2},
		{"x > 1.2.3", "Unable to parse numeric value '1.2.3' to float64", 4},
		{"x =~ '['", "error parsing regexp: missing closing ]: `[`", -1},
	}
	for _, c := range cases {
		var condErr *Error
		if err := Check(c.condition); !errors.As(err, &condErr) {
			t.Errorf("Check(%q) = %v, want *Error", c.condition, err)
		} else if condErr.Reason != c.reason || condErr.Position != c.position {
			t.Errorf("Check(%q) = %q at %d, want %q at %d", c.condition, condErr.Reason, condErr.Position, c.reason, c.position)
		}
	}
}

func TestErrorMessage(t *testing.T) {
	err := &Error{Condition: "amount <=", Position: 9, Reason: "Unexpected end of expression"}
	want := "invalid condition at character 10: Unexpected end of expression\namount <=\n         ^"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	err = &Error{Condition: "x =~ '['", Position: -1, Reason: "bad regexp"}
	if want := `invalid condition "x =~ '['": bad regexp`; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}
