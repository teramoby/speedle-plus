// Licensed under the Universal Permissive License (UPL) Version 1.0.
package ads_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/suid"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
	"github.com/teramoby/speedle-plus/pkg/svcs/gate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// These implementations exist only in tests and depend solely on public APIs.
type exampleStore struct {
	pms.PolicyStoreManager
	gate                                          *exampleGate
	events                                        pms.StorageChangeChannel
	reads, watched, acknowledged, stopped, closed atomic.Int32
}

func (s *exampleStore) RequestGate() gate.Gate { return s.gate }
func (s *exampleStore) RequireWatch() bool     { return true }
func (s *exampleStore) ReadPolicyStore() (*pms.PolicyStore, error) {
	s.reads.Add(1)
	return snapshot(), nil
}
func (s *exampleStore) ReadPolicyStoreForReload() (*pms.PolicyStore, func(), error) {
	return snapshot(), func() { s.acknowledged.Add(1) }, nil
}
func (s *exampleStore) Watch() (pms.StorageChangeChannel, error) {
	s.watched.Add(1)
	return s.events, nil
}
func (s *exampleStore) StopWatch()   { s.stopped.Add(1) }
func (s *exampleStore) Close() error { s.closed.Add(1); return nil }

type builder struct{ s *exampleStore }

func (b builder) NewStore(map[string]interface{}) (pms.PolicyStoreManager, error) { return b.s, nil }
func (b builder) GetStoreParams() map[string]string {
	return map[string]string{"extension-test-option": "Option"}
}
func snapshot() *pms.PolicyStore {
	return &pms.PolicyStore{Services: []*pms.Service{{Name: "local", Type: pms.TypeApplication, Policies: []*pms.Policy{{ID: "allow", Effect: pms.Grant, Principals: [][]string{{"user:alice"}}, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}}}}}}
}

type exampleGate struct {
	mu       sync.Mutex
	events   []gate.Event
	forwards int
}

func (g *exampleGate) Route(service string) gate.Route {
	switch service {
	case "local", "unknown":
		return gate.Route{Kind: gate.Local}
	case "unavailable":
		return gate.Route{Kind: gate.Unavailable, Reason: "example_unavailable"}
	default:
		return gate.Route{Kind: gate.Fallback}
	}
}
func (g *exampleGate) Forward(ctx context.Context, op gate.Operation, request *api.RequestContext) (*gate.Response, error) {
	g.mu.Lock()
	g.forwards++
	g.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	switch request.ServiceName {
	case "failure":
		return nil, errors.New("secret upstream credential must not escape")
	case "quota":
		return nil, &gate.UnavailableError{Reason: "quota_exhausted"}
	case "nil":
		return nil, nil
	case "evaluation-error":
		return &gate.Response{Allowed: true, Reason: api.GRANT_POLICY_FOUND, ErrorMessage: "evaluation failed"}, nil
	}
	if request.ServiceName == "token" && (request.Subject == nil || request.Subject.Asserted || request.Subject.Token != "test-token" || request.Subject.TokenType != "bearer") {
		return nil, fmt.Errorf("fallback identity was changed")
	}
	if request.Subject == nil || len(request.Subject.Principals) != 1 || request.Subject.Principals[0].Name != "alice" || request.Attributes["example"] != "value" {
		return nil, fmt.Errorf("request not preserved")
	}
	return &gate.Response{Allowed: request.ServiceName != "deny", Reason: api.GRANT_POLICY_FOUND, Roles: []string{"reader"}, Permissions: []pms.Permission{{Resource: "book", Actions: []string{"read"}}}}, nil
}
func (g *exampleGate) Audit(_ context.Context, event gate.Event) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.events = append(g.events, event)
}

