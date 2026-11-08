// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsrest

import (
	"net/http"

	"github.com/teramoby/speedle-plus/pkg/httputils"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgate"
)

// HealthResponse is the liveness body.
type HealthResponse struct {
	Status string `json:"status"`
}

// Healthz reports that the process is alive. It deliberately ignores the
// policy source state: a stopped instance must fail authorization closed, not
// be restarted by a liveness probe.
func (e *RESTService) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	httputils.SendOKResponse(w, &HealthResponse{Status: "ok"})
}

// Readyz reports whether the instance can currently answer an authorization
// request. A store without distribution support is always ready.
func (e *RESTService) Readyz(w http.ResponseWriter, _ *http.Request) {
	readiness := e.readiness()
	w.Header().Set("Cache-Control", "no-store")
	if readiness.Ready {
		httputils.SendOKResponse(w, readiness)
		return
	}
	httputils.SendResponse(w, http.StatusServiceUnavailable, readiness)
}

func (e *RESTService) readiness() adsgate.Readiness {
	if e.Gate == nil {
		return adsgate.Readiness{Ready: true}
	}
	return e.Gate.Ready()
}
