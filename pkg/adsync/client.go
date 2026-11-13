// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/http/httpproxy"
)

// APIPrefix is the common path of every policy-source data endpoint.
const APIPrefix = "/policy-source/v1"

const (
	defaultRequestTimeout = 10 * time.Second
	defaultMaxAttempts    = 4
	defaultInitialBackoff = time.Second
	defaultMaxBackoff     = 60 * time.Second
	defaultJitter         = 0.3
	defaultUserAgent      = "speedle-adsync"

	maxErrorResponseBytes = 64 << 10

	changePageLimit = 100
)

// RetryPolicy controls retries for idempotent GET requests. The default is
// four attempts with exponential backoff from one second to sixty seconds and
// ±30% jitter; a successful request resets the sequence naturally because each
// call starts again at the first delay.
type RetryPolicy struct {
	// MaxAttempts includes the first request. One disables retries.
	MaxAttempts int
	// InitialBackoff is the delay before the second attempt.
	InitialBackoff time.Duration
	// MaxBackoff caps both exponential growth and a server Retry-After value.
	MaxBackoff time.Duration
	// Jitter is the fraction of the delay that may be added or removed.
	Jitter float64
	// Sleep suspends between attempts. Tests may replace it.
	Sleep func(context.Context, time.Duration) error
	// Rand returns a value in [0,1) for jitter. Tests may replace it.
	Rand func() float64
}

