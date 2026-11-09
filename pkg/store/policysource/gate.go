// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	stderrors "errors"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

var _ adsync.RequestGate = (*Store)(nil)

// Instance returns the instance-level distribution status.
func (s *Store) Instance() adsync.InstanceStatus {
	return s.machine.Instance()
}

// Services returns the distribution status of every tracked service.
func (s *Store) Services() []adsync.ServiceStatus {
	return s.machine.Services()
}

// FallbackAvailable reports whether a syncing service has a configured Cloud
// fallback outside its bounded retry delay. It is false when fallback is
// disabled or no distinct Cloud destination and credential are configured.
func (s *Store) FallbackAvailable() bool {
	if s == nil || !s.fallback || s.client == nil {
		return false
	}
	return s.client.FallbackAvailable()
}

// ForwardFallback sends one authorization request to the configured Cloud
// fallback with the Cloud credential. The body is forwarded unchanged.
//
// An authentication failure means the credential is no longer valid, so the
// instance stops as revoked and never serves a local decision again. Every
// other failure is reported to the caller, which fails closed.
func (s *Store) ForwardFallback(ctx context.Context, operation adsync.FallbackOperation, body []byte) (*adsync.FallbackResponse, error) {
	if s == nil || !s.fallback || s.client == nil {
		return nil, adsync.ErrFallbackUnavailable
	}
	response, err := s.client.ForwardFallback(ctx, operation, body)
	if err != nil && (stderrors.Is(err, adsync.ErrUnauthorized) || stderrors.Is(err, adsync.ErrForbidden)) {
		s.machine.Stop(adsync.ReasonRevoked)
	}
	return response, err
}
