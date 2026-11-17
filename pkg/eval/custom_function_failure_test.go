// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// failingFunctionServer returns an error response for every call and counts
// the calls it receives.
func failingFunctionServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `{"error":"function unavailable"}`)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func aliceContext(service, resource, action string) adsapi.RequestContext {
	return adsapi.RequestContext{
		Subject:     &adsapi.Subject{Principals: []*adsapi.Principal{{Type: adsapi.PRINCIPAL_TYPE_USER, Name: "alice"}}},
		ServiceName: service, Resource: resource, Action: action,
	}
}

const aliceReadPrincipals = `"principals":[["user:alice"]],"permissions":[{"resource":"r","actions":["read"]}]`

// TestFunctionFailureFailsGrantPolicy checks a grant policy whose condition
// cannot run: the decision is an error, not a denial and not a silent nonmatch.
func TestFunctionFailureFailsGrantPolicy(t *testing.T) {
	server, calls := failingFunctionServer(t)
	p, _ := newFixtureEvaluator(t, fmt.Sprintf(`{
		"functions":[{"name":"boom","funcURL":%q}],
		"services":[{"name":"app","policies":[{"id":"grant","effect":"grant",%s,"condition":"boom() == 1"}]}]}`,
		server.URL, aliceReadPrincipals))
	allowed, reason, err := p.IsAllowed(aliceContext("app", "r", "read"))
	if err == nil || allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if reason != adsapi.ERROR_IN_EVALUATION {
		t.Errorf("reason = %v, want ERROR_IN_EVALUATION", reason)
	}
	if !IsCustomFunctionError(err) || errors.Code(err) != errors.CustomerFuncError {
		t.Errorf("error %v is not a customer function error", err)
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "function unavailable") {
		t.Errorf("error %q does not identify the failed call", err)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Errorf("%d function calls, want 1", got)
	}
}

// TestFunctionFailureFailsDenyPolicy checks that a failing condition on a deny
// policy cannot be ignored: an unconditional grant must not win.
func TestFunctionFailureFailsDenyPolicy(t *testing.T) {
	server, _ := failingFunctionServer(t)
	p, _ := newFixtureEvaluator(t, fmt.Sprintf(`{
		"functions":[{"name":"boom","funcURL":%q}],
		"services":[{"name":"app","policies":[
			{"id":"grant","effect":"grant",%s},
			{"id":"deny","effect":"deny",%s,"condition":"boom() == 1"}]}]}`,
		server.URL, aliceReadPrincipals, aliceReadPrincipals))
	allowed, _, err := p.IsAllowed(aliceContext("app", "r", "read"))
	if err == nil {
		t.Fatalf("allowed=%v, want an error", allowed)
	}
	if !IsCustomFunctionError(err) {
		t.Errorf("error %v is not a customer function error", err)
	}
}

// TestFunctionFailureFailsRolePolicy checks the role-policy path, which runs
// before the decision policies.
func TestFunctionFailureFailsRolePolicy(t *testing.T) {
	server, _ := failingFunctionServer(t)
	p, _ := newFixtureEvaluator(t, fmt.Sprintf(`{
		"functions":[{"name":"boom","funcURL":%q}],
		"services":[{"name":"app",
			"rolePolicies":[{"id":"role","effect":"grant","roles":["admin"],"principals":["user:alice"],"condition":"boom() == 1"}],
			"policies":[{"id":"grant","effect":"grant",%s}]}]}`,
		server.URL, aliceReadPrincipals))
	ctx := aliceContext("app", "r", "read")
	if _, err := p.GetAllGrantedRoles(ctx); !IsCustomFunctionError(err) {
		t.Errorf("GetAllGrantedRoles error %v is not a customer function error", err)
	}
	if _, err := p.GetAllGrantedPermissions(ctx); !IsCustomFunctionError(err) {
		t.Errorf("GetAllGrantedPermissions error %v is not a customer function error", err)
	}
	if allowed, _, err := p.IsAllowed(ctx); err == nil || allowed {
		t.Errorf("allowed=%v err=%v, want an error", allowed, err)
	}
}

// TestFunctionFailureFailsGrantedPermissions covers a policy condition failure
// in the permission calculation path.
func TestFunctionFailureFailsGrantedPermissions(t *testing.T) {
	server, _ := failingFunctionServer(t)
	p, _ := newFixtureEvaluator(t, fmt.Sprintf(`{
		"functions":[{"name":"boom","funcURL":%q}],
		"services":[{"name":"app","policies":[{"id":"grant","effect":"grant",%s,"condition":"boom() == 1"}]}]}`,
		server.URL, aliceReadPrincipals))
	if _, err := p.GetAllGrantedPermissions(aliceContext("app", "r", "read")); !IsCustomFunctionError(err) {
		t.Errorf("GetAllGrantedPermissions error %v is not a customer function error", err)
	}
}

// TestDiagnoseReportsFunctionFailure checks that Diagnose returns its result
// and reports the failed call.
func TestDiagnoseReportsFunctionFailure(t *testing.T) {
	server, _ := failingFunctionServer(t)
	p, _ := newFixtureEvaluator(t, fmt.Sprintf(`{
		"functions":[{"name":"boom","funcURL":%q}],
		"services":[{"name":"app","policies":[{"id":"grant","effect":"grant",%s,"condition":"boom() == 1"}]}]}`,
		server.URL, aliceReadPrincipals))
	result, err := p.Diagnose(aliceContext("app", "r", "read"))
	if err == nil || !IsCustomFunctionError(err) {
		t.Fatalf("error %v is not a customer function error", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not name the function", err)
	}
	if result == nil {
		t.Fatal("Diagnose returned no result")
	}
	if result.Allowed || result.Reason != adsapi.ERROR_IN_EVALUATION {
		t.Errorf("result allowed=%v reason=%v", result.Allowed, result.Reason)
	}
}

// TestFunctionFailureNotCached checks that a failed call is never stored in the
// result cache, even for a cacheable function.
func TestFunctionFailureNotCached(t *testing.T) {
	server, calls := failingFunctionServer(t)
	p, _ := newFixtureEvaluator(t, fmt.Sprintf(`{
		"functions":[{"name":"boom","funcURL":%q,"resultCachable":true,"resultTTL":60}],
		"services":[{"name":"app","policies":[{"id":"grant","effect":"grant",%s,"condition":"boom() == 1"}]}]}`,
		server.URL, aliceReadPrincipals))
	for i := 0; i < 2; i++ {
		if _, _, err := p.IsAllowed(aliceContext("app", "r", "read")); !IsCustomFunctionError(err) {
			t.Fatalf("call %d: error %v is not a customer function error", i, err)
		}
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Errorf("%d function calls, want 2: a failure was cached", got)
	}
	if len(p.RuntimePolicyStore.FunctionResultCache.Results) != 0 {
		t.Errorf("cache holds %v", p.RuntimePolicyStore.FunctionResultCache.Results)
	}
}

// TestConditionErrorsKeepNotMatching pins the unchanged behavior: a condition
// on a missing attribute still does not match, and no error is returned.
func TestConditionErrorsKeepNotMatching(t *testing.T) {
	p, _ := newFixtureEvaluator(t, `{
		"services":[{"name":"app","policies":[
			{"id":"grant","effect":"grant",`+aliceReadPrincipals+`},
			{"id":"deny","effect":"deny",`+aliceReadPrincipals+`,"condition":"missing == 'x'"}]}]}`)
	allowed, _, err := p.IsAllowed(aliceContext("app", "r", "read"))
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v; a missing attribute must not match the deny", allowed, err)
	}
}
