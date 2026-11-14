// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package gate defines optional request routing for decision services. It has
// no transport or distribution dependencies. Implementations must be safe for
// concurrent calls; the Engine does not own or close an explicitly supplied gate.
package gate

import (
	"context"
	"errors"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
)

type Operation string

const (
	IsAllowed             Operation = "IsAllowed"
	AllGrantedRoles       Operation = "GetAllGrantedRoles"
	AllGrantedPermissions Operation = "GetAllGrantedPermissions"
	Diagnose              Operation = "Diagnose"
	Discover              Operation = "Discover"
)

type RouteKind int

const (
	Local       RouteKind = iota // Continue with the local evaluator.
	Fallback                     // Ask Gate.Forward; never fall back to local on failure.
	Unavailable                  // Fail closed without producing an authorization decision.
)

type Route struct {
	Kind   RouteKind
	Reason string
}

// Response is the transport-independent result of a fallback. Only the fields
// for the requested operation are used. ErrorMessage is an evaluation error:
// Allowed is normalized to false and Reason to ERROR_IN_EVALUATION. REST
// IsAllowed returns its normal error body; gRPC returns Internal.
type Response struct {
	Allowed      bool
	Reason       ads.Reason
	ErrorMessage string
	Roles        []string
	Permissions  []pms.Permission
}

// Gate chooses a route by service name before local evaluation. Forward handles
// the three decision/list operations; Diagnose and Discover cannot be forwarded.
// Both transports route before local token assertion. Forward receives the
// original identity, and implementations own fallback authentication,
// cancellation and error policy.
type Gate interface {
	Route(serviceName string) Route
	Forward(context.Context, Operation, *ads.RequestContext) (*Response, error)
}

// GateProvider is optionally implemented by a store (or evaluator).
type GateProvider interface{ RequestGate() Gate }

// Of discovers an evaluator's gate, then its backing store's gate. Evaluators
// can expose the existing DistributionStore accessor without importing gate.
func Of(evaluator any) Gate {
	if provider, ok := evaluator.(GateProvider); ok {
		return provider.RequestGate()
	}
	if provider, ok := evaluator.(interface{ DistributionStore() any }); ok {
		if store, ok := provider.DistributionStore().(GateProvider); ok {
			return store.RequestGate()
		}
	}
	return nil
}

// UnavailableError supplies a public, non-sensitive reason code. Other fallback
// errors map to fallback_unavailable; raw upstream errors are never exposed.
type UnavailableError struct{ Reason string }

func (e *UnavailableError) Error() string { return e.Reason }

func Reason(err error) string {
	var unavailable *UnavailableError
	if errors.As(err, &unavailable) && unavailable != nil && unavailable.Reason != "" {
		return unavailable.Reason
	}
	return "fallback_unavailable"
}

// Event contains no subject, token, credential or request body. Audit is called
// once for each non-local route, including unavailable and fallback failures.
type Event struct {
	Operation   Operation
	ServiceName string
	Route       RouteKind
	Reason      string
	Allowed     bool
}
type Auditor interface{ Audit(context.Context, Event) }

// Dispatch shares routing, fail-closed error mapping and audit semantics across
// REST and gRPC. A false handled result means the caller must evaluate locally.
func Dispatch(ctx context.Context, g Gate, op Operation, request *ads.RequestContext) (response *Response, handled bool, err error) {
	if g == nil {
		return nil, false, nil
	}
	route := g.Route(request.ServiceName)
	if route.Kind == Local {
		return nil, false, nil
	}
	defer func() {
		if audit, ok := g.(Auditor); ok {
			event := Event{Operation: op, ServiceName: request.ServiceName, Route: route.Kind}
			if err != nil {
				event.Reason = Reason(err)
			}
			if response != nil {
				event.Allowed = response.Allowed
				if response.ErrorMessage != "" {
					event.Reason = "evaluation_error"
				}
			}
			audit.Audit(ctx, event)
		}
	}()
	if route.Kind != Fallback {
		return nil, true, &UnavailableError{Reason: route.Reason}
	}
	if op != IsAllowed && op != AllGrantedRoles && op != AllGrantedPermissions {
		return nil, true, &UnavailableError{Reason: "fallback_unavailable"}
	}
	response, err = g.Forward(ctx, op, request)
	if err != nil {
		return nil, true, &UnavailableError{Reason: Reason(err)}
	}
	if response == nil {
		return nil, true, &UnavailableError{Reason: "fallback_unavailable"}
	}
	// Never expose a grant alongside an evaluation failure. Copy before
	// normalizing so a gate can safely reuse an immutable response.
	if response.ErrorMessage != "" {
		failed := *response
		failed.Allowed, failed.Reason = false, ads.ERROR_IN_EVALUATION
		response = &failed
	}
	return response, true, nil
}