func TestExternalStoreGateAndServer(t *testing.T) {
	g := &exampleGate{}
	s := &exampleStore{gate: g, events: make(pms.StorageChangeChannel, 1)}
	name := "extension-server-" + suid.New().String()
	store.Register(name, builder{s})
	if store.GetAllStoreParams()["extension-test-option"] != "Option" {
		t.Fatal("store flags not registered")
	}
	rest, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rpc, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	params := &flags.Parameters{}
	params.Insecure.Value = "true"
	asserter := assertion.NewTestServer(t, nil)
	defer asserter.Close()
	conf := &cfg.Config{StoreConfig: &cfg.StoreConfig{StoreType: name}, AsserterWebhookConfig: &assertion.AsserterConfig{Endpoint: asserter.URL + "/assert"}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var rpcCalls atomic.Int32
	go func() {
		done <- ads.Run(ctx, params, conf,
			ads.WithListeners(rest, rpc),
			ads.WithHTTPServer(func(server *http.Server) {
				base := server.Handler
				server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/extension" {
						w.WriteHeader(204)
						return
					}
					base.ServeHTTP(w, r)
				})
			}),
			ads.WithGRPCOptions(grpc.ChainUnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
				rpcCalls.Add(1)
				return handler(ctx, req)
			})),
		)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run did not stop")
		}
		close(s.events)
		if s.watched.Load() != 1 || s.stopped.Load() != 1 || s.closed.Load() != 1 {
			t.Errorf("lifecycle watch=%d stop=%d close=%d", s.watched.Load(), s.stopped.Load(), s.closed.Load())
		}
	})
	client := &http.Client{Timeout: time.Second}
	url := "http://" + rest.Addr().String()
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, err := client.Get(url + "/extension")
		if err == nil {
			response.Body.Close()
			if response.StatusCode != 204 {
				t.Fatal(response.Status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	conn, err := grpc.NewClient(rpc.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	rpcClient := pb.NewEvaluatorClient(conn)
	request := func(service string) *pb.ContextRequest {
		req := &pb.ContextRequest{ServiceName: service, Resource: "book", Action: "read", Subject: &pb.Subject{Principals: []*pb.Principal{{Type: "user", Name: "alice"}}}, Attributes: map[string]string{"example": "value"}}
		if service == "token" {
			req.Subject.TokenType = "bearer"
			req.Subject.Token = "test-token"
		}
		return req
	}
	callREST := func(service, op string) (int, map[string]interface{}, http.Header) {
		t.Helper()
		token := ""
		if service == "token" {
			token = `,"tokenType":"bearer","token":"test-token"`
		}
		body := fmt.Sprintf(`{"serviceName":%q,"resource":"book","action":"read","subject":{"principals":[{"type":"user","name":"alice"}]%s},"attributes":[{"name":"example","value":"value"}]}`, service, token)
		response, err := client.Post(url+"/authz-check/v1/"+op, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var out map[string]interface{}
		if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, out, response.Header
	}
	for _, service := range []string{"local", "allow", "token", "deny", "unknown", "unavailable", "failure", "quota", "nil", "evaluation-error"} {
		t.Run(service, func(t *testing.T) {
			code, body, header := callREST(service, "is-allowed")
			var md metadata.MD
			result, err := rpcClient.IsAllowed(ctx, request(service), grpc.Header(&md))
			switch service {
			case "unavailable", "failure", "quota", "nil":
				expected := "fallback_unavailable"
				if service == "unavailable" {
					expected = "example_unavailable"
				}
				if service == "quota" {
					expected = "quota_exhausted"
				}
				if code != 503 || body["reason"] != expected || header.Get("Cache-Control") != "no-store" || status.Code(err) != codes.Unavailable || status.Convert(err).Message() != expected {
					t.Fatalf("REST %d %v gRPC %v", code, body, err)
				}
			case "unknown":
				if code != 200 || body["allowed"] != false || body["reason"] != float64(api.SERVICE_NOT_FOUND) || status.Code(err) != codes.Unknown {
					t.Fatalf("unknown local service: %d %v %v", code, body, err)
				}
			case "evaluation-error":
				if code != 200 || body["errorMessage"] != "evaluation failed" || body["allowed"] != false || body["reason"] != float64(api.ERROR_IN_EVALUATION) || status.Code(err) != codes.Internal {
					t.Fatalf("REST %d %v gRPC %v", code, body, err)
				}
			default:
				allowed := service == "local" || service == "allow" || service == "token"
				if code != 200 || body["allowed"] != allowed || err != nil || result.Allowed != allowed {
					t.Fatalf("REST %d %v gRPC %v %v", code, body, result, err)
				}
				if service == "allow" || service == "deny" {
					if header.Get("X-Speedle-Decision-Source") != "upstream" || len(md.Get("x-speedle-decision-source")) != 1 {
						t.Fatal("fallback marker missing")
					}
				}
			}
		})
	}
	for _, op := range []string{"all-granted-roles", "all-granted-permissions"} {
		body := `{"serviceName":"allow","resource":"book","action":"read","subject":{"principals":[{"type":"user","name":"alice"}]},"attributes":[{"name":"example","value":"value"}]}`
		response, err := client.Post(url+"/authz-check/v1/"+op, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var result []interface{}
		decodeErr := json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if decodeErr != nil || response.StatusCode != 200 || len(result) != 1 {
			t.Fatalf("%s: %v %v", op, result, decodeErr)
		}
	}
	roles, err := rpcClient.GetAllGrantedRoles(ctx, request("allow"))
	if err != nil || len(roles.Roles) != 1 || roles.Roles[0] != "reader" {
		t.Fatalf("roles: %v %v", roles, err)
	}
	permissions, err := rpcClient.GetAllPermissions(ctx, request("allow"))
	if err != nil || len(permissions.Permissions) != 1 || permissions.Permissions[0].Resource != "book" {
		t.Fatalf("permissions: %v %v", permissions, err)
	}
	for _, op := range []string{"diagnose", "discover"} {
		code, _, _ := callREST("allow", op)
		if code != 503 {
			t.Fatalf("unsupported fallback %s: %d", op, code)
		}
	}
	if _, err := rpcClient.Diagnose(ctx, request("allow")); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if _, err := rpcClient.Discover(ctx, request("allow")); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if s.reads.Load() != 1 || s.acknowledged.Load() != 1 {
		t.Fatalf("initial reads=%d ack=%d", s.reads.Load(), s.acknowledged.Load())
	}
	s.events <- pms.StoreChangeEvent{Type: pms.FULL_RELOAD}
	deadline = time.Now().Add(time.Second)
	for s.acknowledged.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.acknowledged.Load() != 2 {
		t.Fatal("reload hook not acknowledged")
	}
	if rpcCalls.Load() < 10 {
		t.Fatal("gRPC option not installed")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.events) != 24 {
		t.Fatalf("audit count=%d: %+v", len(g.events), g.events)
	}
	for _, event := range g.events {
		if event.ServiceName == "evaluation-error" && (event.Allowed || event.Reason != "evaluation_error") {
			t.Fatalf("evaluation failure audit: %+v", event)
		}
		if event.ServiceName == "local" || event.ServiceName == "unknown" {
			t.Fatalf("local dispatch produced nonlocal audit: %+v", event)
		}
	}
}

func TestExplicitGateOptionsAndNewWithStore(t *testing.T) {
	s := &exampleStore{gate: &exampleGate{}, events: make(pms.StorageChangeChannel)}
	evaluator, err := eval.NewWithStore(&cfg.Config{}, s)
	if err != nil {
		t.Fatal(err)
	}
	defer close(s.events)
	defer evaluator.(*eval.PolicyEvalImpl).Close()
	disabled, err := adsrest.NewRESTServiceWithEvaluator(evaluator, adsrest.WithGate(nil))
	if err != nil || disabled.Gate != nil {
		t.Fatal("REST nil override")
	}
	explicit := &exampleGate{}
	rest, err := adsrest.NewRESTServiceWithEvaluator(evaluator, adsrest.WithGate(explicit))
	if err != nil || rest.Gate != explicit {
		t.Fatal("REST override")
	}
	rpc, err := adsgrpc.NewGRPCService(evaluator, adsgrpc.WithGate(nil))
	if err != nil {
		t.Fatal(err)
	}
	result, err := rpc.IsAllowed(context.Background(), &pb.ContextRequest{ServiceName: "unavailable"})
	if result != nil || err == nil || status.Code(err) == codes.Unavailable {
		t.Fatalf("gRPC nil override: %v %v", result, err)
	}
	rpc, err = adsgrpc.NewGRPCService(evaluator, adsgrpc.WithGate(explicit))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rpc.IsAllowed(context.Background(), &pb.ContextRequest{ServiceName: "unavailable"}); status.Code(err) != codes.Unavailable {
		t.Fatal("gRPC override", err)
	}
}

// A failed snapshot must never acknowledge an installation that did not happen.
type invalidStore struct{ *exampleStore }

func (s *invalidStore) ReadPolicyStoreForReload() (*pms.PolicyStore, func(), error) {
	ps := snapshot()
	ps.Services = []*pms.Service{nil}
	return ps, func() { s.acknowledged.Add(1) }, nil
}
func TestFailedInstallationDoesNotAcknowledge(t *testing.T) {
	s := &invalidStore{&exampleStore{events: make(pms.StorageChangeChannel)}}
	defer close(s.events)
	if evaluator, err := eval.NewWithStore(&cfg.Config{}, s); err == nil {
		evaluator.(*eval.PolicyEvalImpl).Close()
		t.Fatal("invalid snapshot accepted")
	}
	if s.acknowledged.Load() != 0 || s.stopped.Load() != 1 {
		t.Fatalf("failed install ack=%d stop=%d", s.acknowledged.Load(), s.stopped.Load())
	}
}
