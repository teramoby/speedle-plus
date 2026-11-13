//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package adsrest

import (
	"net/http"

	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs"

	"github.com/gorilla/mux"
)

type route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type routes []route

func initRouters(evaluator eval.InternalEvaluator) (*routes, error) {
	restService, err := NewRESTServiceWithEvaluator(evaluator)
	if err != nil {
		return nil, err
	}
	return initRoutersForService(restService), nil
}

// initRoutersForService builds the route table around one service. Tests use
// it to supply a gate without a real policy source.
func initRoutersForService(restService *RESTService) *routes {
	return &routes{
		route{
			"Healthz",
			"GET",
			"/healthz",
			restService.Healthz,
		},

		route{
			"Readyz",
			"GET",
			"/readyz",
			restService.Readyz,
		},

		route{
			"GetAllGrantedPermissions",
			"POST",
			svcs.PolicyAtzPath + "all-granted-permissions",
			restService.GetAllGrantedPermissions,
		},

		route{
			"GetAllGrantedRoles",
			"POST",
			svcs.PolicyAtzPath + "all-granted-roles",
			restService.GetAllGrantedRoles,
		},

		route{
			"IsAllowed",
			"POST",
			svcs.PolicyAtzPath + "is-allowed",
			restService.IsAllowed,
		},

		route{
			"Diagnose",
			"POST",
			svcs.PolicyAtzPath + "diagnose",
			restService.Diagnose,
		},

		route{
			"Discover",
			"POST",
			svcs.PolicyAtzPath + "discover",
			restService.Discover,
		},
	}
}

func NewRouter(evaluator eval.InternalEvaluator) (*mux.Router, error) {
	routes, err := initRouters(evaluator)
	if err != nil {
		return nil, err
	}
	router := mux.NewRouter().StrictSlash(true)
	for _, route := range *routes {
		var handler http.Handler = route.HandlerFunc

		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
	}
	// The downstream policy-source protocol is served only by the dedicated
	// downstream mTLS listener, never by the application listener.
	return router, nil
}
