//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package httputils

import (
	goerrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

func TestHandleError(t *testing.T) {
	cases := map[error]int{
		errors.New(errors.EntityNotFound, "x"):      http.StatusNotFound,
		errors.New(errors.EntityAlreadyExists, "x"): http.StatusConflict,
		errors.New(errors.SerializationError, "x"):  http.StatusInternalServerError,
		errors.New(errors.StoreError, "x"):          http.StatusInternalServerError,
		errors.New(errors.InvalidRequest, "x"):      http.StatusBadRequest,
		errors.New(errors.ExceedLimit, "x"):         http.StatusForbidden,
		goerrors.New("plain"):                       http.StatusInternalServerError,
	}
	for err, want := range cases {
		rec := httptest.NewRecorder()
		HandleError(rec, err)
		if rec.Code != want {
			t.Errorf("%v: status %d, want %d", err, rec.Code, want)
		}
		if body := rec.Body.String(); body != `{"error":"`+err.Error()+`"}` {
			t.Errorf("%v: body %s", err, body)
		}
	}
}

func TestSendResponses(t *testing.T) {
	cases := []struct {
		send   func(http.ResponseWriter)
		status int
		body   string
	}{
		{func(w http.ResponseWriter) { SendBadRequestResponse(w, &ErrorResponse{Error: "bad"}) }, 400, `{"error":"bad"}`},
		{func(w http.ResponseWriter) { SendInternalErrorResponse(w, nil) }, 500, ""},
		{func(w http.ResponseWriter) { SendCreatedResponse(w, []int{1}) }, 201, "[1]"},
		{func(w http.ResponseWriter) { SendOKResponse(w, map[string]int{"a": 1}) }, 200, `{"a":1}`},
		{SendEmptyListResponse, 200, "[]"},
	}
	for i, c := range cases {
		rec := httptest.NewRecorder()
		c.send(rec)
		if rec.Code != c.status || rec.Body.String() != c.body || rec.Header().Get("Content-Type") != "application/json; charset=UTF-8" {
			t.Errorf("case %d: %d %q %q", i, rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
		}
	}
	rec := httptest.NewRecorder()
	SendPageNotFoundResponse(rec)
	if rec.Code != http.StatusNotFound {
		t.Errorf("not found: %d", rec.Code)
	}
}

func TestAuthenticationChallenges(t *testing.T) {
	cases := []struct {
		send func(http.ResponseWriter)
		want string
	}{
		{func(w http.ResponseWriter) { SendBasicAtzErrorResponse(w, "") }, "Basic"},
		{func(w http.ResponseWriter) { SendBasicAtzErrorResponse(w, "speedle") }, `Basic realm="speedle"`},
		{func(w http.ResponseWriter) { SendBearerAtzErrorResponse(w, "", "", "") }, "Bearer"},
		{func(w http.ResponseWriter) { SendBearerAtzErrorResponse(w, "", "invalid_token", "") }, `Bearer error="invalid_token"`},
		{func(w http.ResponseWriter) { SendBearerAtzErrorResponse(w, "", "", "expired") }, `Bearer error_description="expired"`},
		{func(w http.ResponseWriter) { SendBearerAtzErrorResponse(w, "r", "e", "d") },
			`Bearer realm="r", error="e", error_description="d"`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		c.send(rec)
		if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != c.want {
			t.Errorf("got %d %q, want %q", rec.Code, rec.Header().Get("WWW-Authenticate"), c.want)
		}
	}
}

func TestVerifyContentType(t *testing.T) {
	cases := map[string]bool{
		"application/json":                true,
		"application/json; charset=utf-8": true,
		"text/plain":                      false,
		"":                                false,
	}
	for contentType, want := range cases {
		req := httptest.NewRequest("POST", "/", nil)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		if got := VerifyContentType(rec, req, []string{"application/xml", "application/json"}); got != want {
			t.Errorf("%q: %v, want %v", contentType, got, want)
		}
		if !want && rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%q: status %d", contentType, rec.Code)
		}
	}
}