func (p RetryPolicy) normalized() (RetryPolicy, error) {
	if p.MaxAttempts == 0 {
		p.MaxAttempts = defaultMaxAttempts
	}
	if p.MaxAttempts < 0 {
		return p, fmt.Errorf("adsync: Retry.MaxAttempts must not be negative")
	}
	if p.InitialBackoff == 0 {
		p.InitialBackoff = defaultInitialBackoff
	}
	if p.InitialBackoff < 0 {
		return p, fmt.Errorf("adsync: Retry.InitialBackoff must not be negative")
	}
	if p.MaxBackoff == 0 {
		p.MaxBackoff = defaultMaxBackoff
	}
	if p.MaxBackoff < p.InitialBackoff {
		return p, fmt.Errorf("adsync: Retry.MaxBackoff must not be smaller than InitialBackoff")
	}
	if p.Jitter == 0 {
		p.Jitter = defaultJitter
	}
	if p.Jitter < 0 || p.Jitter > 1 {
		return p, fmt.Errorf("adsync: Retry.Jitter must be between 0 and 1")
	}
	if p.Sleep == nil {
		p.Sleep = sleepContext
	}
	if p.Rand == nil {
		p.Rand = rand.Float64
	}
	return p, nil
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Config configures a Client.
type Config struct {
	// BaseURL is the upstream root (UpstreamURL), for example
	// https://cloud.speedle.io. HTTPS is required; a non-443 port is allowed
	// for testing and internal proxies.
	BaseURL string
	// APIKey selects direct mode and authenticates every request: synchronization, the channel and
	// forwarded authorization requests. It is sent only in the Authorization
	// header to BaseURL and never appears in an error. Empty selects cascade
	// mode, which requires ClientCertFile, ClientKeyFile, CAFile and InstanceIDDir.
	APIKey string
	// InstanceID fixes the instance ID, a UUIDv7. When it is empty the ID is
	// created the first time a request needs it: persisted in InstanceIDDir
	// when that is set (and reused on later starts), otherwise fresh for this
	// process.
	InstanceID    string
	InstanceIDDir string

	// HTTPClient overrides the entire transport. Transport fields below must
	// then be empty.
	HTTPClient *http.Client
	// CAFile adds PEM CAs to system roots in direct mode. Cascade requires it
	// and trusts only this explicit bundle for relay verification.
	CAFile string
	// ClientCertFile and ClientKeyFile enable mutual TLS when both are set.
	ClientCertFile                string
	ClientKeyFile                 string
	TLSReloadInterval             time.Duration
	CRLFiles, CRLURLs             []string
	CRLRefreshInterval, CRLMaxAge time.Duration

	// tlsMaterial is shared by HTTPS and WebSocket transports.
	tlsMaterial *clientTLS
	// Proxy overrides the proxy function. By default HTTPS_PROXY and NO_PROXY
	// are read from the environment when the Client is created.
	Proxy func(*http.Request) (*url.URL, error)
	// Dialer overrides the WebSocket dialer used by Channel. When it is nil and
	// HTTPClient is nil, the dialer is built from CAFile, ClientCertFile,
	// ClientKeyFile and Proxy. With a custom HTTPClient and no Dialer, a
	// Channel can only use long polling.
	Dialer *websocket.Dialer

	// RequestTimeout limits time without response bytes, including the wait
	// for headers. Active downloads have no total-duration limit.
	// SnapshotTimeout is deprecated and ignored; retained for source compatibility.
	RequestTimeout  time.Duration
	SnapshotTimeout time.Duration
	// Retry controls retries of idempotent GET requests.
	Retry RetryPolicy
	// UserAgent is sent on every request.
	UserAgent string
}

// Client is the read-only policy-source protocol client. It has no mutable
// per-service state; Syncer adds resume and caching.
type Client struct {
	base            *url.URL
	tlsMaterial     *clientTLS
	apiKey          string
	http            *http.Client
	dialer          *websocket.Dialer
	requestTimeout  time.Duration
	fallbackTimeout time.Duration
	retry           RetryPolicy
	userAgent       string

	// identityMu guards the lazily created instance ID.
	identityMu  sync.Mutex
	instanceID  string
	instanceDir string

	// fallbackSlots bounds the forwarded requests in flight.
	fallbackSlots chan struct{}

	// fallbackMu guards the bounded retry hint used by the request gate.
	// fallbackRetrySeq counts the deadlines set, so a response can tell
	// whether one was set while it was in flight.
	fallbackMu       sync.Mutex
	fallbackRetryAt  time.Time
	fallbackRetrySeq uint64
	fallbackBackoff  time.Duration
	now              func() time.Time
}

// NewClient validates the configuration and builds the HTTP transport.
func NewClient(config Config) (*Client, error) {
	return NewClientContext(context.Background(), config)
}

// NewClientContext also applies the caller's cancellation to initial CRL fetches.
// Downloads use stall deadlines; this context is the owner's startup lifetime.
func NewClientContext(ctx context.Context, config Config) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if config.APIKey != "" {
		if config.TLSReloadInterval != 0 || config.CRLFiles != nil || config.CRLURLs != nil || config.CRLRefreshInterval != 0 || config.CRLMaxAge != 0 {
			return nil, fmt.Errorf("adsync: TLSReloadInterval, CRLFiles, CRLURLs, CRLRefreshInterval and CRLMaxAge require cascade mode (no APIKey)")
		}
		if err := ValidateAPIKey(config.APIKey); err != nil {
			return nil, err
		}
	} else {
		if (config.CRLFiles != nil || config.CRLURLs != nil) && len(config.CRLFiles)+len(config.CRLURLs) == 0 {
			return nil, fmt.Errorf("adsync: configured CRLFiles/CRLURLs require at least one CRL source")
		}
		if config.ClientCertFile == "" || config.ClientKeyFile == "" || config.CAFile == "" {
			return nil, fmt.Errorf("adsync: cascade requires ClientCertFile, ClientKeyFile and CAFile")
		}
		if config.InstanceIDDir == "" {
			return nil, fmt.Errorf("adsync: CacheDir is required in cascade mode")
		}
		if config.InstanceID != "" {
			return nil, fmt.Errorf("adsync: cascade generates InstanceID in CacheDir")
		}
		if config.HTTPClient != nil || config.Dialer != nil {
			return nil, fmt.Errorf("adsync: cascade requires the managed mTLS transport")
		}
	}
	if config.TLSReloadInterval < 0 || config.CRLRefreshInterval < 0 || config.CRLMaxAge < 0 {
		return nil, fmt.Errorf("adsync: TLS/CRL intervals must not be negative")
	}
	if config.InstanceID != "" && !ValidInstanceID(config.InstanceID) {
		return nil, fmt.Errorf("%w: InstanceID must be a UUIDv7", ErrProtocol)
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("adsync: parse BaseURL: %w", err)
	}
	if base.Scheme != "https" {
		return nil, fmt.Errorf("adsync: BaseURL must use https")
	}
	if base.Hostname() == "" {
		return nil, fmt.Errorf("adsync: BaseURL has no host")
	}
	if base.User != nil {
		return nil, fmt.Errorf("adsync: BaseURL must not contain a credential")
	}
	if base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || strings.Contains(config.BaseURL, "#") {
		return nil, fmt.Errorf("adsync: BaseURL must not contain a query or fragment")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")

	requestTimeout := config.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = defaultRequestTimeout
	}
	if requestTimeout < 0 || config.SnapshotTimeout < 0 {
		return nil, fmt.Errorf("adsync: timeouts must not be negative")
	}
	retry, err := config.Retry.normalized()
	if err != nil {
		return nil, err
	}

	if config.APIKey == "" {
		config.tlsMaterial, err = newClientTLS(ctx, config, base.Hostname())
		if err != nil {
			return nil, err
		}
	}
	httpClient := config.HTTPClient
	dialer := config.Dialer
	if httpClient == nil {
		httpClient, err = newHTTPClient(config)
		if err != nil {
			return nil, err
		}
		if dialer == nil {
			dialer, err = newWebSocketDialer(config)
			if err != nil {
				return nil, err
			}
		}
	} else {
		if config.CAFile != "" || config.ClientCertFile != "" || config.ClientKeyFile != "" || config.Proxy != nil {
			return nil, fmt.Errorf("adsync: HTTPClient cannot be combined with CAFile, client certificates or Proxy")
		}
		// Do not mutate a caller-owned client or retain its redirect policy.
		// The policy source is a fixed endpoint and credentials must not move.
		cloned := *httpClient
		cloned.CheckRedirect = rejectRedirect
		httpClient = &cloned
	}

	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	return &Client{
		base:            base,
		tlsMaterial:     config.tlsMaterial,
		apiKey:          config.APIKey,
		http:            httpClient,
		dialer:          dialer,
		requestTimeout:  requestTimeout,
		fallbackTimeout: DefaultFallbackTimeout,
		fallbackBackoff: DefaultFallbackRetryBackoff,
		retry:           retry,
		userAgent:       userAgent,
		instanceID:      config.InstanceID,
		instanceDir:     config.InstanceIDDir,
		fallbackSlots:   make(chan struct{}, MaxUpstreamInFlight),
		now:             time.Now,
	}, nil
}

