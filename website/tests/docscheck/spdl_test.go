package docscheck

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/eval/function"
	"github.com/teramoby/speedle-plus/pkg/pdl"
)

func TestSPDLExamplesInTheReferenceParse(t *testing.T) {
	policies := []string{
		"grant group staff read,delete journal",
		"deny user carol delete journal",
		"grant user bob read report if department == 'finance'",
	}
	for _, statement := range policies {
		if _, _, err := pdl.ParsePolicy(statement, ""); err != nil {
			t.Errorf("documented policy %q: %v", statement, err)
		}
	}
	if _, _, err := pdl.ParseRolePolicy("grant role manager employee", ""); err != nil {
		t.Errorf("documented role policy %q: %v", "grant role manager employee", err)
	}
}

// The examples under "Examples" state decisions, not only syntax. Evaluate
// them through ADS so a regression in role inheritance, deny precedence or
// condition matching fails this check instead of silently contradicting the
// published reference (and llms-full.txt).
func TestSPDLExamplesDecideAsDocumented(t *testing.T) {
	examples := []string{
		"grant role manager employee",
		"grant group staff read,delete journal\ndeny user carol delete journal",
		"grant user bob read report if department == 'finance'",
	}
	for _, locale := range []string{"", ".zh"} {
		data, err := os.ReadFile(filepath.Join(root(), "docs/hugo/content/docs", "spdl"+locale+".md"))
		if err != nil {
			t.Fatal(err)
		}
		found := regexp.MustCompile("(?s)```text\\n(.*?)\\n```").FindAllStringSubmatch(string(data), -1)
		if len(found) != len(examples) {
			t.Fatalf("%s: expected %d documented SPDL examples, got %d", locale, len(examples), len(found))
		}
		for i := range found {
			if found[i][1] != examples[i] {
				t.Fatalf("%s: example %d is %q, want %q", locale, i, found[i][1], examples[i])
			}
		}
	}

	// The examples are statements, not a service fixture. Supply the role
	// assignments and role policies the prose assumes, then assert every
	// outcome the reference claims.
	fixture := strings.Join([]string{
		"[service.library]",
		"[policy]",
		examples[1],
		examples[2],
		"grant role employee read journal",
		"grant role manager update journal",
		"[rolepolicy]",
		examples[0],
		"grant user alice manager",
		"grant user erin employee",
	}, "\n") + "\n"
	location := filepath.Join(t.TempDir(), "policies.spdl")
	if err := os.WriteFile(location, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	evaluator, err := eval.NewFromFile(location, false)
	if err != nil {
		t.Fatal(err)
	}
	check := func(user, group, resource, action string, attributes map[string]interface{}, want bool) {
		t.Helper()
		principals := []*ads.Principal{{Type: ads.PRINCIPAL_TYPE_USER, Name: user}}
		if group != "" {
			principals = append(principals, &ads.Principal{Type: ads.PRINCIPAL_TYPE_GROUP, Name: group})
		}
		got, _, err := evaluator.IsAllowed(ads.RequestContext{
			Subject:     &ads.Subject{Principals: principals},
			ServiceName: "library",
			Resource:    resource,
			Action:      action,
			Attributes:  attributes,
		})
		if err != nil || got != want {
			t.Fatalf("%s in %s %s %s: got=%v err=%v want=%v", user, group, action, resource, got, err, want)
		}
	}

	// Role inheritance: manager holds the employee role and its permissions.
	check("alice", "", "journal", "read", nil, true)
	check("alice", "", "journal", "update", nil, true)
	check("erin", "", "journal", "read", nil, true)
	check("erin", "", "journal", "update", nil, false)
	check("bob", "", "journal", "read", nil, false)

	// Deny over grant: carol is staff but the deny wins for delete only.
	check("carol", "staff", "journal", "delete", nil, false)
	check("dave", "staff", "journal", "delete", nil, true)
	check("carol", "staff", "journal", "read", nil, true)

	// Condition: only department == 'finance' applies.
	check("bob", "", "report", "read", map[string]interface{}{"department": "finance"}, true)
	check("bob", "", "report", "read", map[string]interface{}{"department": "sales"}, false)
	check("bob", "", "report", "read", nil, false)
}

func TestConditionsExtractedFromBothSPDLReferences(t *testing.T) {
	examples := map[string]map[string]interface{}{
		"a=='abc'": {"a": "abc"}, "a!='abc'": {"a": "xyz"}, "a>='abc'": {"a": "abc"}, "a+b=='ab'": {"a": "a", "b": "b"}, "a=~'^get.*'": {"a": "getBook"},
		"a==123": {"a": float64(123)}, "a-b>123": {"a": float64(200), "b": float64(50)}, "a in (1, 2, 3)": {"a": float64(2)}, "'manager' in a": {"a": []interface{}{"manager"}},
		"IsSubSet(e, ('s1', 's2', 's3'))":                                     {"e": []interface{}{"s1", "s2"}},
		"a in (1, 2, 3) && (b==c || d==3) && IsSubSet(e, ('s1', 's2', 's3'))": {"a": float64(2), "b": float64(1), "c": float64(1), "d": float64(0), "e": []interface{}{"s1"}},
		"request_year==2019 && request_month==12":                             {"request_year": float64(2019), "request_month": float64(12)},
	}
	funcs := map[string]govaluate.ExpressionFunction{"IsSubSet": function.IsSubSet}
	for _, locale := range []string{"", ".zh"} {
		raw, err := os.ReadFile(filepath.Join(root(), "docs/hugo/content/docs", "spdl"+locale+".md"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		start := strings.Index(text, "#### 2.6")
		if start < 0 {
			t.Fatal("missing condition examples")
		}
		rows := regexp.MustCompile(`<tr><td>(.*?)</td><td>`).FindAllStringSubmatch(text[start:], -1)
		if len(rows) != len(examples) {
			t.Fatalf("%s: expected %d examples, got %d", locale, len(examples), len(rows))
		}
		for _, row := range rows {
			condition := strings.TrimSpace(html.UnescapeString(row[1]))
			values, ok := examples[condition]
			if !ok {
				t.Fatalf("unverified documentation condition %q", condition)
			}
			expression, err := govaluate.NewEvaluableExpressionWithFunctions(condition, funcs)
			if err != nil {
				t.Fatalf("%s: %v", condition, err)
			}
			value, err := expression.Evaluate(values)
			if err != nil || value != true {
				t.Fatalf("%s: value=%v error=%v", condition, value, err)
			}
		}
	}
	// Actual parsed policies must restrict the intended complete resource path.
	policy, _, err := pdl.ParsePolicy("grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$", "")
	if err != nil {
		t.Fatal(err)
	}
	pattern, err := regexp.Compile(policy.Permissions[0].ResourceExpression)
	if err != nil {
		t.Fatal(err)
	}
	for resource, want := range map[string]bool{"c1/default/core/pods/pod1": true, "c1/default/core/podssecret": false, "prefixc1/default/core/pods/pod1": false} {
		if pattern.MatchString(resource) != want {
			t.Errorf("unexpected resource match: %s", resource)
		}
	}
}
