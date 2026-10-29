//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pmsrest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

const base = svcs.PolicyMgmtPath

type call struct {
	method, path, body string
	status             int
}

func serve(t *testing.T, router http.Handler, c call) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != c.status {
		t.Errorf("%s %s: status %d, want %d (%s)", c.method, c.path, rec.Code, c.status, rec.Body.String())
	}
	return rec
}

// newFileRouter serves an empty file store kept in a temporary directory;
// discover requests are kept in the working directory, so the test moves
// there.
func newFileRouter(t *testing.T) (*mux.Router, pms.PolicyStoreManager) {
	t.Helper()
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	ps, err := store.NewStore("file", map[string]interface{}{"FileLocation": location})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(ps)
	if err != nil {
		t.Fatal(err)
	}
	return router, ps
}

func TestFunctionHandlers(t *testing.T) {
	router, _ := newFileRouter(t)
	for _, c := range []call{
		{"GET", base + "function", "", 200},
		{"POST", base + "function", `{"name":"f1","funcURL":"https://f1"}`, 201},
		{"POST", base + "function", `{"name":"f1","funcURL":"https://f1"}`, 409},
		{"POST", base + "function", `{`, 400},
		{"GET", base + "function/f1", "", 200},
		{"GET", base + "function/f2", "", 404},
		{"GET", base + "function", "", 200},
		{"DELETE", base + "function/f1", "", 204},
		{"DELETE", base + "function/f1", "", 404},
		{"POST", base + "function", `{"name":"f2","funcURL":"https://f2"}`, 201},
		{"DELETE", base + "function", "", 204},
	} {
		serve(t, router, c)
	}
	rec := serve(t, router, call{"GET", base + "function", "", 200})
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("functions after deleting all: %s", rec.Body.String())
	}
}

func TestServiceAndPolicyHandlers(t *testing.T) {
	router, _ := newFileRouter(t)
	for _, c := range []call{
		{"POST", base + "service", `{"name":"app","type":"application"}`, 201},
		{"POST", base + "service", `{"name":"app","type":"application"}`, 400},
		{"POST", base + "service", `{`, 400},
		{"GET", base + "service/app", "", 200},
		{"GET", base + "service/nope", "", 404},
		{"GET", base + "policy-counts", "", 200},
		{"POST", base + "service/app/policy", `{"name":"p","effect":"grant","principals":[["user:a"]],"permissions":[{"resource":"r","actions":["get"]}]}`, 201},
		{"POST", base + "service/app/policy", `{"name":"p"}`, 400},
		{"POST", base + "service/app/policy", `{`, 400},
		{"POST", base + "service/nope/policy", `{"name":"p","effect":"grant"}`, 404},
		{"GET", base + "service/app/policy", "", 200},
		{"GET", base + "service/app/policy?filter=name+eq+p", "", 200},
		{"GET", base + "service/nope/policy", "", 404},
		{"GET", base + "service/app/policy/nope", "", 404},
		{"DELETE", base + "service/app/policy/nope", "", 404},
		{"POST", base + "service/app/role-policy", `{"name":"rp","effect":"grant","roles":["r"],"principals":["user:a"]}`, 201},
		{"POST", base + "service/app/role-policy", `{"name":"rp"}`, 400},
		{"POST", base + "service/app/role-policy", `{`, 400},
		{"POST", base + "service/nope/role-policy", `{"name":"rp","effect":"grant"}`, 404},
		{"GET", base + "service/app/role-policy", "", 200},
		{"GET", base + "service/nope/role-policy", "", 404},
		{"GET", base + "service/app/role-policy/nope", "", 404},
		{"DELETE", base + "service/app/role-policy/nope", "", 404},
		{"DELETE", base + "service/app/role-policy", "", 204},
		{"GET", base + "service/app/role-policy", "", 200},
		{"DELETE", base + "service/app/policy", "", 204},
		{"GET", base + "service/app/policy", "", 200},
		{"DELETE", base + "service/nope/policy", "", 404},
		{"DELETE", base + "service/nope/role-policy", "", 404},
		{"DELETE", base + "service/app", "", 204},
		{"DELETE", base + "service/app", "", 404},
		{"POST", base + "service", `{"name":"other"}`, 201},
		{"DELETE", base + "service", "", 204},
		{"GET", base + "service", "", 200},
	} {
		serve(t, router, c)
	}

	// Policies and role policies are found by the ID the store assigned.
	serve(t, router, call{"POST", base + "service", `{"name":"app"}`, 201})
	rec := serve(t, router, call{"POST", base + "service/app/policy", `{"name":"p","effect":"deny"}`, 201})
	var policy pms.Policy
	json.Unmarshal(rec.Body.Bytes(), &policy)
	rec = serve(t, router, call{"POST", base + "service/app/role-policy", `{"name":"rp","effect":"deny","roles":["r"]}`, 201})
	var rolePolicy pms.RolePolicy
	json.Unmarshal(rec.Body.Bytes(), &rolePolicy)
	for _, c := range []call{
		{"GET", base + "service/app/policy/" + policy.ID, "", 200},
		{"GET", base + "service/app/role-policy/" + rolePolicy.ID, "", 200},
		{"GET", base + "service/app/role-policy?filter=name+eq+rp", "", 200},
		{"DELETE", base + "service/app/policy/" + policy.ID, "", 204},
		{"DELETE", base + "service/app/role-policy/" + rolePolicy.ID, "", 204},
	} {
		serve(t, router, c)
	}
}

