// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"fmt"
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
	TLSReloadIntervalKey     = "TLSReloadInterval"
	CRLFilesKey              = "CRLFiles"
	CRLURLsKey               = "CRLURLs"
	CRLRefreshIntervalKey    = "CRLRefreshInterval"
	CRLMaxAgeKey             = "CRLMaxAge"
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

	// Command line flag names. Each flag feeds the property key above through
	// the shared GetStoreParams mechanism.
	CacheDirFlag             = "policy-source-cache-dir"
	RequestTimeoutFlag       = "policy-source-request-timeout"
	CAFileFlag               = "policy-source-ca"
	ClientCertFileFlag       = "policy-source-client-cert"
	ClientKeyFileFlag        = "policy-source-client-key"
	HeartbeatIntervalFlag    = "policy-source-heartbeat-interval"
	StaleAfterFlag           = "policy-source-stale-after"
	StopAfterFlag            = "policy-source-stop-after"
	FallbackWhileSyncingFlag = "policy-source-fallback-while-syncing"
	TransportFlag            = "policy-source-transport"
	ServeDownstreamFlag      = "policy-source-serve-downstream"
	// Removed distribution flags stay registered so an explicit use through a
	// flag or environment variable reaches the store's migration error instead
	// of being silently ignored. Their empty defaults are never injected.
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
	// UpstreamURLs retains the preference order for cascade upstream selection.
	UpstreamURLs []string
	// APIKey authenticates every request to the upstream, including
	// forwarded authorization requests in direct mode. Without it, a complete
	// client certificate/key and explicit CAFile select cascade mTLS.
	APIKey cfg.Secret
	// CacheDir is required in cascade mode. It enables the on-disk cache and keeps
	// the instance ID across restarts.
	CacheDir string
	// CAFile supplies the relay trust roots in cascade mode, or adds enterprise
	// TLS inspection roots in direct mode.
	CAFile string
	// ClientCertFile and ClientKeyFile enable mutual TLS when both are set.
	ClientCertFile                string
	ClientKeyFile                 string
	TLSReloadInterval             time.Duration
	CRLFiles, CRLURLs             []string
	CRLRefreshInterval, CRLMaxAge time.Duration
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
	// DisableFallback turns off forwarding authorization requests for a
	// syncing or stale service to the upstream. The zero value keeps
	// forwarding enabled, which is the default.
	DisableFallback bool

	// StartRuntime installs the owned channel and reconciliation loops when the
	// evaluator subscribes to Watch. The registered store builder sets it;
	// low-level in-process tests may leave it false and drive Sync themselves.
	StartRuntime bool
	// InstanceID fixes the UUIDv7 instance ID, for tests and in-process
	// users; empty lets the client create one on first use and keep it in
	// CacheDir. AgentVersion defaults to speedle-ads.
	InstanceID   string
	AgentVersion string
	Transport    adsync.Transport
	// ReconcilePeriod is the fallback retry cadence. Channel notifications and
	// acks trigger immediate passes; zero uses a one-minute safety cadence.
	ReconcilePeriod time.Duration

	// ServeDownstream enables the downstream mTLS listener from verified
	// mirror copies. The listener's endpoint, certificate, key, client CA and
	// CRL sources are server configuration, not store properties.
	ServeDownstream bool
}

// Builder creates policy-source stores.
type Builder struct{}

