//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package condition

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
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

func TestCalledNames(t *testing.T) {
	got := calledNames(`f(a) && g.h (b) && a in (1) && x == 'k(' && [m(] > 1 && "n(" != y && _p2(z)`)
	want := []string{"f", "g.h", "_p2"}
	if len(got) != len(want) {
		t.Fatalf("calledNames = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calledNames = %q, want %q", got, want)
		}
	}
}

func TestCalledNamesSkipsEscapedQuotes(t *testing.T) {
	// More calls than Check retries, after literals with escaped quotes.
	var b strings.Builder
	b.WriteString(`x == '\'' && y == "\"" && [v\]w] > 0`)
	for i := 0; i < maxRetries+2; i++ {
		fmt.Fprintf(&b, " && f%d(a)", i)
	}
	if err := Check(b.String()); err != nil {
		t.Fatal(err)
	}
	if got := len(calledNames(b.String())); got != maxRetries+2 {
		t.Errorf("calledNames found %d calls, want %d", got, maxRetries+2)
	}
}

func TestCheckParsesOnceForManyFunctions(t *testing.T) {
	// Each unknown function used to cost another parse of the whole condition.
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		if i > 0 {
			b.WriteString(" && ")
		}
		fmt.Fprintf(&b, "f%d(x)", i)
	}
	start := time.Now()
	if err := Check(b.String()); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Check took %v", d)
	}
}

func FuzzConditionCheck(f *testing.F) {
	for _, c := range []string{"x > 0", "(x", "a === b", "IsSubSet(x,('a','b'))", "unknown(x)", "x == 'R#D'"} {
		f.Add(c)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8192 {
			return
		}
		err := Check(s)
		if err != nil {
			var detail *Error
			if !errors.As(err, &detail) || detail.Position > len([]rune(s)) {
				t.Fatalf("invalid diagnostic %v", err)
			}
		}
	})
}
