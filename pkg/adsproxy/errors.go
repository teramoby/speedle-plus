// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import "errors"

var (
	// ErrConfig means the proxy is misconfigured.
	ErrConfig = errors.New("adsproxy: invalid configuration")
	// ErrUnknownService means the mirror never held the requested service.
	ErrUnknownService = errors.New("adsproxy: unknown service")
	// ErrServiceRemoved means the mirror held the service and it was removed.
	ErrServiceRemoved = errors.New("adsproxy: service removed")
	// ErrHistoryUnavailable means the mirror cannot produce the requested
	// change page and the downstream must fetch a fresh snapshot.
	ErrHistoryUnavailable = errors.New("adsproxy: history unavailable")
	// ErrSnapshotRequired means the mirror needs a fresh snapshot before it
	// can serve the service again.
	ErrSnapshotRequired = errors.New("adsproxy: fresh snapshot required")
	// ErrIdentityChanged means a snapshot or change page belongs to a
	// different source identity than the copy.
	ErrIdentityChanged = errors.New("adsproxy: source identity changed")
	// ErrTooManyServices means the mirror's replay-protection tombstones are
	// full, so it refuses another service rather than forget a deletion guard.
	ErrTooManyServices = errors.New("adsproxy: too many services")
	// ErrIdentityChurn means a service changed source identity more often than
	// the mirror keeps replay protection for.
	ErrIdentityChurn = errors.New("adsproxy: source identity churn exceeded")

	// ErrTLSMaterial means the downstream listener's certificate, key or
	// client CA is missing or unusable.
	ErrTLSMaterial = errors.New("adsproxy: invalid downstream TLS material")
	// ErrCRL means a configured CRL is malformed, untrusted or outside its
	// validity window.
	ErrCRL = errors.New("adsproxy: invalid downstream CRL")
	// ErrRevoked means a client certificate is listed in a fresh CRL.
	ErrRevoked = errors.New("adsproxy: downstream certificate is revoked")
	// ErrNoFreshCRL means no fresh CRL covers one issuer in a client
	// certificate chain.
	ErrNoFreshCRL = errors.New("adsproxy: no fresh CRL covers the client issuer")
	// ErrStaleTLSGeneration means a handshake used material that a reload
	// replaced before the connection was admitted.
	ErrStaleTLSGeneration = errors.New("adsproxy: TLS material changed during the handshake")
)