func TestDiscoverHandlers(t *testing.T) {
	router, ps := newFileRouter(t)
	manager := ps.(store.DiscoverRequestManager)
	for _, resource := range []string{"r1", "r2"} {
		if err := manager.SaveDiscoverRequest(&ads.RequestContext{
			Subject:     &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}}},
			ServiceName: "app", Resource: resource, Action: "read",
		}); err != nil {
			t.Fatal(err)
		}
	}
	decode := func(rec *httptest.ResponseRecorder) GetDiscoverRequestsResponse {
		var got GetDiscoverRequestsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := decode(serve(t, router, call{"GET", base + "discover-request", "", 200})); len(got.Requests) != 2 {
		t.Errorf("all requests = %+v", got)
	}
	if got := decode(serve(t, router, call{"GET", base + "discover-request?last=true", "", 200})); len(got.Requests) != 1 || got.Requests[0].Resource != "r2" {
		t.Errorf("last request = %+v", got)
	}
	if got := decode(serve(t, router, call{"GET", base + "discover-request?revision=1", "", 200})); got.Revision == 0 {
		t.Errorf("requests since 1 = %+v", got)
	}
	if got := decode(serve(t, router, call{"GET", base + "discover-request/app", "", 200})); len(got.Requests) != 2 {
		t.Errorf("app requests = %+v", got)
	}
	if got := decode(serve(t, router, call{"GET", base + "discover-request/app?last=true", "", 200})); len(got.Requests) != 1 {
		t.Errorf("last app request = %+v", got)
	}
	serve(t, router, call{"GET", base + "discover-request/app?revision=1", "", 200})
	serve(t, router, call{"GET", base + "discover-request?revision=x", "", 400})
	serve(t, router, call{"GET", base + "discover-request/app?revision=x", "", 400})

	rec := serve(t, router, call{"GET", base + "discover-policy/app?principalType=user&principalName=alice", "", 200})
	var policies GetDiscoverPoliciesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &policies); err != nil || len(policies.Services) != 1 {
		t.Errorf("generated policies = %s", rec.Body.String())
	}
	serve(t, router, call{"GET", base + "discover-policy", "", 200})

	serve(t, router, call{"DELETE", base + "discover-request/app", "", 204})
	serve(t, router, call{"GET", base + "discover-request/app?last=true", "", 404})
	serve(t, router, call{"DELETE", base + "discover-request", "", 204})
}

