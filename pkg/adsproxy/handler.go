// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"fmt"
	"net/http"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/svcs"
)

// AuthzHandlers are the three authorization operations a relay serves to its
// direct children, unchanged in body and response.
type AuthzHandlers struct {
	IsAllowed             http.Handler
	AllGrantedRoles       http.Handler
	AllGrantedPermissions http.Handler
}

// DownstreamHandler composes the complete route table of the downstream
// listener: the policy-source protocol and the three POST authorization
// operations. No policy management, health or diagnostic route is reachable
// through it.
//
// Every route is guarded by the same verified-client-certificate check. The
// dedicated listener enforces it during the TLS handshake, but this handler is
// the single composition point for the route table, so it stays fail closed if
// it is ever mounted on another listener or behind a TLS-terminating proxy.
func DownstreamHandler(data http.Handler, authz AuthzHandlers) (http.Handler, error) {
	if data == nil {
		return nil, fmt.Errorf("%w: a policy-source handler is required", ErrConfig)
	}
	if authz.IsAllowed == nil || authz.AllGrantedRoles == nil || authz.AllGrantedPermissions == nil {
		return nil, fmt.Errorf("%w: all three authorization handlers are required", ErrConfig)
	}
	mux := http.NewServeMux()
	mux.Handle(adsync.APIPrefix, data)
	mux.Handle(adsync.APIPrefix+"/", data)
	mux.Handle("POST "+svcs.PolicyAtzPath+"is-allowed", authz.IsAllowed)
	mux.Handle("POST "+svcs.PolicyAtzPath+"all-granted-roles", authz.AllGrantedRoles)
	mux.Handle("POST "+svcs.PolicyAtzPath+"all-granted-permissions", authz.AllGrantedPermissions)
	return requireVerifiedClientCertificate(mux), nil
}

// requireVerifiedClientCertificate rejects every request whose connection does
// not carry a verified client certificate. It mirrors the check Server's own
// handler performs, so a route added here cannot become an unauthenticated
// oracle by accident.
func requireVerifiedClientCertificate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := clientIdentity(r); !ok {
			writeError(w, http.StatusForbidden, "unauthorized", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}
