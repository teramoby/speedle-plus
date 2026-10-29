//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package console serves the policy management console: a web page and an
// API that signs users in and forwards their calls to PMS and ADS.
package console

import (
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// DefaultEndpoint is where the console listens unless told otherwise.
const DefaultEndpoint = "0.0.0.0:6735"

// Config configures the console.
type Config struct {
	// Endpoint is the address the console listens on.
	Endpoint string
	// Insecure serves plain HTTP; otherwise CertPath and KeyPath are used.
	Insecure bool
	CertPath string
	KeyPath  string
	// SecureCookie marks the session cookie Secure even over plain HTTP,
	// for a console behind a proxy that terminates TLS.
	SecureCookie bool

	// PMSURL and ADSURL are the base URLs of the services, such as
	// https://pms:6733.
	PMSURL string
	ADSURL string
	// UpstreamCA verifies PMS and ADS; UpstreamCert and UpstreamKey are the
	// console's client certificate for mutual TLS.
	UpstreamCA      string
	UpstreamCert    string
	UpstreamKey     string
	UpstreamTimeout time.Duration

	// Asserter turns login tokens into principals.
	Asserter assertion.AsserterConfig

	// SessionKeyFile holds the key that signs session cookies. Without it a
	// random key is used, which only suits a single replica.
	SessionKeyFile string
	SessionTTL     time.Duration

	// AuthzService is the Speedle service whose policies grant the read
	// and write actions on the console resource.
	AuthzService  string
	AuthzCacheTTL time.Duration

	// BootstrapAdmins are principals (type:name) with full access that ADS
	// is not asked about. Use them to write the first console policies.
	BootstrapAdmins []string
	// DevNoAuth signs everyone in with full access. Loopback only.
	DevNoAuth bool
}

// DefaultConfig returns the settings used when no flag is given.
func DefaultConfig() Config {
	return Config{
		Endpoint:        DefaultEndpoint,
		UpstreamTimeout: 30 * time.Second,
		SessionTTL:      8 * time.Hour,
		AuthzService:    "speedle-console",
		AuthzCacheTTL:   5 * time.Second,
	}
}

func checkURL(name, value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.Errorf(errors.ConfigError, "%s must be an http or https URL, got %q", name, value)
	}
	return nil
}

// IsLoopback reports whether endpoint only listens on the loopback interface.
func IsLoopback(endpoint string) bool {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Validate reports the first setting that cannot work.
func (c *Config) Validate() error {
	if _, _, err := net.SplitHostPort(c.Endpoint); err != nil {
		return errors.Errorf(errors.ConfigError, "endpoint %q is not host:port", c.Endpoint)
	}
	if !c.Insecure && (c.CertPath == "" || c.KeyPath == "") {
		return errors.New(errors.ConfigError, "TLS needs a certificate and a key; set insecure to serve plain HTTP")
	}
	if err := checkURL("the PMS URL", c.PMSURL); err != nil {
		return err
	}
	if err := checkURL("the ADS URL", c.ADSURL); err != nil {
		return err
	}
	if (c.UpstreamCert == "") != (c.UpstreamKey == "") {
		return errors.New(errors.ConfigError, "the upstream client certificate and key must be set together")
	}
	if c.SessionTTL <= 0 || c.UpstreamTimeout <= 0 || c.AuthzCacheTTL < 0 {
		return errors.New(errors.ConfigError, "the session lifetime and upstream timeout must be positive, and the authorization cache lifetime not negative")
	}
	if c.DevNoAuth {
		if !IsLoopback(c.Endpoint) {
			return errors.Errorf(errors.ConfigError, "development mode only listens on a loopback address, not %q", c.Endpoint)
		}
		return nil
	}
	if c.Asserter.Endpoint == "" {
		return errors.New(errors.ConfigError, "the token asserter endpoint is required to sign users in")
	}
	if c.AuthzService == "" {
		return errors.New(errors.ConfigError, "the authorization service name is required")
	}
	for _, p := range c.BootstrapAdmins {
		if typ, name, ok := strings.Cut(p, ":"); !ok || typ == "" || name == "" {
			return errors.Errorf(errors.ConfigError, "bootstrap admin %q must look like type:name, such as user:alice", p)
		}
	}
	return nil
}
