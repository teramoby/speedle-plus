//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pmsrest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/httputils"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsimpl"
)

// etagOf returns a strong ETag for the JSON form of v. Clients send it back
// in If-Match to detect that an item changed since they read it.
func etagOf(v interface{}) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// preconditionFailed reports whether the request's If-Match names a version
// other than current. It is a best-effort check: the store does not compare
// and swap.
func preconditionFailed(w http.ResponseWriter, r *http.Request, current interface{}) bool {
	ifMatch := r.Header.Get("If-Match")
	if ifMatch == "" || ifMatch == "*" || ifMatch == etagOf(current) {
		return false
	}
	httputils.SendResponse(w, http.StatusPreconditionFailed, &httputils.ErrorResponse{
		Error: "The item was changed since it was read.",
	})
	return true
}

// getUpdateMetaData keeps the creation fields of an item and records who
// updated it and when.
func getUpdateMetaData(r *http.Request, existing map[string]string) map[string]string {
	metaData := map[string]string{}
	for _, key := range []string{"createby", "createtime"} {
		if value, ok := existing[key]; ok {
			metaData[key] = value
		}
	}
	if updater := r.Header.Get(svcs.PrincipalsHeader); updater != "" {
		metaData["updateby"] = updater
	}
	metaData["updatetime"] = time.Unix(time.Now().Unix(), 0).Format(time.RFC3339)
	return metaData
}

// UpdatePolicy replaces a policy, keeping its ID.
func (mgr *RESTService) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	serviceName, policyID := ParseRequestURI(r)
	if len(serviceName) == 0 || len(policyID) == 0 {
		httputils.SendBadRequestResponse(w, &httputils.ErrorResponse{Error: "Invalid service name or policy ID."})
		return
	}
	var policy pms.Policy
	if err := decodeRequestBody(r, &policy); err != nil {
		httputils.HandleError(w, err)
		return
	}
	if policy.ID != "" && policy.ID != policyID {
		httputils.SendBadRequestResponse(w, &httputils.ErrorResponse{Error: "The policy ID in the body differs from the path."})
		return
	}
	policy.ID = policyID
	ctxFields := map[string]interface{}{"serviceName": serviceName, "policy": &policy}

	if err := pmsimpl.CheckPolicyUpdate(serviceName, &policy); err != nil {
		httputils.HandleError(w, err)
		logging.WriteFailedAuditLog("UpdatePolicy", ctxFields, err.Error())
		return
	}
	existing, err := mgr.PolicyStore.GetPolicy(serviceName, policyID)
	if err != nil {
		httputils.HandleError(w, err)
		logging.WriteFailedAuditLog("UpdatePolicy", ctxFields, err.Error())
		return
	}
	if preconditionFailed(w, r, existing) {
		logging.WriteFailedAuditLog("UpdatePolicy", ctxFields, "precondition failed")
		return
	}
	policy.Metadata = getUpdateMetaData(r, existing.Metadata)
	updated, err := mgr.PolicyStore.UpdatePolicy(serviceName, &policy)
	if err != nil {
		httputils.HandleError(w, err)
		logging.WriteFailedAuditLog("UpdatePolicy", ctxFields, err.Error())
		return
	}

	logging.WriteSucceededAuditLog("UpdatePolicy", ctxFields, nil)
	w.Header().Set("ETag", etagOf(updated))
	httputils.SendOKResponse(w, updated)
}

// UpdateRolePolicy replaces a role policy, keeping its ID.
func (mgr *RESTService) UpdateRolePolicy(w http.ResponseWriter, r *http.Request) {
	serviceName, rolePolicyID := ParseRequestURI(r)
	if len(serviceName) == 0 || len(rolePolicyID) == 0 {
		httputils.SendBadRequestResponse(w, &httputils.ErrorResponse{Error: "Invalid service name or role policy ID."})
		return
	}
	var rolePolicy pms.RolePolicy
	if err := decodeRequestBody(r, &rolePolicy); err != nil {
		httputils.HandleError(w, err)
		return
	}
	if rolePolicy.ID != "" && rolePolicy.ID != rolePolicyID {
		httputils.SendBadRequestResponse(w, &httputils.ErrorResponse{Error: "The role policy ID in the body differs from the path."})
		return
	}
	rolePolicy.ID = rolePolicyID
	ctxFields := map[string]interface{}{"serviceName": serviceName, "rolePolicy": &rolePolicy}

	if err := pmsimpl.CheckRolePolicyUpdate(&rolePolicy); err != nil {
		httputils.HandleError(w, err)
		logging.WriteFailedAuditLog("UpdateRolePolicy", ctxFields, err.Error())
		return
	}
	existing, err := mgr.PolicyStore.GetRolePolicy(serviceName, rolePolicyID)
	if err != nil {
		httputils.HandleError(w, err)
		logging.WriteFailedAuditLog("UpdateRolePolicy", ctxFields, err.Error())
		return
	}
	if preconditionFailed(w, r, existing) {
		logging.WriteFailedAuditLog("UpdateRolePolicy", ctxFields, "precondition failed")
		return
	}
	rolePolicy.Metadata = getUpdateMetaData(r, existing.Metadata)
	updated, err := mgr.PolicyStore.UpdateRolePolicy(serviceName, &rolePolicy)
	if err != nil {
		httputils.HandleError(w, err)
		logging.WriteFailedAuditLog("UpdateRolePolicy", ctxFields, err.Error())
		return
	}

	logging.WriteSucceededAuditLog("UpdateRolePolicy", ctxFields, nil)
	w.Header().Set("ETag", etagOf(updated))
	httputils.SendOKResponse(w, updated)
}
