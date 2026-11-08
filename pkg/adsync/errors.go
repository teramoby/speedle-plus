// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Errors returned by this package. They are sentinels so callers can classify
// a failure without inspecting a response body.
var (
	// ErrUnauthorized means the instance credential is missing, malformed,
	// unknown or revoked (HTTP 401).
	ErrUnauthorized = errors.New("adsync: unauthorized")
	// ErrForbidden means the credential cannot use this endpoint (HTTP 403).
	ErrForbidden = errors.New("adsync: forbidden")
	// ErrServiceNotFound means the service does not exist, belongs to another
	// account or is outside the instance's service set (HTTP 404). All three
	// are deliberately indistinguishable.
	ErrServiceNotFound = errors.New("adsync: service not found")
	// ErrSnapshotRequired means the incremental history cannot be resumed
	// (HTTP 410). Reload the snapshot.
	ErrSnapshotRequired = errors.New("adsync: snapshot required")
	// ErrSourceGone means the service or the whole source no longer exists.
	// It is also returned for a signed service-delete change. Delete the local
	// copy instead of retrying.
	ErrSourceGone = errors.New("adsync: policy source is gone")
	// ErrRateLimited means the source asked the client to back off (HTTP 429).
	ErrRateLimited = errors.New("adsync: rate limited")
	// ErrUnavailable means the source is temporarily unavailable (HTTP 5xx or
	// a connection that could not be established).
	ErrUnavailable = errors.New("adsync: policy source unavailable")
	// ErrProtocol means the response did not match the wire contract: an
	// unknown field, a gap in revisions, a malformed page and so on.
	ErrProtocol = errors.New("adsync: protocol violation")
	// ErrIntegrity means a canonical hash or an Ed25519 signature did not
	// verify. The data must not be used.
	ErrIntegrity = errors.New("adsync: integrity check failed")
	// ErrCacheCorrupt means a cache file failed its checksum or its content
	// no longer matches its stored identity.
	ErrCacheCorrupt = errors.New("adsync: cache is corrupt")
	// ErrUnsafeCache means a cache file is not a regular 0600 file.
	ErrUnsafeCache = errors.New("adsync: cache file permissions are unsafe")
	// ErrNotModified is returned by Snapshot when If-None-Match matched.
	ErrNotModified = errors.New("adsync: not modified")
)

// SnapshotRequiredError carries the reason code of a 410 response.
type SnapshotRequiredError struct {
	Reason string
}

func (e *SnapshotRequiredError) Error() string {
	if e.Reason == "" {
		return ErrSnapshotRequired.Error()
	}
	return fmt.Sprintf("%s: %s", ErrSnapshotRequired, e.Reason)
}

// Is makes errors.Is(err, ErrSnapshotRequired) true for every reason, and
// errors.Is(err, ErrSourceGone) true for source_gone.
func (e *SnapshotRequiredError) Is(target error) bool {
	return target == ErrSnapshotRequired ||
		(target == ErrSourceGone && e.Reason == string(snapshotReasonSourceGone))
}

const (
	snapshotReasonHistoryUnavailable = "history_unavailable"
	snapshotReasonEpochMismatch      = "epoch_mismatch"
	snapshotReasonSourceGone         = "source_gone"
	snapshotReasonSignatureRequired  = "signature_required"
)

// statusError is the typed HTTP failure shared by every request. Unwrap
// returns the matching sentinel.
type statusError struct {
	method     string
	path       string
	status     int
	code       string
	reason     string
	retryAfter time.Duration
	cause      error
}

func (e *statusError) Error() string {
	detail := e.code
	if e.reason != "" {
		detail += " (" + e.reason + ")"
	}
	if detail == "" {
		detail = http.StatusText(e.status)
	}
	return fmt.Sprintf("adsync: %s %s: HTTP %d: %s", e.method, e.path, e.status, detail)
}

func (e *statusError) Unwrap() error {
	return e.cause
}

// errorEnvelope is the flat error body from the contract. Unknown fields are
// ignored so a newer source can add detail without breaking classification.
type errorEnvelope struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

func newStatusError(method, path string, status int, body []byte) error {
	var envelope errorEnvelope
	_ = decodeLenient(body, &envelope)
	e := &statusError{
		method: method,
		path:   path,
		status: status,
		code:   envelope.Error,
		reason: envelope.Reason,
	}
	switch status {
	case http.StatusUnauthorized:
		e.cause = ErrUnauthorized
	case http.StatusForbidden:
		e.cause = ErrForbidden
	case http.StatusNotFound:
		e.cause = ErrServiceNotFound
	case http.StatusGone:
		reason := envelope.Reason
		if reason == "" {
			reason = snapshotReasonHistoryUnavailable
		}
		e.cause = &SnapshotRequiredError{Reason: reason}
	case http.StatusTooManyRequests:
		e.cause = ErrRateLimited
	default:
		if status >= 500 {
			e.cause = ErrUnavailable
		} else {
			e.cause = ErrProtocol
		}
	}
	return e
}

// retryAfterDuration parses a Retry-After header. HTTP-date values are not
// used by the policy-source API, so only delay-seconds are accepted.
func retryAfterDuration(header http.Header) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return 0
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	const maxRetryAfter = 24 * time.Hour
	if seconds > int64(maxRetryAfter/time.Second) {
		return maxRetryAfter
	}
	return time.Duration(seconds) * time.Second
}
