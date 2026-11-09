// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsgate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/testutil/adsyncfake"
)

func TestNilGateRoutesLocally(t *testing.T) {
	var gate *Gate
	if gate.Enabled() {
		t.Fatal("a nil gate is enabled")
	}
	if route := gate.Route("books"); route.Kind != RouteLocal {
		t.Fatalf("route = %+v", route)
	}
	if _, err := gate.Forward(context.Background(), adsync.FallbackIsAllowed, []byte("{}")); !errors.Is(err, adsync.ErrFallbackUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if readiness := gate.Ready(); !readiness.Ready {
		t.Fatal("a service without a distribution state is not ready")
	}
}

func TestRoutingUsesThePerServiceState(t *testing.T) {
	source := adsyncfake.New()
	source.SetService(adsync.ServiceStatus{Name: "books", State: adsync.SyncStateInSync})
	source.SetService(adsync.ServiceStatus{Name: "orders", State: adsync.SyncStateSyncing, Reason: adsync.SyncReasonCatchUp})
	source.SetService(adsync.ServiceStatus{Name: "legacy", State: adsync.SyncStateRemoved})
	gate := New(source)

	cases := []struct {
		service string
		kind    RouteKind
		reason  adsync.Reason
	}{
		{"books", RouteLocal, ""},
		{"orders", RouteFallback, ""},
		{"legacy", RouteLocal, ""},
		{"unknown", RouteLocal, ""},
		{"", RouteLocal, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.service, func(t *testing.T) {
			route := gate.Route(testCase.service)
			if route.Kind != testCase.kind || route.Reason != testCase.reason {
				t.Fatalf("route = %+v, want kind %d reason %q", route, testCase.kind, testCase.reason)
			}
		})
	}
}

func TestUnknownServiceWithAPendingRemovalFailsClosed(t *testing.T) {
	source := adsyncfake.New()
	source.SetService(adsync.ServiceStatus{Name: "books-v2", State: adsync.SyncStateInSync})
	// The old name is unknown to the machine because the service was renamed,
	// but its copy is still installed until the new assembly is acknowledged.
	source.SetRemovalPending("books", true)
	gate := New(source)

	if route := gate.Route("books"); route.Kind != RouteUnavailable || route.Reason != adsync.ReasonSourceGone {
		t.Fatalf("route for the previous name = %+v", route)
	}
	if route := gate.Route("unknown"); route.Kind != RouteLocal {
		t.Fatalf("route for an unknown name = %+v", route)
	}
	source.SetRemovalPending("books", false)
	if route := gate.Route("books"); route.Kind != RouteLocal {
		t.Fatalf("route after the removal installed = %+v", route)
	}
}

// TestInSyncServiceWithAPendingRemovalFailsClosed covers the window inside a
// synchronization pass between recording a discovery removal or rename and
// updating the state machine: the machine still reports the old name in_sync,
// but the installed assembly no longer matches the authoritative set.
func TestInSyncServiceWithAPendingRemovalFailsClosed(t *testing.T) {
	source := adsyncfake.New()
	source.SetService(adsync.ServiceStatus{Name: "books", State: adsync.SyncStateInSync})
	source.SetRemovalPending("books", true)
	gate := New(source)

	if route := gate.Route("books"); route.Kind != RouteUnavailable || route.Reason != adsync.ReasonSourceGone {
		t.Fatalf("route for an in_sync service with a pending removal = %+v", route)
	}
	source.SetRemovalPending("books", false)
	if route := gate.Route("books"); route.Kind != RouteLocal {
		t.Fatalf("route after the removal was installed = %+v", route)
	}
}

func TestSyncingWithoutFallbackIsUnavailable(t *testing.T) {
	source := adsyncfake.New()
	source.SetService(adsync.ServiceStatus{Name: "books", State: adsync.SyncStateSyncing})
	source.SetFallbackAvailable(false)
	gate := New(source)
	route := gate.Route("books")
	if route.Kind != RouteUnavailable || route.Reason != adsync.ReasonFallbackUnavailable {
		t.Fatalf("route = %+v", route)
	}
}

func TestStoppedInstanceWinsOverServiceState(t *testing.T) {
	source := adsyncfake.New()
	source.SetInstance(adsync.InstanceStatus{Liveness: adsync.LivenessStopped, Reason: adsync.ReasonRevoked})
	source.SetService(adsync.ServiceStatus{Name: "books", State: adsync.SyncStateInSync})
	gate := New(source)

	for _, service := range []string{"books", "unknown", ""} {
		route := gate.Route(service)
		if route.Kind != RouteUnavailable || route.Reason != adsync.ReasonRevoked {
			t.Fatalf("route for %q = %+v", service, route)
		}
	}

	// A stopped instance without an explicit reason reports the heartbeat loss
	// that caused the stop.
	source.SetInstance(adsync.InstanceStatus{Liveness: adsync.LivenessStopped})
	if route := gate.Route("books"); route.Reason != adsync.ReasonHeartbeatLost {
		t.Fatalf("reason = %q", route.Reason)
	}
}

func TestStaleInstanceStillRoutesByServiceState(t *testing.T) {
	source := adsyncfake.New()
	source.SetInstance(adsync.InstanceStatus{Liveness: adsync.LivenessStale})
	source.SetService(adsync.ServiceStatus{Name: "books", State: adsync.SyncStateInSync})
	gate := New(source)
	if route := gate.Route("books"); route.Kind != RouteLocal {
		t.Fatalf("route = %+v", route)
	}
	if readiness := gate.Ready(); readiness.Ready {
		t.Fatal("a stale instance reported ready")
	}
}

func TestReadyFollowsTheReadinessRules(t *testing.T) {
	cases := []struct {
		name     string
		instance adsync.InstanceStatus
		services []adsync.ServiceStatus
		fallback bool
		want     bool
	}{
		{
			name:     "live with an in_sync service",
			instance: adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			services: []adsync.ServiceStatus{{Name: "books", State: adsync.SyncStateInSync}},
			fallback: true,
			want:     true,
		},
		{
			name:     "live with a syncing service and a reachable fallback",
			instance: adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			services: []adsync.ServiceStatus{{Name: "books", State: adsync.SyncStateSyncing}},
			fallback: true,
			want:     true,
		},
		{
			name:     "live with a syncing service and no fallback",
			instance: adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			services: []adsync.ServiceStatus{{Name: "books", State: adsync.SyncStateSyncing}},
			fallback: false,
			want:     false,
		},
		{
			name:     "live with one unusable service",
			instance: adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			services: []adsync.ServiceStatus{
				{Name: "books", State: adsync.SyncStateInSync},
				{Name: "orders", State: adsync.SyncStateSyncing},
			},
			fallback: false,
			want:     false,
		},
		{
			name:     "stale with complete copies",
			instance: adsync.InstanceStatus{Liveness: adsync.LivenessStale},
			services: []adsync.ServiceStatus{{Name: "books", State: adsync.SyncStateInSync}},
			fallback: true,
			want:     false,
		},
		{
			name:     "stopped",
			instance: adsync.InstanceStatus{Liveness: adsync.LivenessStopped, Reason: adsync.ReasonHeartbeatLost},
			services: []adsync.ServiceStatus{{Name: "books", State: adsync.SyncStateInSync}},
			fallback: true,
			want:     false,
		},
		{
			name:     "only removed tombstones",
			instance: adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			services: []adsync.ServiceStatus{{Name: "legacy", State: adsync.SyncStateRemoved}},
			fallback: false,
			want:     true,
		},
		{
			name:     "live before the first confirmed ack",
			instance: adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			fallback: true,
			want:     false,
		},
		{
			name: "an empty service set confirmed by an ack",
			instance: adsync.InstanceStatus{
				Liveness:           adsync.LivenessLive,
				LastConfirmedAckAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
			},
			fallback: true,
			want:     true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			source := adsyncfake.New()
			source.SetInstance(testCase.instance)
			for _, service := range testCase.services {
				source.SetService(service)
			}
			source.SetFallbackAvailable(testCase.fallback)
			readiness := New(source).Ready()
			if readiness.Ready != testCase.want {
				t.Fatalf("ready = %v, want %v (%+v)", readiness.Ready, testCase.want, readiness)
			}
			if readiness.Liveness != testCase.instance.Liveness {
				t.Fatalf("liveness = %q", readiness.Liveness)
			}
		})
	}
}

