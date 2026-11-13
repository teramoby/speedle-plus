// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgate"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"github.com/teramoby/speedle-plus/testutil/adsyncfake"
	"github.com/teramoby/speedle-plus/testutil/storetest"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const gateTestPolicies = `{"services":[{"name":"app","type":"application",
 "policies":[
  {"id":"p1","name":"p1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"res1","actions":["read"]}]}]}]}`

func newGatedService(t *testing.T) (*GRPCService, *adsyncfake.Source) {
	t.Helper()
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(gateTestPolicies), 0600); err != nil {
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

	service, err := NewGRPCService(adsyncfake.WithGate(inner, source))
	if err != nil {
		t.Fatal(err)
	}
	return service, source
}

// recordingStream captures response metadata as a real server would.
type recordingStream struct {
	header metadata.MD
}

func (s *recordingStream) Method() string { return "/speedle.ads.Evaluator/Test" }
func (s *recordingStream) SetHeader(md metadata.MD) error {
	if s.header == nil {
		s.header = metadata.MD{}
	}
	for key, values := range md {
		s.header.Append(key, values...)
	}
	return nil
}
func (s *recordingStream) SendHeader(md metadata.MD) error { return s.SetHeader(md) }
func (s *recordingStream) SetTrailer(metadata.MD) error    { return nil }

func withRecordingStream(ctx context.Context) (context.Context, *recordingStream) {
	stream := &recordingStream{}
	return grpc.NewContextWithServerTransportStream(ctx, stream), stream
}

func gateRequest(service, user string) *pb.ContextRequest {
	return &pb.ContextRequest{
		Subject:     &pb.Subject{Principals: []*pb.Principal{{Type: "user", Name: user}}},
		ServiceName: service,
		Resource:    "res1",
		Action:      "read",
	}
}

func TestGRPCLocalServiceKeepsEvaluatingLocally(t *testing.T) {
	service, source := newGatedService(t)
	ctx, stream := withRecordingStream(context.Background())
	response, err := service.IsAllowed(ctx, gateRequest("app", "alice"))
	if err != nil {
		t.Fatal(err)
	}
	if !response.Allowed || response.Reason != int32(adsapi.GRANT_POLICY_FOUND) {
		t.Fatalf("response = %+v", response)
	}
	if len(stream.header) != 0 {
		t.Fatalf("a local decision carries metadata %v", stream.header)
	}
	if forwarded := source.Forwarded(); len(forwarded) != 0 {
		t.Fatalf("%d requests were forwarded", len(forwarded))
	}
}

