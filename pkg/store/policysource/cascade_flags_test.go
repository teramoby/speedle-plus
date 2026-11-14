// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
)

func TestCascadeProductionFlagConfiguration(t *testing.T) {
	options := cascadeStoreOptions(t)
	pair, err := tls.LoadX509KeyPair(options.ClientCertFile, options.ClientKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	crl, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: time.Now().Add(-time.Minute), NextUpdate: time.Now().Add(time.Hour)}, pair.Leaf, pair.PrivateKey.(crypto.Signer))
	if err != nil {
		t.Fatal(err)
	}
	crlFile := filepath.Join(t.TempDir(), "relay.crl")
	if err := os.WriteFile(crlFile, crl, 0600); err != nil {
		t.Fatal(err)
	}
	originalFlags := pflag.CommandLine
	for _, test := range []struct {
		name, key   string
		argument    bool
		direct      bool
		environment bool
	}{
		{name: "valid defaults"},
		{name: "direct defaults", direct: true},
		{name: "direct TLS setting", direct: true, key: TLSReloadIntervalKey},
		{name: "removed environment", key: DownstreamCredentialsKey, environment: true},
		{name: "removed config", key: DownstreamCredentialsKey},
		{name: "removed flag", key: DownstreamCredentialsKey, argument: true},
		{name: "removed limit config", key: MaxChainDepthKey},
	} {
		t.Run(test.name, func(t *testing.T) {
			oldArgs, oldFlags := os.Args, pflag.CommandLine
			defer func() { os.Args, pflag.CommandLine = oldArgs, oldFlags }()
			pflag.CommandLine = pflag.NewFlagSet("cascade-startup", pflag.ContinueOnError)
			pflag.CommandLine.SetOutput(io.Discard)
			paramsMap := (Builder{}).GetStoreParams()
			for name := range paramsMap {
				pflag.String(name, originalFlags.Lookup(name).DefValue, "")
			}
			properties := map[string]interface{}{
				UpstreamURLKey:    []string{options.UpstreamURL, "https://backup.example"},
				ClientCertFileKey: options.ClientCertFile, ClientKeyFileKey: options.ClientKeyFile, CAFileKey: options.CAFile, CacheDirKey: options.CacheDir,
				TLSReloadIntervalKey: "1m", CRLFilesKey: []string{crlFile}, CRLURLsKey: []string{}, CRLRefreshIntervalKey: "1h", CRLMaxAgeKey: "24h",
			}
			if test.direct {
				properties[APIKeyKey] = testAPIKey
				properties[UpstreamURLKey] = options.UpstreamURL
				if test.key == "" {
					for _, key := range []string{TLSReloadIntervalKey, CRLFilesKey, CRLURLsKey, CRLRefreshIntervalKey, CRLMaxAgeKey} {
						delete(properties, key)
					}
				}
			}
			if test.environment {
				t.Setenv(flags.FlagToEnv(DownstreamCredentialsFlag), "")
			}
			if test.key != "" && !test.argument && !test.environment {
				properties[test.key] = ""
			}
			data, err := json.Marshal(map[string]interface{}{"storeConfig": map[string]interface{}{"storeType": StoreType, "storeProps": properties}})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			os.Args = []string{"speedle-ads", "--config-file", path}
			if test.argument {
				os.Args = append(os.Args, "--"+DownstreamCredentialsFlag+"=")
			}
			params := &flags.Parameters{}
			params.ParseFlags(flags.DefaultAuthzCheckEndPoint, func() {}, paramsMap)
			config, err := params.Param2Config(paramsMap)
			if err != nil {
				t.Fatal(err)
			}
			props := config.StoreConfig.StoreProps
			for _, key := range []string{TLSReloadIntervalKey, CRLFilesKey, CRLURLsKey, CRLRefreshIntervalKey, CRLMaxAgeKey} {
				if _, exists := props[key]; !exists && !(test.direct && test.key == "") {
					t.Fatalf("config-only %s was lost", key)
				}
			}
			manager, err := (Builder{}).NewStore(props)
			if test.key != "" {
				if err == nil {
					manager.(*Store).Close()
					t.Fatalf("removed %s accepted", test.key)
				}
				if !strings.Contains(err.Error(), test.key) {
					t.Fatalf("wrong migration error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("production defaults prevented startup: %v", err)
			}
			manager.(*Store).Close()
			for _, key := range []string{DownstreamCredentialsKey, MaxDownstreamKey, MaxConnectionsKey, MaxChainDepthKey} {
				if _, exists := props[key]; exists {
					t.Fatalf("removed default %s was injected", key)
				}
			}
		})
	}
}

// TestRemovedPolicySourceFlagsStayOutOfOtherStores proves the removed
// distribution flags are not injected into another store type's properties,
// even when explicitly set.
func TestRemovedPolicySourceFlagsStayOutOfOtherStores(t *testing.T) {
	oldArgs, oldFlags := os.Args, pflag.CommandLine
	defer func() { os.Args, pflag.CommandLine = oldArgs, oldFlags }()
	pflag.CommandLine = pflag.NewFlagSet("store-scope", pflag.ContinueOnError)
	pflag.CommandLine.SetOutput(io.Discard)
	paramsMap := (Builder{}).GetStoreParams()
	for name := range paramsMap {
		pflag.String(name, oldFlags.Lookup(name).DefValue, "")
	}
	os.Args = []string{"speedle-ads", "--store-type=file", "--" + MaxChainDepthFlag + "=3"}
	params := &flags.Parameters{}
	params.ParseFlags(flags.DefaultAuthzCheckEndPoint, func() {}, paramsMap)
	config, err := params.Param2Config(paramsMap)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := config.StoreConfig.StoreProps[MaxChainDepthKey]; exists {
		t.Fatal("a removed policy-source setting leaked into another store's properties")
	}
}
