//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/ads"
)

type fakeAuthn struct{}

func (fakeAuthn) Authenticate(token, tokenType string) ([]*ads.Principal, error) {
	if token != "good" {
		return nil, ErrBadCredentials
	}
	return alice, nil
}

// fakeAuthz grants the actions in allow, or fails with err.
type fakeAuthz struct {
	allow map[string]bool
	err   error
}

func (f *fakeAuthz) Allowed(_ context.Context, _ []*ads.Principal, action string) (bool, error) {
	return f.allow[action], f.err
}

func newGuard(t *testing.T, authz *fakeAuthz) *Guard {
	return &Guard{Codec: newCodec(t), Authenticator: fakeAuthn{}, Authorizer: authz, Bootstrap: map[string]bool{}}
}

type request struct {
	method, origin, csrf string
	cookie               *http.Cookie
}

func (q request) do(h http.Handler) *httptest.ResponseRecorder {
	r := httptest.NewRequest(q.method, "http://console.example/console/api/x", strings.NewReader("{}"))
	if q.origin != "" {
		r.Header.Set("Origin", q.origin)
	}
	if q.csrf != "" {
		r.Header.Set(CSRFHeader, q.csrf)
	}
	if q.cookie != nil {
		r.AddCookie(q.cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func cookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func login(t *testing.T, g *Guard) (*http.Cookie, *MeResponse) {
	t.Helper()
	r := httptest.NewRequest("POST", "http://console.example/console/api/login", strings.NewReader(`{"token":"good"}`))
	rec := httptest.NewRecorder()
	g.Login(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	var me MeResponse
	json.Unmarshal(rec.Body.Bytes(), &me)
	return cookieOf(t, rec), &me
}

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, FromContext(r.Context()).User())
})

func TestRequire(t *testing.T) {
	authz := &fakeAuthz{allow: map[string]bool{ActionRead: true}}
	g := newGuard(t, authz)
	cookie, me := login(t, g)
	if me.User != "alice" || me.CSRFToken == "" || !me.Access.Read || me.Access.Write || me.Access.Bootstrap {
		t.Errorf("me %+v %+v", me, me.Access)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/console" {
		t.Errorf("cookie %+v", cookie)
	}
	read, write := g.Require(ActionRead, ok), g.Require(ActionWrite, ok)
	bad := &http.Cookie{Name: CookieName, Value: cookie.Value + "x"}
	for _, c := range []struct {
		name    string
		handler http.Handler
		req     request
		status  int
	}{
		{"no session", read, request{method: "GET"}, 401},
		{"forged session", read, request{method: "GET", cookie: bad}, 401},
		{"read", read, request{method: "GET", cookie: cookie}, 200},
		{"post without CSRF token", read, request{method: "POST", cookie: cookie}, 403},
		{"post with a wrong CSRF token", read, request{method: "POST", cookie: cookie, csrf: "nope"}, 403},
		{"post from another site", read, request{method: "POST", cookie: cookie, csrf: me.CSRFToken, origin: "https://evil.example"}, 403},
		{"post with a bad origin", read, request{method: "POST", cookie: cookie, csrf: me.CSRFToken, origin: "::"}, 403},
		{"post", read, request{method: "POST", cookie: cookie, csrf: me.CSRFToken, origin: "http://console.example"}, 200},
		{"post without origin", read, request{method: "POST", cookie: cookie, csrf: me.CSRFToken}, 200},
		{"write denied", write, request{method: "GET", cookie: cookie}, 403},
		{"session only", g.Require("", ok), request{method: "GET", cookie: cookie}, 200},
	} {
		if rec := c.req.do(c.handler); rec.Code != c.status {
			t.Errorf("%s: %d, want %d (%s)", c.name, rec.Code, c.status, rec.Body.String())
		} else if c.status == 200 && rec.Body.String() != "alice" {
			t.Errorf("%s: handler saw %q", c.name, rec.Body.String())
		}
	}

	authz.err = fmt.Errorf("ADS down")
	if rec := (request{method: "GET", cookie: cookie}).do(read); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("ADS down: %d", rec.Code)
	}
	if rec := (request{method: "GET", cookie: cookie}).do(g.Require("", http.HandlerFunc(g.Me))); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("me with ADS down: %d", rec.Code)
	}

	// Bootstrap principals need no policy, even with ADS down.
	g.Bootstrap["group:ops@corp"] = true
	if rec := (request{method: "GET", cookie: cookie}).do(write); rec.Code != http.StatusOK {
		t.Errorf("bootstrap write: %d", rec.Code)
	}
	access, err := g.Access(context.Background(), &Session{Principals: alice})
	if err != nil || !access.Write || !access.Bootstrap {
		t.Errorf("bootstrap access %+v, %v", access, err)
	}
}

