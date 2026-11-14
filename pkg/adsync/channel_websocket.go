// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsOutcome describes how one WebSocket connection ended.
type wsOutcome struct {
	// terminal means the failure needs operator action and must not be
	// retried.
	terminal bool
	// upgradeFailed means the WebSocket upgrade never completed.
	upgradeFailed bool
	// noAck means the upgrade completed but no ack arrived before the
	// connection ended.
	noAck bool
	// healthy means the connection received an ack and then stayed up for at
	// least one heartbeat interval. It resets the reconnect backoff.
	healthy bool
	err     error
}

// runWebSocket carries one WebSocket connection until it fails.
func (c *Channel) runWebSocket(ctx context.Context) wsOutcome {
	c.setCurrent(TransportWebSocket)

	header, err := c.requestHeaders()
	if err != nil {
		return wsOutcome{err: &terminalError{err: err}}
	}
	dialCtx, cancel := context.WithTimeout(ctx, c.client.requestTimeout)
	dialer := *c.dialer
	if c.client.tlsMaterial != nil {
		dialer.TLSClientConfig = c.client.tlsMaterial.configForHost(c.client.channelBase().Hostname())
	}
	conn, response, err := dialer.DialContext(dialCtx, c.channelURL(), header)
	cancel()
	if err != nil {
		if response != nil {
			var body []byte
			if response.Body != nil {
				body, _ = io.ReadAll(io.LimitReader(response.Body, maxErrorResponseBytes))
				_ = response.Body.Close()
			}
			// A channel pinned to WebSocket has no fallback, so a permanent
			// HTTP rejection is terminal there exactly as it is for long
			// polling; retrying it would loop forever. Auto mode falls back
			// instead: the poll request then reaches the same classification.
			if c.mode == TransportWebSocket {
				if terminal := c.terminalStatusError(http.MethodGet, APIPrefix+"/channel", response.StatusCode, body); terminal != nil {
					return wsOutcome{terminal: true, err: terminal}
				}
			} else if !c.client.cascade() && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
				c.machine.Stop(ReasonRevoked)
				return wsOutcome{
					terminal: true,
					err:      &terminalError{err: newStatusError(http.MethodGet, APIPrefix+"/channel", response.StatusCode, body)},
				}
			}
		}
		return wsOutcome{
			upgradeFailed: true,
			err:           fmt.Errorf("adsync: open channel: %w", err),
		}
	}
	defer conn.Close()
	// ReadMessage has no context. Closing the socket on cancellation is what
	// lets a production store's Close join the channel immediately instead of
	// waiting for the next heartbeat or read deadline.
	stopOnCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopOnCancel()

	var ackedAt time.Time
	// The first ack is bounded by an absolute deadline. Keepalive traffic must
	// not keep a connection that never gets an ack alive, or auto mode could
	// never observe the missing ack and fall back. Once the first ack arrives,
	// the ordinary keepalive timeout applies.
	firstAckDeadline := time.Now().Add(c.ackTimeout)
	extendDeadline := func() {
		deadline := time.Now().Add(c.readTimeout())
		if ackedAt.IsZero() && deadline.After(firstAckDeadline) {
			deadline = firstAckDeadline
		}
		_ = conn.SetReadDeadline(deadline)
	}
	_ = conn.SetReadDeadline(firstAckDeadline)
	conn.SetPongHandler(func(string) error {
		extendDeadline()
		return nil
	})
	pingHandler := conn.PingHandler()
	conn.SetPingHandler(func(message string) error {
		extendDeadline()
		return pingHandler(message)
	})

	if err := c.writeChannelJSON(conn, c.helloMessage(TransportWebSocket)); err != nil {
		return wsOutcome{err: err}
	}

	writerCtx, stopWriter := context.WithCancel(ctx)
	results := make(chan channelCommandResult, 16)
	cadence := make(chan struct{}, 1)
	var writerDone sync.WaitGroup
	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		c.writeWebSocket(writerCtx, conn, results, cadence)
	}()
	defer func() {
		stopWriter()
		_ = conn.Close()
		writerDone.Wait()
	}()

	var outcome wsOutcome
