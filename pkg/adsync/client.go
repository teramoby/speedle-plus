// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
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
	defaultRequestTimeout  = 10 * time.Second
	defaultSnapshotTimeout = 30 * time.Second
	defaultMaxAttempts     = 4
	defaultInitialBackoff  = time.Second
	defaultMaxBackoff      = 60 * time.Second
	defaultJitter          = 0.3
	defaultUserAgent       = "speedle-adsync"

	maxServicesResponseBytes = 1 << 20
	maxErrorResponseBytes    = 64 << 10

	changePageLimit = maxCommitsPerPage
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
	// BaseURL is the policy source root, for example https://cloud.speedle.io.
	// HTTPS is required; a non-443 port is allowed for testing and internal
	// proxies.
	BaseURL string
	// Credential is the account-scoped adsi_ instance credential. It is sent
	// only in the Authorization header to BaseURL.
	Credential string
	// FallbackBaseURL and FallbackCredential configure the distinct Speedle
	// Cloud authorization fallback. Both must be set together. The upstream
	// policy-source URL and credential are never reused implicitly, because the
	// source may be a proxy with a locally issued downstream credential.
	FallbackBaseURL    string
	FallbackCredential string
	// FallbackHTTPClient optionally uses a separate transport for Cloud, for
	// example when a test source and Cloud use different private roots. Nil
	// reuses HTTPClient's transport configuration, never its destination.
	FallbackHTTPClient *http.Client
	// PublicKeys is the out-of-band trust anchor for snapshot and commit
	// signatures. It is required.
	PublicKeys *PublicKeySet

	// HTTPClient overrides the entire transport. Transport fields below must
	// then be empty.
	HTTPClient *http.Client
	// CAFile adds a PEM-encoded CA bundle to the system roots, for enterprise
	// TLS inspection.
	CAFile string
	// ClientCertFile and ClientKeyFile enable mutual TLS when both are set.
	ClientCertFile string
	ClientKeyFile  string
	// Proxy overrides the proxy function. By default HTTPS_PROXY and NO_PROXY
	// are read from the environment when the Client is created.
	Proxy func(*http.Request) (*url.URL, error)
	// Dialer overrides the WebSocket dialer used by Channel. When it is nil and
	// HTTPClient is nil, the dialer is built from CAFile, ClientCertFile,
	// ClientKeyFile and Proxy. With a custom HTTPClient and no Dialer, a
	// Channel can only use long polling.
	Dialer *websocket.Dialer

	// RequestTimeout bounds one data request. SnapshotTimeout bounds one
	// snapshot request because snapshots may be several megabytes.
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
	credential      string
	keys            *PublicKeySet
	http            *http.Client
	dialer          *websocket.Dialer
	requestTimeout  time.Duration
	snapshotTimeout time.Duration
	fallbackTimeout time.Duration
	retry           RetryPolicy
	userAgent       string

	fallbackBase       *url.URL
	fallbackCredential string
	fallbackHTTP       *http.Client

	// fallbackMu guards the bounded retry hint used by the request gate.
	fallbackMu      sync.Mutex
	fallbackRetryAt time.Time
	fallbackBackoff time.Duration
	now             func() time.Time
}

// NewClient validates the configuration and builds the HTTP transport.
func NewClient(config Config) (*Client, error) {
	if config.PublicKeys == nil {
		return nil, fmt.Errorf("adsync: PublicKeys is required")
	}
	if err := ValidateCredential(config.Credential); err != nil {
		return nil, err
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("adsync: parse BaseURL: %w", err)
	}
	if base.Scheme != "https" {
		return nil, fmt.Errorf("adsync: BaseURL must use https")
	}
	if base.Host == "" {
		return nil, fmt.Errorf("adsync: BaseURL has no host")
	}
	if base.User != nil {
		return nil, fmt.Errorf("adsync: BaseURL must not contain a credential")
	}
	if base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("adsync: BaseURL must not contain a query or fragment")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")

	requestTimeout := config.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = defaultRequestTimeout
	}
	snapshotTimeout := config.SnapshotTimeout
	if snapshotTimeout == 0 {
		snapshotTimeout = defaultSnapshotTimeout
	}
	if requestTimeout < 0 || snapshotTimeout < 0 {
		return nil, fmt.Errorf("adsync: timeouts must not be negative")
	}
	retry, err := config.Retry.normalized()
	if err != nil {
		return nil, err
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

	var fallbackBase *url.URL
	if config.FallbackBaseURL != "" {
		fallbackBase, err = url.Parse(config.FallbackBaseURL)
		if err != nil {
			return nil, fmt.Errorf("adsync: parse FallbackBaseURL: %w", err)
		}
		if fallbackBase.Scheme != "https" || fallbackBase.Host == "" || fallbackBase.User != nil ||
			fallbackBase.RawQuery != "" || fallbackBase.Fragment != "" {
			return nil, fmt.Errorf("adsync: FallbackBaseURL must be an https URL without credentials, query or fragment")
		}
		fallbackBase.Path = strings.TrimSuffix(fallbackBase.Path, "/")
		if err := ValidateCredential(config.FallbackCredential); err != nil {
			return nil, fmt.Errorf("adsync: invalid fallback credential: %w", err)
		}
	} else if config.FallbackCredential != "" {
		return nil, fmt.Errorf("adsync: FallbackBaseURL is required with FallbackCredential")
	}
	fallbackHTTP := config.FallbackHTTPClient
	if fallbackHTTP == nil {
		fallbackHTTP = httpClient
	}

	userAgent := config.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	return &Client{
		base:               base,
		credential:         config.Credential,
		keys:               config.PublicKeys,
		http:               httpClient,
		dialer:             dialer,
		requestTimeout:     requestTimeout,
		snapshotTimeout:    snapshotTimeout,
		fallbackTimeout:    DefaultFallbackTimeout,
		fallbackBackoff:    DefaultFallbackRetryBackoff,
		retry:              retry,
		userAgent:          userAgent,
		fallbackBase:       fallbackBase,
		fallbackCredential: config.FallbackCredential,
		fallbackHTTP:       fallbackHTTP,
		now:                time.Now,
	}, nil
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
	return &websocket.Dialer{
		Proxy:            newProxyFunc(config),
		TLSClientConfig:  tlsConfig,
		HandshakeTimeout: defaultRequestTimeout,
	}, nil
}

