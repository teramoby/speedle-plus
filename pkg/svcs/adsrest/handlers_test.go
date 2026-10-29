//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package adsrest

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

const handlerPolicies = `{"services":[{"name":"app","type":"application",
 "policies":[
  {"id":"p1","name":"p1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"res1","actions":["read"]}]},
  {"id":"p2","name":"p2","effect":"grant","principals":[["role:admin"]],"permissions":[{"resource":"res2","actions":["write"]},{"resourceExpression":"doc.*","actions":["read"]}],"condition":"level > 1"}],
 "rolePolicies":[
  {"id":"rp1","name":"rp1","effect":"grant","roles":["admin"],"principals":["user:alice"],"condition":"level > 0"}]}]}`

func newHandlerRouter(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(handlerPolicies), 0600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	manager := storetest.PolicyStore(t, location)
	evaluator, err := eval.NewWithStore(&cfg.Config{}, manager)
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

func post(t *testing.T, router http.Handler, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("POST", svcs.PolicyAtzPath+path, strings.NewReader(body)))
	if rec.Code != status {
		t.Fatalf("POST %s %s: status %d, want %d (%s)", path, body, rec.Code, status, rec.Body.String())
	}
	return rec
}

const alice = `"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"app"`

func TestIsAllowedHandler(t *testing.T) {
	router := newHandlerRouter(t)
	cases := []struct {
		body    string
		allowed bool
	}{
		{`{` + alice + `,"resource":"res1","action":"read"}`, true},
		{`{` + alice + `,"resource":"res1","action":"write"}`, false},
		{`{` + alice + `,"resource":"res2","action":"write","attributes":[{"name":"level","type":"numeric","value":2}]}`, true},
		{`{` + alice + `,"resource":"doc1","action":"read","attributes":[{"name":"level","value":5}]}`, true},
		{`{` + alice + `,"resource":"res2","action":"write","attributes":[{"name":"level","type":"numeric","value":1}]}`, false},
		{`{"serviceName":"app","resource":"res1","action":"read"}`, false},
	}
	for _, c := range cases {
		rec := post(t, router, "is-allowed", c.body, 200)
		var got IsAllowedResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Allowed != c.allowed {
			t.Errorf("%s: allowed = %v, want %v", c.body, got.Allowed, c.allowed)
		}
	}
	rec := post(t, router, "is-allowed", `{`+alice+`,"resource":"res1","action":"read"}`, 200)
	if rec.Header().Get(svcs.PrincipalsHeader) != "alice" {
		t.Errorf("principals header = %q", rec.Header().Get(svcs.PrincipalsHeader))
	}

	// The condition cannot be evaluated: the error is reported, not hidden.
	rec = post(t, router, "is-allowed", `{`+alice+`,"resource":"res2","action":"write","attributes":[{"name":"level","value":"x"}]}`, 200)
	var got IsAllowedResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Allowed {
		t.Errorf("a failed condition allowed the request: %+v", got)
	}
}

func TestHandlersRejectBadRequests(t *testing.T) {
	router := newHandlerRouter(t)
	bad := []string{
		`{`,
		`{"attributes":[{"name":"a","type":"numeric","value":"1"}]}`,
		`{"attributes":[{"name":"a","type":"unknown","value":"1"}]}`,
		`{"attributes":[{"name":"a","value":null}]}`,
		`{"attributes":[{"name":"a","value":[1,null]}]}`,
		`{"attributes":[{"name":"a","value":[1,"x"]}]}`,
		`{"attributes":[{"name":"a","type":"datetime","value":"yesterday"}]}`,
	}
	for _, path := range []string{"is-allowed", "all-granted-roles", "all-granted-permissions", "diagnose", "discover"} {
		for _, body := range bad {
			post(t, router, path, body, 400)
		}
	}
}

func TestGrantedRolesAndPermissionsHandlers(t *testing.T) {
	router := newHandlerRouter(t)
	rec := post(t, router, "all-granted-roles", `{`+alice+`,"attributes":[{"name":"level","value":1}]}`, 200)
	var roles []string
	json.Unmarshal(rec.Body.Bytes(), &roles)
	if !reflect.DeepEqual(roles, []string{"admin"}) {
		t.Errorf("roles = %v", roles)
	}
	rec = post(t, router, "all-granted-roles", `{`+alice+`}`, 200)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("roles without the condition attribute = %s", rec.Body.String())
	}

	rec = post(t, router, "all-granted-permissions", `{`+alice+`,"attributes":[{"name":"level","value":2}]}`, 200)
	var perms []PermissionResponse
	json.Unmarshal(rec.Body.Bytes(), &perms)
	var got []string
	for _, p := range perms {
		for _, a := range p.Actions {
			got = append(got, p.Resource+":"+a)
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"res1:read", "res2:write"}) {
		t.Errorf("permissions = %v (%s)", got, rec.Body.String())
	}
	rec = post(t, router, "all-granted-permissions", `{"subject":{"principals":[{"type":"user","name":"nobody"}]},"serviceName":"app"}`, 200)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("permissions of nobody = %s", rec.Body.String())
	}
}

