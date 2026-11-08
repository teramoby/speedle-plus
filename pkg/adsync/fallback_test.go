// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newFallbackClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(Config{
		BaseURL:            server.URL,
		Credential:         testCredential,
		FallbackBaseURL:    server.URL,
		FallbackCredential: testCredential,
		PublicKeys:         goldenPublicKeys(t, loadGolden(t)),
		HTTPClient:         server.Client(),
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return client, server
}

func TestForwardFallbackPassesTheCloudResponseThrough(t *testing.T) {
	var (
		mu        sync.Mutex
		gotPath   string
		gotAuth   string
		gotType   string
		gotBody   []byte
		gotMethod string
	)
	cloudBody := []byte("{\"allowed\":true,\"reason\":0}\n")
	client, _ := newFallbackClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotPath, gotAuth, gotType, gotBody, gotMethod = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), body, r.Method
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cloud-Trace", "abc123")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cloudBody)
	})

	requestBody := []byte(`{"serviceName":"books","resource":"r","action":"read"}`)
	response, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, requestBody)
	if err != nil {
		t.Fatalf("ForwardFallback: %v", err)
	}
	if string(response.Body) != string(cloudBody) {
		t.Fatalf("body %q, want %q", response.Body, cloudBody)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if response.Header.Get("X-Cloud-Trace") != "abc123" || response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers were not passed through: %v", response.Header)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost || gotPath != AuthzCheckAPIPrefix+"is-allowed" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer "+testCredential {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotType != "application/json" {
		t.Fatalf("Content-Type = %q", gotType)
	}
	if string(gotBody) != string(requestBody) {
		t.Fatalf("forwarded body %q, want %q", gotBody, requestBody)
	}
}

func TestForwardFallbackRejectsUnknownOperations(t *testing.T) {
	client, _ := newFallbackClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	})
	_, err := client.ForwardFallback(context.Background(), FallbackOperation("diagnose"), []byte("{}"))
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("error = %v, want ErrProtocol", err)
	}
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("empty body error = %v, want ErrProtocol", err)
	}
}

// scriptedTransport returns a fixed sequence of responses. It lets a test
// observe how many attempts reached the transport without a real connection.
type scriptedTransport struct {
	mu      sync.Mutex
	calls   int
	results []func(*http.Request) (*http.Response, error)
}

func (s *scriptedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	s.mu.Lock()
	index := s.calls
	s.calls++
	results := s.results
	s.mu.Unlock()
	if index >= len(results) {
		index = len(results) - 1
	}
	return results[index](request)
}

func (s *scriptedTransport) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func response(status int, body string) func(*http.Request) (*http.Response, error) {
	return func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

func dialFailure(*http.Request) (*http.Response, error) {
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
}

func scriptedClient(t *testing.T, transport *scriptedTransport) *Client {
	t.Helper()
	client, err := NewClient(Config{
		BaseURL:            "https://policy.example",
		Credential:         testCredential,
		FallbackBaseURL:    "https://cloud.example",
		FallbackCredential: testCredential,
		PublicKeys:         goldenPublicKeys(t, loadGolden(t)),
		HTTPClient:         &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return client
}

func TestForwardFallbackRetriesOnlyConnectionFailuresOnce(t *testing.T) {
	t.Run("connection failure is retried once", func(t *testing.T) {
		transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
			dialFailure,
			response(http.StatusOK, `{"allowed":true,"reason":0}`),
		}}
		client := scriptedClient(t, transport)
		result, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}"))
		if err != nil {
			t.Fatalf("ForwardFallback: %v", err)
		}
		if result.StatusCode != http.StatusOK || transport.callCount() != 2 {
			t.Fatalf("status %d after %d attempts", result.StatusCode, transport.callCount())
		}
	})

	t.Run("two connection failures stop", func(t *testing.T) {
		transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
			dialFailure, dialFailure, response(http.StatusOK, `{}`),
		}}
		client := scriptedClient(t, transport)
		if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}")); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("error = %v, want ErrUnavailable", err)
		}
		if transport.callCount() != 2 {
			t.Fatalf("%d attempts, want 2", transport.callCount())
		}
	})

	t.Run("5xx is not retried", func(t *testing.T) {
		transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
			response(http.StatusInternalServerError, `{"error":"source_unavailable"}`),
			response(http.StatusOK, `{}`),
		}}
		client := scriptedClient(t, transport)
		if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}")); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("error = %v, want ErrUnavailable", err)
		}
		if transport.callCount() != 1 {
			t.Fatalf("%d attempts, want 1", transport.callCount())
		}
	})

	t.Run("429 is not retried", func(t *testing.T) {
		transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
			response(http.StatusTooManyRequests, `{"error":"rate_limited"}`),
			response(http.StatusOK, `{}`),
		}}
		client := scriptedClient(t, transport)
		if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}")); !errors.Is(err, ErrRateLimited) {
			t.Fatalf("error = %v, want ErrRateLimited", err)
		}
		if transport.callCount() != 1 {
			t.Fatalf("%d attempts, want 1", transport.callCount())
		}
	})
}

