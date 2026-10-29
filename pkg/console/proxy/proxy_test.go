//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/console/auth"
	"github.com/teramoby/speedle-plus/pkg/svcs"
)

// seen is what the fake upstream received.
type seen struct {
	method, path, rawQuery, body string
	header                       http.Header
}

func upstream(t *testing.T) (*httptest.Server, *seen) {
	t.Helper()
	got := &seen{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = seen{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, string(body), r.Header.Clone()}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"v2"`)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Set-Cookie", "upstream=1")
		w.WriteHeader(http.StatusTeapot)
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	return server, got
}

var session = &auth.Session{Principals: []*ads.Principal{{Type: "group", Name: "ops"}, {Type: "user", Name: "alice"}}, CSRF: "c"}

// newRouter serves the proxy routes as the console does, with session
// signed in when it is not nil.
func newRouter(p *Proxy, s *auth.Session) *mux.Router {
	router := mux.NewRouter()
	router.UseEncodedPath()
	for _, route := range p.Routes() {
		handler := route.Handler
		router.Methods(route.Method).Path(route.Path).Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s != nil {
				r = r.WithContext(auth.WithSession(r.Context(), s))
			}
			handler.ServeHTTP(w, r)
		}))
	}
	return router
}

func send(router http.Handler, method, target, body string, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range header {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	return rec
}

func TestForwardToPMS(t *testing.T) {
	server, got := upstream(t)
	router := newRouter(&Proxy{Client: server.Client(), PMSURL: server.URL + "/", ADSURL: server.URL}, session)

	rec := send(router, "PUT", "/pms/service/my%20app/policy/p1?debug=1", `{"name":"p"}`, map[string]string{
		"If-Match": `"v1"`, "Cookie": "speedle_console=secret", "Authorization": "Bearer x",
		svcs.PrincipalsHeader: "admin", "Content-Type": "text/plain", "X-Other": "1",
	})
	if rec.Code != http.StatusTeapot || rec.Body.String() != `{"ok":true}` || rec.Header().Get("ETag") != `"v2"` {
		t.Errorf("response %d %s %v", rec.Code, rec.Body.String(), rec.Header())
	}
	for _, h := range []string{"Access-Control-Allow-Origin", "Set-Cookie"} {
		if rec.Header().Get(h) != "" {
			t.Errorf("relayed %s", h)
		}
	}
	if got.method != "PUT" || got.path != svcs.PolicyMgmtPath+"service/my%20app/policy/p1" || got.rawQuery != "" || got.body != `{"name":"p"}` {
		t.Errorf("upstream saw %+v", got)
	}
	if got.header.Get(svcs.PrincipalsHeader) != "alice" || got.header.Get("If-Match") != `"v1"` ||
		got.header.Get("Content-Type") != "application/json" {
		t.Errorf("upstream headers %v", got.header)
	}
	for _, h := range []string{"Cookie", "Authorization", "X-Other"} {
		if got.header.Get(h) != "" {
			t.Errorf("forwarded %s", h)
		}
	}

	send(router, "GET", "/pms/service/app/policy?filter=name+eq+p&other=1", "", nil)
	if got.method != "GET" || got.rawQuery != "filter=name+eq+p" || got.body != "" {
		t.Errorf("list forwarded as %+v", got)
	}
	send(router, "GET", "/pms/service/app/role-policy", "", nil)
	if got.rawQuery != "" {
		t.Errorf("empty filter forwarded as %q", got.rawQuery)
	}
}

func TestForwardToADS(t *testing.T) {
	pms, _ := upstream(t)
	adsServer, got := upstream(t)
	router := newRouter(&Proxy{Client: adsServer.Client(), PMSURL: pms.URL, ADSURL: adsServer.URL}, session)
	send(router, "POST", "/ads/diagnose", `{"serviceName":"app"}`, nil)
	if got.path != svcs.PolicyAtzPath+"diagnose" || got.body != `{"serviceName":"app"}` || got.header.Get(svcs.PrincipalsHeader) != "" {
		t.Errorf("ADS saw %+v", got)
	}
}

func TestRejectsBadNames(t *testing.T) {
	server, got := upstream(t)
	router := newRouter(&Proxy{Client: server.Client(), PMSURL: server.URL, ADSURL: server.URL}, session)
	for _, target := range []string{
		"/pms/service/a%2Fb", "/pms/service/..", "/pms/service/%2E", "/pms/service/a%00b",
		"/pms/service/" + strings.Repeat("a", maxNameLength+1),
	} {
		*got = seen{}
		// mux redirects plain dot segments itself; nothing may reach PMS.
		if rec := send(router, "GET", target, "", nil); (rec.Code != http.StatusBadRequest && rec.Code != http.StatusMovedPermanently) || got.method != "" {
			t.Errorf("%s: %d, upstream saw %+v", target, rec.Code, got)
		}
	}
	if rec := send(router, "GET", "/pms/service/caf%C3%A9", "", nil); rec.Code != http.StatusTeapot || got.path != svcs.PolicyMgmtPath+"service/caf%C3%A9" {
		t.Errorf("unicode name: %d %s", rec.Code, got.path)
	}
}

func TestLimitsAndFailures(t *testing.T) {
	server, got := upstream(t)
	router := newRouter(&Proxy{Client: server.Client(), PMSURL: server.URL, ADSURL: server.URL}, session)
	if rec := send(router, "POST", "/pms/service", strings.Repeat("a", MaxRequestBody+1), nil); rec.Code != http.StatusRequestEntityTooLarge || got.method != "" {
		t.Errorf("large body: %d", rec.Code)
	}
	r := httptest.NewRequest("POST", "/pms/service", errReader{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unreadable body: %d", rec.Code)
	}

	server.Close()
	for _, method := range []string{"GET", "DELETE"} {
		if rec := send(router, method, "/pms/service/app", "", nil); rec.Code != http.StatusBadGateway {
			t.Errorf("%s with PMS down: %d", method, rec.Code)
		}
	}
	bad := newRouter(&Proxy{Client: http.DefaultClient, PMSURL: "http://bad host", ADSURL: "http://x"}, nil)
	if rec := send(bad, "GET", "/pms/service", "", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad upstream URL: %d", rec.Code)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestAuditsWrites(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) }))
	defer ok.Close()
	router := newRouter(&Proxy{Client: ok.Client(), PMSURL: ok.URL, ADSURL: ok.URL}, nil)
	if rec := send(router, "POST", "/pms/service", `{}`, nil); rec.Code != http.StatusCreated {
		t.Errorf("create: %d", rec.Code)
	}
}

func TestRoutesNeedAnAction(t *testing.T) {
	routes := (&Proxy{}).Routes()
	if len(routes) != len(specs) {
		t.Fatalf("%d routes", len(routes))
	}
	for _, r := range routes {
		if r.Action != auth.ActionRead && r.Action != auth.ActionWrite {
			t.Errorf("%s %s has action %q", r.Method, r.Path, r.Action)
		}
		if r.Method != http.MethodGet && !strings.HasPrefix(r.Path, "/ads/") && r.Action != auth.ActionWrite {
			t.Errorf("%s %s changes PMS without write", r.Method, r.Path)
		}
	}
}
