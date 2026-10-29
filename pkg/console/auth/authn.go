//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package auth

import (
	stderrors "errors"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion"
)

// ErrBadCredentials means the token was rejected or names nobody.
var ErrBadCredentials = stderrors.New("the token was not accepted")

// Authenticator turns a login token into the user's principals.
type Authenticator interface {
	Authenticate(token, tokenType string) ([]*ads.Principal, error)
}

// AsserterAuthenticator asks the token asserter webhook, as ADS does.
type AsserterAuthenticator struct {
	Asserter assertion.TokenAsserter
}

// Authenticate asserts token and returns the principals it names.
func (a *AsserterAuthenticator) Authenticate(token, tokenType string) ([]*ads.Principal, error) {
	if token == "" {
		return nil, ErrBadCredentials
	}
	resp, err := a.Asserter.AssertToken(token, tokenType, "", nil)
	if err != nil {
		log.Warningf("Console login failed: %v", err)
		return nil, ErrBadCredentials
	}
	var principals []*ads.Principal
	for _, p := range resp.Principals {
		if p != nil && p.Type != "" && p.Name != "" {
			principals = append(principals, p)
		}
	}
	if len(principals) == 0 {
		return nil, ErrBadCredentials
	}
	return principals, nil
}
