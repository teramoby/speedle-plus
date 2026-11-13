// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// testConnectProxy is a minimal HTTP proxy that only implements CONNECT, which
// is what a wss:// connection needs.
type testConnectProxy struct {
	listener net.Listener
	// rewriteTarget maps the CONNECT target to the address actually dialed. It
	// is nil when the target is dialed as-is.
	rewriteTarget func(string) string

	mu       sync.Mutex
	connects []string
}

func newTestConnectProxy(t *testing.T) *testConnectProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for proxy: %v", err)
	}
	proxy := &testConnectProxy{listener: listener}
	go proxy.serve()
	t.Cleanup(func() { _ = listener.Close() })
	return proxy
}

func (p *testConnectProxy) url() string {
	return "http://" + p.listener.Addr().String()
}

func (p *testConnectProxy) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.handle(conn)
	}
}

func (p *testConnectProxy) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	if request.Method != http.MethodConnect {
		_, _ = io.WriteString(conn, "HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\n\r\n")
		return
	}
	target := request.Host
	if target == "" {
		target = request.URL.Host
	}
	p.mu.Lock()
	p.connects = append(p.connects, target)
	p.mu.Unlock()

	dialTarget := target
	if p.rewriteTarget != nil {
		dialTarget = p.rewriteTarget(target)
	}
	upstream, err := net.Dial("tcp", dialTarget)
	if err != nil {
		_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer upstream.Close()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(upstream, reader)
		if closer, ok := upstream.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
		close(done)
	}()
	_, _ = io.Copy(conn, upstream)
	<-done
}

func (p *testConnectProxy) targets() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.connects...)
}

func newChannelTLSServer(t *testing.T, source *channelSource, certificate tls.Certificate) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(source)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	t.Cleanup(func() {
		source.close()
		server.Close()
	})
	return server
}

