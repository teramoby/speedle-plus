//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/console/auth"
	"github.com/teramoby/speedle-plus/pkg/console/proxy"
	"github.com/teramoby/speedle-plus/pkg/console/rolegraph"
	"github.com/teramoby/speedle-plus/pkg/svcs"
)

// upstreamError is a PMS answer the console passes on with its status.
type upstreamError struct {
	status  int
	message string
}

func (e *upstreamError) Error() string { return e.message }

// fetchService reads a service with its policies from PMS. A missing
// service is nil when optional is set.
func (c *Components) fetchService(ctx context.Context, name string, optional bool) (*pms.Service, error) {
	target := strings.TrimSuffix(c.Proxy.PMSURL, "/") + svcs.PolicyMgmtPath + "service/" + url.PathEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, &upstreamError{http.StatusBadRequest, "The service name is not valid."}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Proxy.Client.Do(req)
	if err != nil {
		log.Errorf("Console upstream GET %s failed: %v", target, err)
		return nil, &upstreamError{http.StatusBadGateway, "The policy service is unreachable."}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		if optional {
			return nil, nil
		}
		return nil, &upstreamError{http.StatusNotFound, fmt.Sprintf("Service %q is not found.", name)}
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &upstreamError{http.StatusBadGateway, fmt.Sprintf("The policy service answered %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))}
	}
	var service pms.Service
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&service); err != nil {
		return nil, &upstreamError{http.StatusBadGateway, "The policy service sent a service that could not be read."}
	}
	return &service, nil
}

// pathVar returns a checked path variable of the request.
func pathVar(r *http.Request, name string) (string, bool) {
	value, err := url.PathUnescape(mux.Vars(r)[name])
	return value, err == nil && proxy.ValidName(value)
}

// buildGraph loads the service in the request, and the global service
// unless includeGlobal=false, and analyses their role policies.
func (c *Components) buildGraph(w http.ResponseWriter, r *http.Request) *rolegraph.Graph {
	name, ok := pathVar(r, "service")
	if !ok {
		auth.SendError(w, http.StatusBadRequest, "The service name is not valid.")
		return nil
	}
	service, err := c.fetchService(r.Context(), name, false)
	var global *pms.Service
	if err == nil && r.URL.Query().Get("includeGlobal") != "false" && name != pms.GlobalService {
		global, err = c.fetchService(r.Context(), pms.GlobalService, true)
	}
	if err != nil {
		e := err.(*upstreamError)
		auth.SendError(w, e.status, e.message)
		return nil
	}
	return rolegraph.Build(service, global)
}

// RoleGraph answers GET /services/{service}/role-graph.
func (c *Components) RoleGraph(w http.ResponseWriter, r *http.Request) {
	if g := c.buildGraph(w, r); g != nil {
		auth.SendJSON(w, http.StatusOK, g)
	}
}

// RoleHolders answers GET /services/{service}/roles/{role}/holders.
func (c *Components) RoleHolders(w http.ResponseWriter, r *http.Request) {
	role, ok := pathVar(r, "role")
	if !ok {
		auth.SendError(w, http.StatusBadRequest, "The role name is not valid.")
		return
	}
	if g := c.buildGraph(w, r); g != nil {
		auth.SendJSON(w, http.StatusOK, g.Holders(role))
	}
}