func TestGRPCSyncingServiceForwards(t *testing.T) {
	service, source := newGatedService(t)
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`{"allowed":false,"reason":1}`))

	request := gateRequest("orders", "alice")
	request.Attributes = map[string]string{"level": "high"}
	ctx, stream := withRecordingStream(context.Background())
	response, err := service.IsAllowed(ctx, request)
	if err != nil {
		t.Fatalf("IsAllowed: %v", err)
	}
	if response.Allowed || response.Reason != 1 {
		t.Fatalf("response = %+v", response)
	}
	marker := stream.header.Get("x-speedle-decision-source")
	if len(marker) != 1 || marker[0] != svcs.DecisionSourceUpstream {
		t.Fatalf("metadata = %v", stream.header)
	}

	forwarded := source.Forwarded()
	if len(forwarded) != 1 || forwarded[0].Operation != adsync.FallbackIsAllowed {
		t.Fatalf("forwarded = %+v", forwarded)
	}
	var body struct {
		ServiceName string `json:"serviceName"`
		Resource    string `json:"resource"`
		Action      string `json:"action"`
		Subject     struct {
			Principals []struct{ Type, Name string } `json:"principals"`
		} `json:"subject"`
		Attributes []struct {
			Name  string      `json:"name"`
			Value interface{} `json:"value"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal(forwarded[0].Body, &body); err != nil {
		t.Fatalf("forwarded body is not JSON: %v (%s)", err, forwarded[0].Body)
	}
	if body.ServiceName != "orders" || body.Resource != "res1" || body.Action != "read" {
		t.Fatalf("forwarded body = %+v", body)
	}
	if len(body.Subject.Principals) != 1 || body.Subject.Principals[0].Name != "alice" {
		t.Fatalf("forwarded subject = %+v", body.Subject)
	}
	if len(body.Attributes) != 1 || body.Attributes[0].Name != "level" || body.Attributes[0].Value != "high" {
		t.Fatalf("forwarded attributes = %+v", body.Attributes)
	}
}

func TestGRPCSyncingGrantedOperationsMapCloudResponses(t *testing.T) {
	service, source := newGatedService(t)

	source.SetFallbackResponse(http.StatusOK, nil, []byte(`["admin","editor"]`))
	roles, err := service.GetAllGrantedRoles(context.Background(), gateRequest("orders", "alice"))
	if err != nil || len(roles.Roles) != 2 || roles.Roles[0] != "admin" || roles.Roles[1] != "editor" {
		t.Fatalf("roles = %+v, error %v", roles, err)
	}

	source.SetFallbackResponse(http.StatusOK, nil, []byte(`[]`))
	emptyRoles, err := service.GetAllGrantedRoles(context.Background(), gateRequest("orders", "alice"))
	if err != nil || emptyRoles.Roles == nil || len(emptyRoles.Roles) != 0 {
		t.Fatalf("empty roles = %+v, error %v", emptyRoles, err)
	}

	source.SetFallbackResponse(http.StatusOK, nil, []byte(`[{"resource":"res1","actions":["read","write"]}]`))
	permissions, err := service.GetAllPermissions(context.Background(), gateRequest("orders", "alice"))
	if err != nil || len(permissions.Permissions) != 1 ||
		permissions.Permissions[0].Resource != "res1" || len(permissions.Permissions[0].Actions) != 2 {
		t.Fatalf("permissions = %+v, error %v", permissions, err)
	}

	forwarded := source.Forwarded()
	if len(forwarded) != 3 ||
		forwarded[0].Operation != adsync.FallbackAllGrantedRoles ||
		forwarded[1].Operation != adsync.FallbackAllGrantedRoles ||
		forwarded[2].Operation != adsync.FallbackAllGrantedPermissions {
		t.Fatalf("forwarded = %+v", forwarded)
	}
}

func TestGRPCStoppedInstanceAnswersUnavailable(t *testing.T) {
	service, source := newGatedService(t)
	source.SetInstance(adsync.InstanceStatus{Liveness: adsync.LivenessStopped, Reason: adsync.ReasonRevoked})
	ctx := context.Background()

	calls := map[string]func() error{
		"IsAllowed": func() error {
			_, err := service.IsAllowed(ctx, gateRequest("app", "alice"))
			return err
		},
		"GetAllGrantedRoles": func() error {
			_, err := service.GetAllGrantedRoles(ctx, gateRequest("app", "alice"))
			return err
		},
		"GetAllPermissions": func() error {
			_, err := service.GetAllPermissions(ctx, gateRequest("app", "alice"))
			return err
		},
		"Diagnose": func() error {
			_, err := service.Diagnose(ctx, gateRequest("app", "alice"))
			return err
		},
		"Discover": func() error {
			_, err := service.Discover(ctx, gateRequest("app", "alice"))
			return err
		},
	}
	for name, call := range calls {
		err := call()
		if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != string(adsync.ReasonRevoked) {
			t.Fatalf("%s: error = %v", name, err)
		}
	}
	if forwarded := source.Forwarded(); len(forwarded) != 0 {
		t.Fatalf("%d requests were forwarded", len(forwarded))
	}
}

func TestGRPCSyncingWithoutFallbackAnswersUnavailable(t *testing.T) {
	service, source := newGatedService(t)
	source.SetFallbackAvailable(false)
	_, err := service.IsAllowed(context.Background(), gateRequest("orders", "alice"))
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != string(adsync.ReasonFallbackUnavailable) {
		t.Fatalf("error = %v", err)
	}

	for name, call := range map[string]func() error{
		"Diagnose": func() error {
			_, err := service.Diagnose(context.Background(), gateRequest("orders", "alice"))
			return err
		},
		"Discover": func() error {
			_, err := service.Discover(context.Background(), gateRequest("orders", "alice"))
			return err
		},
	} {
		err := call()
		if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != string(adsync.ReasonFallbackUnavailable) {
			t.Fatalf("%s: error = %v", name, err)
		}
	}
}

func TestGRPCForwardFailuresMapToReasonCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		reason adsync.Reason
	}{
		{"quota", adsync.ErrFallbackQuotaExhausted, adsync.ReasonQuotaExhausted},
		{"revoked", adsync.ErrForbidden, adsync.ReasonRevoked},
		{"unavailable", adsync.ErrUnavailable, adsync.ReasonFallbackUnavailable},
		{"other", errors.New("boom"), adsync.ReasonFallbackUnavailable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service, source := newGatedService(t)
			source.SetFallbackError(testCase.err)
			_, err := service.IsAllowed(context.Background(), gateRequest("orders", "alice"))
			if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != string(testCase.reason) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGRPCCloudEvaluationErrorIsNotADecision(t *testing.T) {
	service, source := newGatedService(t)
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`{"allowed":false,"reason":4,"errorMessage":"SPDL-2002 evaluation failed"}`))
	response, err := service.IsAllowed(context.Background(), gateRequest("orders", "alice"))
	if err == nil {
		t.Fatalf("Cloud evaluation error became a decision: %+v", response)
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %s", status.Code(err))
	}
}

func TestGRPCInvalidCloudResponseFailsClosed(t *testing.T) {
	service, source := newGatedService(t)
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`not json`))
	if _, err := service.IsAllowed(context.Background(), gateRequest("orders", "alice")); status.Code(err) != codes.Internal {
		t.Fatalf("error = %v", err)
	}
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`not json`))
	if _, err := service.GetAllGrantedRoles(context.Background(), gateRequest("orders", "alice")); status.Code(err) != codes.Internal {
		t.Fatalf("roles error = %v", err)
	}
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`not json`))
	if _, err := service.GetAllPermissions(context.Background(), gateRequest("orders", "alice")); status.Code(err) != codes.Internal {
		t.Fatalf("permissions error = %v", err)
	}
}

func TestGRPCRemovedAndUnknownServicesUseTheLocalPath(t *testing.T) {
	service, source := newGatedService(t)
	for _, name := range []string{"legacy", "unknown"} {
		t.Run(name, func(t *testing.T) {
			response, err := service.IsAllowed(context.Background(), gateRequest(name, "alice"))
			if err == nil {
				t.Fatalf("unknown service produced a decision: %+v", response)
			}
			if forwarded := source.Forwarded(); len(forwarded) != 0 {
				t.Fatalf("%d requests were forwarded", len(forwarded))
			}
		})
	}
}

// captureAuditLog redirects the shared audit logger to a JSON buffer for one
// test.
func captureAuditLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	logger := logging.AuditLog()
	out, formatter, level := logger.Out, logger.Formatter, logger.Level
	var buffer bytes.Buffer
	logger.Out = &buffer
	logger.Formatter = &log.JSONFormatter{}
	logger.Level = log.DebugLevel
	t.Cleanup(func() {
		logger.Out, logger.Formatter, logger.Level = out, formatter, level
	})
	return &buffer
}

func lastAuditEntry(t *testing.T, out *bytes.Buffer) map[string]interface{} {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 0 || lines[len(lines)-1] == "" {
		t.Fatal("no audit entry was written")
	}
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatalf("audit entry %q: %v", lines[len(lines)-1], err)
	}
	return entry
}

func TestGRPCFallbackDecisionsAreAudited(t *testing.T) {
	out := captureAuditLog(t)
	service, source := newGatedService(t)

	cases := []struct {
		name      string
		operation adsync.FallbackOperation
		cloudBody string
		call      func(*pb.ContextRequest) error
		resultKey string
	}{
		{"is-allowed", adsync.FallbackIsAllowed, `{"allowed":true,"reason":2}`,
			func(in *pb.ContextRequest) error { _, err := service.IsAllowed(context.Background(), in); return err }, "allowed"},
		{"roles", adsync.FallbackAllGrantedRoles, `["admin"]`,
			func(in *pb.ContextRequest) error {
				_, err := service.GetAllGrantedRoles(context.Background(), in)
				return err
			}, "roles"},
		{"permissions", adsync.FallbackAllGrantedPermissions, `[{"resource":"res1","actions":["read"]}]`,
			func(in *pb.ContextRequest) error {
				_, err := service.GetAllPermissions(context.Background(), in)
				return err
			}, "permissions"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			source.SetFallbackResponse(http.StatusOK, nil, []byte(testCase.cloudBody))
			if err := testCase.call(gateRequest("orders", "alice")); err != nil {
				t.Fatalf("call: %v", err)
			}

			entry := lastAuditEntry(t, out)
			auditName := "[gRPC]" + adsgate.FallbackAuditOperation(testCase.operation)
			if entry["msg"] != "[AUDIT]"+auditName || entry["level"] != "info" || entry["response"] != "succeeded" {
				t.Fatalf("audit entry = %v", entry)
			}
			requestFields, _ := entry["requestValue"].(map[string]interface{})
			if requestFields["serviceName"] != "orders" || requestFields["operation"] != string(testCase.operation) ||
				requestFields["decisionSource"] != svcs.DecisionSourceUpstream {
				t.Fatalf("audit context = %v", entry)
			}
			responseFields, _ := entry["responseValue"].(map[string]interface{})
			if _, found := responseFields[testCase.resultKey]; !found {
				t.Fatalf("audit result %q missing from %v", testCase.resultKey, entry)
			}
		})
	}
}

func TestGRPCFallbackFailuresAreAudited(t *testing.T) {
	out := captureAuditLog(t)
	service, source := newGatedService(t)
	source.SetFallbackError(adsync.ErrUnavailable)

	if _, err := service.IsAllowed(context.Background(), gateRequest("orders", "alice")); status.Code(err) != codes.Unavailable {
		t.Fatalf("error = %v", err)
	}

	entry := lastAuditEntry(t, out)
	if entry["msg"] != "[AUDIT][gRPC]IsAllowed" || entry["level"] != "error" || entry["response"] != "failed" {
		t.Fatalf("audit entry = %v", entry)
	}
	requestFields, _ := entry["requestValue"].(map[string]interface{})
	if requestFields["serviceName"] != "orders" || requestFields["decisionSource"] != svcs.DecisionSourceUpstream {
		t.Fatalf("audit context = %v", entry)
	}
	if reason, _ := entry["reason"].(string); reason == "" {
		t.Fatalf("audit reason = %v", entry["reason"])
	}
}

func TestGRPCDirectModeMarksForwardedDecisionsAsUpstream(t *testing.T) {
	service, source := newGatedService(t)
	source.SetForwardWhileStale(true)
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`{"allowed":true,"reason":0}`))
	ctx, stream := withRecordingStream(context.Background())
	if _, err := service.IsAllowed(ctx, gateRequest("orders", "alice")); err != nil {
		t.Fatal(err)
	}
	if marker := stream.header.Get("x-speedle-decision-source"); len(marker) != 1 || marker[0] != svcs.DecisionSourceUpstream {
		t.Fatalf("metadata = %v", stream.header)
	}
}
