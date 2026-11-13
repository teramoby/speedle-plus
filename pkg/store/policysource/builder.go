// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
)

const (
	// StoreType is the store name used in storeConfig.storeType.
	StoreType = "policy-source"

	// Store property keys. Obsolete keys are retained only for migration errors.
	PolicySourceURLKey       = "PolicySourceURL"
	UpstreamURLKey           = "UpstreamURL"
	APIKeyKey                = "APIKey"
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
	FallbackWhileSyncingKey  = "FallbackWhileSyncing"
	CloudFallbackURLKey      = "CloudFallbackURL"
	CloudCredentialFileKey   = "CloudCredentialFile"
	TransportKey             = "Transport"
	ServeDownstreamKey       = "ServeDownstream"
	DownstreamCredentialsKey = "DownstreamCredentialsFile"
	MaxDownstreamKey         = "MaxDownstream"
	MaxChainDepthKey         = "MaxChainDepth"
	MaxConnectionsKey        = "MaxConnectionsPerDownstream"
	DefaultServicesSelection = "*"

	// Command line flag names. Each flag feeds the property key above through
	// the shared GetStoreParams mechanism.
	CacheDirFlag              = "policy-source-cache-dir"
	RequestTimeoutFlag        = "policy-source-request-timeout"
	CAFileFlag                = "policy-source-ca"
	ClientCertFileFlag        = "policy-source-client-cert"
	ClientKeyFileFlag         = "policy-source-client-key"
	HeartbeatIntervalFlag     = "policy-source-heartbeat-interval"
	StaleAfterFlag            = "policy-source-stale-after"
	StopAfterFlag             = "policy-source-stop-after"
	FallbackWhileSyncingFlag  = "policy-source-fallback-while-syncing"
	TransportFlag             = "policy-source-transport"
	ServeDownstreamFlag       = "policy-source-serve-downstream"
	DownstreamCredentialsFlag = "policy-source-downstream-credentials-file"
	MaxDownstreamFlag         = "policy-source-max-downstream"
	MaxChainDepthFlag         = "policy-source-max-chain-depth"
	MaxConnectionsFlag        = "policy-source-max-connections-per-downstream"
	defaultBootstrapWindow    = time.Minute
)

// Options configures a Store. The builder derives it from store properties;
// tests and in-process users may build it directly.
type Options struct {
	// UpstreamURL is the policy source root. It must use https.
	UpstreamURL string
	// APIKey is configuration-only and reserved for registration in #372.
	APIKey cfg.Secret
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
	// RequestTimeout bounds time without response bytes; zero uses the adsync default.
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
	// DisableFallback turns off routing authorization requests for a syncing
	// service to Cloud. The zero value keeps the fallback enabled, which is
	// the default.
	DisableFallback bool
	// FallbackURL and FallbackCredential are the distinct Speedle Cloud
	// fallback destination and instance credential. Both are required for a
	// usable fallback; neither is ever inferred from a proxy policy source.
	FallbackURL        string
	FallbackCredential string
	// FallbackHTTPClient optionally supplies Cloud's transport independently
	// of the policy-source transport, primarily for private test roots.
	FallbackHTTPClient *http.Client

	// StartRuntime installs the owned channel and reconciliation loops when the
	// evaluator subscribes to Watch. The registered store builder sets it;
	// low-level in-process tests may leave it false and drive Sync themselves.
	StartRuntime bool
	// InstanceID defaults to the id embedded in Credential. AgentVersion
	// defaults to speedle-ads. Chain defaults to just InstanceID.
	InstanceID   string
	AgentVersion string
	Chain        []string
	Transport    adsync.Transport
	// ReconcilePeriod is the fallback retry cadence. Channel notifications and
	// acks trigger immediate passes; zero uses a one-minute safety cadence.
	ReconcilePeriod time.Duration

	// ServeDownstream enables the proxy protocol from verified mirror copies.
	ServeDownstream             bool
	DownstreamCredentialsFile   string
	MaxDownstream               int
	MaxConnectionsPerDownstream int
	MaxChainDepth               int
}

// Builder creates policy-source stores.
type Builder struct{}

