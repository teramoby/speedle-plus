//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package subjectutils

import (
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
)

func TestEncodePrincipal(t *testing.T) {
	cases := map[string]*adsapi.Principal{
		"user:alice":          {Type: "user", Name: "alice"},
		"idd=corp:group:devs": {Type: "group", Name: "devs", IDD: "corp"},
	}
	for want, principal := range cases {
		if got := EncodePrincipal(principal); got != want {
			t.Errorf("EncodePrincipal(%+v) = %q, want %q", principal, got, want)
		}
	}
}
