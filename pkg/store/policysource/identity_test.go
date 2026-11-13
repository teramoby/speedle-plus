// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

func TestStoreIdentityAliases(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	for _, name := range []string{"legacy", "upstream", "both"} {
		t.Run(name, func(t *testing.T) {
			props := fixture.storeProperties()
			if name != "legacy" {
				props[UpstreamURLKey] = props[PolicySourceURLKey]
			}
			if name == "upstream" {
				delete(props, PolicySourceURLKey)
			}
			props[APIKeyKey] = "configuration-only-key"
			props[InstanceNameKey] = "engine-01.local"
			manager, err := (Builder{}).NewStore(props)
			if err != nil {
				t.Fatal(err)
			}
			s := manager.(*Store)
			defer s.Close()
			if s.options.UpstreamURL != fixture.server.URL || s.options.PolicySourceURL != fixture.server.URL || s.options.APIKey != "configuration-only-key" || s.options.InstanceName != "engine-01.local" {
				t.Fatalf("options: %+v", s.options)
			}
			got, err := s.ReadPolicyStore()
			if err != nil || len(got.Services) != 1 || got.Services[0].Name != "books" {
				t.Fatalf("legacy behavior: %v, %v", got, err)
			}
		})
	}
	props := fixture.storeProperties()
	props[UpstreamURLKey] = "https://different.example"
	if _, err := (Builder{}).NewStore(props); err == nil {
		t.Fatal("conflicting aliases accepted")
	}
	options := fixture.options
	options.UpstreamURL = "https://different.example"
	if _, err := Open(context.Background(), options); err == nil {
		t.Fatal("conflicting in-process aliases accepted")
	}
}

func TestInstanceNameValidation(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	for _, name := range []string{"a", strings.Repeat("a", 64), "Engine_1.example-2"} {
		props := fixture.storeProperties()
		props[InstanceNameKey] = name
		if _, err := optionsFromConfig(props); err != nil {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	for _, name := range []any{strings.Repeat("a", 65), "-engine", "with space", "引擎", 1} {
		props := fixture.storeProperties()
		props[InstanceNameKey] = name
		if _, err := optionsFromConfig(props); err == nil {
			t.Fatalf("invalid name %v accepted", name)
		}
	}
	options, err := optionsFromConfig(fixture.storeProperties())
	if err != nil {
		t.Fatal(err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if options.InstanceName != hostname {
		t.Fatalf("default name = %q, want hostname", options.InstanceName)
	}
}

func TestAPIKeyNeverAppearsInErrorsOrDumps(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	const secret = "unique-private-api-key-371"
	t.Setenv("SPDL_API_KEY", "environment-key")
	t.Setenv("SPDL_POLICY_SOURCE_API_KEY", "environment-key")
	for _, key := range []string{UpstreamURLKey, APIKeyKey, InstanceNameKey, RequestTimeoutKey, CredentialFileKey, PublicKeysKey, TransportKey} {
		props := fixture.storeProperties()
		props[APIKeyKey] = secret
		if key == UpstreamURLKey {
			props[key] = "https://" + secret
		} else if key == APIKeyKey {
			props[key] = []string{secret}
		} else if key == InstanceNameKey {
			props[key] = "-" + secret
		} else {
			props[key] = secret
		}
		_, err := (Builder{}).NewStore(props)
		if err == nil || strings.Contains(fmt.Sprintf("%+v", err), secret) {
			t.Fatalf("property %s: unsafe or missing error: %v", key, err)
		}
	}
	props := fixture.storeProperties()
	options, err := optionsFromConfig(props)
	if err != nil || options.APIKey != "" {
		t.Fatalf("APIKey read from environment: %v", err)
	}
	props[APIKeyKey] = cfg.Secret(secret)
	options, err = optionsFromConfig(props)
	if err != nil || string(options.APIKey) != secret {
		t.Fatalf("key not retained: %v", err)
	}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, options), secret) {
			t.Fatal("Options dump exposed APIKey")
		}
	}
	data, err := json.Marshal(options)
	// Options includes non-JSON fields such as clocks and HTTP clients. Even
	// an unsupported-type error must never contain the credential.
	if strings.Contains(string(data)+fmt.Sprint(err), secret) {
		t.Fatal("JSON dump exposed APIKey")
	}
	for flag, property := range (Builder{}).GetStoreParams() {
		if property == APIKeyKey || strings.Contains(flag, "api-key") {
			t.Fatal("APIKey has a flag binding")
		}
	}
	if pflag.Lookup("policy-source-api-key") != nil {
		t.Fatal("APIKey flag registered")
	}
}

func TestLegacyFileExampleIsUnaffected(t *testing.T) {
	// Load the repository's pre-policy-source configuration verbatim. Only
	// the working directory supplies its relative ./ps.json policy file.
	data, err := os.ReadFile("../../cfg/config_file.json")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "config_file.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ps.json"), []byte(`{"services":[{"name":"legacy","policies":[],"rolePolicies":[]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(directory)
	configuration, err := cfg.ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.StoreConfig.StoreType != "file" || configuration.StoreConfig.StoreProps["FileLocation"] != "./ps.json" || !configuration.EnableWatch {
		t.Fatal("legacy example changed")
	}
	manager, err := store.NewStore(configuration.StoreConfig.StoreType, configuration.StoreConfig.StoreProps)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := manager.ReadPolicyStore()
	if err != nil || len(policies.Services) != 1 || policies.Services[0].Name != "legacy" {
		t.Fatalf("legacy example behavior: %v, %v", policies, err)
	}
}