func TestForwardFallbackTimeoutIsNotRetried(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
		func(request *http.Request) (*http.Response, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			<-request.Context().Done()
			return nil, request.Context().Err()
		},
	}}
	client := scriptedClient(t, transport)
	client.fallbackTimeout = 30 * time.Millisecond
	start := time.Now()
	_, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}"))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("%d attempts, want 1", calls)
	}
}

func TestForwardFallbackClassifiesCloudErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":"unauthorized"}`, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, `{"error":"forbidden"}`, ErrForbidden},
		{"not found", http.StatusNotFound, `{"error":"service_not_found"}`, ErrServiceNotFound},
		{"rate limited", http.StatusTooManyRequests, `{"error":"rate_limited"}`, ErrRateLimited},
		{"unavailable", http.StatusServiceUnavailable, `{"error":"source_unavailable"}`, ErrUnavailable},
		{"quota", http.StatusServiceUnavailable, `{"error":"fallback_unavailable"}`, ErrFallbackQuotaExhausted},
		{"daily quota", http.StatusTooManyRequests, `{"error":"daily_quota_exceeded"}`, ErrFallbackQuotaExhausted},
		{"bad request", http.StatusBadRequest, `{"error":"invalid_request"}`, ErrProtocol},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
				response(testCase.status, testCase.body),
			}}
			client := scriptedClient(t, transport)
			_, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}"))
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestFallbackUsesOnlyTheConfiguredCloudDestination(t *testing.T) {
	var mu sync.Mutex
	proxyCalls := 0
	cloudCredential := "adsi_" + testInstanceID + "_" + strings.Repeat("x", 43)
	cloud := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+cloudCredential {
			t.Errorf("Cloud authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"allowed":true,"reason":0}`)
	}))
	defer cloud.Close()
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		proxyCalls++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer proxy.Close()

	client, err := NewClient(Config{
		BaseURL:            proxy.URL,
		Credential:         testCredential,
		FallbackBaseURL:    cloud.URL,
		FallbackCredential: cloudCredential,
		FallbackHTTPClient: cloud.Client(),
		PublicKeys:         goldenPublicKeys(t, loadGolden(t)),
		HTTPClient:         proxy.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{"serviceName":"books"}`)); err != nil {
		t.Fatalf("ForwardFallback: %v", err)
	}
	mu.Lock()
	if proxyCalls != 0 {
		t.Fatalf("the upstream proxy received %d fallback requests", proxyCalls)
	}
	mu.Unlock()

	withoutCloud, err := NewClient(Config{
		BaseURL:    proxy.URL,
		Credential: testCredential,
		PublicKeys: goldenPublicKeys(t, loadGolden(t)),
		HTTPClient: proxy.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient without Cloud: %v", err)
	}
	if withoutCloud.FallbackAvailable() {
		t.Fatal("a proxy credential implicitly enabled Cloud fallback")
	}
	if _, err := withoutCloud.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); !errors.Is(err, ErrFallbackUnavailable) {
		t.Fatalf("fallback without Cloud configuration = %v", err)
	}
}

func TestFallbackTransientFailurePermitsAGateRetry(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
				response(status, `{"error":"source_unavailable"}`),
				response(http.StatusOK, `{"allowed":true,"reason":0}`),
			}}
			client := scriptedClient(t, transport)
			now := time.Date(2026, time.November, 9, 0, 0, 0, 0, time.UTC)
			client.now = func() time.Time { return now }

			if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); err == nil {
				t.Fatal("the injected transient failure did not surface")
			}
			if client.FallbackAvailable() {
				t.Fatal("fallback was not suppressed during its bounded retry delay")
			}
			// This is the gate's suppression window, not a permanent latch.
			if transport.callCount() != 1 {
				t.Fatalf("calls = %d, want no automatic 5xx/429 retry", transport.callCount())
			}
			now = now.Add(DefaultFallbackRetryBackoff)
			if !client.FallbackAvailable() {
				t.Fatal("the gate did not permit another attempt after the retry delay")
			}
			if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); err != nil {
				t.Fatalf("retry after recovery: %v", err)
			}
			if transport.callCount() != 2 {
				t.Fatalf("Cloud calls = %d, want 2", transport.callCount())
			}
		})
	}
}

func TestFallbackAvailabilityFollowsOutcomes(t *testing.T) {
	transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
		dialFailure,
		dialFailure,
		response(http.StatusNotFound, `{"error":"service_not_found"}`),
		response(http.StatusServiceUnavailable, `{"error":"fallback_unavailable"}`),
	}}
	client := scriptedClient(t, transport)
	if !client.FallbackAvailable() {
		t.Fatal("a configured fallback must be available before the first attempt")
	}

	// Two dial failures exhaust the retry and leave Cloud unreachable.
	_, _ = client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}"))
	if client.FallbackAvailable() {
		t.Fatal("Cloud was reachable after a connection failure")
	}

	// Any answer, even a refusal, proves the source is reachable again.
	_, _ = client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}"))
	if !client.FallbackAvailable() {
		t.Fatal("an HTTP answer must restore availability")
	}

	// A quota refusal is an answer, so the source stays reachable.
	_, _ = client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte("{}"))
	if !client.FallbackAvailable() {
		t.Fatal("a quota refusal is not a transport failure")
	}
}
