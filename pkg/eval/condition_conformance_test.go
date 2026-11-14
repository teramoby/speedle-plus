// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
)

// The single case table below follows docs/hugo/content/docs/spdl.md. Expectations
// are explicit snapshots, never calculated using a second expression evaluator.
// A forbidden cell has a well-typed operand probe and a mismatched operand probe;
// neither is required to grant. Allowed cells have both grant and nonmatch probes.
// Authorization and condition outcomes are separate: runtime errors are checked
// on both paths, even though the current public APIs discard those errors.
type conditionCase struct {
	section, row, op, name, expression string
	effect                             string
	errorMessage                       string // Public IsAllowed/REST error (compile failures).
	conditionError                     string // Runtime error logged, then discarded by getPolicyList.
	attributes                         []*adsrest.JsonAttribute
	want                               bool
	difference                         string
	design                             string
}

var conditionOperators = []string{"+", "-", "*", "/", "%", "==", "!=", ">", ">=", "<", "<=", "=~", "in", "&&", "||", "!"}

// Snapshot the fork's diagnostic wording, including its historical "not a
// number" message for regex/type failures. Do not derive expectations by
// evaluating a second expression or calling the function under test.
func operatorError(value, op string) string {
	switch op {
	case "!":
		return fmt.Sprintf("Value '%s' cannot be used with the prefix '%s'", value, op)
	case "&&", "||":
		return fmt.Sprintf("Value '%s' cannot be used with the logical operator '%s', it is not a bool", value, op)
	case "+", "-", "*", "/", "%":
		return fmt.Sprintf("Value '%s' cannot be used with the modifier '%s', it is not a number", value, op)
	default:
		return fmt.Sprintf("Value '%s' cannot be used with the comparator '%s', it is not a number", value, op)
	}
}

func builtinError(fn string) string {
	usage := map[string]string{
		"Sqrt":     "Usage: Sqrt(x)",
		"Max":      "Usage: Max(x1, x2, ...), xi must be numeric",
		"Min":      "Usage: Min(x1, x2, ...), xi must be numeric",
		"Sum":      "Usage: Sum(x1, x2, ...), xi must be numeric",
		"Avg":      "Usage: Avg(x1, x2, ...), xi must be finite numeric",
		"IsSubSet": "Usage: IsSubSet(S1, S2) - homogeneous scalar sets required",
	}
	return "SPDL-2003 " + usage[fn]
}

