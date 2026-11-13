// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// runLongPoll carries the long-poll transport until it fails or the probe
// deadline is reached. Each request body is a heartbeat and each response is a
// confirming ack.
//
// probeAt is the auto-mode deadline for retrying WebSocket; the zero value
// disables probing.
func (c *Channel) runLongPoll(ctx context.Context, probeAt time.Time) (probe bool, healthy bool, err error) {
	c.setCurrent(TransportLongPoll)
	for {
		if !probeAt.IsZero() && !time.Now().Before(probeAt) {
			return true, healthy, nil
		}
		if err := ctx.Err(); err != nil {
			return false, healthy, err
		}
		started := time.Now()
		if err := c.pollOnce(ctx); err != nil {
			return false, healthy, err
		}
		healthy = true

		// A conforming source holds the request up to wait, so the next poll
		// follows immediately. Without a hold (wait=0) keep the heartbeat
		// cadence instead of spinning, and a source that answers a wait-length
		// poll immediately is throttled so it cannot turn this into a busy
		// loop.
		if c.pollWait < time.Second {
			if err := sleepContext(ctx, c.heartbeatInterval()); err != nil {
				return false, healthy, err
			}
			continue
		}
		if elapsed := time.Since(started); elapsed < minPollGap {
			if err := sleepContext(ctx, minPollGap-elapsed); err != nil {
				return false, healthy, err
			}
		}
	}
}

// pollOnce performs one long-poll request and applies its ack.
func (c *Channel) pollOnce(ctx context.Context) error {
	payload, err := json.Marshal(c.heartbeatMessage(TransportLongPoll))
	if err != nil {
		return fmt.Errorf("adsync: encode heartbeat: %w", err)
	}
	if len(payload) > maxChannelFrameBytes {
		return fmt.Errorf("%w: channel frame exceeds %d bytes", ErrProtocol, maxChannelFrameBytes)
	}

	timeout := c.pollWait + c.client.requestTimeout
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.pollURL(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("adsync: build poll request: %w", err)
	}
	header, err := c.requestHeaders()
	if err != nil {
		return &terminalError{err: err}
	}
	request.Header = header
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.http.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("adsync: poll channel: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxChannelFrameBytes+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("adsync: poll channel: read response: %w", err)
	}
	// Classify the status before enforcing the frame bound: a permanent
	// rejection with a large error body must stop the channel, not look like a
	// retryable protocol error.
	if response.StatusCode != http.StatusOK {
		bodyCopy := body
		if terminal := c.terminalStatusError(http.MethodPost, APIPrefix+"/channel/poll", response.StatusCode, bodyCopy); terminal != nil {
			return terminal
		}
		statusErr := newStatusError(http.MethodPost, APIPrefix+"/channel/poll", response.StatusCode, bodyCopy)
		if detailed, ok := statusErr.(*statusError); ok {
			detailed.retryAfter = retryAfterDuration(response.Header)
		}
		return statusErr
	}
	if len(body) > maxChannelFrameBytes {
		return fmt.Errorf("%w: poll response exceeds %d bytes", ErrProtocol, maxChannelFrameBytes)
	}

	message, err := decodeServerMessage(body)
	if err != nil {
		return err
	}
	ack, ok := message.(*channelAck)
	if !ok {
		return fmt.Errorf("%w: poll response is not an ack", ErrProtocol)
	}
	return c.applyAck(ack, nil, nil)
}