// InstanceID returns the instance ID, creating (and, with InstanceIDDir,
// persisting) it on first use. The ID is persisted before it is returned, so
// no request ever carries an ID that a restart would not reuse.
func (c *Client) InstanceID() (string, error) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	if c.instanceID != "" {
		return c.instanceID, nil
	}
	var (
		id  string
		err error
	)
	if c.instanceDir != "" {
		id, _, err = LoadOrCreateInstanceID(c.instanceDir)
	} else {
		id, err = NewInstanceID()
	}
	if err != nil {
		return "", err
	}
	c.instanceID = id
	return id, nil
}

// authorize sets the authentication headers of one policy-source request:
// the API key and the instance ID.
func (c *Client) authorize(header http.Header) error {
	id, err := c.InstanceID()
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		header.Set("Authorization", "Bearer "+c.apiKey)
	} else {
		header.Del("Authorization")
	}
	header.Set(InstanceHeader, id)
	return nil
}

func newHTTPClient(config Config) (*http.Client, error) {
	tlsConfig, err := newTLSConfig(config)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy:                 newProxyFunc(config),
		TLSClientConfig:       tlsConfig,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	if config.tlsMaterial != nil {
		transport.DialContext = config.tlsMaterial.dialContext
	}
	return &http.Client{
		Transport: transport,
		// A policy source is a fixed endpoint. Following a redirect could move
		// the request (and its credential) somewhere else, so surface the 3xx
		// as a protocol error instead.
		CheckRedirect: rejectRedirect,
	}, nil
}

// newWebSocketDialer builds the channel dialer from the same TLS and proxy
// settings as the HTTPS transport, so both honour the same CA bundle, client
// certificate and proxy configuration.
func newWebSocketDialer(config Config) (*websocket.Dialer, error) {
	tlsConfig, err := newTLSConfig(config)
	if err != nil {
		return nil, err
	}
	dialer := &websocket.Dialer{
		Proxy:            newProxyFunc(config),
		TLSClientConfig:  tlsConfig,
		HandshakeTimeout: defaultRequestTimeout,
	}
	if config.tlsMaterial != nil {
		dialer.NetDialContext = config.tlsMaterial.dialContext
	}
	return dialer, nil
}

