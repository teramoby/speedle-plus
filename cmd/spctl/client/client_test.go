//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newClient serves every request with the status and body given for its
// path, and records the last request.
func newClient(t *testing.T, responses map[string]struct {
	status int
	body   string
}) (*Client, *http.Request) {
	t.Helper()
	last := &http.Request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*last = *r
		response, ok := responses[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if response.status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "60")
		}
		w.WriteHeader(response.status)
		io.WriteString(w, response.body)
	}))
	t.Cleanup(server.Close)
	return &Client{PMSEndpoint: server.URL + "/policy-mgmt/v1/", HTTPClient: server.Client()}, last
}

type response = struct {
	status int
	body   string
}

var responses = map[string]response{
	"/policy-mgmt/v1/ok":           {http.StatusOK, `{"name":"a"}`},
	"/policy-mgmt/v1/created":      {http.StatusCreated, `{"name":"b"}`},
	"/policy-mgmt/v1/gone":         {http.StatusNoContent, ""},
	"/policy-mgmt/v1/unauthorized": {http.StatusUnauthorized, ""},
	"/policy-mgmt/v1/forbidden":    {http.StatusForbidden, ""},
	"/policy-mgmt/v1/bad":          {http.StatusBadRequest, `{"error":"no name"}`},
	"/policy-mgmt/v1/badplain":     {http.StatusBadRequest, `oops`},
	"/policy-mgmt/v1/conflict":     {http.StatusConflict, ""},
	"/policy-mgmt/v1/cloudauth":    {http.StatusUnauthorized, `{"error":"unauthorized"}`},
	"/policy-mgmt/v1/scope":        {http.StatusForbidden, `{"error":"insufficient_scope"}`},
	"/policy-mgmt/v1/cloudmissing": {http.StatusNotFound, `{"error":"not_found"}`},
	"/policy-mgmt/v1/servicedupe":  {http.StatusConflict, `{"error":"service_exists"}`},
	"/policy-mgmt/v1/policydupe":   {http.StatusConflict, `{"error":"policy_exists"}`},
	"/policy-mgmt/v1/ratelimited":  {http.StatusTooManyRequests, `{"error":"rate_limited"}`},
	"/policy-mgmt/v1/dailyquota":   {http.StatusTooManyRequests, `{"error":"daily_quota_exceeded"}`},
	"/policy-mgmt/v1/policyquota":  {http.StatusForbidden, `{"error":"policy_quota_exceeded"}`},
	"/policy-mgmt/v1/quotaoffline": {http.StatusServiceUnavailable, `{"error":"quota_unavailable"}`},
}

var cloudErrorCases = map[string]string{
	"cloudauth":    "authentication failed (401 Unauthorized): check the API token and endpoint",
	"scope":        "authorization failed (403 Forbidden): the API token needs pms:read for reads or pms:write for writes",
	"cloudmissing": "requested resource not found (404 Not Found)",
	"servicedupe":  "service already exists (409 Conflict)",
	"policydupe":   "policy already exists (409 Conflict)",
	"ratelimited":  "request rate limited (429 Too Many Requests): retry later with bounded backoff; Retry-After: 60",
	"dailyquota":   "daily quota exceeded (429 Too Many Requests): retry after the quota resets; Retry-After: 60",
	"policyquota":  "quota exceeded (403 Forbidden): reduce policy usage before retrying",
	"quotaoffline": "quota service unavailable (503 Service Unavailable): retry later with bounded backoff",
}

func TestGet(t *testing.T) {
	c, last := newClient(t, responses)
	body, err := c.Get([]string{"ok"}, url.Values{"filter": {"name eq a"}}, "tok")
	if err != nil || string(body) != `{"name":"a"}` {
		t.Fatalf("Get = %s, %v", body, err)
	}
	if last.URL.Query().Get("filter") != "name eq a" || last.Header.Get("Authorization") != "Bearer tok" {
		t.Fatalf("request %v %v", last.URL, last.Header)
	}
	if _, err := c.Get([]string{"ok"}, nil, ""); err != nil {
		t.Fatal(err)
	}
	if got := last.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization header without a token = %q", got)
	}
	cases := map[string]string{
		"missing":      "missing not found",
		"unauthorized": "401 Unauthorized",
		"forbidden":    "403 Forbidden",
		"bad":          "400 Bad Request: no name",
		"badplain":     "400 Bad Request",
		"conflict":     "409 Conflict",
	}
	for path, want := range cases {
		if _, err := c.Get([]string{path}, nil, ""); err == nil || err.Error() != want {
			t.Errorf("Get %s: %v, want %q", path, err, want)
		}
	}
	for path, want := range cloudErrorCases {
		if _, err := c.Get([]string{path}, nil, "tok"); err == nil || err.Error() != want {
			t.Errorf("Get %s: %v, want %q", path, err, want)
		}
	}
}

