//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
)

func respond(body string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}
}

func TestCustomerFunctionErrors(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"status":       respond(`{}`, http.StatusBadGateway),
		"not json":     respond(`nope`, http.StatusOK),
		"error result": respond(`{"error":"no data"}`, http.StatusOK),
	}
	for name, handler := range cases {
		server := httptest.NewServer(handler)
		_, err := CallCustomerFunction(&pms.Function{Name: name, FuncURL: server.URL}, &ext.CustomerFunctionRequest{})
		if err == nil {
			t.Errorf("%s: no error", name)
		}
		server.Close()
	}
	if _, err := CallCustomerFunction(&pms.Function{Name: "ftp", FuncURL: "ftp://x"}, &ext.CustomerFunctionRequest{}); err == nil {
		t.Error("an unsupported scheme must fail")
	}
	if _, err := CallCustomerFunction(&pms.Function{Name: "bad", FuncURL: "http://a b/"}, &ext.CustomerFunctionRequest{}); err == nil {
		t.Error("an invalid URL must fail")
	}
	closed := httptest.NewServer(respond(`{}`, 200))
	closed.Close()
	if _, err := CallCustomerFunction(&pms.Function{Name: "down", FuncURL: closed.URL}, &ext.CustomerFunctionRequest{}); err == nil {
		t.Error("an unreachable function must fail")
	}
	if _, err := CallCustomerFunction(&pms.Function{Name: "params", FuncURL: closed.URL},
		&ext.CustomerFunctionRequest{Params: []interface{}{make(chan int)}}); err == nil {
		t.Error("parameters that cannot be encoded must fail")
	}
}

func pemOf(server *httptest.Server) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
}

func TestHTTPSCustomerFunctionTrust(t *testing.T) {
	server := httptest.NewTLSServer(respond(`{"result":1}`, http.StatusOK))
	defer server.Close()
	request := &ext.CustomerFunctionRequest{}

	if got, err := CallCustomerFunction(&pms.Function{Name: "own-ca", FuncURL: server.URL, CA: pemOf(server)}, request); err != nil || got != float64(1) {
		t.Fatalf("function CA: %v, %v", got, err)
	}

	old := systemCertPool
	defer func() { systemCertPool = old }()
	systemCertPool = func() (*x509.CertPool, error) { return x509.NewCertPool(), nil }
	if _, err := CallCustomerFunction(&pms.Function{Name: "untrusted", FuncURL: server.URL}, request); err == nil {
		t.Fatal("a server signed by an unknown CA must be rejected")
	}

	// A function without its own CA trusts the system roots; it used to
	// start from an empty pool and reject every publicly signed server.
	systemCertPool = func() (*x509.CertPool, error) {
		pool := x509.NewCertPool()
		pool.AddCert(server.Certificate())
		return pool, nil
	}
	if got, err := CallCustomerFunction(&pms.Function{Name: "system-roots", FuncURL: server.URL}, request); err != nil || got != float64(1) {
		t.Fatalf("system roots: %v, %v", got, err)
	}

	// Without system roots only the function's CA is trusted.
	systemCertPool = func() (*x509.CertPool, error) { return nil, errors.New("no system roots") }
	if _, err := CallCustomerFunction(&pms.Function{Name: "own-ca", FuncURL: server.URL, CA: pemOf(server)}, request); err != nil {
		t.Fatalf("function CA without system roots: %v", err)
	}
}

