// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsrest

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	"github.com/teramoby/speedle-plus/testutil/adsyncfake"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

// newGatedRouter builds the REST surface over a real file-backed evaluator and
// a controllable distribution source.
func newGatedRouter(t *testing.T) (http.Handler, *adsyncfake.Source) {
	t.Helper()
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(handlerPolicies), 0600); err != nil {
		t.Fatal(err)
	}
	// The file store writes discover requests to the working directory.
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

	router, err := NewRouter(adsyncfake.WithGate(inner, source))
	if err != nil {
		t.Fatal(err)
	}
	return router, source
}

func postRaw(t *testing.T, router http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, svcs.PolicyAtzPath+path, strings.NewReader(body)))
	return rec
}

const allowedBody = `{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"app","resource":"res1","action":"read"}`

func TestLocalServicesKeepEvaluatingLocally(t *testing.T) {
	router, source := newGatedRouter(t)
	rec := postRaw(t, router, "is-allowed", allowedBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var response IsAllowedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Allowed || response.Reason != int32(adsapi.GRANT_POLICY_FOUND) {
		t.Fatalf("response = %+v", response)
	}
	if rec.Header().Get(svcs.DecisionSourceHeader) != "" {
		t.Fatal("a local decision carries a decision-source marker")
	}
	if forwarded := source.Forwarded(); len(forwarded) != 0 {
		t.Fatalf("%d requests were forwarded", len(forwarded))
	}
}

func TestSyncingServiceForwardsTheRequestUnchanged(t *testing.T) {
	router, source := newGatedRouter(t)
	cloudBody := []byte("{\"allowed\":false,\"reason\":1}\n")
	source.SetFallbackResponse(http.StatusOK, http.Header{
		"Content-Type":  []string{"application/json"},
		"X-Cloud-Trace": []string{"trace-1"},
	}, cloudBody)

	// Unusual spacing and a trailing newline must survive the forward.
	body := `{ "serviceName" : "orders", "resource" : "order:1", "action" : "read" }` + "\n"
	rec := postRaw(t, router, "is-allowed", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != string(cloudBody) {
		t.Fatalf("body %q, want the Cloud bytes %q", got, cloudBody)
	}
	if rec.Header().Get(svcs.DecisionSourceHeader) != svcs.DecisionSourceUpstream {
		t.Fatalf("marker = %q", rec.Header().Get(svcs.DecisionSourceHeader))
	}
	if rec.Header().Get("X-Cloud-Trace") != "trace-1" {
		t.Fatal("Cloud response headers were not passed through")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}

	forwarded := source.Forwarded()
	if len(forwarded) != 1 {
		t.Fatalf("%d forwarded requests, want 1", len(forwarded))
	}
	if forwarded[0].Operation != adsync.FallbackIsAllowed {
		t.Fatalf("operation = %q", forwarded[0].Operation)
	}
	if string(forwarded[0].Body) != body {
		t.Fatalf("forwarded body %q, want %q", forwarded[0].Body, body)
	}
}

func TestRemovedAndUnknownServicesFollowTheLocalPath(t *testing.T) {
	router, source := newGatedRouter(t)
	for _, serviceName := range []string{"legacy", "unknown"} {
		t.Run(serviceName, func(t *testing.T) {
			body := `{"serviceName":"` + serviceName + `","resource":"res1","action":"read"}`
			rec := postRaw(t, router, "is-allowed", body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
			}
			var response IsAllowedResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Allowed || response.Reason != int32(adsapi.SERVICE_NOT_FOUND) || response.ErrorMessage == "" {
				t.Fatalf("response = %+v", response)
			}
			if forwarded := source.Forwarded(); len(forwarded) != 0 {
				t.Fatalf("%d requests were forwarded", len(forwarded))
			}
		})
	}
}

func TestStoppedInstanceAnswers503InsteadOfADecision(t *testing.T) {
	router, source := newGatedRouter(t)
	source.SetInstance(adsync.InstanceStatus{Liveness: adsync.LivenessStopped, Reason: adsync.ReasonRevoked})

	rec := postRaw(t, router, "is-allowed", allowedBody)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var response UnavailableResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != "authorization_unavailable" || response.Reason != string(adsync.ReasonRevoked) {
		t.Fatalf("response = %+v", response)
	}
	if strings.Contains(rec.Body.String(), "errorMessage") {
		t.Fatalf("stopped state returned an evaluation response: %s", rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("headers = %v", rec.Header())
	}
	if forwarded := source.Forwarded(); len(forwarded) != 0 {
		t.Fatalf("%d requests were forwarded", len(forwarded))
	}
}

func TestSyncingWithoutAFallbackAnswers503(t *testing.T) {
	router, source := newGatedRouter(t)
	source.SetFallbackAvailable(false)
	body := `{"serviceName":"orders","resource":"res1","action":"read"}`
	rec := postRaw(t, router, "is-allowed", body)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var response UnavailableResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Reason != string(adsync.ReasonFallbackUnavailable) {
		t.Fatalf("reason = %q", response.Reason)
	}
	if forwarded := source.Forwarded(); len(forwarded) != 0 {
		t.Fatalf("%d requests were forwarded", len(forwarded))
	}
	// Other services are unaffected.
	if rec := postRaw(t, router, "is-allowed", allowedBody); rec.Code != http.StatusOK {
		t.Fatalf("in_sync service status = %d", rec.Code)
	}
}

func TestForwardFailuresMapToReasonCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		reason adsync.Reason
	}{
		{"quota", adsync.ErrFallbackQuotaExhausted, adsync.ReasonQuotaExhausted},
		{"revoked", adsync.ErrUnauthorized, adsync.ReasonRevoked},
		{"unavailable", adsync.ErrUnavailable, adsync.ReasonFallbackUnavailable},
		{"other", errors.New("boom"), adsync.ReasonFallbackUnavailable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			router, source := newGatedRouter(t)
			source.SetFallbackError(testCase.err)
			body := `{"serviceName":"orders","resource":"res1","action":"read"}`
			rec := postRaw(t, router, "is-allowed", body)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
			}
			var response UnavailableResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Reason != string(testCase.reason) {
				t.Fatalf("reason = %q, want %q", response.Reason, testCase.reason)
			}
			if rec.Header().Get(svcs.DecisionSourceHeader) != "" {
				t.Fatal("a failed forward carries the fallback marker")
			}
		})
	}
}