func conditionCases() []conditionCase {
	var cases []conditionCase
	add := func(section, row, op, name, expression string, want bool, attrs ...*adsrest.JsonAttribute) {
		cases = append(cases, conditionCase{section: section, row: row, op: op, name: name, expression: expression, want: want, attributes: attrs})
	}
	attr := func(name, kind string, value interface{}) *adsrest.JsonAttribute {
		return &adsrest.JsonAttribute{Name: name, Type: kind, Value: value}
	}
	// Each pair is a true/false expression using x. Datetimes have numeric Unix
	// values at evaluation time, so their arithmetic is also pinned, although the
	// reference documents only comparison.
	numeric := map[string][2]string{
		"+": {"x + 2 == 7", "x + 2 == 8"}, "-": {"x - 2 == 3", "x - 2 == 4"},
		"*": {"x * 2 == 10", "x * 2 == 11"}, "/": {"x / 2 == 2.5", "x / 2 == 3"}, "%": {"x % 2 == 1", "x % 2 == 0"},
		"==": {"x == 5", "x == 6"}, "!=": {"x != 6", "x != 5"},
		">": {"x > 4", "x > 5"}, ">=": {"x >= 5", "x >= 6"}, "<": {"x < 6", "x < 5"}, "<=": {"x <= 5", "x <= 4"},
		"in": {"x in (4, 5)", "x in (4, 6)"},
	}
	stringsOps := map[string][2]string{
		"+": {"x + 'b' == 'ab'", "x + 'b' == 'ac'"}, "==": {"x == 'a'", "x == 'b'"}, "!=": {"x != 'b'", "x != 'a'"},
		">": {"x > 'A'", "x > 'b'"}, ">=": {"x >= 'a'", "x >= 'b'"}, "<": {"x < 'b'", "x < 'a'"}, "<=": {"x <= 'a'", "x <= 'A'"},
		"=~": {"x =~ '^a$'", "x =~ '^b$'"}, "in": {"x in ('a', 'b')", "x in ('b', 'c')"},
	}
	boolOps := map[string][2]string{
		"==": {"x == true", "x == false"}, "!=": {"x != false", "x != true"},
		"&&": {"x && true", "x && false"}, "||": {"x || false", "false || !x"}, "!": {"!(!x)", "!x"},
		"in": {"x in (true, false)", "x in (false, false)"},
	}
	arrayOps := map[string][2]string{"in": {"5 in x", "6 in x"}, "==": {"x == x", "x == 5"}, "!=": {"x != 5", "x != x"}}
	for _, typ := range []struct {
		name, kind string
		value      interface{}
		pairs      map[string][2]string
	}{
		{"string", "string", "a", stringsOps}, {"numeric", "numeric", 5.0, numeric}, {"bool", "bool", true, boolOps},
		{"datetime", "datetime", "1970-01-01T00:00:05Z", numeric}, {"array", "numeric", []interface{}{4.0, 5.0}, arrayOps},
	} {
		for _, op := range conditionOperators {
			pairs, ok := typ.pairs[op]
			if !ok {
				pairs = [2]string{"x " + op + " x", "x " + op + " other"}
				if op == "!" {
					pairs = [2]string{"!x", "!(x + x)"}
				}
			}
			for i, expression := range pairs {
				name := []string{"positive", "negative"}[i]
				want := ok && i == 0
				add("2.1", typ.name, op, name, expression, want, attr("x", typ.kind, typ.value), attr("other", "bool", false))
				if !ok {
					value := map[string]string{"string": "a", "numeric": "5", "bool": "true", "datetime": "5", "array": "[4 5]"}[typ.name]
					errorOp := op
					if op == "!" && i == 1 {
						switch typ.name {
						case "string":
							value = "aa"
						case "numeric", "datetime":
							value = "10"
						case "array":
							errorOp = "+" // The inner addition fails before ! is evaluated.
						}
					}
					cases[len(cases)-1].conditionError = operatorError(value, errorOp)
				}
				if typ.name == "datetime" && strings.Contains("+-*/%", op) || typ.name == "array" && (op == "==" || op == "!=") {
					cases[len(cases)-1].difference = "known difference, behaviour pinned"
				}
			}
		}
	}
	for _, kind := range []string{"string", "numeric", "bool", "datetime"} {
		value := interface{}(5.0)
		other := interface{}("5")
		otherKind := "string"
		switch kind {
		case "string":
			value = "5"
			other = 5.0
			otherKind = "numeric"
		case "bool":
			value = true
		case "datetime":
			value = "1970-01-01T00:00:05Z"
		}
		for _, op := range []string{"==", "!=", ">", ">=", "<", "<=", "=~", "&&", "||"} {
			// known difference, behaviour pinned: != compares different types as unequal.
			add("2.1", kind, "mismatch-"+op, "wrong-type", "x "+op+" y", (op == "!=" || kind == "bool" && op == "||"), attr("x", kind, value), attr("y", otherKind, other))
			if op != "==" && op != "!=" && !(kind == "bool" && op == "||") {
				errorValue := "5"
				if kind == "bool" && op != "&&" {
					errorValue = "true"
				}
				cases[len(cases)-1].conditionError = operatorError(errorValue, op)
			}
			if op == "!=" || kind == "bool" && op == "||" {
				cases[len(cases)-1].difference = "known difference, behaviour pinned"
			}
		}
	}
	// Every built-in attribute uses every operator valid for its injected type.
	// Self comparisons and broad bounds avoid clock/date rollover assumptions.
	for _, a := range []struct{ name, kind string }{
		{"request_user", "string"}, {"request_groups", "array"}, {"request_entity", "string"},
		{"request_resource", "string"}, {"request_action", "string"}, {"request_time", "datetime"},
		{"request_year", "numeric"}, {"request_month", "numeric"}, {"request_day", "numeric"}, {"request_hour", "numeric"}, {"request_weekday", "string"},
	} {
		pairs := map[string][2]string{}
		x := a.name
		switch a.kind {
		case "array":
			pairs["in"] = [2]string{"'staff' in " + x, "'absent' in " + x}
		case "string":
			pairs = map[string][2]string{
				"+": {x + " + '' == " + x, x + " + '!' == " + x}, "=~": {x + " =~ '^.+$'", x + " =~ '^$'"},
				"==": {x + " == " + x, x + " == ''"}, "!=": {x + " != ''", x + " != " + x},
				">": {x + " > ''", x + " > " + x}, ">=": {x + " >= " + x, x + " >= 'zzzzzzzzzz'"},
				"<": {x + " < 'zzzzzzzzzz'", x + " < " + x}, "<=": {x + " <= " + x, x + " <= ''"},
				"in": {x + " in (" + x + ", '')", x + " in ('', 'absent')"},
			}
		default:
			pairs = map[string][2]string{
				"==": {x + " == " + x, x + " == -1"}, "!=": {x + " != -1", x + " != " + x},
				">": {x + " > -1", x + " > " + x}, ">=": {x + " >= " + x, x + " >= 99999999999"},
				"<": {x + " < 99999999999", x + " < " + x}, "<=": {x + " <= " + x, x + " <= -1"},
				"in": {x + " in (" + x + ", -1)", x + " in (-1, -2)"},
			}
			if a.kind == "numeric" {
				for _, op := range []string{"+", "-", "*", "/", "%"} {
					expression := x + " " + op + " 1"
					expected := x
					switch op {
					case "+":
						expression = "(" + x + " + 1) - " + x
						expected = "1"
					case "-":
						expression = x + " - " + x
						expected = "0"
					case "*":
						expression = x + " * 0"
						expected = "0"
					case "/":
						expression = x + " / 1 - " + x
						expected = "0"
					case "%":
						expected = "0"
					}
					pairs[op] = [2]string{"(" + expression + ") == (" + expected + ")", "(" + expression + ") == -1"}
				}
			}
		}
		for _, op := range conditionOperators {
			if p, ok := pairs[op]; ok {
				for i, e := range p {
					add("2.2", a.name, op, []string{"positive", "negative"}[i], e, i == 0)
				}
			}
		}
	}
	for _, c := range []struct {
		name, expression string
		want             bool
	}{
		{"datetime-constant", "x == '2006-01-02T15:04:05-07:00'", true},
		{"datetime-offset", "x == '2006-01-02T22:04:05Z'", true},
		{"datetime-fraction-truncated", "x == '2006-01-02T22:04:05.9Z'", true},
		{"datetime-not-equal", "x == '2006-01-03T22:04:05Z'", false},
		{"datetime-array", "x in ('2006-01-02T22:04:05Z','2006-01-03T22:04:05Z')", true},
	} {
		add("2.3", "datetime", "constant", c.name, c.expression, c.want, attr("x", "datetime", "2006-01-02T22:04:05Z"))
	}
	for _, c := range []struct {
		fn, expression, name string
		want                 bool
	}{
		{"unregistered", `f\oo(1)`, "escaped-unknown", false},
		{"unregistered", `!f\oo(1)`, "negated-escaped-unknown", false},
		{"Sqrt", "Sqrt(9) == 3", "normal", true}, {"Sqrt", "Sqrt(9) == 4", "nonmatch", false},
		{"Sqrt", "Sqrt(0) == 0", "zero", true}, {"Sqrt", "Sqrt(-1) == 0", "negative", false},
		{"Sqrt", "Sqrt() == 0", "empty", false}, {"Sqrt", "Sqrt(1,2) == 1", "arity", false},
		{"Max", "Max(1,4,2,4) == 4", "duplicates", true}, {"Max", "Max(1) == 1", "singleton", true},
		{"Max", "Max(1,4) == 1", "nonmatch", false}, {"Max", "Max() == 0", "empty", false},
		{"Min", "Min(1,-4,2,-4) == -4", "duplicates", true}, {"Min", "Min(1) == 1", "singleton", true},
		{"Min", "Min(1,4) == 4", "nonmatch", false}, {"Min", "Min() == 0", "empty", false},
		{"Sum", "Sum(1,2,2) == 5", "duplicates", true}, {"Sum", "Sum(1) == 1", "singleton", true},
		{"Sum", "Sum(1,2) == 4", "nonmatch", false}, {"Sum", "Sum() == 0", "empty", true},
		{"Avg", "Avg(1,2,3,2) == 2", "duplicates", true}, {"Avg", "Avg(1) == 1", "singleton", true},
		{"Avg", "Avg(1,2) == 2", "nonmatch", false}, {"Avg", "Avg() == 0", "empty", true},
		{"Max", "Max((1,2)) == 2", "flattened-array", true},
		{"Min", "Min((1,2)) == 1", "flattened-array", true},
		{"Sum", "Sum((1,2)) == 3", "flattened-array", true},
		{"Avg", "Avg((1,2)) == 1.5", "flattened-array", true},
		{"Sum", "Sum(huge,huge) > 0", "overflow", false},
		{"Avg", "Avg(huge,huge,huge,huge,huge,huge,huge,huge,huge,huge,huge) > 0", "scaled-rounding-overflow", false},
		{"IsSubSet", "IsSubSet((0/0,1),(1,2))", "nan-first", false},
		{"IsSubSet", "IsSubSet((1,2),(1,1/0))", "infinity-last", false},
		{"Avg", "Avg(huge,huge) == huge", "scaled-overflow", true},
		{"Avg", "Avg(huge,-huge,1) == 1/3", "cancellation", true},
		{"IsSubSet", "IsSubSet((1,1,2),(1,2,3))", "duplicates", true},
		{"IsSubSet", "IsSubSet((1,4),(1,2,3))", "nonmatch", false},
		{"IsSubSet", "IsSubSet(('a','b'),('a','b','c'))", "strings", true},
		{"IsSubSet", "IsSubSet((true,false),(true,false))", "bools", true},
		{"IsSubSet", "IsSubSet(('2006-01-02T22:04:05Z','2006-01-03T22:04:05Z'),('2006-01-02T22:04:05Z','2006-01-03T22:04:05Z'))", "datetimes", true},
		{"IsSubSet", "IsSubSet(empty,(1,2))", "empty-first", false},
		{"IsSubSet", "IsSubSet((1,2),empty)", "empty-last", false},
		{"IsSubSet", "IsSubSet(empty,empty)", "empty-both", false},
		{"IsSubSet", "IsSubSet()", "arity-zero", false}, {"IsSubSet", "IsSubSet(1)", "arity-one", false},
		{"IsSubSet", "IsSubSet(1,2)", "wrong-last", false},
		{"IsSubSet", "IsSubSet((1,2),(1,2),(1,2))", "arity-three-arrays", false},
		{"IsSubSet", "IsSubSet(1,2,(1,2))", "flattened-first", true},
		{"IsSubSet", "IsSubSet((1,'a'),(1,2))", "mixed-first", false},
		{"IsSubSet", "IsSubSet((1,2),(1,'a'))", "mixed-last", false},
	} {
		add("2.4", c.fn, "call", c.name, c.expression, c.want, attr("huge", "numeric", 1.7976931348623157e308), attr("empty", "numeric", []interface{}{}))
		if c.fn == "unregistered" {
			cases[len(cases)-1].errorMessage = "Undefined function foo"
		}
		// False results are distinct from rejected calls. The fork flattens an
		// empty first argument away, so empty-first/both reject at this layer.
		switch c.name {
		case "negative", "arity", "arity-zero", "arity-one", "wrong-last", "arity-three-arrays", "mixed-first", "mixed-last", "nan-first", "infinity-last", "empty-first", "empty-both":
			cases[len(cases)-1].conditionError = builtinError(c.fn)
		case "empty":
			if c.fn == "Sqrt" || c.fn == "Max" || c.fn == "Min" {
				cases[len(cases)-1].conditionError = builtinError(c.fn)
			}
		case "overflow":
			cases[len(cases)-1].conditionError = "SPDL-2003 Sum result is not finite"
		case "scaled-rounding-overflow":
			cases[len(cases)-1].conditionError = "SPDL-2003 Avg result is not finite"
		}
		if c.name == "scaled-rounding-overflow" || c.name == "flattened-array" || c.name == "flattened-first" || c.name == "empty-first" || c.name == "empty-both" {
			cases[len(cases)-1].difference = "known difference, behaviour pinned"
		}
	}
	for _, fn := range []string{"Sqrt", "Max", "Min", "Sum", "Avg"} {
		for _, value := range []string{"'a'", "true", "(1,2)", "0/0", "1/0", "-1/0"} {
			expression := fn + "(" + value + ") == 0"
			if value == "(1,2)" {
				expression = fn + "(1,(1,2)) == 0"
			}
			add("2.4", fn, "call", "reject-"+value, expression, false)
			cases[len(cases)-1].conditionError = builtinError(fn)
		}
	}
	for i, e := range []string{
		"(1+2)*3 == 9", "Sqrt(4)+1 == 3", "!false && -2 * -3 == 6", "72/2/3 == 12 && 10%4*2 == 4", "10-3-2 == 5 && 1+2*3 == 7", "1+2 == 3 && 2 < 3", "true || false && false", "false || false || true",
	} {
		add("2.5", fmt.Sprintf("precedence-%d", i+1), "order", "positive", e, true)
		add("2.5", fmt.Sprintf("precedence-%d", i+1), "order", "negative", "!("+e+")", false)
	}
	for _, c := range []struct {
		expression string
		want       bool
	}{
		{"missing", false}, {"missing == 1", false}, {"!missing", false}, {"missing || true", false}, {"true || missing", true},
		{"missing || false", false}, {"false || missing", false}, {"missing && true", false}, {"true && missing", false},
		{"missing && false", false}, {"false && missing", false}, {"!(missing == 1)", false}, {"!(true || missing)", false},
	} {
		add("1 / 2.2.2", "missing", "logic", c.expression, c.expression, c.want)
		add("1 / 2.2.2", "missing-deny", "logic", c.expression, c.expression, !c.want)
		cases[len(cases)-1].effect = pms.Deny
		if c.expression != "true || missing" && c.expression != "false && missing" && c.expression != "!(true || missing)" {
			cases[len(cases)-1].conditionError = "No parameter 'missing' found."
			cases[len(cases)-2].conditionError = "No parameter 'missing' found."
			// SPDL §1 explicitly covers conditional denies as well as grants.
			// Keep this documented contract separate from reference deviations.
			for _, index := range []int{len(cases) - 2, len(cases) - 1} {
				cases[index].design = "A condition that references an attribute missing from the request does not match. A conditional `deny` therefore does not apply when its attribute is absent. [SPDL §1](https://engine.speedle.io/docs/spdl/#1-overview)"
			}
		}
		if c.expression == "true || missing" {
			cases[len(cases)-1].difference = "known difference, behaviour pinned"
			cases[len(cases)-2].difference = "known difference, behaviour pinned"
		}
	}
	// Runtime type, regex and arity errors also make a deny inapplicable.
	// Pair each with matching and nonmatching controls to distinguish all three
	// condition outcomes, with an independent base grant in the fixture below.
	for _, c := range []struct {
		name, expression, conditionError string
		want                             bool
		attributes                       []*adsrest.JsonAttribute
	}{
		{"type-error", "x + 1 == 2", operatorError("true", "+"), true, []*adsrest.JsonAttribute{attr("x", "bool", true)}},
		{"type-match", "x + 1 == 2", "", false, []*adsrest.JsonAttribute{attr("x", "numeric", 1.0)}},
		{"type-no-match", "x + 1 == 2", "", true, []*adsrest.JsonAttribute{attr("x", "numeric", 2.0)}},
		{"regex-error", "'a' =~ pattern", "Unable to compile regexp pattern '(': error parsing regexp: missing closing ): `(`", true, []*adsrest.JsonAttribute{attr("pattern", "string", "(")}},
		{"regex-match", "'a' =~ pattern", "", false, []*adsrest.JsonAttribute{attr("pattern", "string", "^a$")}},
		{"regex-no-match", "'a' =~ pattern", "", true, []*adsrest.JsonAttribute{attr("pattern", "string", "^b$")}},
		{"arity-error", "Sqrt(1,2) == 1", builtinError("Sqrt"), true, nil},
		{"arity-match", "Sqrt(4) == 2", "", false, nil},
		{"arity-no-match", "Sqrt(4) == 3", "", true, nil},
	} {
		add("1 / 2.2.2", "runtime-deny", "errors", c.name, c.expression, c.want, c.attributes...)
		last := &cases[len(cases)-1]
		last.effect, last.conditionError = pms.Deny, c.conditionError
		if c.conditionError != "" {
			last.design = "A failed function makes its condition nonmatching, as with other runtime condition errors. [SPDL §2.4.1](https://engine.speedle.io/docs/spdl/#241-built-in-functions); the policy takes effect only when its condition is met [SPDL §1](https://engine.speedle.io/docs/spdl/#1-overview)."
		}
	}
	// known difference, behaviour pinned: the fork accepts these extensions;
	// pin both results so acceptance alone cannot hide a broken evaluation.
	for _, c := range []struct{ name, yes, no string }{
		{"double-quotes", `"a" == 'a'`, `"a" == 'b'`},
		{"escaped-names", "[hyphen-name] == 5", "[hyphen-name] == 6"},
		{"ternary", "(true ? 1 : 2) == 1", "(false ? 1 : 2) == 1"},
		{"coalesce", "(5 ?? 6) == 5", "(5 ?? 6) == 6"},
		{"bitwise", "(5 & 3) == 1 && (5 | 2) == 7 && (5 ^ 1) == 4 && (1 << 2) == 4 && (4 >> 1) == 2 && ~1 == -2", "(5 & 3) == 0"},
		{"exponent", "2 ** 3 ** 2 == 64", "2 ** 3 == 9"},
		{"keyword-attribute", "grant == 5", "grant == 6"},
		{"string-number-inequality", "s != 5", "s == 5"},
		{"single-element-array", "n in nums", "n in (5)"},
	} {
		for i, e := range []string{c.yes, c.no} {
			add("known differences", c.name, "extension", []string{"positive", "negative"}[i], e, i == 0, attr("hyphen-name", "numeric", 5.0), attr("grant", "numeric", 5.0), attr("s", "string", "5"), attr("n", "numeric", 5.0), attr("nums", "numeric", []interface{}{5.0}))
			cases[len(cases)-1].difference = "known difference, behaviour pinned"
			if c.name == "single-element-array" && i == 1 {
				cases[len(cases)-1].conditionError = operatorError("5", "in")
			}
		}
	}
	return cases
}

