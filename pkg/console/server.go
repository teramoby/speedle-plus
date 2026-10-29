//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package console

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
	"github.com/teramoby/speedle-plus/pkg/console/auth"
	"github.com/teramoby/speedle-plus/pkg/console/proxy"
	"github.com/teramoby/speedle-plus/pkg/console/web"
)

const (
	// BasePath is where the page is served.
	BasePath = "/console/"
	// APIPath is the prefix of the console API.
	APIPath = "/console/api"
)

// contentSecurityPolicy allows only the console's own scripts and styles.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// Components are the parts the console is built from. NewComponents fills
// them from a Config; tests may set their own.
type Components struct {
	Guard        *auth.Guard
	Proxy        *proxy.Proxy
	AuthzService string
}

// NewComponents builds the console's parts from cfg.
func NewComponents(cfg *Config) (*Components, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	client, err := newUpstreamClient(cfg.UpstreamCA, cfg.UpstreamCert, cfg.UpstreamKey, cfg.UpstreamTimeout)
	if err != nil {
		return nil, err
	}
	key, generated, err := auth.LoadKey(cfg.SessionKeyFile)
	if err != nil {
		return nil, err
	}
	if generated {
		log.Warning("No session key file was given; sessions end when the console restarts and only work with one replica.")
	}
	codec, err := auth.NewSessionCodec(key, cfg.SessionTTL)
	if err != nil {
		return nil, err
	}
	guard := &auth.Guard{Codec: codec, DevMode: cfg.DevNoAuth, Secure: !cfg.Insecure || cfg.SecureCookie, Bootstrap: map[string]bool{}}
	if cfg.DevNoAuth {
		log.Warning("Development mode: everyone who can reach the console is an administrator.")
	} else {
		asserter, err := assertion.NewAsserter(&cfg.Asserter, nil)
		if err != nil {
			return nil, err
		}
		guard.Authenticator = &auth.AsserterAuthenticator{Asserter: asserter}
		guard.Authorizer = &auth.ADSAuthorizer{Client: client, Endpoint: cfg.ADSURL, Service: cfg.AuthzService, TTL: cfg.AuthzCacheTTL}
		for _, p := range cfg.BootstrapAdmins {
			guard.Bootstrap[p] = true
		}
		if len(cfg.BootstrapAdmins) > 0 {
			log.Warningf("Bootstrap admins %v have full access without policies; remove them once console policies exist in service %q.",
				cfg.BootstrapAdmins, cfg.AuthzService)
		}
	}
	return &Components{Guard: guard, Proxy: &proxy.Proxy{Client: client, PMSURL: cfg.PMSURL, ADSURL: cfg.ADSURL},
		AuthzService: cfg.AuthzService}, nil
}

type publicConfig struct {
	DevMode bool `json:"devMode"`
	// AuthzService holds the console's own policies; the page warns before
	// they are edited.
	AuthzService string `json:"authzService"`
}

// NewRouter routes the page and the console API.
func NewRouter(c *Components) *mux.Router {
	router := mux.NewRouter()
	// Names may contain escaped slashes; keep them inside one segment.
	router.UseEncodedPath()
	router.Use(securityHeaders)

	// Routes sit on the root router: in this mux version a subrouter does
	// not match encoded paths, and would decode names twice.
	api := func(method, path string, handler http.Handler) {
		router.Methods(method).Path(APIPath + path).Handler(handler)
	}
	api(http.MethodGet, "/config", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.SendJSON(w, http.StatusOK, publicConfig{DevMode: c.Guard.DevMode, AuthzService: c.AuthzService})
	}))
	api(http.MethodPost, "/login", http.HandlerFunc(c.Guard.Login))
	api(http.MethodPost, "/logout", http.HandlerFunc(c.Guard.Logout))
	api(http.MethodGet, "/me", c.Guard.Require("", http.HandlerFunc(c.Guard.Me)))
	api(http.MethodGet, "/services/{service}/role-graph", c.Guard.Require(auth.ActionRead, http.HandlerFunc(c.RoleGraph)))
	api(http.MethodGet, "/services/{service}/roles/{role}/holders", c.Guard.Require(auth.ActionRead, http.HandlerFunc(c.RoleHolders)))
	for _, route := range c.Proxy.Routes() {
		api(route.Method, route.Path, c.Guard.Require(route.Action, route.Handler))
	}
	router.PathPrefix(APIPath + "/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.SendError(w, http.StatusNotFound, "No such console API.")
	})

	files := http.StripPrefix(BasePath, http.FileServer(http.FS(web.Static())))
	router.PathPrefix(BasePath).Methods(http.MethodGet, http.MethodHead).Handler(files)
	router.Path("/").Methods(http.MethodGet, http.MethodHead).Handler(http.RedirectHandler(BasePath, http.StatusFound))
	router.Path("/console").Methods(http.MethodGet, http.MethodHead).Handler(http.RedirectHandler(BasePath, http.StatusFound))
	return router
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if !strings.HasPrefix(r.URL.Path, APIPath) {
			h.Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// ServerParams converts cfg to the parameters the shared server helpers take.
func ServerParams(cfg *Config) *flags.Parameters {
	params := &flags.Parameters{}
	params.Endpoint.Value = cfg.Endpoint
	params.Insecure.Value = strconv.FormatBool(cfg.Insecure)
	params.CertPath.Value = cfg.CertPath
	params.KeyPath.Value = cfg.KeyPath
	return params
}

// NewServer returns the console's HTTP server, which sends no CORS headers
// so only the console's own page may call its API.
func NewServer(cfg *Config) (*http.Server, error) {
	components, err := NewComponents(cfg)
	if err != nil {
		return nil, err
	}
	return ServerParams(cfg).NewSameOriginHTTPServer(NewRouter(components))
}
