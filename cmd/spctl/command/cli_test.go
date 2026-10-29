//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsrest"
	"github.com/teramoby/speedle-plus/testutil"
)

// cli runs the fixture spctl with its own home directory, so a user's
// ~/.spctlconfig neither affects nor receives the test settings.
type cli struct {
	t        *testing.T
	home     string
	endpoint string
}

func newCLI(t *testing.T) *cli {
	t.Helper()
	if !filepath.IsAbs(os.Getenv(testutil.SP_APP_NAME)) {
		t.Skip("needs the fixture spctl built by TestMain")
	}
	endpoint := strings.TrimRight(os.Getenv(testutil.PMS_ENDPOINT), "/") + svcs.PolicyMgmtPath
	return &cli{t: t, home: t.TempDir(), endpoint: endpoint}
}

// run runs spctl against the fixture PMS and returns its combined output
// and exit code.
func (c *cli) run(args ...string) (string, int) {
	c.t.Helper()
	return c.runAt(c.endpoint, args...)
}

func (c *cli) runAt(endpoint string, args ...string) (string, int) {
	c.t.Helper()
	if endpoint != "" {
		args = append([]string{"--pms-endpoint", endpoint, "--timeout", "5s"}, args...)
	}
	cmd := exec.Command(os.Getenv(testutil.SP_APP_NAME), args...)
	cmd.Env = append(os.Environ(), "HOME="+c.home)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		c.t.Fatal(err)
	}
	return out.String(), code
}

// expect runs spctl and checks its exit code and that its output contains
// every string in want.
func (c *cli) expect(code int, want []string, args ...string) string {
	c.t.Helper()
	out, got := c.run(args...)
	if got != code {
		c.t.Errorf("spctl %s: exit %d, want %d\n%s", strings.Join(args, " "), got, code, out)
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			c.t.Errorf("spctl %s: output lacks %q\n%s", strings.Join(args, " "), w, out)
		}
	}
	return out
}

func (c *cli) file(name, content string) string {
	c.t.Helper()
	path := filepath.Join(c.t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		c.t.Fatal(err)
	}
	return path
}

func TestCLIUsageErrors(t *testing.T) {
	c := newCLI(t)
	usage := []string{"Usage:"}
	for _, args := range [][]string{
		{"get"}, {"get", "nothing"}, {"get", "service"}, {"get", "policy"}, {"get", "policy", "--service-name", "s"},
		{"get", "function"},
		{"create"}, {"create", "nothing"}, {"create", "service"}, {"create", "policy", "p"},
		{"create", "policy", "--service-name", "s"}, {"create", "function"},
		{"delete"}, {"delete", "nothing"}, {"delete", "service"}, {"delete", "policy"},
		{"delete", "policy", "--service-name", "s"}, {"delete", "function"},
		{"discover"}, {"discover", "nothing"},
	} {
		c.expect(1, usage, args...)
	}
	c.expect(1, []string{"Usage: spctl version"}, "version", "extra")
	c.expect(0, nil, "version")
	c.expect(1, []string{"unknown command"}, "nothing")
}

