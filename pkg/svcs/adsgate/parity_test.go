// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsgate_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/etcd"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
	"github.com/teramoby/speedle-plus/testutil/adsyncfake"
	"github.com/teramoby/speedle-plus/testutil/storetest"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const parityPolicies = `{"services":[{"name":"app","type":"application",
 "policies":[
  {"id":"p1","name":"p1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"res1","actions":["read"]}]}]}]}`

// outcome is the semantic result a PEP observes, independent of the transport.
type outcome struct {
	kind   string // allow, deny, not_found, unavailable
	reason string
}

func newParitySurfaces(t *testing.T) (http.Handler, *adsgrpc.GRPCService, *adsyncfake.Source) {
	t.Helper()
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(parityPolicies), 0600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	manager := storetest.PolicyStore(t, location)
	inner, err := eval.NewWithStore(&cfg.Config{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = inner.(*eval.PolicyEvalImpl).Close() })

	source := adsyncfake.New()
	source.SetService(adsync.ServiceStatus{Name: "app", State: adsync.SyncStateInSync})
	source.SetService(adsync.ServiceStatus{Name: "orders", State: adsync.SyncStateSyncing})
	source.SetService(adsync.ServiceStatus{Name: "legacy", State: adsync.SyncStateRemoved})

	gated := adsyncfake.WithGate(inner, source)
	router, err := adsrest.NewRouter(gated)
	if err != nil {
		t.Fatal(err)
	}
	grpcService, err := adsgrpc.NewGRPCService(gated)
	if err != nil {
		t.Fatal(err)
	}
	return router, grpcService, source
}

func restOutcome(t *testing.T, router http.Handler, serviceName string) outcome {
	t.Helper()
	body := `{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"` +
		serviceName + `","resource":"res1","action":"read"}`
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, svcs.PolicyAtzPath+"is-allowed", strings.NewReader(body)))

	if rec.Code == http.StatusServiceUnavailable {
		var response struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("REST 503 body: %v (%s)", err, rec.Body.String())
		}
		return outcome{kind: "unavailable", reason: response.Reason}
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("REST status = %d (%s)", rec.Code, rec.Body.String())
	}
	var decision adsrest.IsAllowedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
		t.Fatalf("REST body: %v (%s)", err, rec.Body.String())
	}
	switch {
	case decision.Allowed:
		return outcome{kind: "allow"}
	case decision.Reason == int32(adsapi.SERVICE_NOT_FOUND):
		return outcome{kind: "not_found"}
	default:
		return outcome{kind: "deny"}
	}
}

func grpcOutcome(t *testing.T, service *adsgrpc.GRPCService, serviceName string) outcome {
	t.Helper()
	response, err := service.IsAllowed(context.Background(), &pb.ContextRequest{
		Subject:     &pb.Subject{Principals: []*pb.Principal{{Type: "user", Name: "alice"}}},
		ServiceName: serviceName,
		Resource:    "res1",
		Action:      "read",
	})
	if err != nil {
		if status.Code(err) == codes.Unavailable {
			return outcome{kind: "unavailable", reason: status.Convert(err).Message()}
		}
		if strings.Contains(err.Error(), "not found") {
			return outcome{kind: "not_found"}
		}
		t.Fatalf("gRPC error = %v", err)
	}
	if response.Allowed {
		return outcome{kind: "allow"}
	}
	return outcome{kind: "deny"}
}

func TestRESTAndGRPCMakeTheSameRoutingDecision(t *testing.T) {
	cases := []struct {
		name              string
		service           string
		instance          adsync.InstanceStatus
		fallbackAvailable bool
		fallbackBody      string
		fallbackErr       error
		want              outcome
	}{
		{
			name:              "in_sync evaluates locally",
			service:           "app",
			instance:          adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			fallbackAvailable: true,
			want:              outcome{kind: "allow"},
		},
		{
			name:              "syncing forwards the Cloud decision",
			service:           "orders",
			instance:          adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			fallbackAvailable: true,
			fallbackBody:      `{"allowed":true,"reason":0}`,
			want:              outcome{kind: "allow"},
		},
		{
			name:              "syncing without a fallback fails closed",
			service:           "orders",
			instance:          adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			fallbackAvailable: false,
			want:              outcome{kind: "unavailable", reason: string(adsync.ReasonFallbackUnavailable)},
		},
		{
			name:              "stopped fails closed with its reason",
			service:           "app",
			instance:          adsync.InstanceStatus{Liveness: adsync.LivenessStopped, Reason: adsync.ReasonRevoked},
			fallbackAvailable: true,
			want:              outcome{kind: "unavailable", reason: string(adsync.ReasonRevoked)},
		},
		{
			name:              "quota refusal fails closed",
			service:           "orders",
			instance:          adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			fallbackAvailable: true,
			fallbackErr:       adsync.ErrFallbackQuotaExhausted,
			want:              outcome{kind: "unavailable", reason: string(adsync.ReasonQuotaExhausted)},
		},
		{
			name:              "removed follows the local path",
			service:           "legacy",
			instance:          adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			fallbackAvailable: true,
			want:              outcome{kind: "not_found"},
		},
		{
			name:              "unknown follows the local path",
			service:           "missing",
			instance:          adsync.InstanceStatus{Liveness: adsync.LivenessLive},
			fallbackAvailable: true,
			want:              outcome{kind: "not_found"},
		},
	}

	router, grpcService, source := newParitySurfaces(t)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			source.SetInstance(testCase.instance)
			source.SetFallbackAvailable(testCase.fallbackAvailable)
			source.SetFallbackError(testCase.fallbackErr)
			source.SetFallbackResponse(http.StatusOK, nil, []byte(testCase.fallbackBody))

			rest := restOutcome(t, router, testCase.service)
			grpcResult := grpcOutcome(t, grpcService, testCase.service)
			if rest != testCase.want {
				t.Fatalf("REST outcome = %+v, want %+v", rest, testCase.want)
			}
			if grpcResult != testCase.want {
				t.Fatalf("gRPC outcome = %+v, want %+v", grpcResult, testCase.want)
			}
		})
	}
}

func TestRESTAndGRPCForwardTheSameOperation(t *testing.T) {
	router, grpcService, source := newParitySurfaces(t)
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`{"allowed":true,"reason":0}`))

	if got := restOutcome(t, router, "orders"); got.kind != "allow" {
		t.Fatalf("REST outcome = %+v", got)
	}
	if got := grpcOutcome(t, grpcService, "orders"); got.kind != "allow" {
		t.Fatalf("gRPC outcome = %+v", got)
	}
	forwarded := source.Forwarded()
	if len(forwarded) != 2 {
		t.Fatalf("%d forwarded requests, want 2", len(forwarded))
	}
	for index, call := range forwarded {
		if call.Operation != adsync.FallbackIsAllowed {
			t.Fatalf("call %d operation = %q", index, call.Operation)
		}
		if !strings.Contains(string(call.Body), `"serviceName":"orders"`) {
			t.Fatalf("call %d body = %s", index, call.Body)
		}
	}
}
