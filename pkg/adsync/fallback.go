// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	// AuthzCheckAPIPrefix is the upstream endpoint that evaluates
	// authorization requests with the instance's API key. Forwarded calls use
	// the same body as a local evaluation request.
	AuthzCheckAPIPrefix = "/authz-check/v1/"

	// DefaultFallbackTimeout bounds one forwarded authorization request.
	DefaultFallbackTimeout = 3 * time.Second
	// DefaultFallbackRetryBackoff suppresses requests briefly after a
	// transport, rate-limit or 5xx failure. It bounds the outage observation;
	// it never latches fallback off permanently.
	DefaultFallbackRetryBackoff = time.Second

	// MaxUpstreamInFlight bounds the forwarded requests an instance has in
	// flight. A request beyond it is denied, not queued.
	MaxUpstreamInFlight = 20
	// MaxUpstreamRetryAfter caps how long a Retry-After from the upstream
	// suppresses forwarding. A daily_quota_exceeded answer is the exception:
	// its Retry-After is honoured in full, up to the next UTC midnight, when
	// the daily quota resets.
	MaxUpstreamRetryAfter = 60 * time.Second

	// DecisionSourceUpstream marks a response the upstream produced.
	DecisionSourceUpstream = "upstream"
)

// FallbackOperation is one authorization operation that can be forwarded to
// the upstream while a service is syncing or stale. The upstream exposes only
// these three operations.
type FallbackOperation string

const (
	FallbackIsAllowed             FallbackOperation = "is-allowed"
	FallbackAllGrantedRoles       FallbackOperation = "all-granted-roles"
	FallbackAllGrantedPermissions FallbackOperation = "all-granted-permissions"
)

// Valid reports whether the operation may be forwarded.
func (o FallbackOperation) Valid() bool {
	switch o {
	case FallbackIsAllowed, FallbackAllGrantedRoles, FallbackAllGrantedPermissions:
		return true
	default:
		return false
	}
}

// FallbackResponse is the upstream's authorization response. StatusCode,
// Header and Body are passed through to the caller unchanged.
type FallbackResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	// DecisionSource is the decision-source marker for the caller; it is
	// always DecisionSourceUpstream.
	DecisionSource string
}

// FallbackForwarder forwards an authorization request to the upstream with
// the instance's API key. It is
// implemented by the policy-source store and used by the REST and gRPC request
// gates.
type FallbackForwarder interface {
	ForwardFallback(ctx context.Context, operation FallbackOperation, body []byte) (*FallbackResponse, error)
	FallbackAvailable() bool
}

var (
	// ErrFallbackUnavailable means forwarding is not possible for this
	// instance.
	ErrFallbackUnavailable = errors.New("adsync: fallback is not configured")
	// ErrFallbackQuotaExhausted means Cloud refused the fallback call because
	// the instance or account reached its fallback limit. Cloud answered, so
	// the source is reachable.
	ErrFallbackQuotaExhausted = errors.New("adsync: fallback quota exhausted")
)

// fallbackTransportError marks a request that never produced an HTTP response.
// Only these errors may be retried, and only when the failure happened while
// establishing the connection (see isConnectionEstablishmentFailure).
type fallbackTransportError struct {
	err error
}

func (e *fallbackTransportError) Error() string { return e.err.Error() }
func (e *fallbackTransportError) Unwrap() error { return e.err }

// Is classifies every request that produced no HTTP response as an
// unavailable source, while Unwrap still exposes the concrete transport error
// for the retry decision.
func (e *fallbackTransportError) Is(target error) bool {
	return target == ErrUnavailable
}

// fallbackStatusError is a non-2xx response from Cloud.
type fallbackStatusError struct {
	status     int
	code       string
	reason     string
	retryAfter time.Duration
	cause      error
}

func (e *fallbackStatusError) Error() string {
	detail := e.code
	if e.reason != "" {
		detail += " (" + e.reason + ")"
	}
	if detail == "" {
		detail = http.StatusText(e.status)
	}
	return fmt.Sprintf("adsync: POST %s: HTTP %d: %s", AuthzCheckAPIPrefix, e.status, detail)
}

func (e *fallbackStatusError) Unwrap() error { return e.cause }

func newFallbackStatusError(status int, body []byte) error {
	var envelope errorEnvelope
	_ = decodeLenient(body, &envelope)
	e := &fallbackStatusError{status: status, code: envelope.Error, reason: envelope.Reason}
	switch {
	case status == http.StatusServiceUnavailable && envelope.Error == "fallback_unavailable":
		e.cause = ErrFallbackQuotaExhausted
	case status == http.StatusTooManyRequests && envelope.Error == "daily_quota_exceeded":
		e.cause = ErrFallbackQuotaExhausted
	case status == http.StatusUnauthorized:
		e.cause = ErrUnauthorized
	case status == http.StatusForbidden:
		e.cause = ErrForbidden
	case status == http.StatusNotFound:
		e.cause = ErrServiceNotFound
	case status == http.StatusTooManyRequests:
		e.cause = ErrRateLimited
	case status >= 500:
		e.cause = ErrUnavailable
	default:
		e.cause = ErrProtocol
	}
	return e
}

