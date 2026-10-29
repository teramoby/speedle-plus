//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package proxy

import (
	"net/http"

	"github.com/teramoby/speedle-plus/pkg/console/auth"
)

// Upstream names the service a route is forwarded to.
type Upstream int

const (
	// PMS is the policy management service.
	PMS Upstream = iota
	// ADS is the authorization decision service.
	ADS
)

// spec is one forwarded route. Path is relative to the console API and
// Target to the upstream API; both name path variables as {name}.
type spec struct {
	method   string
	path     string
	upstream Upstream
	target   string
	action   string
	query    []string
}

// specs is the allowlist. Anything not listed is not reachable through the
// console: deleting every service, bulk deletes, function writes and the
// discover API stay with spctl.
var specs = []spec{
	{http.MethodGet, "/pms/service", PMS, "service", auth.ActionRead, nil},
	{http.MethodPost, "/pms/service", PMS, "service", auth.ActionWrite, nil},
	{http.MethodGet, "/pms/service/{service}", PMS, "service/{service}", auth.ActionRead, nil},
	{http.MethodDelete, "/pms/service/{service}", PMS, "service/{service}", auth.ActionWrite, nil},
	{http.MethodGet, "/pms/policy-counts", PMS, "policy-counts", auth.ActionRead, nil},

	{http.MethodGet, "/pms/service/{service}/policy", PMS, "service/{service}/policy", auth.ActionRead, []string{"filter"}},
	{http.MethodPost, "/pms/service/{service}/policy", PMS, "service/{service}/policy", auth.ActionWrite, nil},
	{http.MethodGet, "/pms/service/{service}/policy/{id}", PMS, "service/{service}/policy/{id}", auth.ActionRead, nil},
	{http.MethodPut, "/pms/service/{service}/policy/{id}", PMS, "service/{service}/policy/{id}", auth.ActionWrite, nil},
	{http.MethodDelete, "/pms/service/{service}/policy/{id}", PMS, "service/{service}/policy/{id}", auth.ActionWrite, nil},

	{http.MethodGet, "/pms/service/{service}/role-policy", PMS, "service/{service}/role-policy", auth.ActionRead, []string{"filter"}},
	{http.MethodPost, "/pms/service/{service}/role-policy", PMS, "service/{service}/role-policy", auth.ActionWrite, nil},
	{http.MethodGet, "/pms/service/{service}/role-policy/{id}", PMS, "service/{service}/role-policy/{id}", auth.ActionRead, nil},
	{http.MethodPut, "/pms/service/{service}/role-policy/{id}", PMS, "service/{service}/role-policy/{id}", auth.ActionWrite, nil},
	{http.MethodDelete, "/pms/service/{service}/role-policy/{id}", PMS, "service/{service}/role-policy/{id}", auth.ActionWrite, nil},

	{http.MethodGet, "/pms/function", PMS, "function", auth.ActionRead, nil},
	{http.MethodGet, "/pms/function/{function}", PMS, "function/{function}", auth.ActionRead, nil},

	{http.MethodPost, "/ads/is-allowed", ADS, "is-allowed", auth.ActionRead, nil},
	{http.MethodPost, "/ads/all-granted-roles", ADS, "all-granted-roles", auth.ActionRead, nil},
	{http.MethodPost, "/ads/all-granted-permissions", ADS, "all-granted-permissions", auth.ActionRead, nil},
	{http.MethodPost, "/ads/diagnose", ADS, "diagnose", auth.ActionRead, nil},
}