func TestCLIServicePolicyAndFunctionLifecycle(t *testing.T) {
	c := newCLI(t)
	c.expect(0, []string{"service created"}, "create", "service", "cli-app", "--service-type", "k8s")
	c.expect(1, []string{"already exists"}, "create", "service", "cli-app")
	c.expect(0, []string{`"type": "k8s"`}, "get", "service", "cli-app")
	c.expect(1, []string{"not found"}, "get", "service", "cli-missing")

	out := c.expect(0, []string{"policy created"}, "create", "policy", "p1", "--service-name", "cli-app",
		"--pdl-command", "grant user alice read res1")
	var policy struct{ ID string }
	json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &policy)
	c.expect(1, nil, "create", "policy", "p2", "--service-name", "cli-app", "--pdl-command", "not pdl")
	c.expect(0, []string{"policy created"}, "create", "policy", "--service-name", "cli-app",
		"--json-file", c.file("policy.json", `{"name":"p3","effect":"deny","principals":[["user:bob"]]}`))
	c.expect(1, []string{"no such file"}, "create", "policy", "--service-name", "cli-app", "--json-file", "/nonexistent/policy.json")
	c.expect(0, []string{`"name": "p1"`}, "get", "policy", policy.ID, "--service-name", "cli-app")
	c.expect(1, []string{"not found"}, "get", "policy", "nope", "--service-name", "cli-app")
	c.expect(0, []string{`"name": "p3"`}, "get", "policy", "--all", "--service-name", "cli-app")

	out = c.expect(0, []string{"rolepolicy created"}, "create", "rolepolicy", "rp1", "--service-name", "cli-app",
		"--pdl-command", "grant user alice admin")
	var rolePolicy struct{ ID string }
	json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &rolePolicy)
	c.expect(0, []string{"rolepolicy created"}, "create", "rolepolicy", "--service-name", "cli-app",
		"--json-file", c.file("rp.json", `{"name":"rp2","effect":"grant","roles":["r"],"principals":["user:bob"]}`))
	c.expect(0, []string{`"name": "rp1"`}, "get", "rolepolicy", rolePolicy.ID, "--service-name", "cli-app")
	c.expect(0, []string{`"name": "rp2"`}, "get", "rolepolicy", "--all", "--service-name", "cli-app")
	c.expect(0, []string{"deleted"}, "delete", "rolepolicy", rolePolicy.ID, "--service-name", "cli-app")
	c.expect(1, []string{"404"}, "delete", "rolepolicy", rolePolicy.ID, "--service-name", "cli-app")
	c.expect(0, []string{"deleted"}, "delete", "rolepolicy", "--all", "--service-name", "cli-app")
	c.expect(0, []string{"deleted"}, "delete", "policy", policy.ID, "--service-name", "cli-app")
	c.expect(0, []string{"deleted"}, "delete", "policy", "--all", "--service-name", "cli-app")

	c.expect(0, []string{"function created"}, "create", "function", "cli-f1", "--func-url", "https://f/1", "--cachable", "--cache-ttl", "60")
	c.expect(0, []string{"function created"}, "create", "function",
		"--json-file", c.file("f.json", `{"name":"cli-f2","funcURL":"https://f/2"}`))
	c.expect(1, nil, "create", "function", "--json-file", "/nonexistent/f.json")
	c.expect(0, []string{`"resultTTL": 60`}, "get", "function", "cli-f1")
	c.expect(1, []string{"not found"}, "get", "function", "cli-f9")
	c.expect(0, []string{"cli-f2"}, "get", "function", "--all")
	c.expect(0, []string{"deleted"}, "delete", "function", "cli-f1")
	c.expect(1, []string{"404"}, "delete", "function", "cli-f1")
	c.expect(0, []string{"deleted"}, "delete", "function", "--all")

	c.expect(0, []string{"deleted"}, "delete", "service", "cli-app")
	c.expect(1, []string{"404"}, "delete", "service", "cli-app")
}

func TestCLICreateServiceFromFiles(t *testing.T) {
	c := newCLI(t)
	c.expect(0, []string{"service created"}, "create", "service",
		"--json-file", c.file("svc.json", `{"name":"cli-json","policies":[{"name":"p","effect":"grant"}]}`))
	c.expect(0, []string{`"name": "cli-json"`}, "get", "service", "--all")
	spdl := c.file("svc.spdl", "[service.cli-spdl]\n[policy]\ngrant user alice read books\n[rolepolicy]\ngrant user bob role reader\n")
	c.expect(0, []string{"service created"}, "create", "service", "cli-spdl", "--pdl-file", spdl)
	c.expect(0, []string{"grant"}, "get", "policy", "--all", "--service-name", "cli-spdl")
	c.expect(1, []string{"not found"}, "create", "service", "cli-other", "--pdl-file", spdl)
	c.expect(1, nil, "create", "service", "cli-x", "--pdl-file", "/nonexistent/x.spdl")
	c.expect(1, nil, "create", "service", "--json-file", "/nonexistent/svc.json")
	c.expect(0, []string{"deleted"}, "delete", "service", "cli-json", "cli-spdl")
}

