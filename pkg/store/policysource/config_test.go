// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	speedleerrors "github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
)

func TestBuilderRejectsInvalidConfiguration(t *testing.T) {
	valid := map[string]interface{}{UpstreamURLKey: "https://source.example"}
	cases := map[string]interface{}{
		UpstreamURLKey: 42, APIKeyKey: []string{"secret"}, RequestTimeoutKey: "soon",
		HeartbeatIntervalKey: "30", StopAfterKey: "-1s", FallbackWhileSyncingKey: "perhaps",
		TransportKey: "sse", ServeDownstreamKey: true, MaxDownstreamKey: "many", MaxConnectionsKey: 1.5,
		CacheDirKey: 42, CAFileKey: false,
	}
	for key, value := range cases {
		t.Run(key, func(t *testing.T) {
			props := cloneProperties(valid)
			props[key] = value
			if _, err := optionsFromConfig(props); speedleerrors.Code(err) != speedleerrors.ConfigError {
				t.Fatalf("error: %v", err)
			}
		})
	}
	for _, url := range []string{"", "http://source.example", "https://user:password@source.example", "https://source.example?key=secret", "https://source.example#fragment", "://"} {
		if _, err := optionsFromConfig(map[string]interface{}{UpstreamURLKey: url}); err == nil {
			t.Fatalf("invalid URL accepted: %q", url)
		}
	}
	if _, err := optionsFromConfig(nil); err == nil {
		t.Fatal("missing upstream accepted")
	}
	// The builder validates configuration before the registration path is wired in #372.
	if _, err := store.NewStore(StoreType, valid); speedleerrors.Code(err) != speedleerrors.ConfigError {
		t.Fatalf("unregistered client: %v", err)
	}
}

func TestBuilderRegistrationAndParams(t *testing.T) {
	found := false
	for _, name := range store.StoreBuilders() {
		if name == StoreType {
			found = true
		}
	}
	if !found {
		t.Fatal("policy-source store not registered")
	}
	params := store.GetAllStoreParams()
	for flag, key := range (Builder{}).GetStoreParams() {
		if params[flag] != key {
			t.Fatalf("flag %q maps to %q, want %q", flag, params[flag], key)
		}
	}
}

func TestFallbackWhileSyncingProperty(t *testing.T) {
	for _, value := range []interface{}{nil, true, "true", false, "false"} {
		properties := map[string]interface{}{UpstreamURLKey: "https://source.example", FallbackWhileSyncingKey: value}
		options, err := optionsFromConfig(properties)
		if err != nil {
			t.Fatal(err)
		}
		if options.DisableFallback != (value == false || value == "false") {
			t.Fatalf("fallback option for %v", value)
		}
	}
}

func TestOpenValidatesOptions(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	valid := fixture.options

	with := func(mutate func(*Options)) Options {
		options := valid
		mutate(&options)
		return options
	}
	cases := map[string]Options{
		"no public keys": with(func(options *Options) { options.PublicKeys = nil }),
		"no url":         with(func(options *Options) { options.UpstreamURL = "" }),
		"malformed credential": with(func(options *Options) {
			options.Credential = "not-a-credential"
		}),
		"star mixed with a name": with(func(options *Options) {
			options.Services = []string{"*", "books"}
		}),
		"empty service name": with(func(options *Options) {
			options.Services = []string{""}
		}),
		"heartbeat out of range": with(func(options *Options) {
			options.HeartbeatInterval = 5 * time.Second
		}),
		"stale below heartbeat": with(func(options *Options) {
			options.HeartbeatInterval = 30 * time.Second
			options.StaleAfter = 10 * time.Second
		}),
		"client cert without key": with(func(options *Options) {
			options.ClientCertFile = "/tmp/cert.pem"
		}),
		"http client with a ca file": with(func(options *Options) {
			options.CAFile = "/tmp/ca.pem"
		}),
		"cache path is a file": with(func(options *Options) {
			options.CacheDir = writeTestFile(t, "cache")
		}),
	}
	for name, options := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Open(nil, options); speedleerrors.Code(err) != speedleerrors.ConfigError {
				t.Fatalf("error %v, code %s, want %s", err, speedleerrors.Code(err), speedleerrors.ConfigError)
			}
		})
	}
}

func TestNormalizeServices(t *testing.T) {
	cases := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{"nil", nil, []string{"*"}, false},
		{"star", []string{"*"}, []string{"*"}, false},
		{"names", []string{" books ", "orders"}, []string{"books", "orders"}, false},
		{"duplicates", []string{"books", "books"}, []string{"books"}, false},
		{"empty entry", []string{"books", ""}, nil, true},
		{"star mixed", []string{"*", "books"}, nil, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := normalizeServices(testCase.input)
			if testCase.wantErr {
				if speedleerrors.Code(err) != speedleerrors.ConfigError {
					t.Fatalf("error %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("got %v, want %v", got, testCase.want)
			}
		})
	}
}

