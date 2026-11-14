// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	speedleerrors "github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
)

func TestBuilderRejectsInvalidConfiguration(t *testing.T) {
	valid := map[string]interface{}{UpstreamURLKey: "https://source.example"}
	cases := map[string]interface{}{
		UpstreamURLKey: 42, APIKeyKey: []string{"secret"}, RequestTimeoutKey: "soon",
		HeartbeatIntervalKey: "30", StopAfterKey: "-1s", FallbackWhileSyncingKey: "perhaps",
		TransportKey: "sse", MaxDownstreamKey: "many", MaxConnectionsKey: 1.5,
		MaxChainDepthKey: 3, DownstreamCredentialsKey: "/tmp/credentials",
		ServeDownstreamKey: "perhaps", CacheDirKey: 42, CAFileKey: false,
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
		"no API key": with(func(options *Options) { options.APIKey = "" }),
		"no url":     with(func(options *Options) { options.UpstreamURL = "" }),
		"malformed API key": with(func(options *Options) {
			options.APIKey = "not a key"
		}),
		"ULID instance id": with(func(options *Options) {
			options.InstanceID = testServiceID1
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

// TestOpenUsesThePersistedInstanceID opens stores the way production does:
// the instance ID is created on the first request, kept in CacheDir and
// reused by the next start, and a corrupt ID file fails that request instead
// of sending one without an identity.
func TestOpenUsesThePersistedInstanceID(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	cacheDir := filepath.Join(t.TempDir(), "cache")
	persisted := func(options *Options) { options.InstanceID = ""; options.CacheDir = cacheDir }
	first := fixture.open(persisted)
	defer first.Close()
	id, err := first.client.InstanceID()
	if err != nil || !adsync.ValidInstanceID(id) {
		t.Fatalf("instance id = %q, %v", id, err)
	}
	second := fixture.open(persisted)
	defer second.Close()
	if again, _ := second.client.InstanceID(); again != id {
		t.Fatalf("restart id = %q, want %q", again, id)
	}
	if !first.FallbackAvailable() || !first.StaleIsUntrusted() {
		t.Fatal("forwarding is off by default")
	}

	corruptDir := filepath.Join(t.TempDir(), "corrupt")
	if err := os.MkdirAll(corruptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, adsync.InstanceIDFile), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := fixture.options
	options.InstanceID = ""
	options.CacheDir = corruptDir
	options.DisableFallback = true
	before := fixture.source.requestCount()
	corrupt, err := Open(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer corrupt.Close()
	if fixture.source.requestCount() != before {
		t.Fatal("a request was sent without a valid instance id")
	}
	if corrupt.FallbackAvailable() {
		t.Fatal("forwarding with forwarding disabled")
	}
}

// TestRuntimeStoreWaitsForTheFirstAck opens a store for the owned runtime: it
// fetches nothing before the first channel ack, and Sync is a no-op until
// then.
func TestRuntimeStoreWaitsForTheFirstAck(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	options := fixture.options
	options.StartRuntime = true
	opened, err := Open(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if err := opened.Sync(context.Background()); err != nil || fixture.source.requestCount() != 0 {
		t.Fatalf("sync without an ack: %v, %d requests", err, fixture.source.requestCount())
	}
}