readLoop:
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				outcome.err = ctxErr
			} else if websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
				outcome.err = c.frameRejected()
			} else {
				outcome.err = fmt.Errorf("adsync: channel read: %w", err)
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					// A silent connection is half open; the caller decides
					// whether to reconnect or switch transport.
					_ = conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseGoingAway, "heartbeat timeout"),
						time.Now().Add(c.writeTimeout()))
				}
			}
			break
		}
		if err := ctx.Err(); err != nil {
			outcome.err = err
			break
		}
		extendDeadline()
		if messageType != websocket.TextMessage {
			outcome.err = fmt.Errorf("%w: channel sent a non-text frame", ErrProtocol)
			break
		}
		message, err := decodeServerMessage(payload)
		if err != nil {
			outcome.err = err
			break
		}
		switch typed := message.(type) {
		case *channelAck:
			if err := c.applyAck(typed, results, cadence); err != nil {
				outcome.err = err
				break readLoop
			}
			if ackedAt.IsZero() {
				ackedAt = time.Now()
				// That frame may have been capped at the first-ack deadline;
				// refresh it now that keepalive timing applies.
				extendDeadline()
			}
		case *channelNotify:
			if err := c.machine.ObserveHead(Head{ID: typed.ServiceID, Revision: typed.Head}); err != nil {
				outcome.err = err
				break readLoop
			}
			if c.onNotify != nil {
				c.onNotify(Notify{ServiceID: typed.ServiceID, Head: typed.Head})
			}
		}
	}

	outcome.noAck = ackedAt.IsZero()
	outcome.healthy = !ackedAt.IsZero() && time.Since(ackedAt) >= c.heartbeatInterval()
	if outcome.err == nil {
		outcome.err = fmt.Errorf("adsync: channel closed")
	}
	return outcome
}

// writeWebSocket is the connection's only writer: heartbeats, protocol pings
// and command results all go through it. An ack can change the effective
// cadence, so a cadence signal resets a timer whose interval actually changed,
// instead of waiting for the previous, possibly longer, interval to expire.
// Acks that leave the cadence alone do not postpone a heartbeat: a source may
// send them more often than the heartbeat interval.
func (c *Channel) writeWebSocket(ctx context.Context, conn *websocket.Conn, results <-chan channelCommandResult, cadence <-chan struct{}) {
	heartbeatInterval := c.heartbeatInterval()
	pingInterval := c.pingInterval()
	heartbeat := time.NewTimer(heartbeatInterval)
	ping := time.NewTimer(pingInterval)
	defer heartbeat.Stop()
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			// Close the connection so the reader blocked in ReadMessage wakes
			// up; a live peer would otherwise keep extending its deadline.
			_ = conn.Close()
			return
		case <-cadence:
			if next := c.heartbeatInterval(); next != heartbeatInterval {
				heartbeatInterval = next
				resetTimer(heartbeat, heartbeatInterval)
			}
			if next := c.pingInterval(); next != pingInterval {
				pingInterval = next
				resetTimer(ping, pingInterval)
			}
		case result := <-results:
			if err := c.writeChannelJSON(conn, result); err != nil {
				_ = conn.Close()
				return
			}
		case <-heartbeat.C:
			if err := c.writeChannelJSON(conn, c.heartbeatMessage(TransportWebSocket)); err != nil {
				_ = conn.Close()
				return
			}
			heartbeatInterval = c.heartbeatInterval()
			resetTimer(heartbeat, heartbeatInterval)
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.writeTimeout())); err != nil {
				_ = conn.Close()
				return
			}
			pingInterval = c.pingInterval()
			resetTimer(ping, pingInterval)
		}
	}
}

// resetTimer resets a timer to a new delay, draining a tick that already fired
// so it cannot be observed as a spurious heartbeat or ping.
func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func (c *Channel) writeChannelJSON(conn *websocket.Conn, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("adsync: encode channel frame: %w", err)
	}
	c.sentFrameBytes.Store(int64(len(payload)))
	switch frame := value.(type) {
	case channelHello:
		if frame.Seq != nil {
			c.recordSent(*frame.Seq)
		}
	case channelHeartbeat:
		c.recordSent(frame.Seq)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout()))
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return fmt.Errorf("adsync: write channel frame: %w", err)
	}
	return nil
}
