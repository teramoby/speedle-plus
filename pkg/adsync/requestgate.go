// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

// RequestGate is what a policy-source store offers to the REST and gRPC entry
// points: the per-service distribution state and the fallback path. A store
// that does not distribute policy does not implement it, and those entry
// points keep their existing behaviour.
type RequestGate interface {
	StatusProvider
	// Instance reports the instance-level liveness.
	Instance() InstanceStatus
	// Services reports every tracked service, including removed tombstones,
	// sorted by service ID.
	Services() []ServiceStatus
	FallbackForwarder
}