// GetStoreParams maps flag names to store property keys.
func (Builder) GetStoreParams() map[string]string {
	return map[string]string{
		CacheDirFlag:              CacheDirKey,
		RequestTimeoutFlag:        RequestTimeoutKey,
		CAFileFlag:                CAFileKey,
		ClientCertFileFlag:        ClientCertFileKey,
		ClientKeyFileFlag:         ClientKeyFileKey,
		HeartbeatIntervalFlag:     HeartbeatIntervalKey,
		StaleAfterFlag:            StaleAfterKey,
		StopAfterFlag:             StopAfterKey,
		FallbackWhileSyncingFlag:  FallbackWhileSyncingKey,
		TransportFlag:             TransportKey,
		ServeDownstreamFlag:       ServeDownstreamKey,
		DownstreamCredentialsFlag: DownstreamCredentialsKey,
		MaxDownstreamFlag:         MaxDownstreamKey,
		MaxChainDepthFlag:         MaxChainDepthKey,
		MaxConnectionsFlag:        MaxConnectionsKey,
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
	// The registered store is the production entry point. Its evaluator watch
	// starts the owned notification and reconciliation runtime; low-level Open
	// callers remain free to drive synchronization explicitly.
	options.StartRuntime = true
	return Open(context.Background(), options)
}

// optionsFromConfig converts store properties into Options. It performs every
// check that does not need the policy source.
func optionsFromConfig(config map[string]interface{}) (options Options, err error) {
	var secret cfg.Secret
	switch value := config[APIKeyKey].(type) {
	case string:
		secret = cfg.Secret(value)
	case cfg.Secret:
		secret = value
	}
	defer func() { err = redactConfigError(err, secret) }()
	if err := rejectRemovedConfig(config); err != nil {
		return options, err
	}

	for _, property := range []struct {
		key    string
		target *string
	}{
		{UpstreamURLKey, &options.UpstreamURL},
	} {
		value, _, err := stringProperty(config, property.key)
		if err != nil {
			return options, err
		}
		*property.target = value
	}
	if value, ok := config[APIKeyKey].(cfg.Secret); ok {
		options.APIKey = value
	} else {
		value, _, err := stringProperty(config, APIKeyKey)
		if err != nil {
			return options, err
		}
		options.APIKey = cfg.Secret(value)
	}
	if err := validateUpstream(&options); err != nil {
		return options, err
	}

	for _, property := range []struct {
		key    string
		target *string
	}{
		{CacheDirKey, &options.CacheDir},
		{CAFileKey, &options.CAFile},
		{ClientCertFileKey, &options.ClientCertFile},
		{ClientKeyFileKey, &options.ClientKeyFile},
		{DownstreamCredentialsKey, &options.DownstreamCredentialsFile},
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

	fallbackEnabled, set, err := boolProperty(config, FallbackWhileSyncingKey)
	if err != nil {
		return options, err
	}
	options.DisableFallback = set && !fallbackEnabled

	transport, _, err := stringProperty(config, TransportKey)
	if err != nil {
		return options, err
	}
	options.Transport = adsync.Transport(transport)
	switch options.Transport {
	case "", adsync.TransportAuto, adsync.TransportWebSocket, adsync.TransportLongPoll:
	default:
		return options, errors.New(errors.ConfigError, "unknown policy source Transport; use auto, websocket or longpoll")
	}

	serveDownstream, _, err := boolProperty(config, ServeDownstreamKey)
	if err != nil {
		return options, err
	}
	options.ServeDownstream = serveDownstream
	if options.ServeDownstream && options.DownstreamCredentialsFile == "" {
		return options, errors.New(errors.ConfigError, "DownstreamCredentialsFile is required when ServeDownstream is true")
	}
	for _, property := range []struct {
		key    string
		target *int
	}{
		{MaxDownstreamKey, &options.MaxDownstream},
		{MaxChainDepthKey, &options.MaxChainDepth},
		{MaxConnectionsKey, &options.MaxConnectionsPerDownstream},
	} {
		value, err := intProperty(config, property.key)
		if err != nil {
			return options, err
		}
		*property.target = value
	}

	return options, nil
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

// boolProperty reads a boolean store property. Values arrive as strings from
// the flags layer and as booleans from a JSON configuration file.
func boolProperty(config map[string]interface{}, key string) (bool, bool, error) {
	value, exists := config[key]
	if !exists || value == nil {
		return false, false, nil
	}
	switch typed := value.(type) {
	case bool:
		return typed, true, nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return false, false, nil
		}
		parsed, err := strconv.ParseBool(text)
		if err != nil {
			return false, false, errors.Errorf(errors.ConfigError, "configure item %s is not a boolean", key)
		}
		return parsed, true, nil
	default:
		return false, false, errors.Errorf(errors.ConfigError, "configure item %s must be a boolean, got %T", key, value)
	}
}

func intProperty(config map[string]interface{}, key string) (int, error) {
	value, exists := config[key]
	if !exists || value == nil {
		return 0, nil
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		converted := int(typed)
		if float64(converted) != typed {
			return 0, errors.Errorf(errors.ConfigError, "configure item %s must be an integer", key)
		}
		return converted, nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, errors.Errorf(errors.ConfigError, "configure item %s must be an integer", key)
		}
		return parsed, nil
	default:
		return 0, errors.Errorf(errors.ConfigError, "configure item %s must be an integer, got %T", key, value)
	}
}

func durationProperty(config map[string]interface{}, key string) (time.Duration, error) {
	text, set, err := stringProperty(config, key)
	if err != nil || !set || strings.TrimSpace(text) == "" {
		return 0, err
	}
	value, err := time.ParseDuration(strings.TrimSpace(text))
	if err != nil {
		return 0, errors.Errorf(errors.ConfigError, "configure item %s is not a duration", key)
	}
	if value < 0 {
		return 0, errors.Errorf(errors.ConfigError, "configure item %s must not be negative", key)
	}
	return value, nil
}

var _ store.StoreBuilder = Builder{}

func init() {
	pflag.String(CacheDirFlag, "", "Store config: directory for the optional per-service copy cache.")
	pflag.String(RequestTimeoutFlag, "", "Store config: maximum time without policy source response bytes, for example 10s.")
	pflag.String(CAFileFlag, "", "Store config: PEM CA bundle for the policy source.")
	pflag.String(ClientCertFileFlag, "", "Store config: client certificate for policy source mutual TLS.")
	pflag.String(ClientKeyFileFlag, "", "Store config: client key for policy source mutual TLS.")
	pflag.String(HeartbeatIntervalFlag, "", "Store config: heartbeat interval, for example 30s.")
	pflag.String(StaleAfterFlag, "", "Store config: warning threshold before an instance is stale, for example 60s.")
	pflag.String(StopAfterFlag, "", "Store config: threshold after which an instance stops serving, for example 120s.")
	pflag.String(FallbackWhileSyncingFlag, "true", "Store config: route authorization requests for a syncing service to Speedle Cloud.")
	pflag.String(TransportFlag, "auto", "Store config: notification transport: auto, websocket or longpoll.")
	pflag.String(ServeDownstreamFlag, "false", "Store config: serve the policy-source protocol to downstream ADS instances.")
	pflag.String(DownstreamCredentialsFlag, "", "Store config: downstream credential file (0600), reloaded on SIGHUP.")
	pflag.String(MaxDownstreamFlag, "", "Store config: maximum distinct downstream instances.")
	pflag.String(MaxChainDepthFlag, "", "Store config: maximum proxy chain depth.")
	pflag.String(MaxConnectionsFlag, "", "Store config: maximum active channels and held polls per downstream credential.")

	store.Register(StoreType, Builder{})
}

// validateUpstream validates the upstream without echoing URL credentials.
func validateUpstream(options *Options) error {
	if options.UpstreamURL == "" {
		return errors.New(errors.ConfigError, "configure item UpstreamURL is required")
	}
	parsed, err := url.Parse(options.UpstreamURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New(errors.ConfigError, "UpstreamURL must be an https URL without credentials, query or fragment")
	}
	return nil
}

// rejectRemovedConfig prevents silent acceptance of obsolete distribution settings.
// Instance identity and service selection are owned by the upstream registration.
func rejectRemovedConfig(config map[string]interface{}) error {
	for _, property := range []struct{ key, replacement string }{
		{PolicySourceURLKey, "use UpstreamURL"},
		{CredentialFileKey, "use APIKey in the configuration file"},
		{PublicKeysKey, "trust keys are supplied by registration"},
		{ServicesKey, "select services in the Cloud console"},
		{CloudFallbackURLKey, "use UpstreamURL"},
		{CloudCredentialFileKey, "use APIKey in the configuration file"},
		{"InstanceName", "the ADS manages its persisted instance ID"},
	} {
		if _, exists := config[property.key]; exists {
			return errors.Errorf(errors.ConfigError, "%s has been removed; %s", property.key, property.replacement)
		}
	}
	if _, exists := os.LookupEnv(adsync.CredentialEnv); exists {
		return errors.New(errors.ConfigError, "SPEEDLE_ADS_CREDENTIAL has been removed; use APIKey in the configuration file")
	}
	return nil
}

// redactConfigError also covers credentials accidentally repeated in another
// invalid property (for example a duration or file path).
func redactConfigError(err error, key cfg.Secret) error {
	if err == nil || key == "" {
		return err
	}
	text := err.Error()
	// Go error messages commonly use %q. Replace its escaped spelling before
	// the raw spelling, including when it appears inside a larger quoted path.
	quoted := strconv.Quote(string(key))
	for _, spelling := range []string{quoted[1 : len(quoted)-1], string(key)} {
		text = strings.ReplaceAll(text, spelling, "[REDACTED]")
	}
	if text == err.Error() {
		return err
	}
	return errors.New(errors.Code(err), text)
}
