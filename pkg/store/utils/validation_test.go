//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package utils

import (
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
)

func TestValidateFunctionURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		// Allowed
		{"https://example.com/func", false},
		{"http://example.com:8080/func", false},
		{"http://localhost:9000/func", false},
		{"http://127.0.0.1/func", false},
		{"http://[::1]/func", false},
		// Rejected: private / internal / metadata addresses (SSRF)
		{"http://169.254.169.254/latest/meta-data/", true}, // AWS metadata (link-local)
		{"http://10.0.0.5/func", true},                     // private
		{"http://192.168.1.1/func", true},                  // private
		{"http://172.16.0.1/func", true},                   // private
		{"http://[fe80::1]/func", true},                    // IPv6 link-local
		{"http://0.0.0.0/func", true},                      // unspecified
		// Rejected: bad scheme / unparseable
		{"ftp://example.com/func", true},
		{"file:///etc/passwd", true},
		{"://nonsense", true},
	}
	for _, c := range cases {
		err := ValidateFunctionURL(c.url)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateFunctionURL(%q) error = %v, wantErr = %v", c.url, err, c.wantErr)
		}
	}
}

func TestValidateFuncRejectsSSRF(t *testing.T) {
	// A function registered with an internal metadata URL must be rejected.
	fn := &pms.Function{Name: "evil", FuncURL: "http://169.254.169.254/latest/meta-data/"}
	if err := ValidateFunc(fn); err == nil {
		t.Errorf("expected ValidateFunc to reject SSRF metadata URL, got nil")
	}
	// A legitimate function must pass.
	ok := &pms.Function{Name: "good", FuncURL: "https://func.example.com/run"}
	if err := ValidateFunc(ok); err != nil {
		t.Errorf("expected ValidateFunc to accept public URL, got %v", err)
	}
}
