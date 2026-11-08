// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import "errors"

// Errors returned by this package. They are sentinels so callers can classify a
// failure without inspecting a response body.
var (
	// ErrConfig means the mirror or the server was configured with values it
	// cannot serve with.
	ErrConfig = errors.New("adsproxy: invalid configuration")
	// ErrCredentialFile means the downstream credential file is unreadable,
	// unsafe or malformed.
	ErrCredentialFile = errors.New("adsproxy: invalid downstream credential file")
	// ErrUnauthorized means a downstream credential is missing, malformed or
	// unknown.
	ErrUnauthorized = errors.New("adsproxy: unauthorized")
	// ErrUnknownService means the proxy does not hold a copy of the service.
	ErrUnknownService = errors.New("adsproxy: unknown service")
	// ErrServiceRemoved means the service was served before and is gone now.
	ErrServiceRemoved = errors.New("adsproxy: service removed")
	// ErrHistoryUnavailable means the requested revision can no longer be
	// resumed from the proxy's log; the downstream must reload the snapshot.
	ErrHistoryUnavailable = errors.New("adsproxy: history unavailable")
	// ErrSnapshotRequired means the mirror needs a fresh signed snapshot before
	// it can serve the service again.
	ErrSnapshotRequired = errors.New("adsproxy: fresh snapshot required")
	// ErrIdentityChanged means a change page belongs to a different sourceId or
	// epoch than the copy the mirror holds.
	ErrIdentityChanged = errors.New("adsproxy: source identity changed")
	// ErrTooManyServices means a snapshot would add more services than the
	// protocol carries.
	ErrTooManyServices = errors.New("adsproxy: too many services")
	// ErrFanoutLimit means the proxy already serves the configured number of
	// downstream instances.
	ErrFanoutLimit = errors.New("adsproxy: downstream limit reached")
	// ErrChainTooDeep means the downstream is more than MaxChainDepth proxy
	// hops away from Cloud.
	ErrChainTooDeep = errors.New("adsproxy: chain too deep")
	// ErrChainLoop means the downstream chain repeats an instance or places
	// this proxy in an impossible position.
	ErrChainLoop = errors.New("adsproxy: chain loop")
)
