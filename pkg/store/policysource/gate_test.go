// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func TestFallbackForwardsWithTheInstanceCredential(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	fixture.source.mutate(func(source *testSource) {
		source.fallbackBody = `[{"resource":"books","actions":["read"]}]`
	})
	store := fixture.open(nil)

	body := []byte(`{"serviceName":"books","resource":"books","action":"read"}`)
	response, err := store.ForwardFallback(context.Background(), adsync.FallbackAllGrantedPermissions, body)
	if err != nil {
		t.Fatalf("ForwardFallback: %v", err)
	}
	if response.StatusCode != http.StatusOK || string(response.Body) != `[{"resource":"books","actions":["read"]}]` {
		t.Fatalf("response = %d %q", response.StatusCode, response.Body)
	}
	path, forwarded, calls := fixture.source.fallbackRequest()
	if calls != 1 {
		t.Fatalf("%d fallback calls, want 1", calls)
	}
	if path != adsync.AuthzCheckAPIPrefix+"all-granted-permissions" {
		t.Fatalf("path = %q", path)
	}
	if string(forwarded) != string(body) {
		t.Fatalf("forwarded body %q, want %q", forwarded, body)
	}
	if !store.FallbackAvailable() {
		t.Fatal("a successful fallback must stay available")
	}
}

func TestFallbackDisabledFailsClosed(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(func(options *Options) { options.DisableFallback = true })

	if store.FallbackAvailable() {
		t.Fatal("a disabled fallback reported available")
	}
	_, err := store.ForwardFallback(context.Background(), adsync.FallbackIsAllowed, []byte(`{}`))
	if !errors.Is(err, adsync.ErrFallbackUnavailable) {
		t.Fatalf("error = %v, want ErrFallbackUnavailable", err)
	}
	if _, _, calls := fixture.source.fallbackRequest(); calls != 0 {
		t.Fatalf("%d fallback calls, want 0", calls)
	}
}

func TestFallbackUnauthorizedStopsTheInstance(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	fixture.source.mutate(func(source *testSource) {
		source.fallbackStatus = http.StatusUnauthorized
		source.fallbackBody = `{"error":"unauthorized"}`
	})
	store := fixture.open(nil)

	_, err := store.ForwardFallback(context.Background(), adsync.FallbackIsAllowed, []byte(`{}`))
	if !errors.Is(err, adsync.ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
	instance := store.Instance()
	if instance.Liveness != adsync.LivenessStopped || instance.Reason != adsync.ReasonRevoked {
		t.Fatalf("instance after a revoked credential: %+v", instance)
	}
	if store.FallbackAvailable() {
		t.Fatal("a revoked credential reported an available fallback")
	}
}

func TestFallbackQuotaRefusal(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	fixture.source.mutate(func(source *testSource) {
		source.fallbackStatus = http.StatusServiceUnavailable
		source.fallbackBody = `{"error":"fallback_unavailable"}`
	})
	store := fixture.open(nil)

	_, err := store.ForwardFallback(context.Background(), adsync.FallbackIsAllowed, []byte(`{}`))
	if !errors.Is(err, adsync.ErrFallbackQuotaExhausted) {
		t.Fatalf("error = %v, want ErrFallbackQuotaExhausted", err)
	}
	if !store.FallbackAvailable() {
		t.Fatal("Cloud answered, so the fallback path is reachable")
	}
}

func TestFallbackUnreachableCloudIsReportedUnavailable(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	store := fixture.open(nil)
	// Closing the server makes the next dial fail.
	fixture.server.Close()

	_, err := store.ForwardFallback(context.Background(), adsync.FallbackIsAllowed, []byte(`{}`))
	if !errors.Is(err, adsync.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if store.FallbackAvailable() {
		t.Fatal("an unreachable Cloud reported an available fallback")
	}
}

func TestGateViewReportsInstanceAndServices(t *testing.T) {
	fixture := newTestFixture(t, booksService(), ordersService())
	store := fixture.open(nil)

	instance := store.Instance()
	if instance.Liveness != adsync.LivenessLive {
		t.Fatalf("liveness = %q", instance.Liveness)
	}
	services := store.Services()
	if len(services) != 2 {
		t.Fatalf("%d tracked services, want 2", len(services))
	}
	for _, service := range services {
		if service.State != adsync.SyncStateInSync {
			t.Fatalf("service %q state = %q", service.Name, service.State)
		}
	}
	status, known := store.Status("books")
	if !known || status.Service.Name != "books" || status.Service.State != adsync.SyncStateInSync {
		t.Fatalf("books status = %+v, known %v", status.Service, known)
	}
	if _, known := store.Status("missing"); known {
		t.Fatal("an unknown service was reported as tracked")
	}
}
