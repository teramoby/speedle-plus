// Licensed under the Universal Permissive License (UPL) Version 1.0.
package adsgrpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

func newFailingFunctionService(t *testing.T) *GRPCService {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":"function unavailable"}`)
	}))
	t.Cleanup(server.Close)
	policies := fmt.Sprintf(`{
		"functions":[{"name":"boom","funcURL":%q}],
		"services":[{"name":"app",
			"rolePolicies":[{"id":"rp1","effect":"grant","roles":["admin"],"principals":["user:alice"],"condition":"boom() == 1"}],
			"policies":[
				{"id":"p1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"res1","actions":["read"]}]},
				{"id":"p2","effect":"deny","principals":[["user:alice"]],"permissions":[{"resource":"res1","actions":["read"]}],"condition":"boom() == 1"}]}]}`,
		server.URL)
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(policies), 0600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	conf := &cfg.Config{StoreConfig: &cfg.StoreConfig{
		StoreType:  cfg.StorageTypeFile,
		StoreProps: map[string]interface{}{"FileLocation": location},
	}}
	evaluator, err := eval.NewWithStore(conf, storetest.PolicyStore(t, location))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = evaluator.(*eval.PolicyEvalImpl).Close() })
	service, err := NewGRPCService(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// TestGRPCReportsFunctionFailures checks that each decision RPC returns an
// error, rather than an allowed or denied response, when a custom function
// call fails.
func TestGRPCReportsFunctionFailures(t *testing.T) {
	service := newFailingFunctionService(t)
	ctx := context.Background()
	in := request("alice", "res1", "read", nil)
	calls := map[string]func() error{
		"IsAllowed": func() error {
			_, err := service.IsAllowed(ctx, in)
			return err
		},
		"GetAllGrantedRoles": func() error {
			_, err := service.GetAllGrantedRoles(ctx, request("alice", "", "", nil))
			return err
		},
		"GetAllPermissions": func() error {
			_, err := service.GetAllPermissions(ctx, request("alice", "", "", nil))
			return err
		},
		"Diagnose": func() error {
			_, err := service.Diagnose(ctx, in)
			return err
		},
	}
	for name, call := range calls {
		err := call()
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Errorf("%s: error %v does not report the failed function", name, err)
		}
	}
}
