//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package auth

import (
	"fmt"
	"testing"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion"
)

type fakeAsserter struct {
	resp *assertion.AssertResponse
	err  error
}

func (f *fakeAsserter) AssertToken(token, idpType, allowedIDD string, headers map[string]string) (*assertion.AssertResponse, error) {
	return f.resp, f.err
}

func TestAsserterAuthenticator(t *testing.T) {
	ok := &assertion.AssertResponse{Principals: []*ads.Principal{nil, {Type: "user"}, {Type: "user", Name: "alice"}}}
	principals, err := (&AsserterAuthenticator{&fakeAsserter{resp: ok}}).Authenticate("t", "bearer")
	if err != nil || len(principals) != 1 || principals[0].Name != "alice" {
		t.Errorf("principals %v, %v", principals, err)
	}
	for name, a := range map[string]*AsserterAuthenticator{
		"rejected":      {&fakeAsserter{err: fmt.Errorf("invalid token")}},
		"no principals": {&fakeAsserter{resp: &assertion.AssertResponse{}}},
	} {
		if _, err := a.Authenticate("t", ""); err != ErrBadCredentials {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := (&AsserterAuthenticator{&fakeAsserter{resp: ok}}).Authenticate("", ""); err != ErrBadCredentials {
		t.Errorf("empty token: %v", err)
	}
}
