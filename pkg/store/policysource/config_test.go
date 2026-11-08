// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
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
	fixture := newTestFixture(t, booksService())
	valid := fixture.storeProperties()

	missing := func(key string) map[string]interface{} {
		properties := cloneProperties(valid)
		delete(properties, key)
		return properties
	}
	with := func(key string, value interface{}) map[string]interface{} {
		properties := cloneProperties(valid)
		properties[key] = value
		return properties
	}

	badKeys := filepath.Join(t.TempDir(), "bad-keys.json")
	if err := os.WriteFile(badKeys, []byte(`{"keys":`), 0o644); err != nil {
		t.Fatal(err)
	}
	readableCredential := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(readableCredential, []byte(testCredential), 0o644); err != nil {
		t.Fatal(err)
	}
	notAFile := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(notAFile, 0o700); err != nil {
		t.Fatal(err)
	}

	cases := map[string]map[string]interface{}{
		"missing url":                missing(PolicySourceURLKey),
		"empty url":                  with(PolicySourceURLKey, ""),
		"url is not a string":        with(PolicySourceURLKey, 42),
		"url is not https":           with(PolicySourceURLKey, "http://policy.example"),
		"missing credential":         missing(CredentialFileKey),
		"credential file missing":    with(CredentialFileKey, filepath.Join(t.TempDir(), "absent")),
		"credential file is public":  with(CredentialFileKey, readableCredential),
		"credential file is a dir":   with(CredentialFileKey, notAFile),
		"missing public keys":        missing(PublicKeysKey),
		"public keys file is broken": with(PublicKeysKey, badKeys),
		"services mixed with star":   with(ServicesKey, "*,books"),
		"services has empty entry":   with(ServicesKey, "books,,orders"),
		"services is not a list":     with(ServicesKey, map[string]interface{}{"a": 1}),
		"services has a non-string":  with(ServicesKey, []interface{}{"books", 7}),
		"bad request timeout":        with(RequestTimeoutKey, "soon"),
		"negative request timeout":   with(RequestTimeoutKey, "-1s"),
		"bad heartbeat":              with(HeartbeatIntervalKey, "30"),
		"heartbeat out of range":     with(HeartbeatIntervalKey, "1s"),
		"stop before stale":          with(StopAfterKey, "10s"),
		"client cert without key":    with(ClientCertFileKey, "/tmp/cert.pem"),
	}
	for name, properties := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(adsync.CredentialEnv, "")
			if _, err := store.NewStore(StoreType, properties); speedleerrors.Code(err) != speedleerrors.ConfigError {
				t.Fatalf("error %v, code %s, want %s", err, speedleerrors.Code(err), speedleerrors.ConfigError)
			}
		})
	}
}

func TestBuilderRejectsAnUnknownSelectedService(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	properties := fixture.storeProperties()
	properties[ServicesKey] = "books,missing"
	_, err := store.NewStore(StoreType, properties)
	wantErrorCode(t, "unknown selected service", err, speedleerrors.ConfigError)
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error does not name the unknown service: %v", err)
	}
}

func TestBuilderCreatesTheRegisteredStore(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	manager, err := store.NewStore(StoreType, fixture.storeProperties())
	if err != nil {
		t.Fatal(err)
	}
	policyStore, err := manager.ReadPolicyStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(policyStore.Services) != 1 || policyStore.Services[0].Name != "books" {
		t.Fatalf("services %+v", policyStore.Services)
	}

	found := false
	for _, name := range store.StoreBuilders() {
		if name == StoreType {
			found = true
		}
	}
	if !found {
		t.Fatalf("store type %q is not registered: %v", StoreType, store.StoreBuilders())
	}
	params := store.GetAllStoreParams()
	for flag, key := range (Builder{}).GetStoreParams() {
		if params[flag] != key {
			t.Fatalf("flag %q maps to %q, want %q", flag, params[flag], key)
		}
	}
}

func TestCredentialFromEnvironment(t *testing.T) {
	fixture := newTestFixture(t, booksService())
	t.Setenv(adsync.CredentialEnv, testCredential)
	properties := fixture.storeProperties()
	delete(properties, CredentialFileKey)
	manager, err := store.NewStore(StoreType, properties)
	if err != nil {
		t.Fatal(err)
	}
	policyStore, err := manager.ReadPolicyStore()
	if err != nil || len(policyStore.Services) != 1 {
		t.Fatalf("services %+v, error %v", policyStore.Services, err)
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
		"no url":         with(func(options *Options) { options.PolicySourceURL = "" }),
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