// newTLSConfig builds the TLS configuration shared by the HTTPS client and the
// WebSocket dialer.
func newTLSConfig(config Config) (*tls.Config, error) {
	if config.tlsMaterial != nil {
		return config.tlsMaterial.config(), nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if config.CAFile != "" {
		pool, err := loadCertPool(config.CAFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = pool
	}
	if (config.ClientCertFile == "") != (config.ClientKeyFile == "") {
		return nil, fmt.Errorf("adsync: ClientCertFile and ClientKeyFile must be set together")
	}
	if config.ClientCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(config.ClientCertFile, config.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("adsync: load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfig, nil
}

// newProxyFunc returns the configured proxy function, or one that reads
// HTTPS_PROXY and NO_PROXY from the environment.
func newProxyFunc(config Config) func(*http.Request) (*url.URL, error) {
	if config.Proxy != nil {
		return config.Proxy
	}
	environmentProxy := httpproxy.FromEnvironment().ProxyFunc()
	return func(request *http.Request) (*url.URL, error) {
		return environmentProxy(request.URL)
	}
}

func rejectRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func loadCertPool(path string) (*x509.CertPool, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("adsync: read CA file: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("adsync: CA file contains no certificates")
	}
	return pool, nil
}

func (c *Client) endpoint(parts ...string) string {
	target := *c.base
	path := c.base.Path + APIPrefix
	for _, part := range parts {
		path += "/" + url.PathEscape(part)
	}
	target.Path = path
	return target.String()
}

// Services lists the services this instance may sync.
func (c *Client) Services(ctx context.Context) (*ServicesResponse, error) {
	var response ServicesResponse
	_, err := c.get(ctx, c.endpoint("services"), nil, nil, false, &response)
	if err != nil {
		return nil, err
	}
	if err := response.validate(); err != nil {
		return nil, err
	}
	return &response, nil
}

// Snapshot fetches and verifies the snapshot at the source head. When
// ifNoneMatch matches the current identity tuple exactly, it returns
// ErrNotModified instead.
func (c *Client) Snapshot(ctx context.Context, serviceID, ifNoneMatch string) (*SnapshotResponse, bool, error) {
	if !validEntityID(serviceID) {
		return nil, false, fmt.Errorf("%w: invalid service id %q", ErrProtocol, serviceID)
	}
	header := http.Header{}
	if ifNoneMatch != "" {
		header.Set("If-None-Match", ifNoneMatch)
	}
	var response SnapshotResponse
	notModified, err := c.get(ctx, c.endpoint("services", serviceID, "snapshot"),
		nil, header, true, &response)
	if err != nil {
		return nil, false, err
	}
	if notModified {
		return nil, true, nil
	}
	if err := response.validate(); err != nil {
		return nil, false, err
	}
	if response.ServiceID != serviceID {
		return nil, false, fmt.Errorf("%w: snapshot is for service %q, want %q", ErrProtocol, response.ServiceID, serviceID)
	}
	if computed := StateHash(response.Snapshot); computed != response.StateHash {
		return nil, false, fmt.Errorf("%w: snapshot stateHash does not match its content", ErrIntegrity)
	}
	return &response, false, nil
}

// Changes fetches and verifies one page of commits after since. The
// first commit must chain from anchor, which is the caller's current commit
// hash.
func (c *Client) Changes(ctx context.Context, serviceID, sourceID, epoch string, since uint64, anchor Hash, limit int) (*ChangesResponse, error) {
	if !validEntityID(serviceID) {
		return nil, fmt.Errorf("%w: invalid service id %q", ErrProtocol, serviceID)
	}
	if !validEntityID(sourceID) || !validEntityID(epoch) {
		return nil, fmt.Errorf("%w: invalid source identity", ErrProtocol)
	}
	if !validRevision(since) || anchor.IsZero() {
		return nil, fmt.Errorf("%w: invalid resume position", ErrProtocol)
	}
	if limit == 0 {
		limit = changePageLimit
	}
	if limit < 1 {
		return nil, fmt.Errorf("%w: invalid changes page limit %d", ErrProtocol, limit)
	}
	query := url.Values{}
	query.Set("since", strconv.FormatUint(since, 10))
	query.Set("limit", strconv.Itoa(limit))
	var response ChangesResponse
	_, err := c.get(ctx, c.endpoint("services", serviceID, "changes"), query,
		nil, false, &response)
	if err != nil {
		return nil, err
	}
	if err := response.validatePage(serviceID, sourceID, epoch, since, anchor, limit); err != nil {
		return nil, err
	}
	return &response, nil
}

// get performs one idempotent GET, with retries for transport failures,
// HTTP 408/429 and transient 5xx responses.
func (c *Client) get(ctx context.Context, endpoint string, query url.Values, header http.Header, allowNotModified bool, dst any) (bool, error) {
	var lastErr error
	for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
		reflect.ValueOf(dst).Elem().SetZero()
		notModified, retryable, err := c.getOnce(ctx, endpoint, query, header, allowNotModified, dst)
		if err == nil {
			return notModified, nil
		}
		lastErr = err
		if !retryable || attempt == c.retry.MaxAttempts {
			return false, err
		}
		delay := c.retry.delay(attempt, retryAfterOf(err))
		if err := c.retry.Sleep(ctx, delay); err != nil {
			return false, err
		}
	}
	return false, lastErr
}

func (c *Client) getOnce(ctx context.Context, endpoint string, query url.Values, header http.Header, allowNotModified bool, dst any) (bool, bool, error) {
	requestCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	progress := newStallWatch(c.requestTimeout, func() { cancel(context.DeadlineExceeded) })
	defer progress.stop()

	target := endpoint
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, target, nil)
	if err != nil {
		return false, false, fmt.Errorf("adsync: build request: %w", err)
	}
	if err := c.authorize(request.Header); err != nil {
		return false, false, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	path := request.URL.Path

	// A caller-supplied http.Client.Timeout is a total-duration deadline.
	// Clone it here so the stall timeout controls downloads without mutating
	// the client used by channels or fallback requests.
	httpClient := *c.http
	httpClient.Timeout = 0
	response, err := httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return false, false, fmt.Errorf("adsync: GET %s: %w", path, ctx.Err())
		}
		if requestCtx.Err() != nil {
			return false, true, fmt.Errorf("adsync: GET %s: %w", path, context.Cause(requestCtx))
		}
		return false, true, fmt.Errorf("adsync: GET %s: %w", path, err)
	}
	defer response.Body.Close()
	progress.progress()

	switch response.StatusCode {
	case http.StatusOK:
		// Decode directly from the response, without a second full-body copy.
		reader := &progressReader{Reader: response.Body, watch: progress}
		err := decodeStrictReader(reader, dst)
		if err != nil {
			if ctx.Err() != nil {
				return false, false, ctx.Err()
			}
			if requestCtx.Err() != nil {
				return false, true, context.Cause(requestCtx)
			}
			return false, (reader.readErr != nil && reader.readErr != io.EOF) || errors.Is(err, io.ErrUnexpectedEOF), err
		}
		return false, false, nil
	case http.StatusNotModified:
		if allowNotModified {
			return true, false, nil
		}
		return false, false, fmt.Errorf("%w: unexpected 304 from %s", ErrProtocol, path)
	default:
		body, err := io.ReadAll(io.LimitReader(&progressReader{Reader: response.Body, watch: progress}, maxErrorResponseBytes))
		if err != nil {
			if ctx.Err() != nil {
				return false, false, ctx.Err()
			}
			if requestCtx.Err() != nil {
				return false, true, context.Cause(requestCtx)
			}
			return false, true, fmt.Errorf("adsync: read error response: %w", err)
		}
		statusErr := newStatusError(http.MethodGet, path, response.StatusCode, body)
		if detailed, ok := statusErr.(*statusError); ok {
			detailed.retryAfter = retryAfterDuration(response.Header)
		}
		return false, retryableStatus(response.StatusCode), statusErr
	}
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return true
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	// Cloudflare's transient edge errors.
	return status >= 520 && status <= 527
}

func retryAfterOf(err error) time.Duration {
	if detailed, ok := err.(*statusError); ok {
		return detailed.retryAfter
	}
	return 0
}

func (p RetryPolicy) delay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > p.MaxBackoff {
			return p.MaxBackoff
		}
		return retryAfter
	}
	return exponentialDelay(p.InitialBackoff, p.MaxBackoff, p.Jitter, p.Rand, attempt)
}

// exponentialDelay returns the delay before the given attempt: initial doubled
// per attempt up to max, then reduced or extended by up to jitter.
func exponentialDelay(initial, max time.Duration, jitter float64, random func() float64, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := initial
	for index := 1; index < attempt; index++ {
		if delay >= max/2 {
			delay = max
			break
		}
		delay *= 2
	}
	if delay > max {
		delay = max
	}
	if jitter > 0 && random != nil {
		factor := 1 + jitter*(2*random()-1)
		if factor < 0 {
			factor = 0
		}
		delay = time.Duration(float64(delay) * factor)
	}
	return delay
}