func TestReadyReportsServiceDetails(t *testing.T) {
	source := adsyncfake.New()
	lastAck := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	source.SetInstance(adsync.InstanceStatus{
		Liveness:           adsync.LivenessLive,
		LastConfirmedAckAt: lastAck,
		Reason:             adsync.ReasonQuotaExhausted,
	})
	source.SetService(adsync.ServiceStatus{
		ID: "01HF7YAT000000000000000101", Name: "books",
		State: adsync.SyncStateSyncing, Reason: adsync.SyncReasonCatchUp,
		Revision: 3, Head: 5,
	})
	source.SetService(adsync.ServiceStatus{Name: "legacy", State: adsync.SyncStateRemoved})

	readiness := New(source).Ready()
	if len(readiness.Services) != 1 {
		t.Fatalf("services = %+v", readiness.Services)
	}
	service := readiness.Services[0]
	if service.Name != "books" || service.State != adsync.SyncStateSyncing ||
		service.Reason != string(adsync.SyncReasonCatchUp) || service.Revision != 3 ||
		service.Head != 5 || !service.Usable {
		t.Fatalf("service readiness = %+v", service)
	}
	if readiness.LastConfirmedAckAt != "2026-11-08T12:00:00Z" {
		t.Fatalf("lastConfirmedAckAt = %q", readiness.LastConfirmedAckAt)
	}
	if readiness.Reason != adsync.ReasonQuotaExhausted {
		t.Fatalf("reason = %q", readiness.Reason)
	}
}

