package flags

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDefaultIsSecure(t *testing.T) {
	if DefaultInsecure {
		t.Fatal("servers must use TLS unless --insecure=true is given")
	}
}

func TestHTTPServerIsBounded(t *testing.T) {
	params := Parameters{Endpoint: StrParamDetail{Value: "127.0.0.1:0"}, Insecure: StrParamDetail{Value: "true"}}
	server, err := params.NewHTTPServer(http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	if server.ReadHeaderTimeout == 0 || server.ReadTimeout == 0 || server.WriteTimeout == 0 || server.IdleTimeout == 0 {
		t.Errorf("unbounded server timeouts: %+v", server)
	}
}

func preflight(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodOptions, "/policy-mgmt/v1/service/s/policy/p", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	req.Header.Set("Access-Control-Request-Headers", "If-Match")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHTTPServerAllowsPutPreflight(t *testing.T) {
	params := Parameters{Endpoint: StrParamDetail{Value: "127.0.0.1:0"}, Insecure: StrParamDetail{Value: "true"}}
	server, err := params.NewHTTPServer(http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	rec := preflight(t, server.Handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != http.MethodPut {
		t.Errorf("Access-Control-Allow-Methods = %q, want PUT", got)
	}
}

func TestSameOriginHTTPServerHasNoCORS(t *testing.T) {
	params := Parameters{Endpoint: StrParamDetail{Value: "127.0.0.1:0"}, Insecure: StrParamDetail{Value: "true"}}
	server, err := params.NewSameOriginHTTPServer(http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	if server.ReadHeaderTimeout == 0 || server.WriteTimeout == 0 {
		t.Errorf("unbounded server timeouts: %+v", server)
	}
	rec := preflight(t, server.Handler)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want none", got)
	}

	params.Insecure.Value = "false"
	params.ClientCertPath.Value = "missing.pem"
	if _, err := params.NewSameOriginHTTPServer(http.NotFoundHandler()); err == nil {
		t.Error("expected an error for a missing certificate")
	}
}

func TestGRPCPanicBecomesInternalError(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/pb.Evaluator/IsAllowed"}
	_, err := recoverUnary(context.Background(), nil, info, func(context.Context, interface{}) (interface{}, error) {
		panic("boom")
	})
	if status.Code(err) != codes.Internal {
		t.Errorf("got %v, want an Internal error", err)
	}
}
