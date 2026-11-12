package docscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
)

// TestCloudSPDLExamples evaluates the actual quick-start policy and curl body
// in every locale and compares both documented responses with Engine decisions.
func TestCloudSPDLExamples(t *testing.T) {
	for _, locale := range []string{"", ".zh", ".es", ".ja", ".ko"} {
		t.Run("locale"+locale, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root(), "docs/hugo/content/docs/cloud/getting-started"+locale+".md"))
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			policy := regexp.MustCompile("(?s)```text\n(.*?)\n```").FindStringSubmatch(text)
			body := regexp.MustCompile(`--data '([^']+)'`).FindStringSubmatch(text)
			responses := regexp.MustCompile("(?s)```json\n(.*?)\n```").FindAllStringSubmatch(text, -1)
			if len(policy) != 2 || len(body) != 2 || len(responses) != 2 {
				t.Fatal("expected one policy, a curl body and allow/deny responses")
			}
			var published adsrest.JsonContext
			if err := json.Unmarshal([]byte(body[1]), &published); err != nil {
				t.Fatal(err)
			}
			request, err := adsrest.ConvertJSONRequestToContext(&published)
			if err != nil {
				t.Fatal(err)
			}
			if request.Subject == nil || len(request.Subject.Principals) != 1 || request.Subject.Principals[0].IDD == "" {
				t.Fatal("the application request must include the policy's identity domain")
			}
			location := filepath.Join(t.TempDir(), "policies.spdl")
			configuration := "[service." + request.ServiceName + "]\n[policy]\n" + policy[1] + "\n"
			if err := os.WriteFile(location, []byte(configuration), 0600); err != nil {
				t.Fatal(err)
			}
			evaluator, err := eval.NewFromFile(location, false)
			if err != nil {
				t.Fatal(err)
			}
			for i, response := range responses {
				if i == 1 {
					request.Action = "delete"
				}
				var want adsrest.IsAllowedResponse
				if err := json.Unmarshal([]byte(response[1]), &want); err != nil {
					t.Fatal(err)
				}
				if want.Allowed != (i == 0) {
					t.Fatal("expected an allowed response followed by a denied response")
				}
				allowed, reason, err := evaluator.IsAllowed(*request)
				if err != nil || allowed != want.Allowed || int32(reason) != want.Reason || want.ErrorMessage != "" {
					t.Fatalf("action=%s: allowed=%v reason=%v err=%v; documented=%+v", request.Action, allowed, reason, err, want)
				}
			}
			// A matching name in another domain must not inherit this grant.
			request.Action = published.Action
			request.Subject.Principals[0].IDD = "other"
			if allowed, _, err := evaluator.IsAllowed(*request); err != nil || allowed {
				t.Fatalf("other domain: allowed=%v err=%v", allowed, err)
			}
		})
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
	const documentedPolicy = "grant user alice from corp get /books"
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
		for _, required := range []string{
			"idd: authenticatedUserIdd",
			"principal name and identity domain are required",
		} {
			if !strings.Contains(string(text), required) {
				t.Errorf("%s: the Node.js example omits %q", locale, required)
			}
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
