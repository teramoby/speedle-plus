package storetest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForEtcdHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	if err := waitForEtcdHTTP(server.URL, time.Second); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("health requests = %d", calls.Load())
	}
	server.Close()
	if err := waitForEtcdHTTP(server.URL, 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("closed listener readiness error = %v", err)
	}
}