// Use the file-store fixture and router/recorder pattern used by
// pkg/svcs/adsrest/{handlers,feature_contract}_test.go. Those helpers are private
// to that package; an external eval test avoids an eval -> adsrest import cycle.
func TestConditionConformance(t *testing.T) {
	// Observe the real evaluateCondition calls in each authorization path.
	// getPolicyList currently discards their returned errors; requiring only
	// NO_APPLICABLE_POLICIES would otherwise misclassify failures as clean false.
	// As in recovery_test.go, keep this test and its subtests nonparallel while
	// the process-wide logger hook is installed, and restore hooks on cleanup.
	oldHooks := log.StandardLogger().ReplaceHooks(make(log.LevelHooks))
	hook := logtest.NewGlobal()
	t.Cleanup(func() { log.StandardLogger().ReplaceHooks(oldHooks) })
	cases := conditionCases()
	service := &pms.Service{Name: "conformance"}
	for i, c := range cases {
		effect := c.effect
		if effect == "" {
			effect = pms.Grant
		}
		if effect == pms.Deny {
			// SPDL §1/§2.2.2/§2.4.1 make a missing-attribute or runtime-error
			// condition nonmatching. The independent base grant exposes that
			// outcome; conditionError still requires the real runtime error on
			// both paths. Making such denies fail closed changes production
			// semantics and belongs to a separate decision, not this suite.
			service.Policies = append(service.Policies, &pms.Policy{ID: fmt.Sprintf("base-%d", i), Effect: pms.Grant, Permissions: []*pms.Permission{{Resource: fmt.Sprint(i), Actions: []string{"read"}}}})
		}
		service.Policies = append(service.Policies, &pms.Policy{ID: fmt.Sprint(i), Name: fmt.Sprint(i), Effect: effect, Condition: c.expression, Permissions: []*pms.Permission{{Resource: fmt.Sprint(i), Actions: []string{"read"}}}})
	}
	data, err := json.Marshal(&pms.PolicyStore{Services: []*pms.Service{service}})
	if err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, data, 0600); err != nil {
		t.Fatal(err)
	}
	evaluator, err := eval.NewFromFile(location, false)
	if err != nil {
		t.Fatal(err)
	}
	e := evaluator.(*eval.PolicyEvalImpl)
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	router, err := adsrest.NewRouter(e)
	if err != nil {
		t.Fatal(err)
	}
	subject := &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}, {Type: "group", Name: "staff"}, {Type: "entity", Name: "service"}}}
	for i, c := range cases {
		t.Run(c.section+"/"+c.row+"/"+c.op+"/"+c.name, func(t *testing.T) {
			attrs := map[string]interface{}{}
			for _, a := range c.attributes {
				v := a.Value
				if a.Type == "datetime" {
					timestamp, err := time.Parse(time.RFC3339Nano, a.Value.(string))
					if err != nil {
						t.Fatal(err)
					}
					v = float64(timestamp.Unix())
				}
				attrs[a.Name] = v
			}
			ctx := ads.RequestContext{ServiceName: service.Name, Resource: fmt.Sprint(i), Action: "read", Subject: subject, Attributes: attrs}
			hook.Reset()
			direct, reason, err := e.IsAllowed(ctx)
			assertConditionError(t, hook, "file-store IsAllowed", c)
			expectedReason := ads.NO_APPLICABLE_POLICIES
			if c.want {
				expectedReason = ads.GRANT_POLICY_FOUND
			} else if c.effect == pms.Deny {
				expectedReason = ads.DENY_POLICY_FOUND
			}
			if c.errorMessage != "" {
				expectedReason = ads.ERROR_IN_EVALUATION
			}
			actualError := ""
			if err != nil {
				actualError = err.Error()
			}
			if actualError != c.errorMessage || direct != c.want || reason != expectedReason {
				t.Errorf("direct %q: allowed=%v reason=%v err=%v; want %v/%v", c.expression, direct, reason, err, c.want, expectedReason)
			}
			body, err := json.Marshal(map[string]interface{}{"serviceName": service.Name, "resource": ctx.Resource, "action": ctx.Action, "subject": subject, "attributes": c.attributes})
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			hook.Reset()
			router.ServeHTTP(rec, httptest.NewRequest("POST", svcs.PolicyAtzPath+"is-allowed", bytes.NewReader(body)))
			assertConditionError(t, hook, "REST is-allowed", c)
			if rec.Code != 200 {
				t.Fatalf("REST status %d: %s", rec.Code, rec.Body.String())
			}
			var got adsrest.IsAllowedResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Allowed != c.want || got.Allowed != direct || got.Reason != int32(reason) || got.ErrorMessage != c.errorMessage {
				t.Errorf("REST %q: %+v; direct=%v/%v want=%v", c.expression, got, direct, reason, c.want)
			}
		})
	}
}

