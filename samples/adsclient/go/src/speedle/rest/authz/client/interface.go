package client

import "github.com/teramoby/speedle-plus/samples/adsclient/go/src/speedle/api/authz"

// ADSClient is a client interface for ADS service
type ADSClient interface {
	IsAllowed(authz.RequestContext) (bool, error)
}