// ForwardFallback evaluates one authorization request on the upstream with
// the instance's API key. The request body is sent unchanged and the key never
// appears in a URL or a log message.
//
// At most MaxUpstreamInFlight requests are in flight; a request beyond that is
// refused. A request is retried at most once, and only when the connection
// could not be established, so a request that may have reached the upstream is
// never repeated. HTTP 429 and 5xx responses are not retried.
func (c *Client) ForwardFallback(ctx context.Context, operation FallbackOperation, body []byte) (*FallbackResponse, error) {
	if !operation.Valid() {
		return nil, fmt.Errorf("%w: unknown fallback operation %q", ErrProtocol, operation)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty fallback request body", ErrProtocol)
	}
	select {
	case c.fallbackSlots <- struct{}{}:
		defer func() { <-c.fallbackSlots }()
	default:
		return nil, fmt.Errorf("%w: %d forwarded requests are already in flight", ErrUnavailable, MaxUpstreamInFlight)
	}
	for attempt := 0; ; attempt++ {
		started := c.fallbackRetrySequence()
		response, err := c.forwardOnce(ctx, operation, body)
		if err == nil {
			c.recordFallbackOutcome(nil, started)
			response.DecisionSource = DecisionSourceUpstream
			return response, nil
		}
		c.recordFallbackOutcome(err, started)
		if attempt == 0 && ctx.Err() == nil && isConnectionEstablishmentFailure(err) {
			continue
		}
		return nil, err
	}
}

func (c *Client) forwardOnce(ctx context.Context, operation FallbackOperation, body []byte) (*FallbackResponse, error) {
	timeout := c.fallbackTimeout
	if timeout == 0 {
		timeout = DefaultFallbackTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := *c.selectedBase()
	target.Path = target.Path + AuthzCheckAPIPrefix + string(operation)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("adsync: build fallback request: %w", err)
	}
	if err := c.authorize(request.Header); err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", c.userAgent)

	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if requestCtx.Err() != nil {
			return nil, fmt.Errorf("%w: POST %s: %v", ErrUnavailable, target.Path, requestCtx.Err())
		}
		return nil, &fallbackTransportError{err: err}
	}
	defer response.Body.Close()

	reader := io.Reader(response.Body)
	if response.StatusCode < 200 || response.StatusCode > 299 {
		reader = io.LimitReader(reader, maxErrorResponseBytes)
	}
	responseBody, readErr := io.ReadAll(reader)
	if readErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &fallbackTransportError{err: readErr}
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		statusErr := newFallbackStatusError(response.StatusCode, responseBody)
		statusErr.(*fallbackStatusError).retryAfter = retryAfterDuration(response.Header)
		return nil, statusErr
	}
	return &FallbackResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Body:       responseBody,
	}, nil
}

// recordFallbackOutcome remembers a bounded retry delay after a transport,
// rate-limit, authentication or 5xx failure. Other HTTP answers clear the
// delay. A later request is always allowed once the delay expires, so one
// transient error cannot disable fallback forever.
//
// Requests complete out of order. A failure keeps the later of its own
// deadline and the current one. An answer clears the deadline only when no
// other response set one while this request was in flight (started is the
// sequence observed when it was sent), so a response that was already on its
// way never shortens or clears a newer deadline.
func (c *Client) recordFallbackOutcome(err error, started uint64) {
	retryLater := err != nil && (errors.Is(err, ErrUnavailable) || errors.Is(err, ErrRateLimited) ||
		errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden))
	delay := c.fallbackBackoff
	// The upstream's Retry-After is honoured on any failure, up to a minute,
	// or up to the next UTC midnight for an exhausted daily quota; requests
	// are denied meanwhile.
	var statusErr *fallbackStatusError
	if errors.As(err, &statusErr) && statusErr.retryAfter > 0 {
		retryLater = true
		delay = statusErr.retryAfter
		limit := MaxUpstreamRetryAfter
		if statusErr.code == "daily_quota_exceeded" {
			now := c.now().UTC()
			limit = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC).Sub(now)
		}
		if delay > limit {
			delay = limit
		}
	}
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	if retryLater {
		if deadline := c.now().Add(delay); deadline.After(c.fallbackRetryAt) {
			c.fallbackRetryAt = deadline
			c.fallbackRetrySeq++
		}
		return
	}
	if c.fallbackRetrySeq == started {
		c.fallbackRetryAt = time.Time{}
	}
}

func (c *Client) fallbackRetrySequence() uint64 {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	return c.fallbackRetrySeq
}

// FallbackAvailable reports whether forwarding is outside its short retry
// delay. An expired delay permits the next request, which can then restore
// readiness after a successful response.
func (c *Client) FallbackAvailable() bool {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	return c.fallbackRetryAt.IsZero() || !c.now().Before(c.fallbackRetryAt)
}

// isConnectionEstablishmentFailure reports whether the request failed before
// it could be sent: DNS, TCP or proxy CONNECT failures, and TLS handshake or
// certificate errors. A timeout while waiting for a response is not included,
// because the request may have reached the upstream.
func isConnectionEstablishmentFailure(err error) bool {
	var transportErr *fallbackTransportError
	if !errors.As(err, &transportErr) {
		return false
	}
	cause := transportErr.err
	var opErr *net.OpError
	if errors.As(cause, &opErr) && (opErr.Op == "dial" || opErr.Op == "proxyconnect") {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(cause, &dnsErr) {
		return true
	}
	var certificateErr *tls.CertificateVerificationError
	if errors.As(cause, &certificateErr) {
		return true
	}
	var recordErr tls.RecordHeaderError
	if errors.As(cause, &recordErr) {
		return true
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(cause, &unknownAuthority) {
		return true
	}
	var hostnameErr x509.HostnameError
	if errors.As(cause, &hostnameErr) {
		return true
	}
	var invalidCert x509.CertificateInvalidError
	if errors.As(cause, &invalidCert) {
		return true
	}
	return false
}

// ReadFallbackResponse decodes a forwarded authorization response body. The
// REST entry point passes the bytes through unchanged; the gRPC entry point,
// which must map them onto protobuf messages, uses this helper.
func ReadFallbackResponse(body []byte, dst any) error {
	if err := decodeLenient(body, dst); err != nil {
		return fmt.Errorf("%w: invalid fallback response: %v", ErrProtocol, err)
	}
	return nil
}
