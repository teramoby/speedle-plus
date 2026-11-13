// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsrest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/httputils"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgate"
)

// maxAuthzRequestBodyBytes bounds the buffered authorization request. The body
// is kept so a request for a syncing service can be forwarded unchanged.
const maxAuthzRequestBodyBytes = 1 << 20

// UnavailableResponse is the fail-closed answer of a distributed instance. It
// is never a decision: callers must treat any non-2xx response as deny.
type UnavailableResponse struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

// route returns the routing decision for one service. A service without a
// distribution gate keeps the existing local behaviour.
func (e *RESTService) route(serviceName string) adsgate.Route {
	if e.Gate == nil {
		return adsgate.Route{Kind: adsgate.RouteLocal}
	}
	return e.Gate.Route(serviceName)
}

// writeUnavailable writes the 503 that replaces a decision while the instance
// is stopped or a syncing service has no usable fallback. The body always
// carries a reason code from the distribution protocol.
func writeUnavailable(w http.ResponseWriter, reason adsync.Reason) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", "30")
	httputils.SendResponse(w, http.StatusServiceUnavailable, &UnavailableResponse{
		Error:  "authorization_unavailable",
		Reason: string(reason),
	})
}

// writeGateFailure writes the fail-closed answer for a failed forwarding call.
func writeGateFailure(w http.ResponseWriter, err error) {
	writeUnavailable(w, adsgate.UnavailableReason(err))
}

// forward sends the original request body to the configured Cloud fallback with
// the Cloud credential and writes Cloud's response. The body and status are
// unchanged; the only addition is the decision-source marker. Both the
// forwarded decision and a forwarding failure are written to the audit trail,
// like the local path of the same operation.
func (e *RESTService) forward(w http.ResponseWriter, ctx context.Context, operation adsync.FallbackOperation, serviceName string, body []byte) {
	auditFields := adsgate.FallbackAuditFields(operation, serviceName)
	response, err := e.Gate.Forward(ctx, operation, body)
	if err != nil {
		logging.WriteFailedAuditLog(adsgate.FallbackAuditOperation(operation), auditFields, err.Error())
		writeGateFailure(w, err)
		return
	}
	logging.WriteSucceededAuditLog(adsgate.FallbackAuditOperation(operation), auditFields,
		fallbackAuditResult(operation, response))
	copyFallbackResponse(w, response)
}

// fallbackAuditResult records what Cloud decided. The response body is parsed
// for the audit record only; the bytes sent to the caller are unchanged.
func fallbackAuditResult(operation adsync.FallbackOperation, response *adsync.FallbackResponse) map[string]interface{} {
	switch operation {
	case adsync.FallbackAllGrantedRoles:
		var roles []string
		if err := adsync.ReadFallbackResponse(response.Body, &roles); err != nil {
			return nil
		}
		return map[string]interface{}{"roles": roles}
	case adsync.FallbackAllGrantedPermissions:
		var permissions []PermissionResponse
		if err := adsync.ReadFallbackResponse(response.Body, &permissions); err != nil {
			return nil
		}
		return map[string]interface{}{"permissions": permissions}
	default:
		var decision struct {
			Allowed      bool   `json:"allowed"`
			Reason       int32  `json:"reason"`
			ErrorMessage string `json:"errorMessage"`
		}
		if err := adsync.ReadFallbackResponse(response.Body, &decision); err != nil {
			return nil
		}
		result := map[string]interface{}{"allowed": decision.Allowed, "reason": decision.Reason}
		if decision.ErrorMessage != "" {
			result["errorMessage"] = decision.ErrorMessage
		}
		return result
	}
}

// copyFallbackResponse copies a Cloud authorization response. Hop-by-hop
// headers and cookies are not passed on: the Cloud call is a server-to-server
// request made with the instance credential, so its cookies belong to nobody
// on the caller's side.
func copyFallbackResponse(w http.ResponseWriter, response *adsync.FallbackResponse) {
	header := w.Header()
	for name, values := range response.Header {
		if hopByHopHeader(name) || strings.EqualFold(name, "Set-Cookie") ||
			strings.EqualFold(name, svcs.DecisionSourceHeader) {
			continue
		}
		for _, value := range values {
			header.Add(name, value)
		}
	}
	header.Set(svcs.DecisionSourceHeader, svcs.DecisionSourceUpstream)
	if header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", "no-store")
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(response.Body)
}

func hopByHopHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

// decodeJSONContextBody reads and decodes the request body and also returns the
// exact bytes, so a request for a syncing service can be forwarded unchanged.
func decodeJSONContextBody(r *http.Request) (*JsonContext, []byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxAuthzRequestBodyBytes+1))
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.InvalidRequest, "unable to read request")
	}
	if len(body) > maxAuthzRequestBodyBytes {
		return nil, nil, errors.New(errors.InvalidRequest, "request body is too large")
	}
	request, err := decodeJSONContext(bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	return request, body, nil
}
