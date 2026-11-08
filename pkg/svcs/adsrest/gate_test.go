// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsrest

import (
	"encoding/json"
	"errors"
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
	if rec.Header().Get(svcs.DecisionSourceHeader) != svcs.DecisionSourceCloudFallback {
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
	var readiness adsgate.Readiness
	if err := json.Unmarshal(rec.Body.Bytes(), &readiness); err != nil {
		t.Fatal(err)
	}
	if !readiness.Ready || readiness.Liveness != adsync.LivenessLive || len(readiness.Services) != 2 {
		t.Fatalf("readiness = %+v", readiness)
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
	var readiness adsgate.Readiness
	if err := json.Unmarshal(rec.Body.Bytes(), &readiness); err != nil {
		t.Fatal(err)
	}
	if !readiness.Ready {
		t.Fatalf("readiness = %+v", readiness)
	}
}