func cloneProperties(properties map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(properties))
	for key, value := range properties {
		cloned[key] = value
	}
	return cloned
}

func writeTestFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDirectModeConfiguration(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	legacy := fixture.storeProperties()
	keysFile := legacy[PublicKeysKey].(string)

	// An unchanged v1.7.x property set selects the credential mode.
	options, err := optionsFromConfig(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if options.APIKey != "" || options.Credential != testCredential || options.PublicKeys == nil ||
		options.PolicySourceURL != fixture.server.URL || options.FallbackCredential != testCredential {
		t.Fatalf("v1.7.x options = %+v", options)
	}

	direct := map[string]interface{}{UpstreamURLKey: fixture.server.URL, APIKeyKey: testAPIKey}
	options, err = optionsFromConfig(direct)
	if err != nil {
		t.Fatal(err)
	}
	if options.APIKey != testAPIKey || options.Credential != "" || options.PublicKeys != nil || options.PolicySourceURL != fixture.server.URL {
		t.Fatalf("direct options = %+v", options)
	}
	direct[PublicKeysKey] = keysFile
	direct[PolicySourceURLKey] = fixture.server.URL
	if options, err = optionsFromConfig(direct); err != nil || options.PublicKeys == nil {
		t.Fatalf("direct options with keys = %+v, %v", options, err)
	}

	cases := map[string]map[string]interface{}{
		"conflicting URLs":         {PolicySourceURLKey: fixture.server.URL, UpstreamURLKey: "https://other.example", APIKeyKey: testAPIKey},
		"no URL":                   {APIKeyKey: testAPIKey},
		"key is not a string":      {UpstreamURLKey: fixture.server.URL, APIKeyKey: 42},
		"upstream is not a string": {UpstreamURLKey: 42, APIKeyKey: testAPIKey},
		"malformed key":            {UpstreamURLKey: fixture.server.URL, APIKeyKey: "bad " + testAPIKey},
		"key and credential":       {UpstreamURLKey: fixture.server.URL, APIKeyKey: testAPIKey, CredentialFileKey: legacy[CredentialFileKey]},
		"broken keys file":         {UpstreamURLKey: fixture.server.URL, APIKeyKey: testAPIKey, PublicKeysKey: filepath.Join(t.TempDir(), "absent")},
	}
	for name, properties := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := optionsFromConfig(properties)
			wantErrorCode(t, name, err, speedleerrors.ConfigError)
			if strings.Contains(err.Error(), "directModeSecretValue") {
				t.Fatal("the error echoes the key")
			}
		})
	}
}

func TestDirectModeOpenValidation(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	base := Options{PolicySourceURL: fixture.server.URL, APIKey: testAPIKey, HTTPClient: fixture.server.Client()}

	withDownstream := base
	withDownstream.ServeDownstream = true
	_, err := Open(context.Background(), withDownstream)
	wantErrorCode(t, "downstream", err, speedleerrors.ConfigError)

	badID := base
	badID.InstanceID = "01HF7YAT000000000000000A01"
	_, err = Open(context.Background(), badID)
	wantErrorCode(t, "ULID instance id", err, speedleerrors.ConfigError)

	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, adsync.InstanceIDFile), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt := base
	corrupt.CacheDir = cacheDir
	_, err = Open(context.Background(), corrupt)
	wantErrorCode(t, "corrupt instance id", err, speedleerrors.ConfigError)

	fixed := base
	fixed.InstanceID = "01a1d38d-8c69-7ac1-b3d6-cb9ade5c63f4"
	opened, err := Open(context.Background(), fixed)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if opened.client.InstanceID() != fixed.InstanceID || !opened.ForwardWhileStale() {
		t.Fatal("configured instance id or forwarding not used")
	}
	if fixture.source.requestCount() != 0 {
		t.Fatal("a direct-mode store pulled data before its first ack")
	}
	// Without a runtime there is no ack, so Sync is a no-op.
	if err := opened.Sync(context.Background()); err != nil || fixture.source.requestCount() != 0 {
		t.Fatalf("sync without an ack: %v, %d requests", err, fixture.source.requestCount())
	}
	disabled := fixed
	disabled.DisableFallback = true
	opened2, err := Open(context.Background(), disabled)
	if err != nil {
		t.Fatal(err)
	}
	defer opened2.Close()
	if opened2.ForwardWhileStale() {
		t.Fatal("forwarding while stale with forwarding disabled")
	}
}

func TestDirectModeServicesOnlyNarrowTheAssignedSet(t *testing.T) {
	store := &Store{options: Options{APIKey: testAPIKey}, selection: []string{"books", "missing"}}
	assigned := []adsync.Service{{ID: "01HF7YAT000000000000000201", Name: "books"}, {ID: "01HF7YAT000000000000000202", Name: "orders"}}
	selected, err := store.selectServices(assigned)
	if err != nil || len(selected) != 1 || selected[0].Name != "books" {
		t.Fatalf("selected = %+v, %v", selected, err)
	}
}