func TestPublicReadinessReportsAggregatesOnly(t *testing.T) {
	source := adsyncfake.New()
	source.SetInstance(adsync.InstanceStatus{
		Liveness:           adsync.LivenessLive,
		LastConfirmedAckAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
	})
	source.SetService(adsync.ServiceStatus{Name: "books", State: adsync.SyncStateInSync})
	source.SetService(adsync.ServiceStatus{Name: "orders", State: adsync.SyncStateSyncing})
	source.SetService(adsync.ServiceStatus{Name: "legacy", State: adsync.SyncStateRemoved})

	public := New(source).Ready().Public()
	if !public.Ready || public.Liveness != adsync.LivenessLive {
		t.Fatalf("public readiness = %+v", public)
	}
	if public.ServicesTotal != 2 || public.ServicesUsable != 2 {
		t.Fatalf("public readiness counts = %+v", public)
	}
	if public.LastConfirmedAckAt != "2026-11-08T12:00:00Z" {
		t.Fatalf("lastConfirmedAckAt = %q", public.LastConfirmedAckAt)
	}

	source.SetFallbackAvailable(false)
	public = New(source).Ready().Public()
	if public.Ready || public.ServicesUsable != 1 {
		t.Fatalf("public readiness without fallback = %+v", public)
	}
}

func TestUnavailableReason(t *testing.T) {
	cases := map[error]adsync.Reason{
		adsync.ErrFallbackQuotaExhausted:   adsync.ReasonQuotaExhausted,
		adsync.ErrUnauthorized:             adsync.ReasonRevoked,
		adsync.ErrForbidden:                adsync.ReasonRevoked,
		adsync.ErrUnavailable:              adsync.ReasonFallbackUnavailable,
		adsync.ErrFallbackUnavailable:      adsync.ReasonFallbackUnavailable,
		errors.New("unclassified failure"): adsync.ReasonFallbackUnavailable,
	}
	for err, want := range cases {
		if got := UnavailableReason(err); got != want {
			t.Fatalf("%v: reason = %q, want %q", err, got, want)
		}
	}
}
