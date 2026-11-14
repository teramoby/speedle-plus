// Licensed under the Universal Permissive License (UPL) Version 1.0.
package gate_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/svcs/gate"
)

type example struct {
	route     gate.Route
	response  *gate.Response
	err       error
	forwarded bool
	event     gate.Event
}

func (g *example) Route(string) gate.Route { return g.route }
func (g *example) Forward(ctx context.Context, _ gate.Operation, _ *ads.RequestContext) (*gate.Response, error) {
	g.forwarded = true
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return g.response, g.err
}
func (g *example) Audit(_ context.Context, event gate.Event) { g.event = event }
func (g *example) RequestGate() gate.Gate                    { return g }

type backing struct{ store any }

func (e backing) DistributionStore() any { return e.store }

func TestDiscovery(t *testing.T) {
	g := &example{}
	for _, source := range []any{g, backing{g}} {
		if gate.Of(source) != g {
			t.Fatalf("no discovery for %T", source)
		}
	}
	for _, source := range []any{nil, struct{}{}, backing{nil}, backing{struct{}{}}} {
		if gate.Of(source) != nil {
			t.Fatalf("unexpected gate for %T", source)
		}
	}
}

func TestRoutingAndFailurePolicy(t *testing.T) {
	request := &ads.RequestContext{ServiceName: "app"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name               string
		route              gate.Route
		response           *gate.Response
		err                error
		op                 gate.Operation
		ctx                context.Context
		handled, forwarded bool
		reason             string
	}{
		{name: "local", op: gate.IsAllowed},
		{name: "deny route", route: gate.Route{Kind: gate.Unavailable, Reason: "revoked"}, op: gate.IsAllowed, handled: true, reason: "revoked"},
		{name: "unknown route", route: gate.Route{Kind: 99}, op: gate.IsAllowed, handled: true, reason: "fallback_unavailable"},
		{name: "allow", route: gate.Route{Kind: gate.Fallback}, response: &gate.Response{Allowed: true}, op: gate.IsAllowed, handled: true, forwarded: true},
		{name: "nil response", route: gate.Route{Kind: gate.Fallback}, op: gate.IsAllowed, handled: true, forwarded: true, reason: "fallback_unavailable"},
		{name: "generic error", route: gate.Route{Kind: gate.Fallback}, err: errors.New("private data"), op: gate.IsAllowed, handled: true, forwarded: true, reason: "fallback_unavailable"},
		{name: "wrapped reason", route: gate.Route{Kind: gate.Fallback}, err: fmt.Errorf("wrapped: %w", &gate.UnavailableError{Reason: "revoked"}), op: gate.IsAllowed, handled: true, forwarded: true, reason: "revoked"},
		{name: "cancellation", route: gate.Route{Kind: gate.Fallback}, op: gate.IsAllowed, ctx: ctx, handled: true, forwarded: true, reason: "fallback_unavailable"},
		{name: "diagnose", route: gate.Route{Kind: gate.Fallback}, op: gate.Diagnose, handled: true, reason: "fallback_unavailable"},
		{name: "discover", route: gate.Route{Kind: gate.Fallback}, op: gate.Discover, handled: true, reason: "fallback_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &example{route: tc.route, response: tc.response, err: tc.err}
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			response, handled, err := gate.Dispatch(ctx, g, tc.op, request)
			if handled != tc.handled || g.forwarded != tc.forwarded {
				t.Fatalf("handled=%v forwarded=%v", handled, g.forwarded)
			}
			if tc.reason != "" {
				if err == nil || gate.Reason(err) != tc.reason {
					t.Fatalf("err=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if handled && (g.event.Operation != tc.op || g.event.ServiceName != "app" || g.event.Reason != tc.reason) {
				t.Fatalf("audit %+v", g.event)
			}
			if response != nil && response != tc.response {
				t.Fatal("response replaced")
			}
		})
	}
	if _, handled, err := gate.Dispatch(context.Background(), nil, gate.IsAllowed, request); handled || err != nil {
		t.Fatal("nil gate must be local")
	}
	if (&gate.UnavailableError{Reason: "revoked"}).Error() != "revoked" {
		t.Fatal("reason error")
	}
}
