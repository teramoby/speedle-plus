//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package svcs

const (
	// PolicyMgmtPath is the prefix for pms rest service
	PolicyMgmtPath = "/policy-mgmt/v1/"
	// PolicyAtzPath is the prefix for ads rest service
	PolicyAtzPath = "/authz-check/v1/"
	// Header to store asserted pincipals
	PrincipalsHeader = "Speedle-Principals"
	// DecisionSourceHeader marks where an authorization decision was made. It
	// is informational only; callers must not make a security decision from
	// it.
	DecisionSourceHeader = "X-Speedle-Decision-Source"
	// DecisionSourceCloudFallback marks a response that Speedle Cloud produced
	// while the local service was syncing.
	DecisionSourceCloudFallback = "cloud-fallback"
	// DecisionSourceUpstream marks a response the upstream produced for a
	// direct-mode (API key) instance while the local service was syncing or
	// the instance was stale.
	DecisionSourceUpstream = "upstream"
)
