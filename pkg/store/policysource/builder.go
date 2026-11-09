// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
)

const (
	// StoreType is the store name used in storeConfig.storeType.
	StoreType = "policy-source"

	// Store property keys. They match the configuration documented for the
	// policy-source store.
	PolicySourceURLKey       = "PolicySourceURL"
	CredentialFileKey        = "CredentialFile"
	ServicesKey              = "Services"
	PublicKeysKey            = "PolicySourcePublicKeys"
	CacheDirKey              = "CacheDir"
	RequestTimeoutKey        = "RequestTimeout"
	CAFileKey                = "CAFile"
	ClientCertFileKey        = "ClientCertFile"
	ClientKeyFileKey         = "ClientKeyFile"
	HeartbeatIntervalKey     = "HeartbeatInterval"
	StaleAfterKey            = "StaleAfter"
	StopAfterKey             = "StopAfter"
	DefaultServicesSelection = "*"

	// Command line flag names. Each flag feeds the property key above through
	// the shared GetStoreParams mechanism.
	PolicySourceURLFlag    = "policy-source-url"
	CredentialFileFlag     = "policy-source-credential-file"
	ServicesFlag           = "policy-source-services"
	PublicKeysFlag         = "policy-source-public-keys"
	CacheDirFlag           = "policy-source-cache-dir"
	RequestTimeoutFlag     = "policy-source-request-timeout"
	CAFileFlag             = "policy-source-ca"
	ClientCertFileFlag     = "policy-source-client-cert"
	ClientKeyFileFlag      = "policy-source-client-key"
	HeartbeatIntervalFlag  = "policy-source-heartbeat-interval"
	StaleAfterFlag         = "policy-source-stale-after"
	StopAfterFlag          = "policy-source-stop-after"
	defaultBootstrapWindow = time.Minute
)

// Options configures a Store. The builder derives it from store properties;
// tests and in-process users may build it directly.
type Options struct {
	// PolicySourceURL is the policy source root, for example
	// https://cloud.speedle.io. It must use https.
	PolicySourceURL string
	// Credential is the account-scoped adsi_ instance credential.
	Credential string
	// PublicKeys is the out-of-band trust anchor for snapshots and commits.
	PublicKeys *adsync.PublicKeySet
	// Services selects the services to synchronize. Nil or ["*"] means every
	// service the credential can see; otherwise each entry is a service name
	// or service ULID, and every entry must exist.
	Services []string
	// CacheDir enables the optional per-service on-disk copy cache.
	CacheDir string
	// CAFile adds a PEM CA bundle for enterprise TLS inspection.
	CAFile string
	// ClientCertFile and ClientKeyFile enable mutual TLS when both are set.
	ClientCertFile string
	ClientKeyFile  string
	// RequestTimeout bounds one data request; zero uses the adsync default.
	RequestTimeout time.Duration
	// HeartbeatInterval, StaleAfter and StopAfter configure the adsync state
	// machine; zero values use the protocol defaults.
	HeartbeatInterval time.Duration
	StaleAfter        time.Duration
	StopAfter         time.Duration
	// HTTPClient overrides the transport, for tests and in-process users. It
	// cannot be combined with CAFile, client certificates or a Proxy.
	HTTPClient *http.Client
	// Clock supplies the current time to the state machine. Nil uses the
	// system clock.
	Clock adsync.Clock
	// Retry controls retries of idempotent data requests.
	Retry adsync.RetryPolicy
}

// Builder creates policy-source stores.
type Builder struct{}

// GetStoreParams maps flag names to store property keys.
func (Builder) GetStoreParams() map[string]string {
	return map[string]string{
		PolicySourceURLFlag:   PolicySourceURLKey,
		CredentialFileFlag:    CredentialFileKey,
		ServicesFlag:          ServicesKey,
		PublicKeysFlag:        PublicKeysKey,
		CacheDirFlag:          CacheDirKey,
		RequestTimeoutFlag:    RequestTimeoutKey,
		CAFileFlag:            CAFileKey,
		ClientCertFileFlag:    ClientCertFileKey,
		ClientKeyFileFlag:     ClientKeyFileKey,
		HeartbeatIntervalFlag: HeartbeatIntervalKey,
		StaleAfterFlag:        StaleAfterKey,
		StopAfterFlag:         StopAfterKey,
	}
}

// NewStore validates the store properties and opens a policy-source store.
// One bootstrap synchronization is attempted; a transient failure leaves the
// store empty and fail closed, while a configuration error fails startup.
func (Builder) NewStore(config map[string]interface{}) (pms.PolicyStoreManager, error) {
	options, err := optionsFromConfig(config)
	if err != nil {
		return nil, err
	}
	return Open(context.Background(), options)
}

// optionsFromConfig converts store properties into Options. It performs every
// check that does not need the policy source.
func optionsFromConfig(config map[string]interface{}) (Options, error) {
	var options Options

	url, set, err := stringProperty(config, PolicySourceURLKey)
	if err != nil {
		return options, err
	}
	if !set || url == "" {
		return options, errors.New(errors.ConfigError, "configure item PolicySourceURL is not found")
	}
	options.PolicySourceURL = url

	credential, err := credentialFromConfig(config)
	if err != nil {
		return options, err
	}
	options.Credential = credential

	keysPath, set, err := stringProperty(config, PublicKeysKey)
	if err != nil {
		return options, err
	}
	if !set || keysPath == "" {
		return options, errors.New(errors.ConfigError, "configure item PolicySourcePublicKeys is not found")
	}
	keys, err := adsync.LoadPublicKeysFile(keysPath)
	if err != nil {
		return options, errors.Wrap(err, errors.ConfigError, "invalid policy source public keys")
	}
	options.PublicKeys = keys

	services, err := servicesFromConfig(config)
	if err != nil {
		return options, err
	}
	options.Services = services

	for _, property := range []struct {
		key    string
		target *string
	}{
		{CacheDirKey, &options.CacheDir},
		{CAFileKey, &options.CAFile},
		{ClientCertFileKey, &options.ClientCertFile},
		{ClientKeyFileKey, &options.ClientKeyFile},
	} {
		value, _, err := stringProperty(config, property.key)
		if err != nil {
			return options, err
		}
		*property.target = value
	}

	for _, property := range []struct {
		key    string
		target *time.Duration
	}{
		{RequestTimeoutKey, &options.RequestTimeout},
		{HeartbeatIntervalKey, &options.HeartbeatInterval},
		{StaleAfterKey, &options.StaleAfter},
		{StopAfterKey, &options.StopAfter},
	} {
		value, err := durationProperty(config, property.key)
		if err != nil {
			return options, err
		}
		*property.target = value
	}

	return options, nil
}

