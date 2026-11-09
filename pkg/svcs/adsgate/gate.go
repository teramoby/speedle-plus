// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package adsgate routes authorization requests by the distribution state of
// the service they name. It is the shared decision point of the REST and gRPC
// entry points, so both surfaces make the same routing choice.
package adsgate

import (
	"context"
	"errors"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs"
)

// RouteKind is how one request must be handled.
type RouteKind int

const (
	// RouteLocal evaluates the request with the local store. It also covers a
	// service the instance does not know and one that was removed, so the
	// evaluator produces the usual SERVICE_NOT_FOUND result.
	RouteLocal RouteKind = iota
	// RouteFallback forwards the request to the configured Cloud fallback
	// with the Cloud credential.
	RouteFallback
	// RouteUnavailable fails closed with the given reason.
	RouteUnavailable
)

// Route is the decision for one service name.
type Route struct {
	Kind   RouteKind
	Reason adsync.Reason
}

// Gate routes requests using the state of a policy-source store. A nil *Gate
// is valid and always routes locally, which is what a store without
// distribution support does.
type Gate struct {
	source adsync.RequestGate
}

// New returns a gate for the given source. A nil source yields a nil gate.
func New(source adsync.RequestGate) *Gate {
	if source == nil {
		return nil
	}
	return &Gate{source: source}
}

// Of returns the gate of the store behind the evaluator, or nil when the
// evaluator does not distribute policy.
func Of(evaluator eval.InternalEvaluator) *Gate {
	provider, ok := evaluator.(interface{ DistributionStore() any })
	if !ok {
		return nil
	}
	source, _ := provider.DistributionStore().(adsync.RequestGate)
	return New(source)
}

// Enabled reports whether the gate has a distribution state to consult.
func (g *Gate) Enabled() bool {
	return g != nil && g.source != nil
}

// Route decides how a request for serviceName is handled. The instance-level
// stop wins over the service state: a stopped instance returns no decision at
// all. An unknown service name is never forwarded, so a caller cannot use the
// instance credential to probe services that are not part of its set.
func (g *Gate) Route(serviceName string) Route {
	if !g.Enabled() {
		return Route{Kind: RouteLocal}
	}
	instance := g.source.Instance()
	if instance.Liveness == adsync.LivenessStopped {
		return Route{Kind: RouteUnavailable, Reason: StoppedReason(instance)}
	}
	// A pending removal means the installed assembly may still authorize a
	// service the source already removed or renamed. This holds for every
	// machine state, including in_sync: a pass records the discovery removal
	// before the state machine learns it, and a renamed service's previous
	// name stays guarded until the replacement assembly is installed. Fail
	// closed until the evaluator acknowledges that installation.
	if removals, ok := g.source.(interface{ RemovalInstalled(string) bool }); ok && !removals.RemovalInstalled(serviceName) {
		return Route{Kind: RouteUnavailable, Reason: adsync.ReasonSourceGone}
	}
	status, known := g.source.Status(serviceName)
	if !known {
		return Route{Kind: RouteLocal}
	}
	if status.Service.State == adsync.SyncStateRemoved {
		return Route{Kind: RouteLocal}
	}
	switch status.Service.State {
	case adsync.SyncStateSyncing:
		if g.source.FallbackAvailable() {
			return Route{Kind: RouteFallback}
		}
		return Route{Kind: RouteUnavailable, Reason: adsync.ReasonFallbackUnavailable}
	default:
		return Route{Kind: RouteLocal}
	}
}

// Forward sends one authorization request to Cloud. The caller already
// decided that the service is syncing.
func (g *Gate) Forward(ctx context.Context, operation adsync.FallbackOperation, body []byte) (*adsync.FallbackResponse, error) {
	if !g.Enabled() {
		return nil, adsync.ErrFallbackUnavailable
	}
	return g.source.ForwardFallback(ctx, operation, body)
}

// FallbackAuditFields describes one forwarded authorization call. It records
// the operation, the service and the decision source, and deliberately leaves
// out the request body, the subject token and the instance credential.
func FallbackAuditFields(operation adsync.FallbackOperation, serviceName string) map[string]interface{} {
	return map[string]interface{}{
		"operation":      string(operation),
		"serviceName":    serviceName,
		"decisionSource": svcs.DecisionSourceCloudFallback,
	}
}

// FallbackAuditOperation is the audit operation name of a forwarded
// authorization call. It matches the name the local evaluation path uses for
// the same operation, so an audit trail shows both decision sources alike.
func FallbackAuditOperation(operation adsync.FallbackOperation) string {
	switch operation {
	case adsync.FallbackAllGrantedRoles:
		return "GetAllGrantedRoles"
	case adsync.FallbackAllGrantedPermissions:
		return "GetAllGrantedPermissions"
	default:
		return "IsAllowed"
	}
}

