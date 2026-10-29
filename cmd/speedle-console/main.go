//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/console"
)

var gitCommit string
var productVersion string
var goVersion string

func printVersionInfo(w io.Writer) {
	fmt.Fprintf(w, "speedle-console:\n")
	fmt.Fprintf(w, " Version:       %s\n", productVersion)
	fmt.Fprintf(w, " Go Version:    %s\n", goVersion)
	fmt.Fprintf(w, " Git commit:    %s\n", gitCommit)
}

// errVersion means --version was given and the version printed.
var errVersion = fmt.Errorf("version printed")

// parseFlags reads the configuration from args. Every flag can also be set
// with an environment variable: --pms-url is SPEEDLE_CONSOLE_PMS_URL.
func parseFlags(args []string, getenv func(string) string, out io.Writer) (*console.Config, error) {
	cfg := console.DefaultConfig()
	fs := flag.NewFlagSet("speedle-console", flag.ContinueOnError)
	fs.SetOutput(out)
	var bootstrap string
	var version bool
	fs.StringVar(&cfg.Endpoint, "endpoint", cfg.Endpoint, "Address the console listens on.")
	fs.BoolVar(&cfg.Insecure, "insecure", false, "Serve plain HTTP instead of HTTPS.")
	fs.StringVar(&cfg.CertPath, "cert", "", "TLS certificate of the console.")
	fs.StringVar(&cfg.KeyPath, "key", "", "TLS private key of the console.")
	fs.StringVar(&cfg.PMSURL, "pms-url", "", "Base URL of the policy management service, such as https://pms:6733.")
	fs.StringVar(&cfg.ADSURL, "ads-url", "", "Base URL of the authorization decision service, such as https://ads:6734.")
	fs.StringVar(&cfg.UpstreamCA, "upstream-ca", "", "CA certificate that verifies PMS and ADS.")
	fs.StringVar(&cfg.UpstreamCert, "upstream-cert", "", "Client certificate the console presents to PMS and ADS.")
	fs.StringVar(&cfg.UpstreamKey, "upstream-key", "", "Private key of the upstream client certificate.")
	fs.DurationVar(&cfg.UpstreamTimeout, "upstream-timeout", cfg.UpstreamTimeout, "Timeout of calls to PMS and ADS.")
	fs.StringVar(&cfg.Asserter.Endpoint, "asserter-endpoint", "", "Token asserter webhook that signs users in.")
	fs.StringVar(&cfg.Asserter.CACert, "asserter-ca", "", "CA certificate that verifies the token asserter.")
	fs.StringVar(&cfg.Asserter.ClientCert, "asserter-cert", "", "Client certificate the console presents to the token asserter.")
	fs.StringVar(&cfg.Asserter.ClientKey, "asserter-key", "", "Private key of the token asserter client certificate.")
	fs.StringVar(&cfg.SessionKeyFile, "session-key-file", "", "File with the key (32 bytes or more) that signs session cookies. Required with more than one replica.")
	fs.DurationVar(&cfg.SessionTTL, "session-ttl", cfg.SessionTTL, "How long a sign-in lasts.")
	fs.StringVar(&cfg.AuthzService, "authz-service", cfg.AuthzService, "Speedle service whose policies grant read and write on the console resource.")
	fs.DurationVar(&cfg.AuthzCacheTTL, "authz-cache-ttl", cfg.AuthzCacheTTL, "How long authorization decisions are remembered.")
	fs.StringVar(&bootstrap, "bootstrap-admins", "", "Comma-separated principals (type:name) with full access without policies. Remove once console policies exist.")
	fs.BoolVar(&cfg.DevNoAuth, "dev-no-auth", false, "Development only: no sign-in, everyone is an administrator. Needs a loopback endpoint.")
	fs.BoolVar(&version, "version", false, "Print the version and exit.")

	// Environment variables first, so flags on the command line win.
	var envErr error
	fs.VisitAll(func(f *flag.Flag) {
		name := "SPEEDLE_CONSOLE_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if value := getenv(name); value != "" && envErr == nil {
			if err := f.Value.Set(value); err != nil {
				envErr = fmt.Errorf("%s: %v", name, err)
			}
		}
	})
	if envErr != nil {
		return nil, envErr
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if version {
		printVersionInfo(out)
		return nil, errVersion
	}
	for _, p := range strings.Split(bootstrap, ",") {
		if p = strings.TrimSpace(p); p != "" {
			cfg.BootstrapAdmins = append(cfg.BootstrapAdmins, p)
		}
	}
	return &cfg, cfg.Validate()
}

// run serves the console until it fails or stop receives a signal.
func run(cfg *console.Config, stop <-chan os.Signal) error {
	server, err := console.NewServer(cfg)
	if err != nil {
		return err
	}
	errChan := make(chan error, 1)
	go func() {
		log.Infof("Starting the policy management console on %s...", cfg.Endpoint)
		errChan <- console.ServerParams(cfg).ListenAndServe(server)
	}()
	select {
	case err = <-errChan:
	case <-stop:
		log.Info("Interrupt signal")
	}
	log.Info("Stopping the console...")
	server.Shutdown(context.Background())
	return err
}

func main() {
	cfg, err := parseFlags(os.Args[1:], os.Getenv, os.Stderr)
	if err == errVersion {
		return
	}
	if err != nil {
		if err != flag.ErrHelp {
			log.Error(err)
		}
		os.Exit(2)
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	if err := run(cfg, stop); err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
