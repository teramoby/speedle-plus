// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgate"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
)

// TestStaleInstanceWithoutForwardingDenies checks that a stale instance with
// forwarding disabled never answers from its old copy: it denies with
// 503 authorization_unavailable until an ack arrives again.
func TestStaleInstanceWithoutForwardingDenies(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	clock := newTestClock()
	store := fixture.open(func(options *Options) {
		options.Clock = clock
		options.DisableFallback = true
		options.HeartbeatInterval = 15 * time.Second
		options.StaleAfter = 20 * time.Second
		options.StopAfter = 60 * time.Second
	})
	if err := store.Confirm(); err != nil {
		t.Fatal(err)
	}
	engine, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, store)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := engine.(*eval.PolicyEvalImpl)
	t.Cleanup(func() { _ = evaluator.Close() })
	router, err := adsrest.NewRouter(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	isAllowed := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		body := `{"subject":{"principals":[{"type":"user","name":"alice"}]},"serviceName":"books","resource":"books","action":"read"}`
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, svcs.PolicyAtzPath+"is-allowed", strings.NewReader(body)))
		return rec
	}
	e2eWaitFor(t, "the evaluator to install the copy", func() bool {
		status, _ := store.Status("books")
		return status.Service.State == adsync.SyncStateInSync
	})
	if rec := isAllowed(); rec.Code != http.StatusOK {
		t.Fatalf("live instance: %d %s", rec.Code, rec.Body.String())
	}

	clock.Advance(25 * time.Second)
	if liveness := store.Instance().Liveness; liveness != adsync.LivenessStale {
		t.Fatalf("liveness = %s", liveness)
	}
	route := adsgate.New(store).Route("books")
	if route.Kind != adsgate.RouteUnavailable || route.Reason != adsync.ReasonFallbackUnavailable {
		t.Fatalf("stale route without forwarding = %+v", route)
	}
	rec := isAllowed()
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "authorization_unavailable") {
		t.Fatalf("stale instance without forwarding: %d %s", rec.Code, rec.Body.String())
	}
	if fixture.source.requestsForSuffix(adsync.AuthzCheckAPIPrefix) != 0 {
		t.Fatal("a request was forwarded with forwarding disabled")
	}

	// A confirming ack makes the copy trusted again.
	if err := store.Confirm(); err != nil {
		t.Fatal(err)
	}
	if rec := isAllowed(); rec.Code != http.StatusOK {
		t.Fatalf("after a confirming ack: %d %s", rec.Code, rec.Body.String())
	}
}
