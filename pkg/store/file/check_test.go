//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import (
	"strings"
	"testing"
)

func errorTexts(errs []error) []string {
	texts := make([]string, len(errs))
	for i, err := range errs {
		texts[i] = err.Error()
	}
	return texts
}

func TestCheckSPDLListsEveryError(t *testing.T) {
	spdl := `[service.orders]
[policy]
GRANT USER alice read /orders
GRANT USER alice approve /orders if amount <=
GRAN USER bob read /orders
[rolepolicy]
GRANT USER carol manager on /orders if a b
[service.hr]
[policy]
GRANT ROLE manager read /hr
`
	ps, errs := CheckSPDL(strings.NewReader(spdl), "p.spdl")
	got := errorTexts(errs)
	want := []string{
		"p.spdl:4: Invalid condition: Unexpected end of expression\nGRANT USER alice approve /orders if amount <=\n                                             ^",
		"p.spdl:5: Not found valid effect\nGRAN USER bob read /orders\n^",
		"p.spdl:7: Invalid condition: Cannot transition token types from VARIABLE [a] to VARIABLE [b]\nGRANT USER carol manager on /orders if a b\n                                         ^",
	}
	if strings.Join(got, "\n--\n") != strings.Join(want, "\n--\n") {
		t.Fatalf("errors:\n%s\nwant:\n%s", strings.Join(got, "\n--\n"), strings.Join(want, "\n--\n"))
	}
	// The valid lines around the errors are still read.
	if len(ps.Services) != 2 || len(ps.Services[0].Policies) != 1 || len(ps.Services[1].Policies) != 1 {
		t.Errorf("store %+v", ps.Services)
	}
}

func TestCheckSPDLReportsSectionErrors(t *testing.T) {
	_, errs := CheckSPDL(strings.NewReader("[policy]\n[service.a\n[nosuch]\n"), "p.spdl")
	if len(errs) != 3 {
		t.Fatalf("errors %q, want 3", errorTexts(errs))
	}
}

func TestCheckJSONReportsPolicyProblems(t *testing.T) {
	store := `{"services":[
 {"name":"orders","policies":[
  {"name":"p1","effect":"Deny","permissions":[{"resource":"/o","actions":["read"]}]},
  {"effect":"grant","condition":"amount >"}
 ],"rolePolicies":[{"name":"rp1","effect":"grant"}, {"effect":"deny","roles":["r"],"condition":"(a"}]},
 {"policies":[{"effect":"grant","permissions":[{"resource":"/x"}]}]}
]}`
	_, errs := CheckJSON(strings.NewReader(store), "s.json")
	want := []string{
		`s.json: service "orders" policy "p1": effect "Deny" is neither "grant" nor "deny"`,
		`s.json: service "orders" policy 2: no permissions`,
		"s.json: service \"orders\" policy 2: invalid condition at character 9: Unexpected end of expression\namount >\n        ^",
		`s.json: service "orders" role policy "rp1": no roles`,
		"s.json: service \"orders\" role policy 2: invalid condition at character 1: Unbalanced parenthesis\n(a\n^",
		`s.json: service 2: no name`,
	}
	if got := errorTexts(errs); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("errors:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestCheckJSONReportsWhereTheFileIsWrong(t *testing.T) {
	cases := []struct{ name, json, want string }{
		{"syntax", "{\"services\":[{\"name\":\"x\",}]}", "s.json:1:26: invalid character '}' looking for beginning of object key string"},
		{"unknown field", "{\"services\":[{\"name\":\"x\",\n \"policies\":[{\"effect\":\"grant\",\"conditon\":\"a\"}]}]}", `s.json:2:32: json: unknown field "conditon"`},
		{"type", "{\"services\":[{\"name\":7}]}", "s.json:1:23: json: cannot unmarshal number into Go struct field Service.services.name of type string"},
	}
	for _, c := range cases {
		_, errs := CheckJSON(strings.NewReader(c.json), "s.json")
		if len(errs) != 1 || errs[0].Error() != c.want {
			t.Errorf("%s: errors %q, want %q", c.name, errorTexts(errs), c.want)
		}
	}
}

func TestCheckJSONAcceptsAValidStore(t *testing.T) {
	store := `{"services":[{"name":"orders","policies":[
  {"name":"p1","effect":"grant","permissions":[{"resource":"/o","actions":["read"]}],"principals":[["user:alice"]],"condition":"amount <= 5"}],
  "rolePolicies":[{"effect":"deny","roles":["r"],"principals":["user:bob"]}]}]}`
	ps, errs := CheckJSON(strings.NewReader(store), "s.json")
	if len(errs) != 0 || len(ps.Services) != 1 {
		t.Fatalf("errors %q, store %+v", errorTexts(errs), ps)
	}
}

func TestLineAndColumn(t *testing.T) {
	data := []byte("ab\ncd\n")
	for _, c := range []struct {
		offset    int64
		line, col int
	}{{0, 1, 1}, {1, 1, 2}, {3, 2, 1}, {4, 2, 2}, {99, 3, 1}} {
		if line, col := lineAndColumn(data, c.offset); line != c.line || col != c.col {
			t.Errorf("lineAndColumn(%d) = %d:%d, want %d:%d", c.offset, line, col, c.line, c.col)
		}
	}
}

func TestCheckJSONReportsNullEntries(t *testing.T) {
	_, errs := CheckJSON(strings.NewReader(`{"services":[null,{"name":"a","policies":[null],"rolePolicies":[null]}]}`), "s.json")
	want := []string{
		`s.json: service 1: null`,
		`s.json: service "a" policy 1: null`,
		`s.json: service "a" role policy 1: null`,
	}
	if got := errorTexts(errs); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("errors:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestCheckJSONRejectsDataAfterTheStore(t *testing.T) {
	cases := []struct{ name, json, want string }{
		{"second value", `{"services":[]} {"services":[]}`, "s.json:1:17: unexpected data after the policy store"},
		{"garbage", "{\"services\":[]}\n garbage", "s.json:2:2: invalid character 'g' looking for beginning of value"},
	}
	for _, c := range cases {
		_, errs := CheckJSON(strings.NewReader(c.json), "s.json")
		if len(errs) != 1 || errs[0].Error() != c.want {
			t.Errorf("%s: errors %q, want %q", c.name, errorTexts(errs), c.want)
		}
	}
	// Trailing white space is fine.
	if _, errs := CheckJSON(strings.NewReader("{\"services\":[]}\n\n"), "s.json"); len(errs) != 0 {
		t.Errorf("trailing new lines: errors %q", errorTexts(errs))
	}
}
