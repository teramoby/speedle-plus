// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsrest

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type brokenRequestBody struct{}

func (brokenRequestBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (brokenRequestBody) Close() error             { return nil }

func TestBoundedAuthorizationBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		body io.ReadCloser
		want string
	}{
		{"read failure", brokenRequestBody{}, "unable to read request"},
		{"too large", io.NopCloser(strings.NewReader(strings.Repeat("x", maxAuthzRequestBodyBytes+1))), "request body is too large"},
		{"invalid JSON", io.NopCloser(strings.NewReader("{")), "unable to decode request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", nil)
			request.Body = tc.body
			if _, err := decodeBoundedJSONContext(request); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
