//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package proxy forwards the console's allowlisted API calls to PMS and ADS.
package proxy

import (
	"bytes"
	stderrors "errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/console/auth"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs"
)

const (
	// MaxRequestBody caps what a browser may send upstream.
	MaxRequestBody = 1 << 20
	// maxResponseBody caps what is relayed back.
	maxResponseBody = 32 << 20
	maxNameLength   = 512
)

// Proxy forwards requests to PMS and ADS with the signed-in user's identity.
type Proxy struct {
	Client *http.Client
	PMSURL string
	ADSURL string
}

// Route is one allowlisted console API route.
type Route struct {
	Method  string
	Path    string
	Action  string
	Handler http.Handler
}

// Routes returns the allowlisted routes. Callers must protect each handler
// so it requires Action, and match paths without decoding them (see
// mux.Router.UseEncodedPath) so an escaped slash stays inside a name.
func (p *Proxy) Routes() []Route {
	routes := make([]Route, 0, len(specs))
	for _, s := range specs {
		s := s
		routes = append(routes, Route{Method: s.method, Path: s.path, Action: s.action,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { p.forward(w, r, s) })})
	}
	return routes
}

// ValidName accepts the names PMS stores: anything printable without a
// slash, except the dot segments a server would resolve.
func ValidName(name string) bool {
	if name == "" || name == "." || name == ".." || len(name) > maxNameLength || strings.Contains(name, "/") {
		return false
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// targetPath fills the route's upstream path with the request's variables,
// checked and escaped again.
func targetPath(r *http.Request, s spec) (string, bool) {
	vars := mux.Vars(r)
	target := s.target
	for name, raw := range vars {
		value, err := url.PathUnescape(raw)
		if err != nil || !ValidName(value) {
			return "", false
		}
		target = strings.ReplaceAll(target, "{"+name+"}", url.PathEscape(value))
	}
	return target, !strings.Contains(target, "{")
}

func (p *Proxy) baseURL(u Upstream) string {
	if u == ADS {
		return strings.TrimSuffix(p.ADSURL, "/") + svcs.PolicyAtzPath
	}
	return strings.TrimSuffix(p.PMSURL, "/") + svcs.PolicyMgmtPath
}

func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, s spec) {
	session := auth.FromContext(r.Context())
	target, ok := targetPath(r, s)
	if !ok {
		auth.SendError(w, http.StatusBadRequest, "The name in the path is not valid.")
		return
	}
	query := url.Values{}
	for _, key := range s.query {
		if value := r.URL.Query().Get(key); value != "" {
			query.Set(key, value)
		}
	}
	upstreamURL := p.baseURL(s.upstream) + target
	if len(query) > 0 {
		upstreamURL += "?" + query.Encode()
	}

	var body io.Reader
	if r.Body != nil && s.method != http.MethodGet && s.method != http.MethodDelete {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxRequestBody))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if stderrors.As(err, &tooLarge) {
				auth.SendError(w, http.StatusRequestEntityTooLarge, "The request body is too large.")
			} else {
				auth.SendError(w, http.StatusBadRequest, "The request body could not be read.")
			}
			return
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(r.Context(), s.method, upstreamURL, body)
	if err != nil {
		auth.SendError(w, http.StatusBadRequest, "The request could not be forwarded.")
		return
	}
	// Only these headers reach the upstream; cookies, Authorization and
	// any Speedle-Principals the browser sent are dropped.
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if ifMatch := r.Header.Get("If-Match"); ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	if session != nil && s.upstream == PMS {
		req.Header.Set(svcs.PrincipalsHeader, session.User())
	}

	resp, err := p.Client.Do(req)
	if err != nil {
		log.Errorf("Console upstream %s %s failed: %v", s.method, upstreamURL, err)
		auth.SendError(w, http.StatusBadGateway, "The policy service is unreachable.")
		p.audit(r, s, target, session, 0)
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "ETag"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, io.LimitReader(resp.Body, maxResponseBody))
	p.audit(r, s, target, session, resp.StatusCode)
}

// audit records every write made through the console.
func (p *Proxy) audit(r *http.Request, s spec, target string, session *auth.Session, status int) {
	if s.action != auth.ActionWrite {
		return
	}
	fields := map[string]interface{}{"method": s.method, "target": target}
	if session != nil {
		fields["principals"] = session.Strings()
	}
	if status >= 200 && status < 300 {
		logging.WriteSucceededAuditLog("ConsoleWrite", fields, map[string]interface{}{"status": status})
	} else if status == 0 {
		logging.WriteFailedAuditLog("ConsoleWrite", fields, "upstream unreachable")
	} else {
		logging.WriteFailedAuditLog("ConsoleWrite", fields, http.StatusText(status))
	}
}