func assertConditionError(t *testing.T, hook *logtest.Hook, path string, c conditionCase) {
	t.Helper()
	var got []string
	const prefix = "Error happens in evaluating condition ("
	for _, entry := range hook.AllEntries() {
		if strings.HasPrefix(entry.Message, prefix) {
			got = append(got, entry.Message)
			if entry.Level != log.ErrorLevel {
				t.Errorf("%s: condition failure logged at %v, want error", path, entry.Level)
			}
		}
	}
	var want []string
	if c.conditionError != "" {
		want = []string{prefix + c.expression + "): " + c.conditionError}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: condition errors = %q, want %q", path, got, want)
	}
}

func (c conditionCase) outcome() string {
	if c.errorMessage != "" || c.conditionError != "" {
		return "error"
	}
	match := c.want
	if c.effect == pms.Deny {
		match = !match
	}
	if match {
		return "match"
	}
	return "no-match"
}

// Keep the reviewed inventory independent of the generated counts. Removing an
// entire family must fail even when row counts and totals remain self-consistent.
// Update this snapshot deliberately when the conformance contract grows.
func checkConditionInventory(cases []conditionCase) error {
	expected := map[string]int{
		"1 / 2.2.2 / missing":                          13,
		"1 / 2.2.2 / missing-deny":                     13,
		"1 / 2.2.2 / runtime-deny":                     9,
		"2.1 / array":                                  32,
		"2.1 / bool":                                   41,
		"2.1 / datetime":                               41,
		"2.1 / numeric":                                41,
		"2.1 / string":                                 41,
		"2.2 / request_action":                         18,
		"2.2 / request_day":                            24,
		"2.2 / request_entity":                         18,
		"2.2 / request_groups":                         2,
		"2.2 / request_hour":                           24,
		"2.2 / request_month":                          24,
		"2.2 / request_resource":                       18,
		"2.2 / request_time":                           14,
		"2.2 / request_user":                           18,
		"2.2 / request_weekday":                        18,
		"2.2 / request_year":                           24,
		"2.3 / datetime":                               5,
		"2.4 / Avg":                                    14,
		"2.4 / IsSubSet":                               17,
		"2.4 / Max":                                    11,
		"2.4 / Min":                                    11,
		"2.4 / Sqrt":                                   12,
		"2.4 / Sum":                                    12,
		"2.4 / unregistered":                           2,
		"2.5 / precedence-1":                           2,
		"2.5 / precedence-2":                           2,
		"2.5 / precedence-3":                           2,
		"2.5 / precedence-4":                           2,
		"2.5 / precedence-5":                           2,
		"2.5 / precedence-6":                           2,
		"2.5 / precedence-7":                           2,
		"2.5 / precedence-8":                           2,
		"known differences / bitwise":                  2,
		"known differences / coalesce":                 2,
		"known differences / double-quotes":            2,
		"known differences / escaped-names":            2,
		"known differences / exponent":                 2,
		"known differences / keyword-attribute":        2,
		"known differences / single-element-array":     2,
		"known differences / string-number-inequality": 2,
		"known differences / ternary":                  2,
	}
	actual := map[string]int{}
	for _, c := range cases {
		actual[c.section+" / "+c.row]++
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("condition case inventory changed: got %v; want %v", actual, expected)
	}
	return nil
}

