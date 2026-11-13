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
	// AuthzCheckAPIPrefix is the Cloud endpoint that evaluates authorization
	// requests and accepts an instance credential. Fallback calls use the same
	// body as a local evaluation request.
	AuthzCheckAPIPrefix = "/authz-check/v1/"

	// DefaultFallbackTimeout bounds one forwarded authorization request.
	DefaultFallbackTimeout = 3 * time.Second
	// DefaultFallbackRetryBackoff suppresses requests briefly after a
	// transport, rate-limit or 5xx failure. It bounds the outage observation;
	// it never latches fallback off permanently.
	DefaultFallbackRetryBackoff = time.Second
)

// FallbackOperation is one authorization operation that can be forwarded to
// Cloud while a service is syncing. Cloud exposes only these three operations.
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

// FallbackResponse is Cloud's authorization response. StatusCode, Header and
// Body are passed through to the caller unchanged.
type FallbackResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// FallbackForwarder forwards an authorization request to the configured
// Speedle Cloud destination with the configured Cloud credential. It is
// implemented by the policy-source store and used by the REST and gRPC request
// gates.
type FallbackForwarder interface {
	ForwardFallback(ctx context.Context, operation FallbackOperation, body []byte) (*FallbackResponse, error)
	FallbackAvailable() bool
}

var (
	// ErrFallbackUnavailable means no fallback is configured for this instance,
	// for example because the feature is disabled or no credential was loaded.
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
	status int
	code   string
	reason string
	cause  error
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

// ForwardFallback evaluates one authorization request on the configured Cloud
// destination with the Cloud credential. The request body is sent unchanged and
// the credential never appears in a URL or a log message.
//
// A request is retried at most once, and only when the connection could not be
// established, so a request that may have reached Cloud is never repeated.
// HTTP 429 and 5xx responses are not retried.
func (c *Client) ForwardFallback(ctx context.Context, operation FallbackOperation, body []byte) (*FallbackResponse, error) {
	if c.fallbackBase == nil || c.fallbackCredential == "" || c.fallbackHTTP == nil {
		return nil, ErrFallbackUnavailable
	}
	if !operation.Valid() {
		return nil, fmt.Errorf("%w: unknown fallback operation %q", ErrProtocol, operation)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("%w: empty fallback request body", ErrProtocol)
	}
	for attempt := 0; ; attempt++ {
		response, err := c.forwardOnce(ctx, operation, body)
		if err == nil {
			c.recordFallbackOutcome(nil)
			return response, nil
		}
		c.recordFallbackOutcome(err)
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

	target := *c.fallbackBase
	target.Path = c.fallbackBase.Path + AuthzCheckAPIPrefix + string(operation)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("adsync: build fallback request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.fallbackCredential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", c.userAgent)

	response, err := c.fallbackHTTP.Do(request)
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
		return nil, newFallbackStatusError(response.StatusCode, responseBody)
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
func (c *Client) recordFallbackOutcome(err error) {
	retryLater := err != nil && (errors.Is(err, ErrUnavailable) || errors.Is(err, ErrRateLimited) ||
		errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden))
	c.fallbackMu.Lock()
	if retryLater {
		c.fallbackRetryAt = c.now().Add(c.fallbackBackoff)
	} else {
		c.fallbackRetryAt = time.Time{}
	}
	c.fallbackMu.Unlock()
}

// FallbackAvailable reports whether fallback is configured and not inside its
// short retry delay. Configuration and transient reachability are deliberately
// separate: an expired delay permits the next request, which can then restore
// readiness after a successful response.
func (c *Client) FallbackAvailable() bool {
	if c.fallbackBase == nil || c.fallbackCredential == "" || c.fallbackHTTP == nil {
		return false
	}
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	return c.fallbackRetryAt.IsZero() || !c.now().Before(c.fallbackRetryAt)
}

// isConnectionEstablishmentFailure reports whether the request failed before
// it could be sent: DNS, TCP or proxy CONNECT failures, and TLS handshake or
// certificate errors. A timeout while waiting for a response is not included,
// because the request may have reached Cloud.
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
