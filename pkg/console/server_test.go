//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package console

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/console/auth"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsrest"
)

// consolePolicies let the asserter test server's user read the console.
const consolePolicies = `{"services":[{"name":"speedle-console","type":"application",
 "policies":[{"id":"read","name":"read","effect":"grant","principals":[["user:testUser"]],
  "permissions":[{"resource":"console","actions":["read"]}]}]}]}`

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// backends starts PMS, ADS and a token asserter, and returns a console
// configuration that uses them.
func backends(t *testing.T) Config {
	t.Helper()
	ps, err := store.NewStore("file", map[string]interface{}{"FileLocation": writeFile(t, "pms.json", "{}")})
	if err != nil {
		t.Fatal(err)
	}
	pmsRouter, err := pmsrest.NewRouter(ps)
	if err != nil {
		t.Fatal(err)
	}
	pms := httptest.NewServer(pmsRouter)
	t.Cleanup(pms.Close)

	service, err := adsrest.NewRESTService(&cfg.Config{StoreConfig: &cfg.StoreConfig{
		StoreType: cfg.StorageTypeFile, StoreProps: map[string]interface{}{"FileLocation": writeFile(t, "ads.json", consolePolicies)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	adsRouter, err := adsrest.NewRouter(service.Evaluator)
	if err != nil {
		t.Fatal(err)
	}
	ads := httptest.NewServer(adsRouter)
	t.Cleanup(ads.Close)

	asserter := assertion.NewTestServer(t, nil)
	t.Cleanup(asserter.Close)

	c := DefaultConfig()
	c.Endpoint, c.Insecure = "127.0.0.1:0", true
	c.PMSURL, c.ADSURL, c.Asserter.Endpoint = pms.URL, ads.URL, asserter.URL+"/assert"
	return c
}

// browser talks to a console the way its page does.
type browser struct {
	t      *testing.T
	base   string
	client *http.Client
	csrf   string
}

func open(t *testing.T, c Config) *browser {
	t.Helper()
	components, err := NewComponents(&c)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewRouter(components))
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	client := server.Client()
	client.Jar = jar
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &browser{t: t, base: server.URL, client: client}
}

func (b *browser) do(method, path, body string, header map[string]string, status int) *http.Response {
	b.t.Helper()
	req, _ := http.NewRequest(method, b.base+path, strings.NewReader(body))
	if b.csrf != "" {
		req.Header.Set(auth.CSRFHeader, b.csrf)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	if resp.StatusCode != status {
		b.t.Errorf("%s %s: %d, want %d (%s)", method, path, resp.StatusCode, status, raw)
	}
	return resp
}

func (b *browser) login(token string) *auth.MeResponse {
	b.t.Helper()
	resp := b.do("POST", "/console/api/login", `{"token":"`+token+`"}`, nil, http.StatusOK)
	var me auth.MeResponse
	json.NewDecoder(resp.Body).Decode(&me)
	b.csrf = me.CSRFToken
	return &me
}

func decode(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func TestPageAndHeaders(t *testing.T) {
	b := open(t, backends(t))
	resp := b.do("GET", "/console/", "", nil, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `<div id="app">`) {
		t.Errorf("page %s", body)
	}
	if resp.Header.Get("Content-Security-Policy") == "" || resp.Header.Get("X-Frame-Options") != "DENY" ||
		resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("headers %v", resp.Header)
	}
	for _, path := range []string{"/", "/console"} {
		if resp := b.do("GET", path, "", nil, http.StatusFound); resp.Header.Get("Location") != BasePath {
			t.Errorf("%s redirects to %q", path, resp.Header.Get("Location"))
		}
	}
	b.do("GET", "/console/api/nope", "", nil, http.StatusNotFound)
	b.do("DELETE", "/console/api/pms/service", "", nil, http.StatusNotFound)
	var config publicConfig
	decode(t, b.do("GET", "/console/api/config", "", nil, http.StatusOK), &config)
	if config.DevMode {
		t.Error("dev mode reported")
	}
	if resp := b.do("GET", "/console/api/me", "", nil, http.StatusUnauthorized); resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS header sent")
	}
}

func TestReadOnlyUser(t *testing.T) {
	b := open(t, backends(t))
	b.do("POST", "/console/api/login", `{"token":"test-token"}`, nil, http.StatusUnauthorized)
	me := b.login("abc")
	if me.User != "testUser" || !me.Access.Read || me.Access.Write {
		t.Fatalf("me %+v %+v", me, me.Access)
	}
	b.do("GET", "/console/api/me", "", nil, http.StatusOK)
	b.do("GET", "/console/api/pms/service", "", nil, http.StatusOK)
	b.do("POST", "/console/api/pms/service", `{"name":"app"}`, nil, http.StatusForbidden)
	b.do("POST", "/console/api/ads/is-allowed", `{"subject":{"principals":[{"type":"user","name":"testUser"}]},"serviceName":"speedle-console","resource":"console","action":"read"}`, nil, http.StatusOK)
	b.do("POST", "/console/api/logout", "", nil, http.StatusNoContent)
	b.do("GET", "/console/api/me", "", nil, http.StatusUnauthorized)
}

func TestAdministrator(t *testing.T) {
	c := backends(t)
	c.BootstrapAdmins = []string{"user:testUser"}
	b := open(t, c)
	if me := b.login("abc"); !me.Access.Write || !me.Access.Bootstrap {
		t.Fatalf("access %+v", me.Access)
	}
	b.do("POST", "/console/api/pms/service", `{"name":"app"}`, nil, http.StatusCreated)
	var created struct{ ID string }
	decode(t, b.do("POST", "/console/api/pms/service/app/policy", `{"name":"p","effect":"grant","principals":[["user:bob"]],"permissions":[{"resource":"r","actions":["get"]}]}`, nil, http.StatusCreated), &created)
	policy := "/console/api/pms/service/app/policy/" + created.ID
	etag := b.do("GET", policy, "", nil, http.StatusOK).Header.Get("ETag")
	resp := b.do("PUT", policy, `{"name":"p2","effect":"deny","principals":[["user:bob"]]}`, map[string]string{"If-Match": etag}, http.StatusOK)
	var updated struct{ Metadata map[string]string }
	decode(t, resp, &updated)
	if updated.Metadata["updateby"] != "testUser" {
		t.Errorf("metadata %v", updated.Metadata)
	}
	b.do("PUT", policy, `{"name":"p3","effect":"deny"}`, map[string]string{"If-Match": etag}, http.StatusPreconditionFailed)
	b.do("POST", "/console/api/ads/diagnose", `{"subject":{"principals":[{"type":"user","name":"bob"}]},"serviceName":"speedle-console","resource":"console","action":"read"}`, nil, http.StatusOK)
	b.do("DELETE", policy, "", nil, http.StatusNoContent)

	// Cross-site and CSRF-less writes are refused.
	b.do("DELETE", "/console/api/pms/service/app", "", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden)
	b.csrf = ""
	b.do("DELETE", "/console/api/pms/service/app", "", nil, http.StatusForbidden)
}

func TestADSOutageFailsClosed(t *testing.T) {
	c := backends(t)
	c.ADSURL = "http://127.0.0.1:1"
	b := open(t, c)
	b.do("POST", "/console/api/login", `{"token":"abc"}`, nil, http.StatusServiceUnavailable)
}

func TestDevMode(t *testing.T) {
	c := backends(t)
	c.DevNoAuth = true
	b := open(t, c)
	var me auth.MeResponse
	decode(t, b.do("GET", "/console/api/me", "", nil, http.StatusOK), &me)
	if !me.DevMode || !me.Access.Write {
		t.Errorf("dev me %+v", me)
	}
	b.csrf = me.CSRFToken
	b.do("POST", "/console/api/pms/service", `{"name":"app"}`, nil, http.StatusCreated)
	var config publicConfig
	decode(t, b.do("GET", "/console/api/config", "", nil, http.StatusOK), &config)
	if !config.DevMode {
		t.Error("dev mode not reported")
	}
}

func TestNewComponentsFailures(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"invalid config":   func(c *Config) { c.PMSURL = "" },
		"bad upstream CA":  func(c *Config) { c.UpstreamCA = "missing.crt" },
		"bad session key":  func(c *Config) { c.SessionKeyFile = "missing.key" },
		"bad asserter TLS": func(c *Config) { c.Asserter.Endpoint, c.Asserter.CACert = "https://asserter/assert", "missing.crt" },
	} {
		c := validConfig()
		change(&c)
		if _, err := NewComponents(&c); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, err := NewServer(&c); err == nil {
			t.Errorf("%s: server built", name)
		}
	}
	c := validConfig()
	c.SessionKeyFile = writeFile(t, "key", strings.Repeat("k", auth.MinKeySize))
	server, err := NewServer(&c)
	if err != nil || server.Handler == nil {
		t.Errorf("server %v, %v", server, err)
	}
}

func TestServerParams(t *testing.T) {
	c := validConfig()
	c.Insecure, c.CertPath, c.KeyPath = false, "c.pem", "k.pem"
	p := ServerParams(&c)
	if p.Endpoint.Value != c.Endpoint || p.Insecure.Value != "false" || p.CertPath.Value != "c.pem" || p.KeyPath.Value != "k.pem" {
		t.Errorf("params %+v", p)
	}
}

func TestSecurityHeadersOverTLS(t *testing.T) {
	c := backends(t)
	components, err := NewComponents(&c)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(NewRouter(components))
	defer server.Close()
	resp, err := server.Client().Get(server.URL + "/console/api/config")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Strict-Transport-Security") == "" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("headers %v", resp.Header)
	}
}
