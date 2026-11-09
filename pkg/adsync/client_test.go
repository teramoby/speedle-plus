// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServicesDiscovery(t *testing.T) {
	first := newFixtureService(testServiceID1, "books")
	second := newFixtureService(testServiceID2, "orders")
	fixture := newFixture(t, first, second)
	client := fixture.newClient(nil)

	response, err := client.Services(context.Background())
	if err != nil {
		t.Fatalf("Services: %v", err)
	}
	if response.ServicesVersion != 1 {
		t.Errorf("servicesVersion = %d, want 1", response.ServicesVersion)
	}
	if len(response.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(response.Services))
	}
	byID := map[string]Service{}
	for _, service := range response.Services {
		byID[service.ID] = service
	}
	for _, expected := range []*fixtureService{first, second} {
		service, found := byID[expected.ID]
		if !found {
			t.Fatalf("service %s missing from discovery", expected.ID)
		}
		if service.Name != expected.Name || service.Epoch != expected.Epoch || service.Revision != expected.Revision {
			t.Errorf("service %s = %+v", expected.ID, service)
		}
		if service.HeadStateHash != expected.StateHash {
			t.Errorf("service %s headStateHash = %s, want %s", expected.ID, service.HeadStateHash, expected.StateHash)
		}
	}

	fixture.source.mu.Lock()
	defer fixture.source.mu.Unlock()
	if len(fixture.source.requests) == 0 {
		t.Fatal("no request reached the fixture source")
	}
	if got := fixture.source.requests[0].Authorization; got != "Bearer "+testCredential {
		t.Errorf("Authorization = %q", got)
	}
	if got := fixture.source.requests[0].Path; got != APIPrefix+"/services" {
		t.Errorf("path = %q", got)
	}
}

func TestSnapshotAndNotModified(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
			Name: stringPointer("read-books"), SPDL: stringPointer(`grant principal "user:alice" to read on "/books/*"`)})
	client := fixture.newClient(nil)

	response, notModified, err := client.Snapshot(context.Background(), service.ID, "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if notModified {
		t.Fatal("first snapshot reported not modified")
	}
	if response.ServiceID != service.ID || response.Revision != service.Revision {
		t.Errorf("snapshot identity = %s@%d", response.ServiceID, response.Revision)
	}
	if response.StateHash != StateHash(response.Snapshot) {
		t.Errorf("snapshot state hash does not match its content")
	}
	if len(response.Snapshot.Policies) != 1 {
		t.Errorf("snapshot has %d policies, want 1", len(response.Snapshot.Policies))
	}

	etag := `"` + response.SourceID + "-" + response.Epoch + "-" +
		strconv.FormatUint(response.Revision, 10) + "-" + response.StateHash.String() + `"`
	again, notModified, err := client.Snapshot(context.Background(), service.ID, etag)
	if err != nil {
		t.Fatalf("conditional Snapshot: %v", err)
	}
	if !notModified || again != nil {
		t.Fatalf("conditional Snapshot = %v, %v; want not modified", again, notModified)
	}
}

func TestSnapshotRejectsBadHash(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	fixture.source.configure(func(source *fixtureSource) { source.corruptSnapshotHash = true })
	client := fixture.newClient(nil)

	if _, _, err := client.Snapshot(context.Background(), testServiceID1, ""); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Snapshot error = %v, want ErrIntegrity", err)
	}
}

func TestSnapshotRejectsBadSignature(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	fixture.source.configure(func(source *fixtureSource) { source.corruptSnapshotSignature = true })
	client := fixture.newClient(nil)

	if _, _, err := client.Snapshot(context.Background(), testServiceID1, ""); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Snapshot error = %v, want ErrIntegrity", err)
	}
}

func TestChangesVerifiesChain(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	fixture.source.configure(func(source *fixtureSource) {
		source.pageSize = 1
	})
	client := fixture.newClient(nil)
	ctx := context.Background()

	snapshot, _, err := client.Snapshot(ctx, service.ID, "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "authz", ID: "01HF7YAT000000000000000201",
			Name: stringPointer("read-books"), SPDL: stringPointer("grant read")})
	service.publish(t, fixture.signer,
		Change{Op: "upsert", Kind: "service", ID: service.ID, Name: stringPointer("books-v2")})

	page, err := client.Changes(ctx, service.ID, snapshot.SourceID, snapshot.Epoch,
		snapshot.Revision, snapshot.CommitHash, 100)
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if page.From != 2 || page.To != 2 || page.Head != 3 || page.NextSince != 2 {
		t.Fatalf("page envelope = from %d to %d head %d next %d", page.From, page.To, page.Head, page.NextSince)
	}
	if len(page.Commits) != 1 {
		t.Fatalf("page has %d commits, want 1 due to pageSize", len(page.Commits))
	}
	if page.Commits[0].Signature.KeyID != fixture.signer.keyID {
		t.Errorf("commit signature keyId = %q", page.Commits[0].Signature.KeyID)
	}

	// The next page resumes at the server-provided continuation. Since is
	// exclusive, so nextSince is the last revision returned on this page.
	next, err := client.Changes(ctx, service.ID, snapshot.SourceID, snapshot.Epoch,
		page.NextSince, page.Commits[0].Hash, 100)
	if err != nil {
		t.Fatalf("second Changes: %v", err)
	}
	if next.From != 3 || next.To != 3 || len(next.Commits) != 1 {
		t.Fatalf("second page = from %d to %d commits %d", next.From, next.To, len(next.Commits))
	}
	if next.Commits[0].PrevHash != page.Commits[0].Hash {
		t.Errorf("second page does not chain from the first")
	}
}

