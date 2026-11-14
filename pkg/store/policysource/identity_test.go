// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

func TestUpstreamConfiguration(t *testing.T) {
	for _, key := range []any{"configuration-only-key", cfg.Secret("configuration-only-key")} {
		props := map[string]interface{}{UpstreamURLKey: "https://source.example", APIKeyKey: key}
		options, err := optionsFromConfig(props)
		if err != nil {
			t.Fatal(err)
		}
		if options.UpstreamURL != "https://source.example" || options.APIKey != "configuration-only-key" || options.InstanceID != "" {
			t.Fatalf("unexpected options: %+v", options)
		}
	}
}

func TestRemovedConfigurationRejected(t *testing.T) {
	for key, guidance := range map[string]string{
		"PolicySourceURL": "UpstreamURL", "CredentialFile": "APIKey", "PolicySourcePublicKeys": "removed",
		"Services": "Cloud console", "CloudFallbackURL": "UpstreamURL", "CloudCredentialFile": "APIKey", "InstanceName": "instance ID",
	} {
		for _, value := range []any{nil, "", "old-value", 42} {
			props := map[string]interface{}{UpstreamURLKey: "https://source.example", key: value}
			_, err := optionsFromConfig(props)
			if err == nil || !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), guidance) {
				t.Fatalf("%s: %v", key, err)
			}
		}
	}
	t.Setenv("SPEEDLE_ADS_CREDENTIAL", "removed-secret")
	_, err := optionsFromConfig(map[string]interface{}{UpstreamURLKey: "https://source.example"})
	if err == nil || !strings.Contains(err.Error(), "APIKey") || strings.Contains(err.Error(), "removed-secret") {
		t.Fatalf("legacy environment: %v", err)
	}
}

func TestAPIKeyNeverAppearsInErrorsOrDumps(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	const secret = "unique-private-api-key-371"
	t.Setenv("SPDL_API_KEY", "environment-key")
	t.Setenv("SPDL_POLICY_SOURCE_API_KEY", "environment-key")
	for _, key := range []string{UpstreamURLKey, APIKeyKey, "InstanceName", RequestTimeoutKey, CredentialFileKey, PublicKeysKey, TransportKey} {
		props := fixture.storeProperties()
		props[APIKeyKey] = secret
		if key == UpstreamURLKey {
			props[key] = "http://" + secret
		} else if key == APIKeyKey {
			props[key] = []string{secret}
		} else if key == "InstanceName" {
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
	if err == nil || options.APIKey != "" || !strings.Contains(err.Error(), APIKeyKey) || strings.Contains(err.Error(), "environment-key") {
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

func TestRedactedConfigDumpRejectedAtStartup(t *testing.T) {
	configuration := cfg.StoreConfig{
		StoreType: "policy-source",
		StoreProps: map[string]interface{}{
			UpstreamURLKey: "https://source.example",
			APIKeyKey:      cfg.Secret("private-key-for-config-roundtrip"),
		},
	}
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"APIKey":`+strconv.Quote(cfg.RedactedSecret)) {
		t.Fatal("dump does not contain the expected redaction placeholder")
	}
	var reloaded cfg.StoreConfig
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	_, err = (Builder{}).NewStore(reloaded.StoreProps)
	if err == nil || !strings.Contains(err.Error(), "invalid APIKey") || !strings.Contains(err.Error(), "redacted placeholder") {
		t.Fatalf("startup did not reject the dumped key clearly: %v", err)
	}
	if strings.Contains(err.Error(), cfg.RedactedSecret) || strings.Contains(err.Error(), "private-key-for-config-roundtrip") {
		t.Fatal("startup error echoes the API key value")
	}
}

func TestEngineWithoutPolicySourceIsUnaffected(t *testing.T) {
	t.Setenv("SPEEDLE_ADS_CREDENTIAL", "obsolete-environment-must-not-affect-file-store")
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

func TestEscapedAPIKeyIsRedactedFromErrors(t *testing.T) {
	for _, secret := range []string{`private"quoted`, `private\backslash`, "private\nnewline", "private\ttab", "private\x01control"} {
		quoted := strconv.Quote(secret)
		escaped := quoted[1 : len(quoted)-1]
		for _, property := range []string{TransportKey, RequestTimeoutKey} {
			props := map[string]interface{}{UpstreamURLKey: "https://source.example", APIKeyKey: secret, property: secret}
			_, err := optionsFromConfig(props)
			if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), escaped) {
				t.Fatalf("%s exposed an API key: %v", property, err)
			}
		}
		// Underlying client/file errors may quote a value repeated by accident.
		err := redactConfigError(fmt.Errorf("invalid property %q", secret), cfg.Secret(secret))
		if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), escaped) {
			t.Fatalf("quoted error exposed API key: %v", err)
		}
	}
}
