package docscheck

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"github.com/teramoby/speedle-plus/pkg/eval/function"
	"github.com/teramoby/speedle-plus/pkg/pdl"
)

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