func TestChangesRejectsBadSignature(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	client := fixture.newClient(nil)
	snapshot, _, err := client.Snapshot(context.Background(), service.ID, "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	service.publish(t, fixture.signer,
		Change{Op: "delete", Kind: "authz", ID: "01HF7YAT000000000000000201"})
	fixture.source.configure(func(source *fixtureSource) { source.corruptCommitSignature = true })

	_, err = client.Changes(context.Background(), service.ID, snapshot.SourceID, snapshot.Epoch,
		snapshot.Revision, snapshot.CommitHash, 100)
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Changes error = %v, want ErrIntegrity", err)
	}
}

func TestChangesRejectsGap(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	client := fixture.newClient(nil)
	snapshot, _, err := client.Snapshot(context.Background(), service.ID, "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	service.publish(t, fixture.signer,
		Change{Op: "delete", Kind: "authz", ID: "01HF7YAT000000000000000201"})
	service.publish(t, fixture.signer,
		Change{Op: "delete", Kind: "authz", ID: "01HF7YAT000000000000000202"})
	fixture.source.configure(func(source *fixtureSource) { source.dropCommitRevision = 2 })

	_, err = client.Changes(context.Background(), service.ID, snapshot.SourceID, snapshot.Epoch,
		snapshot.Revision, snapshot.CommitHash, 100)
	if !errors.Is(err, ErrProtocol) {
		t.Fatalf("Changes error = %v, want ErrProtocol", err)
	}
}

func TestRequestTimeout(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	fixture.source.configure(func(source *fixtureSource) { source.delay = 200 * time.Millisecond })
	client := fixture.newClient(func(config *Config) {
		config.RequestTimeout = 20 * time.Millisecond
		config.SnapshotTimeout = 20 * time.Millisecond
	})

	started := time.Now()
	_, err := client.Services(context.Background())
	if err == nil {
		t.Fatal("Services succeeded despite the timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("timeout took %v", elapsed)
	}
}

func TestRetriesWithBackoff(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	fixture.source.configure(func(source *fixtureSource) {
		source.failRemaining = 2
		source.failStatus = http.StatusServiceUnavailable
	})
	var delays []time.Duration
	client := fixture.newClient(func(config *Config) {
		config.Retry = RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     100 * time.Millisecond,
			Jitter:         0.5,
			Rand:           func() float64 { return 0.5 },
			Sleep: func(_ context.Context, delay time.Duration) error {
				delays = append(delays, delay)
				return nil
			},
		}
	})

	if _, err := client.Services(context.Background()); err != nil {
		t.Fatalf("Services: %v", err)
	}
	if len(delays) != 2 {
		t.Fatalf("delays = %v, want two retries", delays)
	}
	if delays[0] != 10*time.Millisecond || delays[1] != 20*time.Millisecond {
		t.Errorf("delays = %v, want [10ms 20ms]", delays)
	}
	if count := fixture.requestCount(APIPrefix + "/services"); count != 3 {
		t.Errorf("request count = %d, want 3", count)
	}
}

func TestRetryHonoursRetryAfter(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	fixture.source.configure(func(source *fixtureSource) {
		source.failRemaining = 1
		source.failStatus = http.StatusTooManyRequests
		source.failRetryAfter = "2"
	})
	var delays []time.Duration
	client := fixture.newClient(func(config *Config) {
		config.Retry = RetryPolicy{
			MaxAttempts:    2,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Second,
			Jitter:         0,
			Sleep: func(_ context.Context, delay time.Duration) error {
				delays = append(delays, delay)
				return nil
			},
		}
	})

	if _, err := client.Services(context.Background()); err != nil {
		t.Fatalf("Services: %v", err)
	}
	if len(delays) != 1 || delays[0] != 2*time.Second {
		t.Errorf("delays = %v, want [2s] from Retry-After", delays)
	}
}