func TestPost(t *testing.T) {
	c, last := newClient(t, responses)
	for _, path := range []string{"ok", "created"} {
		if _, err := c.Post([]string{path}, strings.NewReader(`{}`), ""); err != nil {
			t.Errorf("Post %s: %v", path, err)
		}
	}
	if last.Header.Get("Content-Type") != "application/json" {
		t.Errorf("content type %q", last.Header.Get("Content-Type"))
	}
	cases := map[string]string{
		"unauthorized": "401 Unauthorized",
		"forbidden":    "403 Forbidden",
		"bad":          "400 Bad Request: no name",
		"badplain":     "400 Bad Request",
		"conflict":     "409 Conflict",
	}
	for path, want := range cases {
		if _, err := c.Post([]string{path}, nil, ""); err == nil || err.Error() != want {
			t.Errorf("Post %s: %v, want %q", path, err, want)
		}
	}
	for path, want := range cloudErrorCases {
		if _, err := c.Post([]string{path}, nil, "tok"); err == nil || err.Error() != want {
			t.Errorf("Post %s: %v, want %q", path, err, want)
		}
	}
}

func TestDelete(t *testing.T) {
	c, last := newClient(t, responses)
	if err := c.Delete([]string{"gone"}, ""); err != nil {
		t.Fatal(err)
	}
	if last.Method != "DELETE" {
		t.Fatalf("method %s", last.Method)
	}
	cases := map[string]string{
		"unauthorized": "401 Unauthorized",
		"forbidden":    "403 Forbidden",
		"bad":          "400 Bad Request: no name",
		"badplain":     "400 Bad Request",
		"ok":           "200 OK",
	}
	for path, want := range cases {
		if err := c.Delete([]string{path}, ""); err == nil || err.Error() != want {
			t.Errorf("Delete %s: %v, want %q", path, err, want)
		}
	}
	for path, want := range cloudErrorCases {
		if err := c.Delete([]string{path}, "tok"); err == nil || err.Error() != want {
			t.Errorf("Delete %s: %v, want %q", path, err, want)
		}
	}
}

func TestUnreachableAndInvalidEndpoints(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	c := &Client{PMSEndpoint: server.URL, HTTPClient: http.DefaultClient}
	if _, err := c.Get([]string{"x"}, nil, ""); err == nil {
		t.Error("Get on a closed server must fail")
	}
	if _, err := c.Post([]string{"x"}, nil, ""); err == nil {
		t.Error("Post on a closed server must fail")
	}
	if err := c.Delete([]string{"x"}, ""); err == nil {
		t.Error("Delete on a closed server must fail")
	}

	c.PMSEndpoint = "http://[::1"
	if _, err := c.Get(nil, nil, ""); err == nil {
		t.Error("Get with an invalid endpoint must fail")
	}
	if _, err := c.Post(nil, nil, ""); err == nil {
		t.Error("Post with an invalid endpoint must fail")
	}
	if err := c.Delete(nil, ""); err == nil {
		t.Error("Delete with an invalid endpoint must fail")
	}
	// A request URL that parses but cannot be sent.
	bad := &url.URL{Scheme: "http", Host: "a b"}
	if _, err := c.get(bad, nil, nil, ""); err == nil {
		t.Error("get with an invalid host must fail")
	}
	if _, err := c.post(bad, nil, nil, ""); err == nil {
		t.Error("post with an invalid host must fail")
	}
	if err := c.delete(bad, ""); err == nil {
		t.Error("delete with an invalid host must fail")
	}
}

func TestGetURL(t *testing.T) {
	u, err := getURL("https://pms:6733/policy-mgmt/v1/", []string{"service", "", "app"})
	if err != nil || u.String() != "https://pms:6733/policy-mgmt/v1/service/app" {
		t.Fatalf("getURL = %v, %v", u, err)
	}
}

func TestValidateTokenTransport(t *testing.T) {
	cases := []struct {
		endpoint string
		token    string
		allowed  bool
	}{
		{"https://pms.example.com/policy-mgmt/v1", "tok", true},
		{"http://localhost:6733/policy-mgmt/v1", "tok", true},
		{"http://LOCALHOST.:6733/policy-mgmt/v1", "tok", true},
		{"http://127.0.0.1:6733/policy-mgmt/v1", "tok", true},
		{"http://[::1]:6733/policy-mgmt/v1", "tok", true},
		{"http://192.0.2.1/policy-mgmt/v1", "tok", false},
		{"http://pms.example.com/policy-mgmt/v1", "tok", false},
		{"ftp://127.0.0.1/policy-mgmt/v1", "tok", false},
		{"http://192.0.2.1/policy-mgmt/v1", "", true},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.endpoint)
		if err != nil {
			t.Fatal(err)
		}
		err = validateTokenTransport(u, tc.token)
		if (err == nil) != tc.allowed {
			t.Errorf("validateTokenTransport(%q, token set %t) = %v, allowed %t", tc.endpoint, tc.token != "", err, tc.allowed)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRemoteHTTPTokenIsRefusedBeforeRequest(t *testing.T) {
	calls := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})}
	c := &Client{PMSEndpoint: "http://192.0.2.1/policy-mgmt/v1", HTTPClient: httpClient}
	if _, err := c.Get([]string{"service"}, nil, "secret"); err == nil || !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("Get error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("remote HTTP request was attempted %d times", calls)
	}
}

func TestTokenRedirectDowngradeIsRefused(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://192.0.2.1/policy-mgmt/v1/service")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	httpClient := server.Client()
	httpClient.Timeout = time.Second
	c := &Client{PMSEndpoint: server.URL + "/policy-mgmt/v1", HTTPClient: httpClient}
	_, err := c.Get([]string{"service"}, nil, "secret")
	if err == nil || !strings.Contains(err.Error(), "insecure HTTP") {
		t.Fatalf("redirect error = %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatal("redirect error exposed the token")
	}
}
