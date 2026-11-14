// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsrest

import (
	"net/http"

	"github.com/teramoby/speedle-plus/pkg/httputils"
)

// HealthResponse is the liveness body.
type HealthResponse struct {
	Status string `json:"status"`
}

// Healthz reports that the HTTP process is alive.
func (e *RESTService) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	httputils.SendOKResponse(w, &HealthResponse{Status: "ok"})
}

// Readyz reports readiness of the local decision service.
func (e *RESTService) Readyz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// Keep the response shape used by existing local-store deployments.
	httputils.SendOKResponse(w, &struct {
		Ready          bool `json:"ready"`
		ServicesTotal  int  `json:"servicesTotal"`
		ServicesUsable int  `json:"servicesUsable"`
	}{Ready: true})
}
