//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package adsrest

import (
	"net/http"

	"github.com/teramoby/speedle-plus/pkg/httputils"
	"github.com/teramoby/speedle-plus/pkg/logging"
)

func (e *RESTService) Discover(w http.ResponseWriter, r *http.Request) {
	if !httputils.VerifyContentType(w, r, []string{"application/json"}) {
		return
	}
	jsonRequest, err := DecodeJSONContext(r)
	if err != nil {
		httputils.HandleError(w, err)
		return
	}

	context, err := ConvertJSONRequestToContext(jsonRequest)
	if err != nil {
		httputils.HandleError(w, err)
		return
	}

	// assert token
	if assertErr := e.Evaluator.AssertToken(context); assertErr != nil {
		httputils.HandleError(w, assertErr)
		logging.WriteSimpleFailedAuditLog("Discover", context, assertErr.Error())
		return
	}

	result, reason, err := e.Evaluator.Discover(*context)
	if err != nil {
		httputils.HandleError(w, err)
		logging.WriteSimpleFailedAuditLog("Discovery", context, err.Error())
		return
	}
	response := IsAllowedResponse{
		Allowed: result,
		Reason:  int32(reason),
	}

	// Audit log
	logging.WriteSimpleSucceededAuditLog("Discovery", context, nil)

	httputils.SendOKResponse(w, &response)
}