func TestCLIDiscover(t *testing.T) {
	c := newCLI(t)
	c.expect(0, []string{"All requests are deleted"}, "discover", "reset")
	// Without recorded requests the file store answers 404 and etcd an
	// empty result.
	if testutil.GetOSEnv("STORE_TYPE", "file") == "etcd" {
		c.expect(0, []string{"no policy discovered"}, "discover", "policy", "--service-name", "cli-none")
		c.expect(0, []string{"[]"}, "discover", "request")
	} else {
		c.expect(1, []string{"fail to discover policy", "not found"}, "discover", "policy", "--service-name", "cli-none")
		c.expect(1, []string{"not found"}, "discover", "request")
	}
	c.expect(0, []string{"pls specify service name"}, "discover", "policy")
	c.expect(1, []string{"not found"}, "discover", "request", "--last", "--service-name", "cli-none")
	c.expect(0, []string{"are deleted"}, "discover", "reset", "--service-name", "cli-none")
}

// fakeDiscoverPMS serves recorded discover requests: the last request, then
// one more since its revision, then an error that ends --force.
func fakeDiscoverPMS(t *testing.T) string {
	t.Helper()
	request := &ads.RequestContext{ServiceName: "svc", Resource: "r", Action: "a",
		Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}}}}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/fail"):
			w.WriteHeader(http.StatusInternalServerError)
		case strings.Contains(r.URL.Path, "discover-policy"):
			json.NewEncoder(w).Encode(pmsrest.GetDiscoverPoliciesResponse{Revision: 1})
		case r.URL.Query().Get("revision") != "":
			calls++
			if calls > 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(pmsrest.GetDiscoverRequestsResponse{Requests: []*ads.RequestContext{request}, Revision: 2})
		case strings.HasSuffix(r.URL.Path, "/svc") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(pmsrest.GetDiscoverRequestsResponse{Requests: []*ads.RequestContext{request}, Revision: 1})
		case r.URL.Path == "/policy-mgmt/v1/discover-request/broken":
			w.Write([]byte(`not json`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + svcs.PolicyMgmtPath
}

func TestCLIDiscoverAgainstRecordedRequests(t *testing.T) {
	c := newCLI(t)
	endpoint := fakeDiscoverPMS(t)
	out, code := c.runAt(endpoint, "discover", "request", "--last", "--force", "--service-name", "svc")
	if code != 1 || strings.Count(out, `"resource": "r"`) != 2 {
		t.Errorf("--force: exit %d\n%s", code, out)
	}
	out, code = c.runAt(endpoint, "discover", "request", "--last", "--service-name", "svc")
	if code != 0 || !strings.Contains(out, `"resource": "r"`) {
		t.Errorf("--last: exit %d\n%s", code, out)
	}
	out, code = c.runAt(endpoint, "discover", "request", "--service-name", "broken")
	if code != 1 {
		t.Errorf("malformed response: exit %d\n%s", code, out)
	}
	out, _ = c.runAt(endpoint, "discover", "policy", "--service-name", "svc", "--principal-type", "user",
		"--principal-name", "alice", "--principal-IDD", "corp")
	if !strings.Contains(out, "no policy discovered") {
		t.Errorf("discover policy: %s", out)
	}
	out, _ = c.runAt(endpoint, "discover", "policy", "--service-name", "fail")
	if !strings.Contains(out, "fail to discover policy") {
		t.Errorf("discover policy error: %s", out)
	}
	if _, code := c.runAt(endpoint, "discover", "reset", "--service-name", "svc"); code != 1 {
		t.Errorf("reset error: exit %d", code)
	}
	for _, args := range [][]string{
		{"get", "service", "--all"}, {"get", "policy", "--all", "--service-name", "s"}, {"get", "function", "--all"},
		{"delete", "service", "--all"}, {"create", "function", "f"},
	} {
		if _, code := c.runAt(endpoint, args...); code != 1 {
			t.Errorf("spctl %v against a failing PMS: exit %d", args, code)
		}
	}
}

func TestCLIConfig(t *testing.T) {
	c := newCLI(t)
	c.expect(0, nil, "config", "timeout", "7s", "skipverify", "true")
	saved, err := os.ReadFile(filepath.Join(c.home, ".spctlconfig"))
	if err != nil || !strings.Contains(string(saved), "timeout=7s") {
		t.Fatalf("config file %q, %v", saved, err)
	}
	// Settings from the config file become the flag defaults.
	out, code := c.runAt("", "config", "--list")
	if code != 0 || !strings.Contains(out, "timeout = 7s") || !strings.Contains(out, "skipverify = true") {
		t.Errorf("config --list: exit %d\n%s", code, out)
	}

	// An unwritable home fails.
	c.home = filepath.Join(c.home, "missing")
	c.expect(1, []string{"Failed to set config"}, "config", "timeout", "1s")
}

func writeTLSFiles(t *testing.T) (cert, key string) {
	t.Helper()
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "cli"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	keyDER, _ := x509.MarshalECPrivateKey(privateKey)
	dir := t.TempDir()
	cert, key = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600)
	return cert, key
}

