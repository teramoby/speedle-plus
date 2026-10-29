//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/ads"
)

const (
	// CookieName names the session cookie.
	CookieName = "speedle_console"
	// CSRFHeader carries the session's CSRF token on unsafe requests.
	CSRFHeader = "X-CSRF-Token"
	cookiePath = "/console"
	maxBody    = 64 << 10
)

// DevPrincipal is the user every request acts as in development mode.
var DevPrincipal = &ads.Principal{Type: ads.PRINCIPAL_TYPE_USER, Name: "console-dev"}

type contextKey struct{}

// FromContext returns the session that Guard.Require stored.
func FromContext(ctx context.Context) *Session {
	s, _ := ctx.Value(contextKey{}).(*Session)
	return s
}

// WithSession stores a session in ctx, for handlers and their tests.
func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// Guard signs users in and out and protects the console API.
type Guard struct {
	Codec         *SessionCodec
	Authenticator Authenticator
	Authorizer    Authorizer
	// Bootstrap principals (type:name) may read and write without asking
	// ADS, so the first console policy can be written.
	Bootstrap map[string]bool
	// DevMode signs everyone in as DevPrincipal with full access.
	DevMode bool
	// Secure marks the cookie Secure; set it whenever the console uses TLS.
	Secure bool
}

// Access is what a session may do.
type Access struct {
	Read      bool `json:"read"`
	Write     bool `json:"write"`
	Bootstrap bool `json:"bootstrap,omitempty"`
}

type errorBody struct {
	Error string `json:"error"`
}

// SendJSON writes v as JSON with status.
func SendJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// SendError writes {"error": message} with status.
func SendError(w http.ResponseWriter, status int, message string) {
	SendJSON(w, status, errorBody{Error: message})
}

func (g *Guard) setCookie(w http.ResponseWriter, s *Session) error {
	value, err := g.Codec.Encode(s)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: value, Path: cookiePath,
		MaxAge: int(g.Codec.TTL().Seconds()), HttpOnly: true, Secure: g.Secure, SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (g *Guard) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: cookiePath,
		MaxAge: -1, HttpOnly: true, Secure: g.Secure, SameSite: http.SameSiteStrictMode,
	})
}

// session returns the request's session. In development mode a missing or
// invalid session is replaced by a new one for DevPrincipal.
func (g *Guard) session(w http.ResponseWriter, r *http.Request) (*Session, error) {
	if c, err := r.Cookie(CookieName); err == nil {
		s, err := g.Codec.Decode(c.Value)
		if err == nil {
			return s, nil
		}
		if !g.DevMode {
			return nil, err
		}
	} else if !g.DevMode {
		return nil, ErrInvalidSession
	}
	s, err := g.Codec.New([]*ads.Principal{DevPrincipal})
	if err != nil {
		return nil, err
	}
	if err := g.setCookie(w, s); err != nil {
		return nil, err
	}
	return s, nil
}

// sameOrigin rejects cross-site requests: an Origin header, when present,
// must name the host the request was sent to.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}

func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// checkRequest enforces the origin and CSRF rules on unsafe requests.
func checkRequest(r *http.Request, s *Session) bool {
	if safeMethod(r.Method) {
		return true
	}
	token := r.Header.Get(CSRFHeader)
	return sameOrigin(r) && token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.CSRF)) == 1
}

func (g *Guard) isBootstrap(s *Session) bool {
	for _, p := range s.Strings() {
		if g.Bootstrap[p] {
			return true
		}
	}
	return false
}

// Allowed reports whether the session may perform action. An error means
// the decision could not be made.
func (g *Guard) Allowed(ctx context.Context, s *Session, action string) (bool, error) {
	if g.DevMode {
		return true, nil
	}
	if g.isBootstrap(s) {
		log.Warningf("Console bootstrap principal %v used for %s; remove --bootstrap-admins once console policies exist.", s.Strings(), action)
		return true, nil
	}
	return g.Authorizer.Allowed(ctx, s.Principals, action)
}

// Access reports what the session may do.
func (g *Guard) Access(ctx context.Context, s *Session) (*Access, error) {
	read, err := g.Allowed(ctx, s, ActionRead)
	if err != nil {
		return nil, err
	}
	write, err := g.Allowed(ctx, s, ActionWrite)
	if err != nil {
		return nil, err
	}
	return &Access{Read: read, Write: write, Bootstrap: !g.DevMode && g.isBootstrap(s)}, nil
}

// Require lets a request through when it carries a valid session that may
// perform action, and stores the session in the request context. An empty
// action only requires a session.
func (g *Guard) Require(action string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := g.session(w, r)
		if err != nil {
			SendError(w, http.StatusUnauthorized, "Sign in to use the console.")
			return
		}
		if !checkRequest(r, s) {
			SendError(w, http.StatusForbidden, "The request failed the cross-site request checks.")
			return
		}
		if action != "" {
			allowed, err := g.Allowed(r.Context(), s, action)
			if err != nil {
				log.Errorf("Console authorization failed: %v", err)
				SendError(w, http.StatusServiceUnavailable, "The authorization service is unavailable.")
				return
			}
			if !allowed {
				SendError(w, http.StatusForbidden, "You may not "+action+" in the console.")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), s)))
	})
}

type loginRequest struct {
	Token     string `json:"token"`
	TokenType string `json:"tokenType"`
}

// MeResponse describes the signed-in user.
type MeResponse struct {
	Principals []string `json:"principals"`
	User       string   `json:"user"`
	CSRFToken  string   `json:"csrfToken"`
	Access     *Access  `json:"access"`
	DevMode    bool     `json:"devMode,omitempty"`
}

func (g *Guard) me(w http.ResponseWriter, r *http.Request, s *Session) {
	access, err := g.Access(r.Context(), s)
	if err != nil {
		log.Errorf("Console authorization failed: %v", err)
		SendError(w, http.StatusServiceUnavailable, "The authorization service is unavailable.")
		return
	}
	SendJSON(w, http.StatusOK, &MeResponse{
		Principals: s.Strings(), User: s.User(), CSRFToken: s.CSRF, Access: access, DevMode: g.DevMode,
	})
}

// Login exchanges a token for a session cookie.
func (g *Guard) Login(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		SendError(w, http.StatusForbidden, "The request failed the cross-site request checks.")
		return
	}
	if g.DevMode {
		s, err := g.session(w, r)
		if err != nil {
			SendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		g.me(w, r, s)
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&req); err != nil {
		SendError(w, http.StatusBadRequest, "The login request is not valid JSON.")
		return
	}
	principals, err := g.Authenticator.Authenticate(req.Token, req.TokenType)
	if err != nil {
		SendError(w, http.StatusUnauthorized, "The token was not accepted.")
		return
	}
	s, err := g.Codec.New(principals)
	if err == nil {
		err = g.setCookie(w, s)
	}
	if err != nil {
		SendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Infof("Console login: %v", s.Strings())
	g.me(w, r, s)
}

// Logout clears the session cookie. Sessions are stateless, so a copied
// cookie stays valid until it expires.
func (g *Guard) Logout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		SendError(w, http.StatusForbidden, "The request failed the cross-site request checks.")
		return
	}
	g.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me describes the signed-in user, including the CSRF token the page must
// send on unsafe requests.
func (g *Guard) Me(w http.ResponseWriter, r *http.Request) {
	g.me(w, r, FromContext(r.Context()))
}
