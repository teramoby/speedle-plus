// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
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
