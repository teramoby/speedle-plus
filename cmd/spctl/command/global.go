//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

var globalFlags struct {
	PMSEndpoint        string
	Token              string
	Timeout            time.Duration
	CertFile           string
	KeyFile            string
	CAFile             string
	InsecureSkipVerify bool
}

const (
	configFile      = ".spctlconfig"
	cloudConfigFile = ".speedle"
)

func environmentFlagName(name string) string {
	return "SPCTL_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// applyEnvironmentConfig overlays SPCTL_* variables on file settings. Cobra
// applies explicit command-line flags afterwards, so they take precedence.
func applyEnvironmentConfig(flags *pflag.FlagSet) {
	flags.VisitAll(func(f *pflag.Flag) {
		name := environmentFlagName(f.Name)
		value, present := os.LookupEnv(name)
		if !present {
			return
		}
		if err := flags.Set(f.Name, value); err != nil {
			fmt.Fprintf(os.Stderr, "Ignoring invalid %s setting: %v\n", name, err)
		}
	})
}

func httpClient() (*http.Client, error) {
	if globalFlags.Token != "" && globalFlags.InsecureSkipVerify {
		return nil, fmt.Errorf("refusing to send an API token with --skipverify; use --cacert to trust the server certificate")
	}

	var tr *http.Transport
	proxy := os.Getenv("http_proxy")
	if proxy == "" {
		tr = &http.Transport{}
	} else {
		tr = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}
	}

	if strings.HasPrefix(strings.ToLower(globalFlags.PMSEndpoint), "http:") {
		return &http.Client{
			Timeout:   globalFlags.Timeout,
			Transport: tr,
		}, nil
	}

	// PMSEndpoint is https, setup tls
	tlsConfig := &tls.Config{}
	tlsConfig.InsecureSkipVerify = globalFlags.InsecureSkipVerify
	if globalFlags.CAFile != "" {
		caCert, err := os.ReadFile(globalFlags.CAFile)
		if err != nil {
			log.Fatal(err)
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	if globalFlags.CertFile != "" {
		if globalFlags.KeyFile == "" {
			err := fmt.Errorf("TLS KeyFile is not specified")
			log.Fatal(err)
			return nil, err
		}
		cert, err := tls.LoadX509KeyPair(globalFlags.CertFile, globalFlags.KeyFile)
		if err != nil {
			log.Fatal(err)
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return &http.Client{
		Timeout: globalFlags.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

// readConfigFile reads the legacy settings first and the Cloud settings second.
// Environment variables are applied next, and command-line flags are applied
// last by Cobra: flag > environment > Cloud file > legacy file.
func readConfigFile() (map[string]string, error) {
	flags, err := readConfig(configFile)
	if err != nil {
		return flags, err
	}
	cloudFlags, err := readConfig(cloudConfigFile)
	if err != nil {
		return flags, err
	}
	for name, value := range cloudFlags {
		flags[name] = value
	}
	return flags, nil
}

func readConfig(name string) (map[string]string, error) {
	flags := make(map[string]string)
	// $HOME, so a different home (as in the tests) keeps its own settings.
	home, err := os.UserHomeDir()
	if err != nil {
		return flags, err
	}
	cfg := path.Join(home, name)
	f, err := os.Open(cfg)
	if os.IsNotExist(err) {
		return flags, nil
	}
	if err != nil {
		return flags, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		kv := strings.SplitN(scanner.Text(), "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			flags[k] = v
		}
	}
	if err := scanner.Err(); err != nil {
		return flags, err
	}
	if name == cloudConfigFile && flags["token"] != "" {
		info, err := f.Stat()
		if err != nil {
			return flags, err
		}
		if info.Mode().Perm()&0077 != 0 {
			fmt.Fprintf(os.Stderr, "warning: %s contains an API token and has mode %04o; restrict it with chmod 0600 %q\n", cfg, info.Mode().Perm(), cfg)
		}
	}
	return flags, nil
}

func writeConfigFile(flags map[string]string) error {
	return writeConfig(configFile, flags)
}

func writeCloudConfigFile(flags map[string]string) error {
	return writeConfig(cloudConfigFile, flags)
}

func writeConfig(name string, flags map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfg := path.Join(home, name)
	f, err := os.OpenFile(cfg, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for name, val := range flags {
		fmt.Fprintf(w, "%s=%s\n", name, val)
	}
	return w.Flush()
}