func TestGrantedOperationsForwardToTheMatchingCloudEndpoint(t *testing.T) {
	router, source := newGatedRouter(t)
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`[]`))

	for path, operation := range map[string]adsync.FallbackOperation{
		"all-granted-roles":       adsync.FallbackAllGrantedRoles,
		"all-granted-permissions": adsync.FallbackAllGrantedPermissions,
	} {
		source.SetFallbackResponse(http.StatusOK, nil, []byte(`[]`))
		rec := postRaw(t, router, path, `{"serviceName":"orders"}`)
		if rec.Code != http.StatusOK || rec.Body.String() != "[]" {
			t.Fatalf("%s: status %d body %q", path, rec.Code, rec.Body.String())
		}
		forwarded := source.Forwarded()
		if len(forwarded) == 0 || forwarded[len(forwarded)-1].Operation != operation {
			t.Fatalf("%s: forwarded %+v", path, forwarded)
		}
	}
}

func TestDiagnoseAndDiscoverFailClosedWhileSyncing(t *testing.T) {
	router, source := newGatedRouter(t)
	body := `{"serviceName":"orders","resource":"res1","action":"read"}`
	for _, path := range []string{"diagnose", "discover"} {
		rec := postRaw(t, router, path, body)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d (%s)", path, rec.Code, rec.Body.String())
		}
		var response UnavailableResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Reason != string(adsync.ReasonFallbackUnavailable) {
			t.Fatalf("%s: reason = %q", path, response.Reason)
		}
	}
	if forwarded := source.Forwarded(); len(forwarded) != 0 {
		t.Fatalf("%d requests were forwarded", len(forwarded))
	}
}

func TestHealthAndReadiness(t *testing.T) {
	router, source := newGatedRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d", rec.Code)
	}
	var health HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil || health.Status != "ok" {
		t.Fatalf("healthz body = %s (%v)", rec.Body.String(), err)
	}

	ready := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		return rec
	}

	rec = ready()
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz status = %d (%s)", rec.Code, rec.Body.String())
	}
	var readiness adsgate.PublicReadiness
	if err := json.Unmarshal(rec.Body.Bytes(), &readiness); err != nil {
		t.Fatal(err)
	}
	if !readiness.Ready || readiness.Liveness != adsync.LivenessLive ||
		readiness.ServicesTotal != 2 || readiness.ServicesUsable != 2 {
		t.Fatalf("readiness = %+v", readiness)
	}
	// The endpoint is unauthenticated: service identities, revisions and
	// revocation state must not appear in the body.
	for _, leaked := range []string{"app", "orders", "revision", "head", `"usable":`, "state"} {
		if strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("readyz body exposes %q: %s", leaked, rec.Body.String())
		}
	}

	source.SetInstance(adsync.InstanceStatus{Liveness: adsync.LivenessStale})
	rec = ready()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("stale readyz status = %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &readiness); err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || readiness.Liveness != adsync.LivenessStale {
		t.Fatalf("stale readiness = %+v", readiness)
	}

	// A stopped instance keeps its stop reason internal: the unauthenticated
	// body reports only that the instance is not ready.
	source.SetInstance(adsync.InstanceStatus{Liveness: adsync.LivenessStopped, Reason: adsync.ReasonRevoked})
	rec = ready()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("stopped readyz status = %d", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "reason") || strings.Contains(body, "revoked") {
		t.Fatalf("readyz body exposes the stop reason: %s", body)
	}

	source.SetInstance(adsync.InstanceStatus{Liveness: adsync.LivenessLive})
	source.SetFallbackAvailable(false)
	rec = ready()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("syncing without fallback readyz status = %d", rec.Code)
	}
}