// newTLSConfig builds the TLS configuration shared by the HTTPS client and the
// WebSocket dialer.
func newTLSConfig(config Config) (*tls.Config, error) {
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

// Services lists the services this credential and instance may sync.
func (c *Client) Services(ctx context.Context) (*ServicesResponse, error) {
	body, _, err := c.get(ctx, c.endpoint("services"), nil, c.requestTimeout, nil,
		maxServicesResponseBytes, false)
	if err != nil {
		return nil, err
	}
	var response ServicesResponse
	if err := decodeStrict(body, &response); err != nil {
		return nil, err
	}
	if err := response.validate(); err != nil {
		return nil, err
	}
	return &response, nil
}

// Snapshot fetches and verifies the signed snapshot at the source head. When
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
	body, notModified, err := c.get(ctx, c.endpoint("services", serviceID, "snapshot"),
		nil, c.snapshotTimeout, header, maxSnapshotResponseBytes, true)
	if err != nil {
		return nil, false, err
	}
	if notModified {
		return nil, true, nil
	}
	var response SnapshotResponse
	if err := decodeStrict(body, &response); err != nil {
		return nil, false, err
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
	if err := c.keys.verifySnapshot(&response); err != nil {
		return nil, false, err
	}
	return &response, false, nil
}

// Changes fetches and verifies one page of signed commits after since. The
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
	if limit < 1 || limit > maxCommitsPerPage {
		return nil, fmt.Errorf("%w: invalid changes page limit %d", ErrProtocol, limit)
	}
	query := url.Values{}
	query.Set("since", strconv.FormatUint(since, 10))
	query.Set("limit", strconv.Itoa(limit))
	body, _, err := c.get(ctx, c.endpoint("services", serviceID, "changes"), query,
		c.requestTimeout, nil, maxChangesResponseBytes, false)
	if err != nil {
		return nil, err
	}
	var response ChangesResponse
	if err := decodeStrict(body, &response); err != nil {
		return nil, err
	}
	if err := response.validatePage(serviceID, sourceID, epoch, since, anchor, limit); err != nil {
		return nil, err
	}
	for index := range response.Commits {
		if err := c.keys.verifyCommit(sourceID, epoch, &response.Commits[index]); err != nil {
			return nil, err
		}
	}
	return &response, nil
}

// get performs one idempotent GET, with retries for transport failures,
// HTTP 408/429 and transient 5xx responses.
func (c *Client) get(ctx context.Context, endpoint string, query url.Values, timeout time.Duration, header http.Header, maxBytes int64, allowNotModified bool) ([]byte, bool, error) {
	var lastErr error
	for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
		body, notModified, retryable, err := c.getOnce(ctx, endpoint, query, timeout, header, maxBytes, allowNotModified)
		if err == nil {
			return body, notModified, nil
		}
		lastErr = err
		if !retryable || attempt == c.retry.MaxAttempts {
			return nil, false, err
		}
		delay := c.retry.delay(attempt, retryAfterOf(err))
		if err := c.retry.Sleep(ctx, delay); err != nil {
			return nil, false, err
		}
	}
	return nil, false, lastErr
}

func (c *Client) getOnce(ctx context.Context, endpoint string, query url.Values, timeout time.Duration, header http.Header, maxBytes int64, allowNotModified bool) ([]byte, bool, bool, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := endpoint
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, target, nil)
	if err != nil {
		return nil, false, false, fmt.Errorf("adsync: build request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.credential)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	path := request.URL.Path

	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, false, fmt.Errorf("adsync: GET %s: %w", path, ctx.Err())
		}
		if requestCtx.Err() != nil {
			return nil, false, true, fmt.Errorf("adsync: GET %s: %w", path, requestCtx.Err())
		}
		return nil, false, true, fmt.Errorf("adsync: GET %s: %w", path, err)
	}
	defer response.Body.Close()

	limit := maxBytes
	if limit <= 0 {
		limit = maxErrorResponseBytes
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil {
		if ctx.Err() != nil {
			return nil, false, false, fmt.Errorf("adsync: GET %s: %w", path, ctx.Err())
		}
		return nil, false, true, fmt.Errorf("adsync: GET %s: read response: %w", path, readErr)
	}
	if int64(len(body)) > limit {
		return nil, false, false, fmt.Errorf("%w: response from %s exceeds %d bytes", ErrProtocol, path, limit)
	}

	switch response.StatusCode {
	case http.StatusOK:
		return body, false, false, nil
	case http.StatusNotModified:
		if allowNotModified {
			return nil, true, false, nil
		}
		return nil, false, false, fmt.Errorf("%w: unexpected 304 from %s", ErrProtocol, path)
	default:
		statusErr := newStatusError(http.MethodGet, path, response.StatusCode, body)
		if detailed, ok := statusErr.(*statusError); ok {
			detailed.retryAfter = retryAfterDuration(response.Header)
		}
		return nil, false, retryableStatus(response.StatusCode), statusErr
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
