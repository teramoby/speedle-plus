package flags

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// NewHealthServer creates an optional, separate loopback listener for container
// exec probes. Business APIs retain their TLS/mTLS policy. Never expose their
// handlers or store error details through this listener.
func (k *Parameters) NewHealthServer(ready func(context.Context) error) (*http.Server, error) {
	address := k.HealthEndpoint.Value
	if address == "" {
		return nil, nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return nil, fmt.Errorf("health-endpoint must use a loopback IP address and port")
	}
	if ready == nil {
		return nil, fmt.Errorf("health listener requires a readiness check")
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if r.URL.Path != "/livez" && r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/readyz" {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("ok\n"))
		}
	})
	return &http.Server{
		Addr: address, Handler: handler,
		ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second,
		WriteTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second,
		MaxHeaderBytes: 4096,
	}, nil
}