func TestReadyzWithoutADistributionStateIsReady(t *testing.T) {
	router := newHandlerRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var readiness adsgate.PublicReadiness
	if err := json.Unmarshal(rec.Body.Bytes(), &readiness); err != nil {
		t.Fatal(err)
	}
	if !readiness.Ready || readiness.ServicesTotal != 0 {
		t.Fatalf("readiness = %+v", readiness)
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

func TestFallbackDecisionsAreAudited(t *testing.T) {
	out := captureAuditLog(t)
	router, source := newGatedRouter(t)

	cases := []struct {
		path      string
		operation adsync.FallbackOperation
		cloudBody string
		auditName string
		resultKey string
	}{
		{"is-allowed", adsync.FallbackIsAllowed, `{"allowed":true,"reason":2}`, "IsAllowed", "allowed"},
		{"all-granted-roles", adsync.FallbackAllGrantedRoles, `["admin"]`, "GetAllGrantedRoles", "roles"},
		{"all-granted-permissions", adsync.FallbackAllGrantedPermissions, `[{"resource":"res1","actions":["read"]}]`, "GetAllGrantedPermissions", "permissions"},
	}
	for _, testCase := range cases {
		t.Run(testCase.auditName, func(t *testing.T) {
			source.SetFallbackResponse(http.StatusOK, nil, []byte(testCase.cloudBody))
			body := `{"subject":{"token":"secret-token","principals":[{"type":"user","name":"alice"}]},"serviceName":"orders","resource":"res1","action":"read"}`
			rec := postRaw(t, router, testCase.path, body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
			}

			entry := lastAuditEntry(t, out)
			if entry["msg"] != "[AUDIT]"+testCase.auditName || entry["level"] != "info" || entry["response"] != "succeeded" {
				t.Fatalf("audit entry = %v", entry)
			}
			if entry["serviceName"] != "orders" || entry["operation"] != string(testCase.operation) ||
				entry["decisionSource"] != svcs.DecisionSourceUpstream {
				t.Fatalf("audit context = %v", entry)
			}
			if _, found := entry[testCase.resultKey]; !found {
				t.Fatalf("audit result %q missing from %v", testCase.resultKey, entry)
			}
			if strings.Contains(out.String(), "secret-token") {
				t.Fatal("the audit record contains the subject token")
			}
		})
	}
}

func TestFallbackFailuresAreAudited(t *testing.T) {
	out := captureAuditLog(t)
	router, source := newGatedRouter(t)
	source.SetFallbackError(adsync.ErrUnavailable)

	rec := postRaw(t, router, "is-allowed", `{"serviceName":"orders","resource":"res1","action":"read"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}

	entry := lastAuditEntry(t, out)
	if entry["msg"] != "[AUDIT]IsAllowed" || entry["level"] != "error" || entry["response"] != "failed" {
		t.Fatalf("audit entry = %v", entry)
	}
	if entry["serviceName"] != "orders" || entry["decisionSource"] != svcs.DecisionSourceUpstream {
		t.Fatalf("audit context = %v", entry)
	}
	if reason, _ := entry["reason"].(string); !strings.Contains(reason, adsync.ErrUnavailable.Error()) {
		t.Fatalf("audit reason = %v", entry["reason"])
	}
}

func TestDirectModeMarksForwardedDecisionsAsUpstream(t *testing.T) {
	router, source := newGatedRouter(t)
	source.SetStaleIsUntrusted(true)
	source.SetFallbackResponse(http.StatusOK, nil, []byte(`{"allowed":true,"reason":0}`))
	rec := postRaw(t, router, "is-allowed", `{"serviceName":"orders","resource":"order:1","action":"read"}`)
	if rec.Code != http.StatusOK || rec.Header().Get(svcs.DecisionSourceHeader) != svcs.DecisionSourceUpstream {
		t.Fatalf("status %d marker %q", rec.Code, rec.Header().Get(svcs.DecisionSourceHeader))
	}
}