func TestDiagnoseHandler(t *testing.T) {
	router := newHandlerRouter(t)
	rec := post(t, router, "diagnose", `{`+alice+`,"resource":"res2","action":"write","attributes":[{"name":"level","value":3}]}`, 200)
	var got EvaluationDebugResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Allowed || got.RequestContext.Resource != "res2" || !reflect.DeepEqual(got.GrantedRoles, []string{"admin"}) {
		t.Fatalf("diagnose = %+v", got)
	}
	if len(got.RolePolicies) != 1 || got.RolePolicies[0].Condition.ConditionExpression != "level > 0" {
		t.Fatalf("role policies = %+v", got.RolePolicies)
	}
	var p2 *PolicyResponse
	for i := range got.Policies {
		if got.Policies[i].ID == "p2" {
			p2 = &got.Policies[i]
		}
	}
	if p2 == nil || p2.Condition.EvaluationResult != "true" || p2.Permissions[1].ResourceExpression != "doc.*" {
		t.Fatalf("policy p2 = %+v", p2)
	}
}

func TestDiscoverHandler(t *testing.T) {
	router := newHandlerRouter(t)
	if os.Getenv("STORE_TYPE") == "mysql" {
		rec := post(t, router, "discover", `{"serviceName":"app","resource":"r","action":"read"}`, 200)
		if !strings.Contains(rec.Body.String(), "unsupported store type") {
			t.Fatal(rec.Body.String())
		}
		return
	}
	rec := post(t, router, "discover", `{"subject":{"principals":[{"type":"user","name":"carol"}]},"serviceName":"app","resource":"x","action":"y"}`, 200)
	var got IsAllowedResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.Allowed {
		t.Fatalf("discover = %+v", got)
	}
}

// failingEvaluator fails every evaluation.
type failingEvaluator struct {
	eval.InternalEvaluator
}

var errEvaluation = errors.New("evaluation failed")

func (failingEvaluator) AssertToken(*adsapi.RequestContext) error { return nil }
func (failingEvaluator) IsAllowed(adsapi.RequestContext) (bool, adsapi.Reason, error) {
	return false, adsapi.ERROR_IN_EVALUATION, errEvaluation
}
func (failingEvaluator) GetAllGrantedRoles(adsapi.RequestContext) ([]string, error) {
	return nil, errEvaluation
}
func (failingEvaluator) GetAllGrantedPermissions(adsapi.RequestContext) ([]pms.Permission, error) {
	return nil, errEvaluation
}
func (failingEvaluator) Discover(adsapi.RequestContext) (bool, adsapi.Reason, error) {
	return false, adsapi.ERROR_IN_EVALUATION, errEvaluation
}
func (failingEvaluator) Diagnose(adsapi.RequestContext) (*adsapi.EvaluationResult, error) {
	return nil, errEvaluation
}

func TestHandlersReportEvaluationErrors(t *testing.T) {
	router, _ := NewRouter(failingEvaluator{})
	body := `{` + alice + `}`
	for _, path := range []string{"is-allowed", "discover"} {
		rec := post(t, router, path, body, 200)
		var got IsAllowedResponse
		json.Unmarshal(rec.Body.Bytes(), &got)
		if got.Allowed || got.ErrorMessage != errEvaluation.Error() {
			t.Errorf("%s: response %+v", path, got)
		}
	}
	for _, path := range []string{"all-granted-roles", "all-granted-permissions", "diagnose"} {
		post(t, router, path, body, 500)
	}
}

func TestConvertValues(t *testing.T) {
	got, err := ConvValue("datetime", "2018-01-02T03:04:05Z")
	if err != nil || got != float64(1514862245) {
		t.Errorf("datetime = %v, %v", got, err)
	}
	got, err = ConvValue("string", []interface{}{"a", "b"})
	if err != nil || !reflect.DeepEqual(got, []interface{}{"a", "b"}) {
		t.Errorf("string array = %v, %v", got, err)
	}
	got, err = ConvValue("", true)
	if err != nil || got != true {
		t.Errorf("untyped bool = %v, %v", got, err)
	}
	if _, err := ConvValue("", []interface{}{nil}); err == nil {
		t.Error("a null array item must be rejected")
	}
	for _, layout := range []string{"Mon Jan 02 15:04:05 -0700 2006", "Mon Jan  2 15:04:05 MST 2006"} {
		if _, err := ParseDateTime(layout); err != nil {
			t.Errorf("ParseDateTime(%q): %v", layout, err)
		}
	}
	if DuplicateAttributeMap(nil) != nil {
		t.Error("nil attributes must stay nil")
	}
	attrs := map[string]interface{}{"a": 1}
	dup := DuplicateAttributeMap(attrs)
	dup["a"] = 2
	if attrs["a"] != 1 {
		t.Error("DuplicateAttributeMap must copy")
	}
	if DumpPrincipals(nil) != nil {
		t.Error("nil principals must stay nil")
	}
	ConvertAPIPolicy2PolicyResponse(nil, nil)
	ConvertAPIRolePolicy2RolePolicyResponse(nil, nil)
}
