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
)

var globalFlags struct {
	PMSEndpoint        string
	Timeout            time.Duration
	CertFile           string
	KeyFile            string
	CAFile             string
	InsecureSkipVerify bool
}

const (
	configFile = ".spctlconfig"
)

func httpClient() (*http.Client, error) {
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

func readConfigFile() (map[string]string, error) {
	flags := make(map[string]string)
	// $HOME, so a different home (as in the tests) keeps its own settings.
	home, err := os.UserHomeDir()
	if err != nil {
		return flags, err
	}
	cfg := path.Join(home, configFile)
	if _, err = os.Stat(cfg); !os.IsNotExist(err) {
		f, err := os.Open(cfg)
		if err != nil {
			return flags, err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			kv := strings.Split(scanner.Text(), "=")
			if len(kv) == 2 {
				k := strings.Trim(kv[0], " ")
				v := strings.Trim(kv[1], " ")
				flags[k] = v
			}
		}
	}
	return flags, nil
}

func writeConfigFile(flags map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfg := path.Join(home, configFile)
	f, err := os.OpenFile(cfg, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for name, val := range flags {
		fmt.Fprintf(w, "%s=%s\n", name, val)
	}
	return w.Flush()
}
