// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsrest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gorilla/mux"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgate"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

// TestEngineWithoutPolicySourceStoreIsUnchanged pins that an Engine whose
// store is not the policy-source store behaves as it did before policy
// distribution existed: no distribution gate, the original route table,
// local decisions without a decision-source header, no policy-source
// endpoints and a ready readiness probe.
func TestEngineWithoutPolicySourceStoreIsUnchanged(t *testing.T) {
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, []byte(handlerPolicies), 0o600); err != nil {
		t.Fatal(err)
	}
	evaluator, err := eval.NewWithStore(&cfg.Config{}, storetest.PolicyStore(t, location))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = evaluator.(*eval.PolicyEvalImpl).Close() })
	if gate := adsgate.Of(evaluator); gate.Enabled() {
		t.Fatal("a file-store evaluator has an enabled distribution gate")
	}
	router, err := NewRouter(evaluator)
	if err != nil {
		t.Fatal(err)
	}

	var routes []string
	if err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		routes = append(routes, route.GetName())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"Healthz", "Readyz", "GetAllGrantedPermissions", "GetAllGrantedRoles", "IsAllowed", "Diagnose", "Discover"}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("routes = %v, want %v", routes, want)
	}

	for _, path := range []string{"is-allowed", "all-granted-roles", "all-granted-permissions"} {
		rec := post(t, router, path, `{`+alice+`,"resource":"res1","action":"read"}`, http.StatusOK)
		if source := rec.Header().Get(svcs.DecisionSourceHeader); source != "" {
			t.Fatalf("%s: decision source %q on a local decision", path, source)
		}
	}
	rec := post(t, router, "is-allowed", `{`+alice+`,"resource":"res1","action":"read"}`, http.StatusOK)
	var decision IsAllowedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil || !decision.Allowed {
		t.Fatalf("local decision = %s, %v", rec.Body.String(), err)
	}

	probe := httptest.NewRecorder()
	router.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, adsync.APIPrefix+"/services", nil))
	if probe.Code != http.StatusNotFound {
		t.Fatalf("policy-source endpoint status = %d", probe.Code)
	}
	ready := httptest.NewRecorder()
	router.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("readyz = %d (%s)", ready.Code, ready.Body.String())
	}
}