func TestCustomerFunctionViaDelegator(t *testing.T) {
	var got Request2Delegator
	delegator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{"result":"via delegator"}`)
	}))
	defer delegator.Close()
	cf := &pms.Function{Name: "f", FuncURL: "https://customer/f"}
	result, err := CallCustomerFunctionViaDelegator(delegator.URL, cf, &ext.CustomerFunctionRequest{Params: []interface{}{1.0}})
	if err != nil || result != "via delegator" {
		t.Fatalf("delegator result %v, %v", result, err)
	}
	if got.Function.FuncURL != cf.FuncURL || got.Request.Params[0] != 1.0 {
		t.Fatalf("delegator request %+v", got)
	}
	if _, err := CallCustomerFunctionViaDelegator("http://a b/", cf, &ext.CustomerFunctionRequest{}); err == nil {
		t.Fatal("an invalid delegator URL must fail")
	}
	if _, err := CallCustomerFunctionViaDelegator(delegator.URL, cf, &ext.CustomerFunctionRequest{Params: []interface{}{make(chan int)}}); err == nil {
		t.Fatal("parameters that cannot be encoded must fail")
	}

	// An evaluator with a function service endpoint sends calls there.
	p, _ := newFileEvaluator(t, `{"functions":[{"name":"f","funcURL":"https://customer/f"}],
		"services":[{"name":"app","policies":[{"id":"1","effect":"grant","principals":[["user:alice"]],
		"permissions":[{"resource":"r","actions":["read"]}],"condition":"f() == 'via delegator'"}]}]}`)
	p.RuntimePolicyStore.FuncSvcEndpoint = delegator.URL
	if !allowed(t, p, "app", "alice", "r", "read") {
		t.Fatal("the delegated function result was not used")
	}
}

func TestFunctionResultCacheExpiry(t *testing.T) {
	cache := &FuncResultCache{Results: map[string]FuncResult{}}
	expiring := &pms.Function{Name: "f", ResultCachable: true, ResultTTL: 60}
	cache.AddToCache("f(1)", expiring, "fresh")
	cache.AddToCache("g(1)", &pms.Function{Name: "g", ResultCachable: true}, "forever")
	cache.AddToCache("h(1)", &pms.Function{Name: "h"}, "not cachable")
	if cache.ReadFromCache("f(1)", expiring) != "fresh" || cache.ReadFromCache("h(1)", &pms.Function{Name: "h"}) != nil {
		t.Fatal("unexpected cache content")
	}

	// Expire f(1).
	cache.Results["f(1)"] = FuncResult{Result: "stale", TTL: time.Now().Unix() - 1}
	if got := cache.ReadFromCache("f(1)", expiring); got != nil {
		t.Fatalf("an expired result was returned: %v", got)
	}
	if _, ok := cache.Results["f(1)"]; ok {
		t.Fatal("an expired result read was kept")
	}
	cache.deleteIfExpired("g(1)")
	cache.deleteIfExpired("missing")

	cache.Results["f(2)"] = FuncResult{Result: "stale", TTL: time.Now().Unix() - 1}
	p := &PolicyEvalImpl{RuntimePolicyStore: NewRuntimePolicyStore()}
	p.RuntimePolicyStore.FunctionResultCache = cache
	p.CleanExpiredFunctionResult()
	if _, ok := cache.Results["f(2)"]; ok {
		t.Fatal("CleanExpiredFunctionResult kept an expired result")
	}
	if _, ok := cache.Results["g(1)"]; !ok {
		t.Fatal("CleanExpiredFunctionResult dropped a result without TTL")
	}

	cache.DeleteFromCache("g")
	if len(cache.Results) != 0 {
		t.Fatalf("results of g left: %v", cache.Results)
	}
}

func TestAssertToken(t *testing.T) {
	p, _ := newFileEvaluator(t, `{"services":[{"name":"app","policies":[{"id":"1","effect":"grant",
		"principals":[["user:alice"]],"permissions":[{"resource":"r","actions":["read"]}]}]}]}`)
	calls := 0
	p.SetAsserterFunc(func(ctx *adsapi.RequestContext) error {
		calls++
		if ctx.Subject.Token == "bad" {
			return errors.New("invalid token")
		}
		ctx.Subject.Principals = append(ctx.Subject.Principals, &adsapi.Principal{Type: "user", Name: "alice"})
		return nil
	})
	ctx := adsapi.RequestContext{Subject: &adsapi.Subject{TokenType: "bearer", Token: "good"}, ServiceName: "app", Resource: "r", Action: "read"}
	if ok, _, err := p.IsAllowed(ctx); !ok || err != nil {
		t.Fatalf("asserted user: %v, %v", ok, err)
	}
	asserted := &adsapi.RequestContext{Subject: &adsapi.Subject{TokenType: "bearer", Token: "good", Asserted: true}}
	if err := p.AssertToken(asserted); err != nil || calls != 1 {
		t.Fatalf("an asserted subject was asserted again: %v, %d calls", err, calls)
	}
	ctx.Subject = &adsapi.Subject{TokenType: "bearer", Token: "bad"}
	if _, _, err := p.IsAllowed(ctx); err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("a rejected token: %v", err)
	}
}