func TestConditionConformanceInventory(t *testing.T) {
	if err := checkConditionInventory(conditionCases()); err != nil {
		t.Fatal(err)
	}
	for _, family := range []string{"2.4 / IsSubSet", "known differences / coalesce"} {
		t.Run("removed-"+family, func(t *testing.T) {
			var remaining []conditionCase
			for _, c := range conditionCases() {
				if c.section+" / "+c.row != family {
					remaining = append(remaining, c)
				}
			}
			if err := checkConditionInventory(remaining); err == nil {
				t.Fatal("removed case family was not detected")
			}
		})
	}
}

func TestConditionConformanceMatrix(t *testing.T) {
	if err := checkConditionInventory(conditionCases()); err != nil {
		t.Fatal(err)
	}
	counts := map[string]map[string]map[string]int{}
	sections := map[string]int{}
	totals := map[string]int{}
	for _, c := range conditionCases() {
		key := c.section + " / " + c.row
		if counts[key] == nil {
			counts[key] = map[string]map[string]int{}
		}
		if counts[key][c.op] == nil {
			counts[key][c.op] = map[string]int{}
		}
		counts[key][c.op][c.outcome()]++
		totals[c.outcome()]++
		sections[c.section]++
	}
	var rows []string
	for row := range counts {
		rows = append(rows, row)
	}
	sort.Strings(rows)
	header := append([]string(nil), conditionOperators...)
	for i := range header {
		header[i] = strings.ReplaceAll(header[i], "|", `\|`)
	}
	t.Log("Matrix cells are match / no-match / error counts. Runtime errors are asserted from the actual condition-error log in each path; public APIs currently discard them. Compile errors are returned by both APIs.")
	t.Log("\n| Reference / row | " + strings.Join(header, " | ") + " | other |\n|---|" + strings.Repeat("---|", len(conditionOperators)+1))
	for _, row := range rows {
		line := "| " + row + " |"
		other := map[string]int{}
		for _, outcomes := range counts[row] {
			for outcome, n := range outcomes {
				other[outcome] += n
			}
		}
		cell := func(outcomes map[string]int) string {
			return fmt.Sprintf(" %d/%d/%d |", outcomes["match"], outcomes["no-match"], outcomes["error"])
		}
		for _, op := range conditionOperators {
			line += cell(counts[row][op])
			for outcome, n := range counts[row][op] {
				other[outcome] -= n
			}
		}
		t.Log(line + cell(other))
	}
	t.Logf("Outcome totals: %d match / %d no-match / %d error", totals["match"], totals["no-match"], totals["error"])
	t.Log("Error cases (runtime errors are currently discarded by both public authorization paths):")
	for _, c := range conditionCases() {
		if c.conditionError != "" {
			t.Logf("Error: %s/%s/%s/%s: %s => %s; API allowed=%v, runtime error discarded", c.section, c.row, c.op, c.name, c.expression, c.conditionError, c.want)
		} else if c.errorMessage != "" {
			t.Logf("Error: %s/%s/%s/%s: %s => %s; API ERROR_IN_EVALUATION", c.section, c.row, c.op, c.name, c.expression, c.errorMessage)
		}
	}
	t.Log("By-design behaviours (condition outcome / expected authorization):")
	for _, c := range conditionCases() {
		if c.design != "" {
			t.Logf("Design: %s/%s/%s: %s => %s; API allowed=%v (%s)", c.row, c.op, c.name, c.expression, c.outcome(), c.want, c.design)
		}
		if (c.row == "missing" || c.row == "missing-deny") && c.conditionError != "" && (c.design == "" || c.difference != "") {
			t.Errorf("missing-attribute runtime error %s/%s must be reported as by design, not a known difference", c.row, c.name)
		}
	}
	t.Log("Known differences / open deviations (name / expression / expected authorization):")
	for _, c := range conditionCases() {
		if c.difference != "" {
			t.Logf("Difference: %s/%s/%s: %s => %v (%s)", c.row, c.op, c.name, c.expression, c.want, c.difference)
		}
	}
	var names []string
	for s := range sections {
		names = append(names, s)
	}
	sort.Strings(names)
	for _, s := range names {
		t.Logf("Section %s: %d cases", s, sections[s])
	}
	for _, typ := range []string{"string", "numeric", "bool", "datetime", "array"} {
		for _, op := range conditionOperators {
			n := 0
			for _, count := range counts["2.1 / "+typ][op] {
				n += count
			}
			if n != 2 {
				t.Errorf("missing positive/negative cell %s %s", typ, op)
			}
		}
	}
}

