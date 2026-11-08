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

	dialCtx, cancel := context.WithTimeout(ctx, c.client.requestTimeout)
	conn, response, err := c.dialer.DialContext(dialCtx, c.channelURL(), c.requestHeaders())
	cancel()
	if err != nil {
		if response != nil {
			var body []byte
			if response.Body != nil {
				body, _ = io.ReadAll(io.LimitReader(response.Body, maxErrorResponseBytes))
				_ = response.Body.Close()
			}
			if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
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

	conn.SetReadLimit(maxChannelFrameBytes)
	extendDeadline := func() {
		_ = conn.SetReadDeadline(time.Now().Add(c.readTimeout()))
	}
	_ = conn.SetReadDeadline(time.Now().Add(c.ackTimeout))
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
	var writerDone sync.WaitGroup
	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		c.writeWebSocket(writerCtx, conn, results)
	}()
	defer func() {
		stopWriter()
		_ = conn.Close()
		writerDone.Wait()
	}()

	var ackedAt time.Time
	var outcome wsOutcome
readLoop:
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				outcome.err = ctxErr
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
			if err := c.applyAck(typed, results); err != nil {
				outcome.err = err
				break readLoop
			}
			if ackedAt.IsZero() {
				ackedAt = time.Now()
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
// and command results all go through it.
func (c *Channel) writeWebSocket(ctx context.Context, conn *websocket.Conn, results <-chan channelCommandResult) {
	heartbeat := time.NewTimer(c.heartbeatInterval())
	ping := time.NewTimer(c.pingInterval())
	defer heartbeat.Stop()
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
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
			heartbeat.Reset(c.heartbeatInterval())
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.writeTimeout())); err != nil {
				_ = conn.Close()
				return
			}
			ping.Reset(c.pingInterval())
		}
	}
}

func (c *Channel) writeChannelJSON(conn *websocket.Conn, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("adsync: encode channel frame: %w", err)
	}
	if len(payload) > maxChannelFrameBytes {
		return fmt.Errorf("%w: channel frame exceeds %d bytes", ErrProtocol, maxChannelFrameBytes)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout()))
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return fmt.Errorf("adsync: write channel frame: %w", err)
	}
	return nil
}
