package client

import (
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
	"github.com/teramoby/speedle-plus/samples/adsclient/go/src/speedle/api/authz"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

func TestIsAllowed(t *testing.T) {
	location := filepath.Join(t.TempDir(), "policies.json")
	if err := os.WriteFile(location, []byte(`{"services":[{"name":"acao","policies":[{"id":"sample","effect":"grant","principals":[["user:alan.cao"]],"permissions":[{"resource":"/home/acao/Downloads","actions":["read"]}]}]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	manager := storetest.PolicyStore(t, location)
	evaluator, err := eval.NewWithStore(&cfg.Config{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = evaluator.(*eval.PolicyEvalImpl).Close() })
	router, err := adsrest.NewRouter(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(map[string]string{authz.HOST_PROP: host, authz.ADS_PORT_PROP: port, authz.IS_SECURE_PROP: "false"})
	if err != nil {
		t.Fatal(err)
	}
	request := authz.RequestContext{
		Subject:     &authz.Subject{Principals: []*authz.Principal{{Type: "user", Name: "alan.cao"}}},
		ServiceName: "acao", Resource: "/home/acao/Downloads", Action: "read",
	}
	allowed, err := client.IsAllowed(request)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("sample client did not receive the configured grant")
	}
}