func TestNoRetryOnUnauthorized(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	fixture.source.configure(func(source *fixtureSource) {
		source.failRemaining = 1
		source.failStatus = http.StatusUnauthorized
	})
	client := fixture.newClient(func(config *Config) {
		config.Retry.MaxAttempts = 3
	})

	_, err := client.Services(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
	if count := fixture.requestCount(APIPrefix + "/services"); count != 1 {
		t.Errorf("request count = %d, want 1", count)
	}
}

func TestCAFileRequired(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	client := fixture.newClient(func(config *Config) { config.CAFile = "" })
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("Services succeeded without trusting the test CA")
	}
}

func TestConfigValidation(t *testing.T) {
	keys := goldenPublicKeys(t, loadGolden(t))
	valid := Config{
		BaseURL:    "https://cloud.example",
		Credential: testCredential,
		PublicKeys: keys,
	}
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"http base URL", func(config *Config) { config.BaseURL = "http://cloud.example" }},
		{"credential in URL", func(config *Config) { config.BaseURL = "https://user@cloud.example" }},
		{"missing keys", func(config *Config) { config.PublicKeys = nil }},
		{"bad credential", func(config *Config) { config.Credential = "adsi_short" }},
		{"client cert without key", func(config *Config) { config.ClientCertFile = "/tmp/cert.pem" }},
		{"negative timeout", func(config *Config) { config.RequestTimeout = -time.Second }},
		{"negative attempts", func(config *Config) { config.Retry.MaxAttempts = -1 }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			config := valid
			testCase.mutate(&config)
			if _, err := NewClient(config); err == nil {
				t.Fatal("NewClient accepted an invalid configuration")
			}
		})
	}
}

type connectProxy struct {
	listener   net.Listener
	url        string
	mu         sync.Mutex
	connects   int
	lastTarget string
}

func startConnectProxy(t *testing.T, target string) *connectProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for proxy: %v", err)
	}
	proxy := &connectProxy{listener: listener, url: "http://" + listener.Addr().String()}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go proxy.handle(connection, target)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return proxy
}

func (p *connectProxy) handle(connection net.Conn, target string) {
	defer connection.Close()
	reader := bufio.NewReader(connection)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	if request.Method != http.MethodConnect {
		_, _ = io.WriteString(connection, "HTTP/1.1 405 Method Not Allowed\r\n\r\n")
		return
	}
	p.mu.Lock()
	p.connects++
	p.lastTarget = request.Host
	p.mu.Unlock()

	upstream, err := net.Dial("tcp", target)
	if err != nil {
		_, _ = io.WriteString(connection, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return
	}
	defer upstream.Close()
	if _, err := io.WriteString(connection, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, reader)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(connection, upstream)
		done <- struct{}{}
	}()
	<-done
}

func (p *connectProxy) counts() (int, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.connects, p.lastTarget
}

func TestConnectProxyFromEnvironment(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	target := fixture.server.Listener.Addr().String()
	proxy := startConnectProxy(t, target)
	t.Setenv("HTTPS_PROXY", proxy.url)
	t.Setenv("https_proxy", proxy.url)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	port := fixture.server.Listener.Addr().(*net.TCPAddr).Port
	// The httptest certificate is valid for example.com. The proxy tunnel
	// reaches the local server even though the URL host is not loopback, so
	// the environment proxy is actually used.
	baseURL := fmt.Sprintf("https://example.com:%d", port)
	client := fixture.newClientAt(baseURL, nil)
	response, err := client.Services(context.Background())
	if err != nil {
		t.Fatalf("Services through CONNECT proxy: %v", err)
	}
	if len(response.Services) != 1 {
		t.Fatalf("got %d services through the proxy", len(response.Services))
	}
	connects, lastTarget := proxy.counts()
	if connects != 1 {
		t.Errorf("CONNECT count = %d, want 1", connects)
	}
	if want := fmt.Sprintf("example.com:%d", port); lastTarget != want {
		t.Errorf("CONNECT target = %q, want %q", lastTarget, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCustomHTTPClientRejectsRedirectsWithoutMutation(t *testing.T) {
	requests := 0
	custom := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		status := http.StatusFound
		body := `{}`
		header := http.Header{"Location": {"http://elsewhere.example/policy-source/v1/services"}}
		if requests > 1 {
			status = http.StatusOK
			body = `{"servicesVersion":1,"services":[]}`
			header = http.Header{}
		}
		return &http.Response{
			StatusCode: status,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	client, err := NewClient(Config{
		BaseURL:    "https://source.example",
		Credential: testCredential,
		PublicKeys: goldenPublicKeys(t, loadGolden(t)),
		HTTPClient: custom,
		Retry:      RetryPolicy{MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.Services(context.Background()); !errors.Is(err, ErrProtocol) {
		t.Fatalf("Services redirect error = %v, want ErrProtocol", err)
	}
	if requests != 1 {
		t.Errorf("HTTP requests = %d, want 1 (redirect not followed)", requests)
	}
	if custom.CheckRedirect != nil {
		t.Error("NewClient mutated the caller's HTTP client")
	}
}

func stringPointer(value string) *string {
	return &value
}
