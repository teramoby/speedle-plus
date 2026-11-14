// Licensed under the Universal Permissive License (UPL) Version 1.0.
package ads

import (
	"github.com/teramoby/speedle-plus/pkg/svcs/gate"
	"google.golang.org/grpc"
	"net"
	"net/http"
)

type settings struct {
	gate                       gate.Gate
	gateSet                    bool
	httpListener, grpcListener net.Listener
	httpServer                 func(*http.Server)
	grpcOptions                []grpc.ServerOption
}

// Option configures the reusable decision server.
type Option func(*settings)

// WithGate overrides store-supplied discovery on both transports. Nil disables it.
func WithGate(g gate.Gate) Option { return func(s *settings) { s.gate, s.gateSet = g, true } }

// WithListeners uses pre-bound listeners. Nil keeps the configured endpoint.
// Run owns and closes supplied listeners, including on startup failure. REST
// still uses the TLS configuration in Parameters; gRPC uses its server options.
func WithListeners(rest, rpc net.Listener) Option {
	return func(s *settings) { s.httpListener, s.grpcListener = rest, rpc }
}

// WithHTTPServer configures the server after the standard handler, limits and
// TLS setup, before serving. A wrapper can mount additional routes here.
func WithHTTPServer(configure func(*http.Server)) Option {
	return func(s *settings) { s.httpServer = configure }
}

// WithGRPCOptions appends options to the standard TLS/server configuration.
func WithGRPCOptions(options ...grpc.ServerOption) Option {
	return func(s *settings) { s.grpcOptions = append(s.grpcOptions, options...) }
}
