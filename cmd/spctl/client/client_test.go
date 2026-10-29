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
