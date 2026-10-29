package docscheck

import (
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestOverviewExamples(t *testing.T) {
	for _, locale := range []string{"", ".zh"} {
		data, err := os.ReadFile(filepath.Join(root(), "docs/hugo/content/docs/overview"+locale+".md"))
		if err != nil {
			t.Fatal(err)
		}
		blocks := regexp.MustCompile("(?s)```text\\n(.*?)\\n```").FindAllSubmatch(data, -1)
		if len(blocks) != 4 {
			t.Fatalf("Expected 4 overview policies, got %d", len(blocks))
		}
		for i, b := range blocks {
			text := "[service.library]\n[policy]\n" + string(b[1]) + "\n"
			if i == 0 {
				text = "[service.library]\n[policy]\ngrant role librarian read,update book\n[rolepolicy]\ngrant user alice librarian\n"
				if string(b[1]) != "grant role librarian read,update book\ngrant user alice librarian" {
					t.Fatal("RBAC example drift")
				}
			}
			loc := filepath.Join(t.TempDir(), "policies.spdl")
			if err := os.WriteFile(loc, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			evaluator, err := eval.NewFromFile(loc, false)
			if err != nil {
				t.Fatal(err)
			}
			check := func(user, idd, group, resource, action string, attributes map[string]interface{}, want bool) {
				t.Helper()
				principals := []*ads.Principal{{Type: "user", Name: user, IDD: idd}}
				if group != "" {
					principals = append(principals, &ads.Principal{Type: "group", Name: group})
				}
				got, _, err := evaluator.IsAllowed(ads.RequestContext{Subject: &ads.Subject{Principals: principals}, ServiceName: "library", Resource: resource, Action: action, Attributes: attributes})
				if err != nil || got != want {
					t.Fatalf("%s example %d user %s: got=%v err=%v want=%v", locale, i, user, got, err, want)
				}
			}
			switch i {
			case 0:
				check("alice", "", "", "book", "update", nil, true)
				check("bob", "", "", "book", "update", nil, false)
			case 1:
				check("bob", "", "", "report", "read", map[string]interface{}{"department": "finance"}, true)
				check("bob", "", "", "report", "read", map[string]interface{}{"department": "sales"}, false)
				check("bob", "", "", "report", "read", nil, false)
			case 2:
				check("carol", "", "staff", "journal", "delete", nil, false)
				check("erin", "", "staff", "journal", "delete", nil, true)
			case 3:
				check("dave", "github", "", "code", "read", nil, true)
				check("dave", "corp", "", "code", "read", nil, false)
				check("dave", "", "", "code", "read", nil, false)
			}
		}
	}
}
