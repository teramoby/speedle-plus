package docscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/pdl"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
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

// isAllowedExample returns the request body of the fenced http IsAllowed
// example in a Cloud documentation page.
func isAllowedExample(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root(), path))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "```http" {
			continue
		}
		request := -1
		for j := i + 1; j < len(lines) && strings.TrimSpace(lines[j]) != "```"; j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "POST /authz-check/v1/is-allowed") {
				request = j
				break
			}
		}
		if request < 0 {
			continue
		}
		for k := request + 1; k < len(lines) && strings.TrimSpace(lines[k]) != "```"; k++ {
			if body := strings.TrimSpace(lines[k]); strings.HasPrefix(body, "{") {
				return body
			}
		}
	}
	t.Fatalf("%s: missing IsAllowed request example", path)
	return ""
}

// TestCloudAuthorizationExamplesCarryIdentityDomain checks that the published
// IsAllowed examples send the identity domain the page documents. Without
// idd, a principal named alice matches every tenant that reuses that name.
func TestCloudAuthorizationExamplesCarryIdentityDomain(t *testing.T) {
	const documentedPolicy = "grant user alice get /books"
	for _, locale := range []string{"", ".zh"} {
		downloadLanguage := "en"
		if locale != "" {
			downloadLanguage = "zh-CN"
		}
		page := "docs/hugo/content/docs/cloud/authorization-api" + locale + ".md"
		download := "docs/hugo/static/docs/cloud/quickstart." + downloadLanguage + ".md"

		body := isAllowedExample(t, page)
		if body != isAllowedExample(t, download) {
			t.Errorf("%s: download and page disagree on the IsAllowed request", locale)
		}
		text, err := os.ReadFile(filepath.Join(root(), page))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(text), "idd: authenticatedUserIdd") {
			t.Errorf("%s: the Node.js example omits idd", locale)
		}

		var published adsrest.JsonContext
		if err := json.Unmarshal([]byte(body), &published); err != nil {
			t.Fatalf("%s: %v", locale, err)
		}
		request, err := adsrest.ConvertJSONRequestToContext(&published)
		if err != nil {
			t.Fatalf("%s: %v", locale, err)
		}
		if request.Subject == nil || len(request.Subject.Principals) != 1 {
			t.Fatalf("%s: unexpected principals: %+v", locale, request.Subject)
		}
		if idd := request.Subject.Principals[0].IDD; idd == "" {
			t.Fatalf("%s: the IsAllowed example omits idd", locale)
		}

		location := filepath.Join(t.TempDir(), "policies.spdl")
		configuration := "[service.books]\n[policy]\n" + documentedPolicy + "\n"
		if err := os.WriteFile(location, []byte(configuration), 0600); err != nil {
			t.Fatal(err)
		}
		evaluator, err := eval.NewFromFile(location, false)
		if err != nil {
			t.Fatalf("%s: %v", locale, err)
		}
		if allowed, _, err := evaluator.IsAllowed(*request); err != nil || !allowed {
			t.Fatalf("%s: documented request against the getting-started policy: allowed=%v err=%v", locale, allowed, err)
		}

		// The domain is part of the decision, so the page's from example is exact.
		scoped := filepath.Join(t.TempDir(), "scoped.spdl")
		configuration = "[service.books]\n[policy]\ngrant user alice from " +
			request.Subject.Principals[0].IDD + " get /books\n"
		if err := os.WriteFile(scoped, []byte(configuration), 0600); err != nil {
			t.Fatal(err)
		}
		evaluator, err = eval.NewFromFile(scoped, false)
		if err != nil {
			t.Fatalf("%s: %v", locale, err)
		}
		if allowed, _, err := evaluator.IsAllowed(*request); err != nil || !allowed {
			t.Fatalf("%s: from-scoped policy: allowed=%v err=%v", locale, allowed, err)
		}
		other := *request.Subject.Principals[0]
		other.IDD = "other"
		request.Subject.Principals[0] = &other
		if allowed, _, err := evaluator.IsAllowed(*request); err != nil || allowed {
			t.Fatalf("%s: from-scoped policy accepted another domain: allowed=%v err=%v", locale, allowed, err)
		}
	}
}
