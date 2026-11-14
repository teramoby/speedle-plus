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
type conditionCase struct {
	section, row, op, name, expression string
	attributes                         []*adsrest.JsonAttribute
	want                               bool
	difference                         string
}

var conditionOperators = []string{"+", "-", "*", "/", "%", "==", "!=", ">", ">=", "<", "<=", "=~", "in", "&&", "||", "!"}

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
		"==": {"x == 5", "x == 6"}, "! =": {"", ""}, "!=": {"x != 6", "x != 5"},
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
			}
			if a.kind == "numeric" {
				for _, op := range []string{"+", "-", "*", "/", "%"} {
					expression := x + " " + op + " 1"
					expected := x
					switch op {
					case "+":
						expected = x + " + 1"
					case "-":
						expected = x + " - 1"
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
		{"Sum", "Sum(huge,huge) > 0", "overflow", false},
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
		{"IsSubSet", "IsSubSet((1,'a'),(1,2))", "mixed-first", false},
		{"IsSubSet", "IsSubSet((1,2),(1,'a'))", "mixed-last", false},
	} {
		add("2.4", c.fn, "call", c.name, c.expression, c.want, attr("huge", "numeric", 1.7976931348623157e308), attr("empty", "numeric", []interface{}{}))
	}
	for _, fn := range []string{"Sqrt", "Max", "Min", "Sum", "Avg"} {
		for _, value := range []string{"'a'", "true", "(1,2)", "0/0", "1/0", "-1/0"} {
			add("2.4", fn, "call", "reject-"+value, fn+"("+value+") == 0", false)
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
		}
	}
	return cases
}

// Use the file-store fixture and router/recorder pattern used by
// pkg/svcs/adsrest/{handlers,feature_contract}_test.go. Those helpers are private
// to that package; an external eval test avoids an eval -> adsrest import cycle.
func TestConditionConformance(t *testing.T) {
	cases := conditionCases()
	service := &pms.Service{Name: "conformance"}
	for i, c := range cases {
		service.Policies = append(service.Policies, &pms.Policy{ID: fmt.Sprint(i), Name: fmt.Sprint(i), Effect: pms.Grant, Condition: c.expression, Permissions: []*pms.Permission{{Resource: fmt.Sprint(i), Actions: []string{"read"}}}})
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
				v, err := adsrest.ConvValue(a.Type, a.Value)
				if err != nil {
					t.Fatal(err)
				}
				attrs[a.Name] = v
			}
			ctx := ads.RequestContext{ServiceName: service.Name, Resource: fmt.Sprint(i), Action: "read", Subject: subject, Attributes: attrs}
			direct, reason, err := e.IsAllowed(ctx)
			expectedReason := ads.NO_APPLICABLE_POLICIES
			if c.want {
				expectedReason = ads.GRANT_POLICY_FOUND
			}
			if err != nil || direct != c.want || reason != expectedReason {
				t.Errorf("direct %q: allowed=%v reason=%v err=%v; want %v/%v", c.expression, direct, reason, err, c.want, expectedReason)
			}
			body, err := json.Marshal(map[string]interface{}{"serviceName": service.Name, "resource": ctx.Resource, "action": ctx.Action, "subject": subject, "attributes": c.attributes})
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("POST", svcs.PolicyAtzPath+"is-allowed", bytes.NewReader(body)))
			if rec.Code != 200 {
				t.Fatalf("REST status %d: %s", rec.Code, rec.Body.String())
			}
			var got adsrest.IsAllowedResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Allowed != c.want || got.Allowed != direct || got.Reason != int32(reason) || got.ErrorMessage != "" {
				t.Errorf("REST %q: %+v; direct=%v/%v want=%v", c.expression, got, direct, reason, c.want)
			}
		})
	}
}

func TestConditionConformanceMatrix(t *testing.T) {
	counts := map[string]map[string]int{}
	sections := map[string]int{}
	for _, c := range conditionCases() {
		key := c.section + " / " + c.row
		if counts[key] == nil {
			counts[key] = map[string]int{}
		}
		counts[key][c.op]++
		sections[c.section]++
	}
	var rows []string
	for row := range counts {
		rows = append(rows, row)
	}
	sort.Strings(rows)
	t.Log("\n| Reference / row | " + strings.Join(conditionOperators, " | ") + " | other |\n|---|" + strings.Repeat("---|", len(conditionOperators)+1))
	for _, row := range rows {
		line := "| " + row + " |"
		total := 0
		for _, op := range conditionOperators {
			n := counts[row][op]
			total += n
			line += fmt.Sprintf(" %d |", n)
		}
		other := 0
		for _, n := range counts[row] {
			other += n
		}
		t.Log(line + fmt.Sprintf(" %d |", other-total))
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
			if counts["2.1 / "+typ][op] < 2 {
				t.Errorf("missing positive/negative cell %s %s", typ, op)
			}
		}
	}
}

// Diagnose exposes the actual injected Go types, before govaluate sanitizes
// integer values to float64. No PEP overrides are used in these assertions.
func TestConditionConformanceInjectedTypes(t *testing.T) {
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, []byte(`{"services":[{"name":"app"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	evaluator, err := eval.NewFromFile(location, false)
	if err != nil {
		t.Fatal(err)
	}
	e := evaluator.(*eval.PolicyEvalImpl)
	defer e.Close()
	result, err := e.Diagnose(ads.RequestContext{ServiceName: "app", Resource: "book", Action: "read", Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}, {Type: "group", Name: "staff"}, {Type: "entity", Name: "service"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]interface{}{"request_user": "alice", "request_groups": []interface{}{"staff"}, "request_entity": "service", "request_resource": "book", "request_action": "read", "request_time": int64(0), "request_year": 0, "request_month": 0, "request_day": 0, "request_hour": 0, "request_weekday": time.Now().Weekday().String()} {
		got, ok := result.Attributes[name]
		if !ok || reflect.TypeOf(got) != reflect.TypeOf(want) {
			t.Errorf("%s: %T (%v), want %T", name, got, got, want)
		}
	}
}