func TestChannelThroughConnectProxy(t *testing.T) {
	const sourceHost = "policy-source.test"
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	// The source host must not be a loopback address: the environment proxy
	// function bypasses loopback destinations and the CONNECT would never
	// happen.
	pki := newTestPKIForHosts(t, []string{sourceHost}, nil)
	server := newChannelTLSServer(t, source, pki.serverCertificate)
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	baseURL := "https://" + net.JoinHostPort(sourceHost, port)

	proxy := newTestConnectProxy(t)
	proxy.rewriteTarget = func(target string) string {
		_, targetPort, err := net.SplitHostPort(target)
		if err != nil {
			return target
		}
		return net.JoinHostPort("127.0.0.1", targetPort)
	}
	t.Setenv("HTTPS_PROXY", proxy.url())
	t.Setenv("https_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	client, err := NewClient(Config{
		BaseURL:        baseURL,
		APIKey:         testAPIKey,
		InstanceID:     testInstanceID,
		CAFile:         writeTestFile(t, t.TempDir(), "ca.pem", pki.caPEM, 0o644),
		RequestTimeout: 2 * time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	runChannel(t, channel)

	requireInSync(t, machine, "books")
	waitFor(t, 2*time.Second, "a CONNECT through the proxy", func() bool {
		return len(proxy.targets()) > 0
	})
	want := net.JoinHostPort(sourceHost, port)
	if got := proxy.targets()[0]; got != want {
		t.Fatalf("CONNECT target = %q, want %q", got, want)
	}
}

func TestChannelRequiresTrustedCA(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	// No CAFile: the httptest certificate is signed by an unknown authority.
	client, err := NewClient(Config{
		BaseURL:        server.URL,
		APIKey:         testAPIKey,
		InstanceID:     testInstanceID,
		RequestTimeout: time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = channel.Run(ctx)

	_, connections, _, _, hellos, _ := source.stats()
	if connections != 0 || len(hellos) != 0 {
		t.Fatalf("connections = %d, hellos = %d; an untrusted certificate must not connect", connections, len(hellos))
	}
	if _, known := machine.Status("books"); known {
		t.Fatal("the channel applied an ack over an untrusted connection")
	}
}

func newChannelMTLSServer(t *testing.T, source *channelSource, pki *testPKI, version uint16) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(source)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateActive {
			tlsConn, ok := conn.(*tls.Conn)
			if !ok {
				t.Errorf("authenticated connection has type %T; want *tls.Conn", conn)
				return
			}
			if got := tlsConn.ConnectionState().Version; got != version {
				t.Errorf("negotiated TLS version = %#x; want %#x", got, version)
			}
		}
	}
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{pki.serverCertificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pki.caPool,
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	t.Cleanup(func() {
		source.close()
		server.Close()
	})
	return server
}

func TestChannelClientCertificate(t *testing.T) {
	for _, version := range []struct {
		name string
		id   uint16
	}{
		{"TLS12", tls.VersionTLS12},
		{"TLS13", tls.VersionTLS13},
	} {
		t.Run(version.name, func(t *testing.T) {
			testChannelClientCertificate(t, version.id)
		})
	}
}

func testChannelClientCertificate(t *testing.T, version uint16) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	pki := newTestPKI(t)
	server := newChannelMTLSServer(t, source, pki, version)
	directory := t.TempDir()
	caFile := writeTestFile(t, directory, "ca.pem", pki.caPEM, 0o644)
	certFile := writeTestFile(t, directory, "client.pem", pki.clientCertPEM, 0o600)
	keyFile := writeTestFile(t, directory, "client.key", pki.clientKeyPEM, 0o600)

	client, err := NewClient(Config{
		BaseURL:        server.URL,
		APIKey:         testAPIKey,
		InstanceID:     testInstanceID,
		CAFile:         caFile,
		ClientCertFile: certFile,
		ClientKeyFile:  keyFile,
		RequestTimeout: 2 * time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	machine := newChannelMachine(t)
	if err := machine.ApplyAttempt(machine.BeginSync(books.ID), source.serviceState(books.ID)); err != nil {
		t.Fatalf("Applied: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	channel := newTestChannel(t, client, machine, ChannelOptions{
		Transport: TransportWebSocket,
		OnAck:     cancel,
	})
	channel.dialer.TLSClientConfig.MinVersion = version
	channel.dialer.TLSClientConfig.MaxVersion = version
	// Authentication is independent of the helper's 300ms first-ack timeout,
	// which can expire under race-detector scheduling pressure.
	channel.ackTimeout = defaultAckTimeout
	// Wait for a real ack and join this connection before testing rejection.
	// ApplyAttempt alone marks books in sync, even before the TLS dial starts.
	outcome := channel.runWebSocket(ctx)
	if !errors.Is(outcome.err, context.Canceled) || len(channel.Services()) != 1 {
		t.Fatalf("authenticated channel did not receive an ack: %+v", outcome)
	}

	// TLS 1.3 can finish the client-side handshake before the server's missing
	// certificate alert arrives. DialContext also writes the upgrade request
	// and reads its response, which must fail before any hello reaches source.
	withoutCertificate, err := NewClient(Config{
		BaseURL:        server.URL,
		APIKey:         testAPIKey,
		InstanceID:     testInstanceID,
		CAFile:         caFile,
		RequestTimeout: time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("NewClient without a certificate: %v", err)
	}
	_, connectionsBefore, _, _, _, _ := source.stats()
	plainMachine := newChannelMachine(t)
	plainChannel := newTestChannel(t, withoutCertificate, plainMachine, ChannelOptions{Transport: TransportWebSocket})
	plainChannel.dialer.TLSClientConfig.MinVersion = version
	plainChannel.dialer.TLSClientConfig.MaxVersion = version
	plainCtx, plainCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer plainCancel()
	outcome = plainChannel.runWebSocket(plainCtx)
	var networkError net.Error
	if !outcome.upgradeFailed || outcome.err == nil || plainCtx.Err() != nil ||
		(errors.As(outcome.err, &networkError) && networkError.Timeout()) {
		t.Fatalf("certificate-less channel was not rejected during upgrade: %+v (context: %v)", outcome, plainCtx.Err())
	}
	_, connectionsAfter, _, _, _, _ := source.stats()
	if connectionsAfter != connectionsBefore {
		t.Fatal("a client without a certificate reached the WebSocket handler")
	}
	if len(plainChannel.Services()) != 0 {
		t.Fatal("a client without a certificate received a channel ack")
	}
}
