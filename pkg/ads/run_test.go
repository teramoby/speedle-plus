//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package ads

import (
	"context"
	"encoding/json"
	_ "github.com/teramoby/speedle-plus/pkg/store/etcd"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func fixtureConfig(t *testing.T) *cfg.Config {
	t.Helper()
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, []byte(`{"services":[{"name":"app","type":"application"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	return &cfg.Config{StoreConfig: storetest.ConfigFromFile(t, location)}
}

func insecureParams(endpoint, grpcEndpoint string) *flags.Parameters {
	params := &flags.Parameters{}
	params.Endpoint.Value, params.GRPCEndpoint.Value, params.Insecure.Value = endpoint, grpcEndpoint, "true"
	return params
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the server did not come up")
}

func TestRunServesUntilStopped(t *testing.T) {
	address, grpcAddress := freeAddress(t), freeAddress(t)
	conf := fixtureConfig(t)
	conf.LogConfig = &logging.LogConfig{Level: "loud"}
	conf.AuditLogConfig = &logging.LogConfig{Formatter: "xml"}
	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- run(insecureParams(address, grpcAddress), conf, stop, WithGate(nil)) }()

	waitFor(t, func() bool {
		resp, err := http.Post("http://"+address+"/authz-check/v1/is-allowed", "application/json", strings.NewReader(`{}`))
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	})
	waitFor(t, func() bool {
		conn, err := net.Dial("tcp", grpcAddress)
		if err == nil {
			conn.Close()
		}
		return err == nil
	})
	stop <- os.Interrupt
	if err := <-done; err != nil {
		t.Fatalf("run returned %v after an interrupt", err)
	}
}

func TestRunReportsServeErrors(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	conf := fixtureConfig(t)
	conf.LogConfig = &logging.LogConfig{}
	conf.AuditLogConfig = &logging.LogConfig{}
	if err := run(insecureParams(busy.Addr().String(), ""), conf, nil); err == nil {
		t.Fatal("run must fail when the REST endpoint is taken")
	}
	if err := run(insecureParams(freeAddress(t), busy.Addr().String()), fixtureConfig(t), nil); err == nil {
		t.Fatal("run must fail when the gRPC endpoint is taken")
	}
}

func TestRunReportsSetupErrors(t *testing.T) {
	conf := fixtureConfig(t)
	conf.StoreConfig.StoreType = "nosuchstore"
	if err := run(insecureParams(freeAddress(t), ""), conf, nil); err == nil {
		t.Fatal("run must fail with an unknown store")
	}

	params := insecureParams(freeAddress(t), freeAddress(t))
	params.Insecure.Value = "false"
	if err := run(params, fixtureConfig(t), nil); err == nil {
		t.Fatal("run must fail when the gRPC certificates are missing")
	}
	params.GRPCEndpoint.Value = ""
	if err := run(params, fixtureConfig(t), nil); err == nil {
		t.Fatal("run must fail when the REST certificates are missing")
	}
	params.ClientCertPath.Value = filepath.Join(t.TempDir(), "missing.pem")
	if err := run(params, fixtureConfig(t), nil); err == nil {
		t.Fatal("run must fail with a missing client CA")
	}
}

func TestEvaluatorAssertsTokens(t *testing.T) {
	asserter := assertion.NewTestServer(t, nil)
	defer asserter.Close()
	conf := fixtureConfig(t)
	conf.AsserterWebhookConfig = &assertion.AsserterConfig{Endpoint: asserter.URL + "/assert"}
	evaluator, err := newEvaluator(conf)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &adsapi.RequestContext{Subject: &adsapi.Subject{TokenType: "bearer", Token: "good"}}
	if err := evaluator.AssertToken(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Subject.Principals) != 1 || ctx.Subject.Principals[0].Name != "testUser" {
		t.Fatalf("asserted principals %+v", ctx.Subject.Principals)
	}
	// The test asserter rejects "test-token".
	ctx = &adsapi.RequestContext{Subject: &adsapi.Subject{TokenType: "bearer", Token: "test-token"}}
	if err := evaluator.AssertToken(ctx); err == nil {
		t.Fatal("a rejected token must fail")
	}
	// Without a token nothing is asserted.
	ctx = &adsapi.RequestContext{Subject: &adsapi.Subject{}}
	if err := evaluator.AssertToken(ctx); err != nil || len(ctx.Subject.Principals) != 0 {
		t.Fatalf("no token: %v %+v", err, ctx.Subject.Principals)
	}
}

func TestIsAllowedReturnsAssertedUser(t *testing.T) {
	asserter := assertion.NewTestServer(t, nil)
	defer asserter.Close()
	address := freeAddress(t)
	conf := fixtureConfig(t)
	conf.AsserterWebhookConfig = &assertion.AsserterConfig{Endpoint: asserter.URL + "/assert"}
	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- run(insecureParams(address, ""), conf, stop) }()
	defer func() { stop <- os.Interrupt; <-done }()

	var resp *http.Response
	waitFor(t, func() bool {
		var err error
		resp, err = http.Post("http://"+address+"/authz-check/v1/is-allowed", "application/json",
			strings.NewReader(`{"subject":{"tokenType":"bearer","token":"good"},"serviceName":"app","resource":"r","action":"a"}`))
		return err == nil
	})
	defer resp.Body.Close()
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if resp.Header.Get(svcs.PrincipalsHeader) != "testUser" || body["allowed"] != false {
		t.Fatalf("response %v %v", resp.Header, body)
	}
}

func TestRunRejectsRemovedPolicySource(t *testing.T) {
	conf := &cfg.Config{StoreConfig: &cfg.StoreConfig{StoreType: "policy-source"}}
	stop := make(chan os.Signal)
	close(stop)
	err := run(insecureParams("127.0.0.1:0", ""), conf, stop)
	if err == nil || !strings.Contains(err.Error(), "v1.8.0") || !strings.Contains(err.Error(), "policy-source") {
		t.Fatalf("startup error = %v", err)
	}
}

func run(params *flags.Parameters, conf *cfg.Config, stop <-chan os.Signal, options ...Option) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if stop != nil {
		go func() {
			select {
			case <-stop:
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	return Run(ctx, params, conf, options...)
}

func TestRunBoundsShutdownWithActiveHandler(t *testing.T) {
	rest, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rpc, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := rest.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, insecureParams("", ""), fixtureConfig(t), WithListeners(rest, rpc), WithShutdownTimeout(20*time.Millisecond), WithHTTPServer(func(server *http.Server) {
			base := server.Handler
			server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/block" {
					close(started)
					<-release
					return
				}
				base.ServeHTTP(w, r)
			})
		}))
	}()
	client := &http.Client{Timeout: time.Second}
	waitFor(t, func() bool {
		response, err := client.Get("http://" + address + "/healthz")
		if err != nil {
			return false
		}
		response.Body.Close()
		return response.StatusCode == 200
	})
	requestDone := make(chan error, 1)
	go func() {
		response, err := client.Get("http://" + address + "/block")
		if response != nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked on HTTP handler")
	}
	select {
	case err := <-requestDone:
		if err == nil {
			t.Fatal("active connection was not closed")
		}
	case <-time.After(time.Second):
		t.Fatal("active connection remains open")
	}
	if conn, err := net.DialTimeout("tcp", rpc.Addr().String(), time.Second); err == nil {
		conn.Close()
		t.Fatal("gRPC listener remained open")
	}
}

func TestRunClosesSuppliedListenersOnSetupFailure(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		err = Run(context.Background(), insecureParams("", ""), fixtureConfig(t), WithListeners(listener, nil), WithShutdownTimeout(timeout))
		if err == nil {
			t.Fatal("nonpositive shutdown timeout accepted")
		}
		if conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
			conn.Close()
			t.Fatal("failed startup retained listener")
		}
	}
}
