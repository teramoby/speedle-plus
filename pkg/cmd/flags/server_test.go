package flags

import (
	"context"
	"net/http"
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

func TestGRPCPanicBecomesInternalError(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/pb.Evaluator/IsAllowed"}
	_, err := recoverUnary(context.Background(), nil, info, func(context.Context, interface{}) (interface{}, error) {
		panic("boom")
	})
	if status.Code(err) != codes.Internal {
		t.Errorf("got %v, want an Internal error", err)
	}
}
