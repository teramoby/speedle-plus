package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/pdl"
)

func TestCloudSPDLExamples(t *testing.T) {
	for _, locale := range []string{"", ".zh"} {
		data, err := os.ReadFile(filepath.Join(root(), "docs/hugo/content/docs/cloud/getting-started"+locale+".md"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		const authorization = "grant user alice get /books"
		if !strings.Contains(text, "```text\n"+authorization+"\n```") {
			t.Fatalf("%s: missing authorization example", locale)
		}

		location := filepath.Join(t.TempDir(), "policies.spdl")
		configuration := "[service.books]\n[policy]\n" + authorization + "\n"
		if err := os.WriteFile(location, []byte(configuration), 0600); err != nil {
			t.Fatal(err)
		}
		evaluator, err := eval.NewFromFile(location, false)
		if err != nil {
			t.Fatalf("%s: %v", locale, err)
		}
		for _, check := range []struct {
			user     string
			resource string
			action   string
			want     bool
		}{
			{user: "alice", resource: "/books", action: "get", want: true},
			{user: "bob", resource: "/books", action: "get", want: false},
			{user: "alice", resource: "/books", action: "write", want: false},
			{user: "alice", resource: "/other", action: "get", want: false},
		} {
			got, _, err := evaluator.IsAllowed(ads.RequestContext{
				Subject:     &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: check.user}}},
				ServiceName: "books",
				Resource:    check.resource,
				Action:      check.action,
				Attributes:  map[string]interface{}{"level": 3},
			})
			if err != nil || got != check.want {
				t.Fatalf("%s: %s %s %s: got=%v err=%v want=%v", locale, check.user, check.action, check.resource, got, err, check.want)
			}
		}

		for _, statement := range []string{"grant user alice editor", "grant user alice editor on /books"} {
			if !strings.Contains(text, "`"+statement+"`") {
				t.Errorf("%s: missing role example %q", locale, statement)
			}
			if _, _, err := pdl.ParseRolePolicy(statement, "rp"); err != nil {
				t.Errorf("%s: %q: %v", locale, statement, err)
			}
		}
		const explicitAuthorization = "grant (user alice) role editor"
		if !strings.Contains(text, "`"+explicitAuthorization+"`") {
			t.Errorf("%s: missing explicit authorization example", locale)
		}
		if _, _, err := pdl.ParsePolicy(explicitAuthorization, "p"); err != nil {
			t.Errorf("%s: %q: %v", locale, explicitAuthorization, err)
		}

		// Cloud rejects this statement because both Engine parsers accept it.
		const ambiguous = "grant user alice role editor"
		if !strings.Contains(text, "`"+ambiguous+"`") {
			t.Errorf("%s: missing ambiguity example", locale)
		}
		if _, _, err := pdl.ParsePolicy(ambiguous, "p"); err != nil {
			t.Errorf("%s: ambiguity no longer exercised by the authorization parser: %v", locale, err)
		}
		if _, _, err := pdl.ParseRolePolicy(ambiguous, "rp"); err != nil {
			t.Errorf("%s: ambiguity no longer exercised by the role parser: %v", locale, err)
		}
	}
}
