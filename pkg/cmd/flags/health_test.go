package flags

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivateHealthListener(t *testing.T) {
	var params Parameters
	if server, err := params.NewHealthServer(nil); err != nil || server != nil {
		t.Fatalf("default must be disabled: %v %v", server, err)
	}
	for _, address := range []string{"0.0.0.0:8081", "[::]:8081", ":8081", "192.0.2.1:8081", "localhost:8081", "invalid"} {
		params.HealthEndpoint.Value = address
		if _, err := params.NewHealthServer(func(context.Context) error { return nil }); err == nil {
			t.Fatalf("accepted non-loopback or ambiguous address %q", address)
		}
	}
	params.HealthEndpoint.Value = "127.0.0.1:8081"
	healthy := true
	checks := 0
	server, err := params.NewHealthServer(func(ctx context.Context) error {
		checks++
		if _, bounded := ctx.Deadline(); !bounded {
			t.Error("readiness needs a bounded context")
		}
		if !healthy {
			return errors.New("secret backend connection details")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		healthy      bool
		status       int
	}{
		{"GET", "/readyz", true, 200}, {"GET", "/readyz", false, 503},
		{"GET", "/livez", false, 200}, {"HEAD", "/readyz", true, 200},
		{"POST", "/readyz", true, 405}, {"GET", "/policy-mgmt/v1/service", true, 404},
	} {
		healthy = tc.healthy
		before := checks
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.status || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, response.Code, response.Body)
		}
		if tc.path == "/livez" && checks != before {
			t.Fatal("liveness must not restart a process when its backend is unavailable")
		}
	}
}
