//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package console

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/console/rolegraph"
)

func TestRoleGraphAndHolders(t *testing.T) {
	c := backends(t)
	c.BootstrapAdmins = []string{"user:testUser"}
	b := open(t, c)
	b.login("abc")
	b.do("POST", "/console/api/pms/service", `{"name":"app"}`, nil, http.StatusCreated)
	b.do("POST", "/console/api/pms/service", `{"name":"global"}`, nil, http.StatusCreated)
	b.do("POST", "/console/api/pms/service/app/role-policy", `{"name":"m","effect":"grant","roles":["manager"],"principals":["user:alice"]}`, nil, http.StatusCreated)
	b.do("POST", "/console/api/pms/service/app/role-policy", `{"name":"r","effect":"grant","roles":["reader"],"principals":["role:manager"]}`, nil, http.StatusCreated)
	b.do("POST", "/console/api/pms/service/global/role-policy", `{"name":"g","effect":"grant","roles":["reader"],"principals":["group:all"]}`, nil, http.StatusCreated)
	b.do("POST", "/console/api/pms/service/app/policy", `{"name":"p","effect":"grant","principals":[["role:reader"]],"permissions":[{"resource":"r","actions":["get"]}]}`, nil, http.StatusCreated)

	var g rolegraph.Graph
	decode(t, b.do("GET", "/console/api/services/app/role-graph", "", nil, http.StatusOK), &g)
	if g.Service != "app" || len(g.Edges) != 3 || len(g.References["reader"]) != 1 {
		t.Errorf("graph %+v", g)
	}
	decode(t, b.do("GET", "/console/api/services/app/role-graph?includeGlobal=false", "", nil, http.StatusOK), &g)
	if len(g.Edges) != 2 {
		t.Errorf("%d edges without global", len(g.Edges))
	}
	var h rolegraph.Holders
	decode(t, b.do("GET", "/console/api/services/app/roles/reader/holders", "", nil, http.StatusOK), &h)
	if len(h.Holders) != 2 || h.Holders[1].Principal != "user:alice" || len(h.Holders[1].Path) != 3 {
		t.Errorf("holders %+v", h.Holders)
	}
	decode(t, b.do("GET", "/console/api/services/global/role-graph", "", nil, http.StatusOK), &g)
	if len(g.Edges) != 1 {
		t.Errorf("global graph has %d edges", len(g.Edges))
	}

	b.do("GET", "/console/api/services/nope/role-graph", "", nil, http.StatusNotFound)
	b.do("GET", "/console/api/services/a%2Fb/role-graph", "", nil, http.StatusBadRequest)
	b.do("GET", "/console/api/services/app/roles/a%2Fb/holders", "", nil, http.StatusBadRequest)
	b.do("POST", "/console/api/logout", "", nil, http.StatusNoContent)
	b.do("GET", "/console/api/services/app/role-graph", "", nil, http.StatusUnauthorized)

	// Without the global service the graph still builds.
	b.login("abc")
	b.do("DELETE", "/console/api/pms/service/global", "", nil, http.StatusNoContent)
	decode(t, b.do("GET", "/console/api/services/app/role-graph", "", nil, http.StatusOK), &g)
	if len(g.Edges) != 2 {
		t.Errorf("%d edges after deleting global", len(g.Edges))
	}
}

func TestRoleGraphUpstreamFailures(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"error status": func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) },
		"not JSON":     func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{")) },
	} {
		pms := httptest.NewServer(handler)
		c := backends(t)
		c.DevNoAuth, c.PMSURL = true, pms.URL
		b := open(t, c)
		b.do("GET", "/console/api/services/app/role-graph", "", nil, http.StatusBadGateway)
		pms.Close()
		b.do("GET", "/console/api/services/app/role-graph", "", nil, http.StatusBadGateway)
		t.Log(name)
	}
}
