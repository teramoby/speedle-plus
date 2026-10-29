//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pmsrest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/svcs"
)

// put sends a PUT with the given principal and If-Match, and checks the status.
func put(t *testing.T, router http.Handler, path, body, principal, ifMatch string, status int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	if principal != "" {
		req.Header.Set(svcs.PrincipalsHeader, principal)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Errorf("PUT %s: status %d, want %d (%s)", path, rec.Code, status, rec.Body.String())
	}
	return rec
}

func createService(t *testing.T, router *mux.Router) {
	t.Helper()
	serve(t, router, call{"POST", base + "service", `{"name":"app"}`, 201})
}

func TestUpdatePolicyHandler(t *testing.T) {
	router, _ := newFileRouter(t)
	createService(t, router)
	req := httptest.NewRequest("POST", base+"service/app/policy", strings.NewReader(
		`{"name":"p","effect":"grant","principals":[["user:a"]],"permissions":[{"resource":"r","actions":["get"]}]}`))
	req.Header.Set(svcs.PrincipalsHeader, "user:creator")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var created pms.Policy
	json.Unmarshal(rec.Body.Bytes(), &created)
	path := base + "service/app/policy/" + created.ID

	etag := serve(t, router, call{"GET", path, "", 200}).Header().Get("ETag")
	if etag == "" {
		t.Fatal("GET returned no ETag")
	}

	body := `{"name":"p2","effect":"deny","principals":[["user:b"]],"permissions":[{"resource":"r2","actions":["put"]}]}`
	rec = put(t, router, path, body, "user:editor", etag, 200)
	var updated pms.Policy
	json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.ID != created.ID || updated.Name != "p2" || updated.Effect != pms.Deny {
		t.Errorf("updated %+v", updated)
	}
	if updated.Metadata["createby"] != "user:creator" || updated.Metadata["createtime"] != created.Metadata["createtime"] ||
		updated.Metadata["updateby"] != "user:editor" || updated.Metadata["updatetime"] == "" {
		t.Errorf("metadata %v", updated.Metadata)
	}
	newTag := rec.Header().Get("ETag")
	if newTag == "" || newTag == etag {
		t.Errorf("ETag after the update = %q, before %q", newTag, etag)
	}
	if got := serve(t, router, call{"GET", path, "", 200}).Header().Get("ETag"); got != newTag {
		t.Errorf("GET ETag %q, PUT ETag %q", got, newTag)
	}

	// The old ETag no longer matches; "*" and no If-Match always do.
	put(t, router, path, body, "", etag, http.StatusPreconditionFailed)
	put(t, router, path, body, "", "*", 200)
	rec = put(t, router, path, `{"id":"`+created.ID+`","name":"p3","effect":"grant"}`, "", "", 200)
	var anonymous pms.Policy
	json.Unmarshal(rec.Body.Bytes(), &anonymous)
	if _, ok := anonymous.Metadata["updateby"]; ok {
		t.Errorf("updateby without a principal: %v", anonymous.Metadata)
	}

	put(t, router, path, `{`, "", "", 400)
	put(t, router, path, `{"id":"other","name":"p","effect":"grant"}`, "", "", 400)
	put(t, router, path, `{"name":"p"}`, "", "", 400)
	put(t, router, base+"service/app/policy/nope", body, "", "", 404)
	put(t, router, base+"service/nope/policy/"+created.ID, body, "", "", 404)
	put(t, router, base+"service/"+pms.GlobalService+"/policy/x", body, "", "", 400)
}

func TestUpdateRolePolicyHandler(t *testing.T) {
	router, _ := newFileRouter(t)
	createService(t, router)
	rec := serve(t, router, call{"POST", base + "service/app/role-policy", `{"name":"rp","effect":"grant","roles":["r"],"principals":["user:a"]}`, 201})
	var created pms.RolePolicy
	json.Unmarshal(rec.Body.Bytes(), &created)
	path := base + "service/app/role-policy/" + created.ID

	etag := serve(t, router, call{"GET", path, "", 200}).Header().Get("ETag")
	if etag == "" {
		t.Fatal("GET returned no ETag")
	}
	body := `{"name":"rp2","effect":"deny","roles":["auditor"],"principals":["group:ops"]}`
	rec = put(t, router, path, body, "user:editor", etag, 200)
	var updated pms.RolePolicy
	json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.ID != created.ID || updated.Roles[0] != "auditor" || updated.Metadata["updateby"] != "user:editor" {
		t.Errorf("updated %+v", updated)
	}
	if rec.Header().Get("ETag") == etag {
		t.Error("the ETag did not change")
	}

	put(t, router, path, body, "", etag, http.StatusPreconditionFailed)
	put(t, router, path, `{`, "", "", 400)
	put(t, router, path, `{"id":"other","name":"rp","effect":"grant"}`, "", "", 400)
	put(t, router, path, `{"name":"rp"}`, "", "", 400)
	put(t, router, base+"service/app/role-policy/nope", body, "", "", 404)
}

// updateFailingStore finds items but fails to update them.
type updateFailingStore struct {
	pms.PolicyStoreManager
}

func (updateFailingStore) UpdatePolicy(string, *pms.Policy) (*pms.Policy, error) {
	return nil, errors.New(errors.StoreError, "update failed")
}

func (updateFailingStore) UpdateRolePolicy(string, *pms.RolePolicy) (*pms.RolePolicy, error) {
	return nil, errors.New(errors.StoreError, "update failed")
}

func TestUpdateReportsStoreErrors(t *testing.T) {
	_, ps := newFileRouter(t)
	if err := ps.CreateService(&pms.Service{Name: "app"}); err != nil {
		t.Fatal(err)
	}
	policy, err := ps.CreatePolicy("app", &pms.Policy{Name: "p", Effect: pms.Grant})
	if err != nil {
		t.Fatal(err)
	}
	rolePolicy, err := ps.CreateRolePolicy("app", &pms.RolePolicy{Name: "rp", Effect: pms.Grant})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(updateFailingStore{ps})
	if err != nil {
		t.Fatal(err)
	}
	put(t, router, base+"service/app/policy/"+policy.ID, `{"name":"p","effect":"deny"}`, "", "", 500)
	put(t, router, base+"service/app/role-policy/"+rolePolicy.ID, `{"name":"rp","effect":"deny"}`, "", "", 500)
}