// credentialFromConfig reads the instance credential from CredentialFile, or
// from SPEEDLE_ADS_CREDENTIAL when no file is configured. The value is never
// echoed in an error.
func credentialFromConfig(config map[string]interface{}) (string, error) {
	path, set, err := stringProperty(config, CredentialFileKey)
	if err != nil {
		return "", err
	}
	if set && path != "" {
		credential, err := adsync.ReadCredentialFile(path)
		if err != nil {
			return "", errors.Wrap(err, errors.ConfigError, "invalid policy source credential file")
		}
		return credential, nil
	}
	credential, err := adsync.CredentialFromEnvironment()
	if err != nil {
		return "", errors.Wrap(err, errors.ConfigError, "invalid policy source credential")
	}
	if credential == "" {
		return "", errors.Errorf(errors.ConfigError,
			"configure item CredentialFile is not found and %s is not set", adsync.CredentialEnv)
	}
	return credential, nil
}

// servicesFromConfig accepts ["*"], a comma-separated list, or a JSON string
// array (already joined by the flags layer, but accepted directly too).
func servicesFromConfig(config map[string]interface{}) ([]string, error) {
	value, exists := config[ServicesKey]
	if !exists || value == nil {
		return []string{DefaultServicesSelection}, nil
	}
	var entries []string
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, errors.New(errors.ConfigError, "configure item Services must list at least one service or *")
		}
		entries = strings.Split(typed, ",")
	case []string:
		entries = typed
	case []interface{}:
		entries = make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, errors.Errorf(errors.ConfigError, "configure item Services must contain only strings, got %T", item)
			}
			entries = append(entries, text)
		}
	default:
		return nil, errors.Errorf(errors.ConfigError, "configure item Services must be a string list, got %T", value)
	}
	return normalizeServices(entries)
}

// normalizeServices cleans a service selection. Nil and ["*"] mean every
// visible service.
func normalizeServices(entries []string) ([]string, error) {
	if len(entries) == 0 {
		return []string{DefaultServicesSelection}, nil
	}
	cleaned := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	star := false
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, errors.New(errors.ConfigError, "configure item Services contains an empty service name")
		}
		if entry == DefaultServicesSelection {
			star = true
			continue
		}
		if _, duplicate := seen[entry]; duplicate {
			continue
		}
		seen[entry] = struct{}{}
		cleaned = append(cleaned, entry)
	}
	if star && len(cleaned) > 0 {
		return nil, errors.New(errors.ConfigError, "configure item Services cannot combine * with explicit services")
	}
	if star || len(cleaned) == 0 {
		return []string{DefaultServicesSelection}, nil
	}
	return cleaned, nil
}

func stringProperty(config map[string]interface{}, key string) (string, bool, error) {
	value, exists := config[key]
	if !exists || value == nil {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", false, errors.Errorf(errors.ConfigError, "configure item %s must be a string, got %T", key, value)
	}
	return text, true, nil
}

func durationProperty(config map[string]interface{}, key string) (time.Duration, error) {
	text, set, err := stringProperty(config, key)
	if err != nil || !set || strings.TrimSpace(text) == "" {
		return 0, err
	}
	value, err := time.ParseDuration(strings.TrimSpace(text))
	if err != nil {
		return 0, errors.Wrapf(err, errors.ConfigError, "configure item %s is not a duration", key)
	}
	if value < 0 {
		return 0, errors.Errorf(errors.ConfigError, "configure item %s must not be negative", key)
	}
	return value, nil
}

var _ store.StoreBuilder = Builder{}

func init() {
	pflag.String(PolicySourceURLFlag, "", "Store config: policy source base URL (https).")
	pflag.String(CredentialFileFlag, "", "Store config: file holding the ADS instance credential (0600).")
	pflag.String(ServicesFlag, DefaultServicesSelection, "Store config: comma-separated services to sync, or * for all visible services.")
	pflag.String(PublicKeysFlag, "", "Store config: policy source public keys file.")
	pflag.String(CacheDirFlag, "", "Store config: directory for the optional per-service copy cache.")
	pflag.String(RequestTimeoutFlag, "", "Store config: timeout of one policy source data request, for example 10s.")
	pflag.String(CAFileFlag, "", "Store config: PEM CA bundle for the policy source.")
	pflag.String(ClientCertFileFlag, "", "Store config: client certificate for policy source mutual TLS.")
	pflag.String(ClientKeyFileFlag, "", "Store config: client key for policy source mutual TLS.")
	pflag.String(HeartbeatIntervalFlag, "", "Store config: heartbeat interval, for example 30s.")
	pflag.String(StaleAfterFlag, "", "Store config: warning threshold before an instance is stale, for example 60s.")
	pflag.String(StopAfterFlag, "", "Store config: threshold after which an instance stops serving, for example 120s.")

	store.Register(StoreType, Builder{})
}