// StoppedReason returns the reason code of a stopped instance, defaulting to
// the heartbeat loss that caused the transition.
func StoppedReason(instance adsync.InstanceStatus) adsync.Reason {
	if instance.Reason != "" {
		return instance.Reason
	}
	return adsync.ReasonHeartbeatLost
}

// UnavailableReason maps a failed fallback call to the reason reported to the
// caller. Everything the gate cannot classify fails closed as an unavailable
// fallback.
func UnavailableReason(err error) adsync.Reason {
	switch {
	case errors.Is(err, adsync.ErrFallbackQuotaExhausted):
		return adsync.ReasonQuotaExhausted
	case errors.Is(err, adsync.ErrUnauthorized), errors.Is(err, adsync.ErrForbidden):
		return adsync.ReasonRevoked
	default:
		return adsync.ReasonFallbackUnavailable
	}
}

// ServiceReadiness is the readiness view of one tracked service.
type ServiceReadiness struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	State    adsync.SyncState `json:"state"`
	Reason   string           `json:"reason,omitempty"`
	Revision uint64           `json:"revision"`
	Head     uint64           `json:"head"`
	// Usable reports whether this service has a path to a decision right now:
	// a local copy for in_sync, or a reachable Cloud fallback for syncing.
	Usable bool `json:"usable"`
}

// Readiness is the internal readiness view. It names services and their
// revisions, so it is never written to an unauthenticated HTTP response; the
// public body is PublicReadiness.
type Readiness struct {
	Ready              bool               `json:"ready"`
	Liveness           adsync.Liveness    `json:"liveness,omitempty"`
	Reason             adsync.Reason      `json:"reason,omitempty"`
	LastConfirmedAckAt string             `json:"lastConfirmedAckAt,omitempty"`
	Services           []ServiceReadiness `json:"services,omitempty"`
}

// PublicReadiness is the unauthenticated readiness body. It answers whether
// the instance can currently serve authorization requests and reports only
// aggregate counts, so an unauthenticated caller cannot enumerate tenant
// services, their revisions or their revocation state. The internal stop
// reason stays out of this body for the same reason.
type PublicReadiness struct {
	Ready              bool            `json:"ready"`
	Liveness           adsync.Liveness `json:"liveness,omitempty"`
	LastConfirmedAckAt string          `json:"lastConfirmedAckAt,omitempty"`
	ServicesTotal      int             `json:"servicesTotal"`
	ServicesUsable     int             `json:"servicesUsable"`
}

// Public reduces the detailed readiness view to the unauthenticated body.
// Removed services are already excluded from Readiness.Services.
func (r Readiness) Public() PublicReadiness {
	public := PublicReadiness{
		Ready:              r.Ready,
		Liveness:           r.Liveness,
		LastConfirmedAckAt: r.LastConfirmedAckAt,
		ServicesTotal:      len(r.Services),
	}
	for _, service := range r.Services {
		if service.Usable {
			public.ServicesUsable++
		}
	}
	return public
}

// Ready reports whether the instance can currently answer an authorization
// request, following the distribution rules:
//
//   - a stopped instance never can;
//   - a stale instance still evaluates complete local copies but is not ready,
//     so load balancers move traffic away while heartbeats are missing;
//   - a live instance is ready when every service it serves is usable: an
//     in_sync service evaluates locally, and a syncing service is usable only
//     while the Cloud fallback can be reached.
//
// Removed services are not served at all and do not affect readiness. An
// instance without a distribution state is always ready.
func (g *Gate) Ready() Readiness {
	if !g.Enabled() {
		return Readiness{Ready: true}
	}
	instance := g.source.Instance()
	readiness := Readiness{
		Liveness: instance.Liveness,
		Reason:   instance.Reason,
	}
	if !instance.LastConfirmedAckAt.IsZero() {
		readiness.LastConfirmedAckAt = instance.LastConfirmedAckAt.UTC().Format(time.RFC3339Nano)
	}
	ready := instance.Liveness == adsync.LivenessLive
	services := g.source.Services()
	if len(services) == 0 && instance.LastConfirmedAckAt.IsZero() {
		// The service set has never been confirmed, so the instance cannot tell
		// a real service from an unknown one and cannot answer for any of them.
		ready = false
	}
	fallback := g.source.FallbackAvailable()
	for _, service := range services {
		if service.State == adsync.SyncStateRemoved {
			continue
		}
		usable := service.State == adsync.SyncStateInSync ||
			(service.State == adsync.SyncStateSyncing && fallback)
		if !usable {
			ready = false
		}
		readiness.Services = append(readiness.Services, ServiceReadiness{
			ID:       service.ID,
			Name:     service.Name,
			State:    service.State,
			Reason:   string(service.Reason),
			Revision: service.Revision,
			Head:     service.Head,
			Usable:   usable,
		})
	}
	readiness.Ready = ready
	return readiness
}
