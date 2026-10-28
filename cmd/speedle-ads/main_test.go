//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"encoding/json"
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

func fileConfig(t *testing.T) *cfg.Config {
	t.Helper()
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, []byte(`{"services":[{"name":"app","type":"application"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	return &cfg.Config{StoreConfig: &cfg.StoreConfig{StoreType: "file", StoreProps: map[string]interface{}{"FileLocation": location}}}
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
	conf := fileConfig(t)
	conf.LogConfig = &logging.LogConfig{Level: "loud"}
	conf.AuditLogConfig = &logging.LogConfig{Formatter: "xml"}
	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- run(insecureParams(address, grpcAddress), conf, stop) }()

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
	conf := fileConfig(t)
	conf.LogConfig = &logging.LogConfig{}
	conf.AuditLogConfig = &logging.LogConfig{}
	if err := run(insecureParams(busy.Addr().String(), ""), conf, nil); err == nil {
		t.Fatal("run must fail when the REST endpoint is taken")
	}
	if err := run(insecureParams(freeAddress(t), busy.Addr().String()), fileConfig(t), nil); err == nil {
		t.Fatal("run must fail when the gRPC endpoint is taken")
	}
}

func TestRunReportsSetupErrors(t *testing.T) {
	conf := fileConfig(t)
	conf.StoreConfig.StoreType = "nosuchstore"
	if err := run(insecureParams(freeAddress(t), ""), conf, nil); err == nil {
		t.Fatal("run must fail with an unknown store")
	}

	params := insecureParams(freeAddress(t), freeAddress(t))
	params.Insecure.Value = "false"
	if err := run(params, fileConfig(t), nil); err == nil {
		t.Fatal("run must fail when the gRPC certificates are missing")
	}
	params.GRPCEndpoint.Value = ""
	if err := run(params, fileConfig(t), nil); err == nil {
		t.Fatal("run must fail when the REST certificates are missing")
	}
	params.ClientCertPath.Value = filepath.Join(t.TempDir(), "missing.pem")
	if err := run(params, fileConfig(t), nil); err == nil {
		t.Fatal("run must fail with a missing client CA")
	}
}

func TestEvaluatorAssertsTokens(t *testing.T) {
	asserter := assertion.NewTestServer(t, nil)
	defer asserter.Close()
	conf := fileConfig(t)
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
	conf := fileConfig(t)
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

func TestPrintVersionInfo(t *testing.T) {
	productVersion, goVersion, gitCommit = "1.0", "go1.26", "abc"
	printVersionInfo()
}