func TestCLIHTTPSClient(t *testing.T) {
	c := newCLI(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer server.Close()
	endpoint := server.URL + svcs.PolicyMgmtPath
	caFile := c.file("ca.pem", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})))
	cert, key := writeTLSFiles(t)

	if out, code := c.runAt(endpoint, "get", "service", "--all", "--cacert", caFile, "--cert", cert, "--key", key); code != 0 {
		t.Errorf("trusted server: exit %d\n%s", code, out)
	}
	if out, code := c.runAt(endpoint, "get", "service", "--all", "--skipverify"); code != 0 {
		t.Errorf("--skipverify: exit %d\n%s", code, out)
	}
	if _, code := c.runAt(endpoint, "get", "service", "--all"); code != 1 {
		t.Errorf("an untrusted server must fail, exit %d", code)
	}
	for _, args := range [][]string{
		{"--cacert", "/nonexistent/ca.pem"},
		{"--cert", cert},
		{"--cert", cert, "--key", "/nonexistent/key.pem"},
	} {
		if _, code := c.runAt(endpoint, append([]string{"get", "service", "--all"}, args...)...); code != 1 {
			t.Errorf("TLS settings %v: exit %d, want 1", args, code)
		}
	}
	// A proxy setting is honoured for plain HTTP.
	cmd := exec.Command(os.Getenv(testutil.SP_APP_NAME), "--pms-endpoint", c.endpoint, "get", "service", "--all")
	cmd.Env = append(os.Environ(), "HOME="+c.home, "http_proxy=http://127.0.0.1:1", "NO_PROXY=127.0.0.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("with http_proxy: %v\n%s", err, out)
	}
}

func TestCLICheck(t *testing.T) {
	c := newCLI(t)
	// check works without PMS.
	if out, code := c.runAt("", "check", "policy", "grant user alice read /x if a > 1"); code != 0 || out != "policy OK\n" {
		t.Errorf("valid policy: exit %d\n%s", code, out)
	}
	if out, code := c.runAt("", "check", "policy", "grant user alice read /x if a >"); code != 1 || !strings.Contains(out, "Invalid condition: Unexpected end of expression") {
		t.Errorf("invalid policy: exit %d\n%s", code, out)
	}
	spdl := filepath.Join(t.TempDir(), "p.spdl")
	os.WriteFile(spdl, []byte("[service.a]\n[policy]\nGRANT USER alice read /x if a >\n"), 0600)
	if out, code := c.runAt("", "check", "--file", spdl); code != 1 || !strings.Contains(out, spdl+":3: Invalid condition") {
		t.Errorf("invalid file: exit %d\n%s", code, out)
	}
	if out, code := c.runAt("", "check"); code != 1 || !strings.Contains(out, "Usage:") {
		t.Errorf("no arguments: exit %d\n%s", code, out)
	}
}
