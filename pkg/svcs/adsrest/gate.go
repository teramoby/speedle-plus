// Licensed under the Universal Permissive License (UPL) Version 1.0.
package adsrest

import (
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/httputils"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs/gate"
	"net/http"
)

func (e *RESTService) dispatch(w http.ResponseWriter, r *http.Request, op gate.Operation, request *ads.RequestContext) bool {
	response, handled, err := gate.Dispatch(r.Context(), e.Gate, op, request)
	if !handled {
		return false
	}
	fields := map[string]interface{}{"serviceName": request.ServiceName, "decisionSource": "upstream"}
	if err != nil {
		logging.WriteFailedAuditLog(string(op), fields, gate.Reason(err))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Retry-After", "30")
		httputils.SendResponse(w, http.StatusServiceUnavailable, &struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
		}{"authorization_unavailable", gate.Reason(err)})
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Speedle-Decision-Source", "upstream")
	if response.ErrorMessage != "" {
		logging.WriteFailedAuditLog(string(op), fields, response.ErrorMessage)
	} else {
		logging.WriteSucceededAuditLog(string(op), fields, map[string]interface{}{"allowed": response.Allowed, "reason": response.Reason, "roles": response.Roles, "permissions": response.Permissions})
	}
	if response.ErrorMessage != "" && op != gate.IsAllowed {
		httputils.SendResponse(w, http.StatusInternalServerError, &struct {
			Error string `json:"error"`
		}{response.ErrorMessage})
		return true
	}
	if response.ErrorMessage == "" {
		for key, values := range gate.FilterMetadata(response.Metadata) {
			// Middleware may already own additional headers on this response.
			if _, exists := w.Header()[http.CanonicalHeaderKey(key)]; !exists {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
		}
	}
	switch op {
	case gate.AllGrantedRoles:
		if len(response.Roles) == 0 {
			httputils.SendEmptyListResponse(w)
		} else {
			httputils.SendOKResponse(w, response.Roles)
		}
	case gate.AllGrantedPermissions:
		permissions := make([]PermissionResponse, 0, len(response.Permissions))
		for _, permission := range response.Permissions {
			permissions = append(permissions, PermissionResponse{Resource: permission.Resource, Actions: permission.Actions})
		}
		httputils.SendOKResponse(w, permissions)
	default:
		httputils.SendOKResponse(w, &IsAllowedResponse{Allowed: response.Allowed, Reason: int32(response.Reason), ErrorMessage: response.ErrorMessage})
	}
	return true
}
