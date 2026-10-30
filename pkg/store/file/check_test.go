//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import (
	"encoding/json"
	"errors"
	"github.com/teramoby/speedle-plus/testutil/pdltest"
	"reflect"
	"strings"
	"testing"
)

func TestCheckJSONRejectsStoreWideDuplicateIDs(t *testing.T) {
	data := `{"services":[{"name":"app","policies":[{"id":"same","effect":"grant","permissions":[{"resource":"book","actions":["read"]}]}]},{"name":"global","rolePolicies":[{"id":"same","effect":"deny","principals":["user:alice"],"roles":["reader"]}]}]}`
	_, problems := CheckJSON(strings.NewReader(data), "duplicate.json")
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "duplicate.json") || !strings.Contains(problems[0].Error(), `duplicate policy ID "same"`) {
		t.Fatalf("duplicate snapshot diagnostics: %v", problems)
	}
}

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
 ],"rolePolicies":[{"name":"rp1","effect":"grant","principals":["user:alice"]}, {"effect":"deny","principals":["user:alice"],"roles":["r"],"condition":"(a"}]},
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

func TestCheckJSONTypeError(t *testing.T) {
	_, errs := CheckJSON(strings.NewReader(`{"services":[{"name":7}]}`), "s.json")
	if len(errs) != 1 {
		t.Fatalf("errors %v", errs)
	}
	var typeErr *json.UnmarshalTypeError
	if !errors.As(errs[0], &typeErr) || typeErr.Value != "number" || typeErr.Type != reflect.TypeOf("") || !strings.HasPrefix(errs[0].Error(), "s.json:1:23:") || !strings.HasSuffix(typeErr.Field, "name") {
		t.Fatalf("unstable/missing diagnostic: %v", errs[0])
	}
}

func TestSPDLGrammarCorpus(t *testing.T) {
	for _, c := range pdltest.Cases {
		t.Run(c.Name, func(t *testing.T) {
			section := c.Kind
			ps, errs := CheckSPDL(strings.NewReader("[service.app]\n["+section+"]\n"+c.Text+"\n"), "corpus.spdl")
			if (len(errs) == 0) != c.Valid {
				t.Fatalf("valid=%v errors=%v", c.Valid, errs)
			}
			if c.Valid && c.Name == "hash" && ps.Services[0].Policies[0].Permissions[0].Resource != "book#1" {
				t.Fatal("quoted hash was truncated")
			}
		})
	}
}
func TestSPDLQuotedComment(t *testing.T) {
	ps, errs := CheckSPDL(strings.NewReader("[service.app]\n[policy]\ngrant user alice read 'book#1' if department == 'R#D' # comment\n"), "comment.spdl")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	p := ps.Services[0].Policies[0]
	if p.Condition != "department == 'R#D'" || p.Permissions[0].Resource != "book#1" {
		t.Fatalf("%+v", p)
	}
}
func FuzzCheckSPDL(f *testing.F) {
	for _, c := range pdltest.Cases {
		f.Add("[service.app]\n[" + c.Kind + "]\n" + c.Text + "\n")
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) <= 8192 {
			CheckSPDL(strings.NewReader(s), "fuzz.spdl")
		}
	})
}

func TestSPDLLiteralBackslashAndEscapedConditionComments(t *testing.T) {
	for _, line := range []string{
		`grant user 'a\' read 'book#1' # comment`,
		`grant user "a'b" read 'book#1' if x == 'a\' # b' # comment`,
		`grant user if read 'book#1' # comment`,
	} {
		ps, errs := CheckSPDL(strings.NewReader("[service.app]\n[policy]\n"+line+"\n"), "quotes.spdl")
		if len(errs) > 0 {
			t.Fatalf("%s: %v", line, errs)
		}
		if ps.Services[0].Policies[0].Permissions[0].Resource != "book#1" {
			t.Fatal("resource truncated")
		}
	}
}

func TestCheckJSONRejectsMissingRolePrincipals(t *testing.T) {
	for _, effect := range []string{"grant", "deny"} {
		for _, field := range []string{"", `,"principals":null`, `,"principals":[]`, `,"principals":[""]`, `,"principals":["user:alice",""]`} {
			data := `{"services":[{"name":"app","rolePolicies":[{"effect":"` + effect + `","roles":["reader"]` + field + `}]}]}`
			_, problems := CheckJSON(strings.NewReader(data), "invalid.json")
			if len(problems) != 1 || !strings.Contains(problems[0].Error(), "principal") {
				t.Fatalf("%s: %v", data, problems)
			}
		}
	}
}