// Diagnose exposes the actual injected Go types, before govaluate sanitizes
// integer values to float64, and verifies values against the same injected
// instant. Calendar built-ins derive from the ADS server clock; a PEP override
// of request_time replaces only that attribute, not the calendar attributes.
// No PEP overrides are used here, and deriving the calendar from the observed
// Unix timestamp avoids midnight/hour rollover races between separate reads.
// populateContext uses now := time.Now() and derives all calendar fields from
// that same local-zone instant; this test deliberately pins that implementation
// contract rather than assuming UTC. TZ=UTC and TZ=Pacific/Kiritimati exercise
// this contract across different hour/day boundaries without changing host settings.
func TestConditionConformanceInjectedTypesAndValues(t *testing.T) {
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, []byte(`{"services":[{"name":"app","policies":[{"id":"p","effect":"grant"}]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	evaluator, err := eval.NewFromFile(location, false)
	if err != nil {
		t.Fatal(err)
	}
	e := evaluator.(*eval.PolicyEvalImpl)
	defer e.Close()
	before := time.Now().Unix()
	result, err := e.Diagnose(ads.RequestContext{ServiceName: "app", Resource: "book", Action: "read", Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}, {Type: "group", Name: "staff"}, {Type: "entity", Name: "service"}}}})
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now().Unix()
	timestamp, ok := result.Attributes["request_time"].(int64)
	if !ok {
		t.Fatalf("request_time = %T, want int64", result.Attributes["request_time"])
	}
	if timestamp < before || timestamp > after {
		t.Fatalf("request_time %d is outside request window [%d,%d]", timestamp, before, after)
	}
	instant := time.Unix(timestamp, 0).In(time.Local)
	for name, want := range map[string]interface{}{
		"request_user": "alice", "request_groups": []interface{}{"staff"}, "request_entity": "service",
		"request_resource": "book", "request_action": "read", "request_time": timestamp,
		"request_year": instant.Year(), "request_month": int(instant.Month()), "request_day": instant.Day(),
		"request_hour": instant.Hour(), "request_weekday": instant.Weekday().String(),
	} {
		got, ok := result.Attributes[name]
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %T (%v), want %T (%v)", name, got, got, want, want)
		}
	}
}
