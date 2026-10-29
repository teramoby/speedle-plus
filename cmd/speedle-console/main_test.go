//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"bytes"
	"flag"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

var required = []string{"--insecure", "--pms-url", "http://pms:6733", "--ads-url", "http://ads:6734", "--asserter-endpoint", "http://a/assert"}

func TestParseFlags(t *testing.T) {
	args := append([]string{"--bootstrap-admins", " user:alice, ,group:ops", "--authz-cache-ttl", "1s", "--secure-cookie"}, required...)
	cfg, err := parseFlags(args, env(map[string]string{
		"SPEEDLE_CONSOLE_AUTHZ_SERVICE": "my-console",
		"SPEEDLE_CONSOLE_PMS_URL":       "http://ignored",
	}), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PMSURL != "http://pms:6733" || cfg.AuthzService != "my-console" || cfg.AuthzCacheTTL != time.Second ||
		strings.Join(cfg.BootstrapAdmins, "|") != "user:alice|group:ops" || cfg.Endpoint != "0.0.0.0:6735" || !cfg.SecureCookie {
		t.Errorf("config %+v", cfg)
	}
}

func TestParseFlagsFailures(t *testing.T) {
	var out bytes.Buffer
	if _, err := parseFlags([]string{"--version"}, env(nil), &out); err != errVersion || !strings.Contains(out.String(), "speedle-console") {
		t.Errorf("version: %v %q", err, out.String())
	}
	if _, err := parseFlags([]string{"-h"}, env(nil), &out); err != flag.ErrHelp {
		t.Errorf("help: %v", err)
	}
	if _, err := parseFlags([]string{"--nope"}, env(nil), &out); err == nil {
		t.Error("unknown flag accepted")
	}
	if _, err := parseFlags(required, env(map[string]string{"SPEEDLE_CONSOLE_SESSION_TTL": "soon"}), &out); err == nil {
		t.Error("bad environment value accepted")
	}
	if _, err := parseFlags(nil, env(nil), &out); err == nil {
		t.Error("missing URLs accepted")
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func TestRunServesUntilStopped(t *testing.T) {
	address := freeAddress(t)
	cfg, err := parseFlags([]string{"--insecure", "--dev-no-auth", "--endpoint", address,
		"--pms-url", "http://127.0.0.1:1", "--ads-url", "http://127.0.0.1:1"}, env(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- run(cfg, stop) }()
	up := false
	for i := 0; i < 200 && !up; i++ {
		if resp, err := http.Get("http://" + address + "/console/api/config"); err == nil {
			resp.Body.Close()
			up = resp.StatusCode == http.StatusOK
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !up {
		t.Fatal("the console did not come up")
	}
	stop <- os.Interrupt
	if err := <-done; err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestRunReportsFailures(t *testing.T) {
	cfg, _ := parseFlags(required, env(nil), &bytes.Buffer{})
	cfg.UpstreamCA = "missing.crt"
	if err := run(cfg, make(chan os.Signal)); err == nil {
		t.Error("bad configuration accepted")
	}
	// The address is taken.
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	defer listener.Close()
	cfg, _ = parseFlags(required, env(nil), &bytes.Buffer{})
	cfg.Endpoint = listener.Addr().String()
	if err := run(cfg, make(chan os.Signal)); err == nil {
		t.Error("listening on a taken address succeeded")
	}
}