func TestAccessReportsWriteFailures(t *testing.T) {
	g := newGuard(t, &fakeAuthz{allow: map[string]bool{ActionRead: true}})
	g.Authorizer = writeFails{}
	if _, err := g.Access(context.Background(), &Session{Principals: alice}); err == nil {
		t.Error("no error")
	}
}

type writeFails struct{}

func (writeFails) Allowed(_ context.Context, _ []*ads.Principal, action string) (bool, error) {
	if action == ActionWrite {
		return false, fmt.Errorf("down")
	}
	return true, nil
}

func TestLogin(t *testing.T) {
	g := newGuard(t, &fakeAuthz{})
	g.Secure = true
	for _, c := range []struct {
		name, body, origin string
		status             int
	}{
		{"not JSON", "{", "", 400},
		{"bad token", `{"token":"bad"}`, "", 401},
		{"from another site", `{"token":"good"}`, "https://evil.example", 403},
		{"too large", `{"token":"` + strings.Repeat("a", maxBody) + `"}`, "", 400},
	} {
		r := httptest.NewRequest("POST", "http://console.example/console/api/login", strings.NewReader(c.body))
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		g.Login(rec, r)
		if rec.Code != c.status {
			t.Errorf("%s: %d, want %d", c.name, rec.Code, c.status)
		}
	}
	cookie, _ := login(t, g)
	if !cookie.Secure {
		t.Error("the cookie is not Secure")
	}

	// Access failures after a good login are reported, not swallowed.
	g.Authorizer = &fakeAuthz{err: fmt.Errorf("down")}
	r := httptest.NewRequest("POST", "http://console.example/console/api/login", strings.NewReader(`{"token":"good"}`))
	rec := httptest.NewRecorder()
	g.Login(rec, r)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("login with ADS down: %d", rec.Code)
	}
}

func TestLogout(t *testing.T) {
	g := newGuard(t, &fakeAuthz{})
	rec := httptest.NewRecorder()
	g.Logout(rec, httptest.NewRequest("POST", "http://console.example/console/api/logout", nil))
	if rec.Code != http.StatusNoContent || cookieOf(t, rec).MaxAge >= 0 {
		t.Errorf("logout: %d %+v", rec.Code, rec.Result().Cookies())
	}
	r := httptest.NewRequest("POST", "http://console.example/console/api/logout", nil)
	r.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	g.Logout(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site logout: %d", rec.Code)
	}
}

func TestDevMode(t *testing.T) {
	g := &Guard{Codec: newCodec(t), DevMode: true}
	rec := (request{method: "GET"}).do(g.Require(ActionWrite, ok))
	if rec.Code != http.StatusOK || rec.Body.String() != DevPrincipal.Name {
		t.Fatalf("dev request: %d %s", rec.Code, rec.Body.String())
	}
	cookie := cookieOf(t, rec)
	// A broken cookie is replaced too.
	rec = (request{method: "GET", cookie: &http.Cookie{Name: CookieName, Value: "x"}}).do(g.Require(ActionRead, ok))
	if rec.Code != http.StatusOK {
		t.Errorf("dev request with a bad cookie: %d", rec.Code)
	}
	// Unsafe requests still need the CSRF token.
	if rec := (request{method: "POST", cookie: cookie}).do(g.Require(ActionWrite, ok)); rec.Code != http.StatusForbidden {
		t.Errorf("dev post without CSRF: %d", rec.Code)
	}
	r := httptest.NewRequest("POST", "http://console.example/console/api/login", nil)
	rec = httptest.NewRecorder()
	g.Login(rec, r)
	var me MeResponse
	json.Unmarshal(rec.Body.Bytes(), &me)
	if rec.Code != http.StatusOK || !me.DevMode || !me.Access.Write || me.Access.Bootstrap {
		t.Errorf("dev login: %d %s", rec.Code, rec.Body.String())
	}
}
