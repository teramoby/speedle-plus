//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package console

import (
	"testing"
)

func validConfig() Config {
	cfg := DefaultConfig()
	cfg.Endpoint = "127.0.0.1:6735"
	cfg.Insecure = true
	cfg.PMSURL = "http://pms:6733"
	cfg.ADSURL = "https://ads:6734"
	cfg.Asserter.Endpoint = "http://asserter/assert"
	return cfg
}

func TestValidate(t *testing.T) {
	good := validConfig()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Config){
		"endpoint":             func(c *Config) { c.Endpoint = "nope" },
		"TLS without files":    func(c *Config) { c.Insecure = false },
		"PMS URL":              func(c *Config) { c.PMSURL = "pms:6733" },
		"ADS URL":              func(c *Config) { c.ADSURL = "ftp://ads" },
		"bad URL":              func(c *Config) { c.ADSURL = "http://%zz" },
		"half a client pair":   func(c *Config) { c.UpstreamCert = "c.pem" },
		"session lifetime":     func(c *Config) { c.SessionTTL = 0 },
		"upstream timeout":     func(c *Config) { c.UpstreamTimeout = 0 },
		"cache lifetime":       func(c *Config) { c.AuthzCacheTTL = -1 },
		"no asserter":          func(c *Config) { c.Asserter.Endpoint = "" },
		"no authz service":     func(c *Config) { c.AuthzService = "" },
		"bad bootstrap admin":  func(c *Config) { c.BootstrapAdmins = []string{"alice"} },
		"empty bootstrap name": func(c *Config) { c.BootstrapAdmins = []string{"user:"} },
		"dev mode in public":   func(c *Config) { c.DevNoAuth, c.Endpoint = true, "0.0.0.0:6735" },
	} {
		cfg := validConfig()
		change(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for name, change := range map[string]func(*Config){
		"TLS":             func(c *Config) { c.Insecure, c.CertPath, c.KeyPath = false, "c", "k" },
		"bootstrap admin": func(c *Config) { c.BootstrapAdmins = []string{"user:alice"} },
		"dev mode":        func(c *Config) { c.DevNoAuth, c.Asserter.Endpoint = true, "" },
	} {
		cfg := validConfig()
		change(&cfg)
		if err := cfg.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	for endpoint, want := range map[string]bool{
		"127.0.0.1:1": true, "[::1]:1": true, "localhost:1": true, "LOCALHOST:1": true,
		"0.0.0.0:1": false, ":1": false, "example.com:1": false, "10.0.0.1:1": false, "nope": false,
	} {
		if got := IsLoopback(endpoint); got != want {
			t.Errorf("IsLoopback(%q) = %v", endpoint, got)
		}
	}
}
