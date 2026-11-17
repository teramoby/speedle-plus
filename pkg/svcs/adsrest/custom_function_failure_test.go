// Licensed under the Universal Permissive License (UPL) Version 1.0.
package adsrest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

// newFailingFunctionRouter serves policies whose conditions call a custom
// function that always returns an error.
func newFailingFunctionRouter(t *testing.T) http.Handler {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":"function unavailable"}`)
	}))
	t.Cleanup(server.Close)
	policies := fmt.Sprintf(`{
		"functions":[{"name":"boom","funcURL":%q}],
		"services":[{"name":"app",
			"rolePolicies":[{"id":"rp1","effect":"grant","roles":["admin"],"principals":["user:alice"],"condition":"boom() == 1"}],
			"policies":[
				{"id":"p1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"res1","actions":["read"]}]},
				{"id":"p2","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"res1","actions":["read"]}],"condition":"boom() == 1"}]}]}`,
		server.URL)
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(policies), 0600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	evaluator, err := eval.NewWithStore(&cfg.Config{}, storetest.PolicyStore(t, location))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = evaluator.(*eval.PolicyEvalImpl).Close() })
	router, err := NewRouter(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

// TestDecisionHandlersReportFunctionFailures checks that a failed custom
// function call reaches the caller as an error response, never as an
// allowed or denied decision body.
func TestDecisionHandlersReportFunctionFailures(t *testing.T) {
	router := newFailingFunctionRouter(t)
	body := `{` + alice + `,"resource":"res1","action":"read"}`
	for _, path := range []string{"is-allowed", "all-granted-roles", "all-granted-permissions", "diagnose"} {
		rec := post(t, router, path, body, http.StatusInternalServerError)
		var response map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("%s: body %q: %v", path, rec.Body.String(), err)
		}
		if _, decided := response["allowed"]; decided {
			t.Errorf("%s: returned a decision body: %s", path, rec.Body.String())
		}
		message, _ := response["error"].(string)
		if !strings.Contains(message, "boom") {
			t.Errorf("%s: error %q does not name the failed function", path, message)
		}
	}
}