func TestHandlersReportStoreErrors(t *testing.T) {
	failing := storetest.NewFailingStore()
	failing.CountsSucceed = true
	router, err := NewRouter(failing)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []call{
		{"POST", base + "service", `{"name":"app"}`, 500},
		{"DELETE", base + "service", "", 500},
		{"DELETE", base + "service/app", "", 500},
		{"GET", base + "service/app", "", 500},
		{"GET", base + "service", "", 500},
		{"GET", base + "policy-counts", "", 500},
		{"POST", base + "service/app/policy", `{"name":"p","effect":"grant"}`, 500},
		{"DELETE", base + "service/app/policy", "", 500},
		{"DELETE", base + "service/app/policy/id", "", 500},
		{"GET", base + "service/app/policy/id", "", 500},
		{"GET", base + "service/app/policy", "", 500},
		{"PUT", base + "service/app/policy/id", `{"name":"p","effect":"grant"}`, 500},
		{"POST", base + "service/app/role-policy", `{"name":"rp","effect":"grant"}`, 500},
		{"DELETE", base + "service/app/role-policy", "", 500},
		{"DELETE", base + "service/app/role-policy/id", "", 500},
		{"GET", base + "service/app/role-policy/id", "", 500},
		{"GET", base + "service/app/role-policy", "", 500},
		{"PUT", base + "service/app/role-policy/id", `{"name":"rp","effect":"grant"}`, 500},
		{"POST", base + "function", `{"name":"f"}`, 500},
		{"DELETE", base + "function", "", 500},
		{"DELETE", base + "function/f", "", 500},
		{"GET", base + "function/f", "", 500},
		{"GET", base + "function", "", 500},
		{"GET", base + "discover-request", "", 500},
		{"GET", base + "discover-request?last=true", "", 500},
		{"GET", base + "discover-request?revision=2", "", 500},
		{"GET", base + "discover-request/app", "", 500},
		{"GET", base + "discover-request/app?last=true", "", 500},
		{"GET", base + "discover-request/app?revision=2", "", 500},
		{"DELETE", base + "discover-request/app", "", 500},
		{"DELETE", base + "discover-request", "", 500},
		{"GET", base + "discover-policy/app", "", 500},
	} {
		serve(t, router, c)
	}

	// The limit checks fail before the write when the counts fail.
	router, _ = NewRouter(storetest.NewFailingStore())
	for _, c := range []call{
		{"POST", base + "service", `{"name":"app"}`, 500},
		{"POST", base + "service/app/policy", `{"name":"p","effect":"grant"}`, 500},
		{"POST", base + "service/app/role-policy", `{"name":"rp","effect":"grant"}`, 500},
		{"POST", base + "function", `{"name":"f"}`, 500},
	} {
		serve(t, router, c)
	}
}

// noDiscoverStore hides the discover methods of the store it wraps.
type noDiscoverStore struct {
	pms.PolicyStoreManager
}

func TestDiscoverNeedsADiscoverStore(t *testing.T) {
	router, _ := NewRouter(noDiscoverStore{storetest.NewFailingStore()})
	for _, c := range []call{
		{"GET", base + "discover-request", "", 400},
		{"GET", base + "discover-request/app", "", 400},
		{"DELETE", base + "discover-request/app", "", 400},
		{"DELETE", base + "discover-request", "", 400},
		{"GET", base + "discover-policy/app", "", 400},
	} {
		serve(t, router, c)
	}
}

func TestHandlersRejectMissingNames(t *testing.T) {
	failing := storetest.NewFailingStore()
	mgr, _ := NewRestService(failing)
	// Requests that bypass the router carry no service name or ID.
	for name, handler := range map[string]http.HandlerFunc{
		"DeleteService":         mgr.DeleteService,
		"GetService":            mgr.GetService,
		"CreatePolicy":          mgr.CreatePolicy,
		"DeletePolicies":        mgr.DeletePolicies,
		"DeletePolicy":          mgr.DeletePolicy,
		"GetPolicy":             mgr.GetPolicy,
		"UpdatePolicy":          mgr.UpdatePolicy,
		"ListPolicies":          mgr.ListPolicies,
		"CreateRolePolicy":      mgr.CreateRolePolicy,
		"DeleteRolePolicies":    mgr.DeleteRolePolicies,
		"DeleteRolePolicy":      mgr.DeleteRolePolicy,
		"GetRolePolicy":         mgr.GetRolePolicy,
		"UpdateRolePolicy":      mgr.UpdateRolePolicy,
		"ListRolePolicies":      mgr.ListRolePolicies,
		"DeleteFunction":        mgr.DeleteFunction,
		"GetFunction":           mgr.GetFunction,
		"GetDiscoverRequests":   mgr.GetDiscoverRequests,
		"ResetDiscoverRequests": mgr.ResetDiscoverRequests,
	} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest("GET", "/x", nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
}

func TestDecodeServiceRequest(t *testing.T) {
	got, err := decodeServiceRequest(httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"a","type":"k8s"}`)))
	if err != nil || got.Name != "a" || got.Type != "k8s" {
		t.Fatalf("decodeServiceRequest = %+v, %v", got, err)
	}
	if _, err := decodeServiceRequest(httptest.NewRequest("POST", "/", strings.NewReader(`{`))); err == nil {
		t.Fatal("a malformed body must fail")
	}
}