// GetStoreParams maps flag names to store property keys.
func (Builder) GetStoreParams() map[string]string {
	return map[string]string{
		CacheDirFlag:             CacheDirKey,
		RequestTimeoutFlag:       RequestTimeoutKey,
		CAFileFlag:               CAFileKey,
		ClientCertFileFlag:       ClientCertFileKey,
		ClientKeyFileFlag:        ClientKeyFileKey,
		HeartbeatIntervalFlag:    HeartbeatIntervalKey,
		StaleAfterFlag:           StaleAfterKey,
		StopAfterFlag:            StopAfterKey,
		FallbackWhileSyncingFlag: FallbackWhileSyncingKey,
		TransportFlag:            TransportKey,
		ServeDownstreamFlag:      ServeDownstreamKey,

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

	options.UpstreamURLs, err = upstreamProperty(config[UpstreamURLKey])
	if err != nil {
		return options, err
	}
	if len(options.UpstreamURLs) > 0 {
		options.UpstreamURL = options.UpstreamURLs[0]
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

	if options.APIKey != "" {
		for _, key := range []string{TLSReloadIntervalKey, CRLFilesKey, CRLURLsKey, CRLRefreshIntervalKey, CRLMaxAgeKey} {
			if _, present := config[key]; present {
				return options, errors.Errorf(errors.ConfigError, "%s requires cascade mode without APIKey", key)
			}
		}
	}

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
	if err := validateCredentials(options); err != nil {
		return options, err
	}

	for _, property := range []struct {
		key    string
		target *time.Duration
	}{
		{RequestTimeoutKey, &options.RequestTimeout},
		{HeartbeatIntervalKey, &options.HeartbeatInterval},
		{StaleAfterKey, &options.StaleAfter},
		{StopAfterKey, &options.StopAfter},
		{TLSReloadIntervalKey, &options.TLSReloadInterval},
		{CRLRefreshIntervalKey, &options.CRLRefreshInterval},
		{CRLMaxAgeKey, &options.CRLMaxAge},
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
	for _, property := range []struct {
		key    string
		target *[]string
	}{
		{CRLFilesKey, &options.CRLFiles}, {CRLURLsKey, &options.CRLURLs},
	} {
		if value, exists := config[property.key]; exists {
			entries, err := stringList(value)
			if err != nil {
				return options, errors.Errorf(errors.ConfigError, "%s must be a list of nonempty strings", property.key)
			}
			*property.target = entries
		}
	}
	_, crlFilesSet := config[CRLFilesKey]
	_, crlURLsSet := config[CRLURLsKey]
	if (crlFilesSet || crlURLsSet) && len(options.CRLFiles)+len(options.CRLURLs) == 0 {
		return options, errors.New(errors.ConfigError, "configured CRLFiles/CRLURLs require at least one CRL source")
	}

	if err := validateUpstream(&options); err != nil {
		return options, err
	}

	return options, nil
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
	pflag.String(ServeDownstreamFlag, "false", "Store config: serve the policy-source protocol to downstream ADS instances on the downstream mTLS listener.")
	pflag.String(DownstreamCredentialsFlag, "", "Removed: configure ClientCertFile, ClientKeyFile, CAFile and relay downstreamClientCAPath.")
	pflag.String(MaxDownstreamFlag, "", "Removed: downstream connection limits are not supported.")
	pflag.String(MaxChainDepthFlag, "", "Removed: chain depth limits are not supported.")
	pflag.String(MaxConnectionsFlag, "", "Removed: downstream connection limits are not supported.")

	store.Register(StoreType, Builder{})
}

// validateCredentials selects cascade only with complete mTLS settings.
func validateCredentials(options Options) error {
	if options.APIKey != "" {
		return nil
	}
	if options.ClientCertFile == "" || options.ClientKeyFile == "" || options.CAFile == "" {
		return errors.New(errors.ConfigError, "configure APIKey for direct mode or complete ClientCertFile, ClientKeyFile and CAFile for cascade mode")
	}
	if options.CacheDir == "" {
		return errors.New(errors.ConfigError, "CacheDir is required in cascade mode")
	}
	if options.InstanceID != "" {
		return errors.New(errors.ConfigError, "cascade generates InstanceID in CacheDir")
	}
	return nil
}

// validateUpstream validates the upstream without echoing URL credentials.
func validateUpstream(options *Options) error {
	if options.UpstreamURL == "" && len(options.UpstreamURLs) == 0 {
		return errors.New(errors.ConfigError, "configure item UpstreamURL is required")
	}
	if len(options.UpstreamURLs) == 0 {
		options.UpstreamURLs = []string{options.UpstreamURL}
	}
	if options.UpstreamURL != "" && options.UpstreamURL != options.UpstreamURLs[0] {
		return errors.New(errors.ConfigError, "UpstreamURL and UpstreamURLs disagree")
	}
	options.UpstreamURLs = append([]string(nil), options.UpstreamURLs...)
	seen := make(map[string]bool)
	for i, entry := range options.UpstreamURLs {
		parsed, err := url.Parse(entry)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(entry, "#") {
			return errors.New(errors.ConfigError, "UpstreamURL must be an https URL without credentials, query or fragment")
		}
		parsed.Host = strings.ToLower(parsed.Host)
		if parsed.Port() == "443" {
			parsed.Host = strings.TrimSuffix(parsed.Host, ":443")
		}
		normalized := strings.TrimSuffix(parsed.String(), "/")
		if seen[normalized] {
			return errors.New(errors.ConfigError, "UpstreamURL contains duplicate URLs")
		}
		seen[normalized] = true
		options.UpstreamURLs[i] = normalized
	}
	options.UpstreamURL = options.UpstreamURLs[0]
	if options.APIKey != "" && len(options.UpstreamURLs) != 1 {
		return errors.New(errors.ConfigError, "APIKey direct mode requires a single UpstreamURL")
	}
	return nil
}

func stringList(value interface{}) ([]string, error) {
	var entries []string
	switch value := value.(type) {
	case []string:
		entries = append([]string(nil), value...)
	case []interface{}:
		for _, entry := range value {
			text, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("not a string")
			}
			entries = append(entries, text)
		}
	default:
		return nil, fmt.Errorf("not a list")
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			return nil, fmt.Errorf("empty entry")
		}
	}
	return entries, nil
}

func upstreamProperty(value interface{}) ([]string, error) {
	if text, ok := value.(string); ok {
		return []string{text}, nil
	}
	entries, err := stringList(value)
	if err != nil || len(entries) == 0 {
		return nil, errors.New(errors.ConfigError, "UpstreamURL must be one HTTPS URL or a nonempty ordered list")
	}
	return entries, nil
}

// removedCredentialEnv is the environment variable that used to carry the
// instance credential.
const removedCredentialEnv = "SPEEDLE_ADS_CREDENTIAL"

// rejectRemovedConfig prevents silent acceptance of obsolete distribution settings.
// Instance identity and service selection are owned by the upstream registration.
func rejectRemovedConfig(config map[string]interface{}) error {
	for _, property := range []struct{ key, replacement string }{
		{PolicySourceURLKey, "use UpstreamURL"},
		{CredentialFileKey, "use APIKey in the configuration file"},
		{PublicKeysKey, "signatures are no longer verified"},
		{ServicesKey, "select services in the Cloud console"},
		{CloudFallbackURLKey, "use UpstreamURL"},
		{CloudCredentialFileKey, "use APIKey in the configuration file"},
		{"InstanceName", "the ADS manages its persisted instance ID"},
		{"InstanceID", "the ADS generates its persisted UUIDv7 in CacheDir"},
		{DownstreamCredentialsKey, "downstream clients authenticate with mTLS; configure ClientCertFile, ClientKeyFile and CAFile on the downstream and serverConfig downstreamClientCAPath on the relay"},
		{MaxDownstreamKey, "there is no downstream fan-out limit"},
		{MaxChainDepthKey, "there is no chain depth limit"},
		{MaxConnectionsKey, "there is no downstream connection limit"},
	} {
		if _, exists := config[property.key]; exists {
			return errors.Errorf(errors.ConfigError, "%s has been removed; %s", property.key, property.replacement)
		}
	}
	if _, exists := os.LookupEnv(removedCredentialEnv); exists {
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
