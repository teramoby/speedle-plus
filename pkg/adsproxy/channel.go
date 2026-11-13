// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

const (
	// maxChannelFrameBytes is the application-level bound on one channel frame.
	// An ack carries every advertised service, and the mirror holds at most
	// maxTombstones services; one worst-case entry (36-byte id, 64-byte name,
	// two 20-digit revisions, 36-byte epoch and a 64-character state hash) is
	// under 320 bytes, so a full ack stays well under this bound. It matches
	// the one-megabyte response-page bound of the protocol.
	maxChannelFrameBytes = 1 << 20
	// maxPollWait is the longest hold a long-poll request may ask for.
	maxPollWait = 30 * time.Second
	// defaultPollWait matches the contract's wait=25 default.
	defaultPollWait = 25 * time.Second
	// helloTimeout bounds how long a WebSocket may stay silent before its
	// hello frame.
	helloTimeout = 10 * time.Second
	// channelWriteTimeout bounds one channel write.
	channelWriteTimeout = 10 * time.Second
	// heartbeatFrameSlack is how many heartbeat intervals a WebSocket may stay
	// silent before it is treated as half open. Protocol pings extend the
	// deadline independently.
	heartbeatFrameSlack = 2
)

// helloMessage is the downstream's first WebSocket frame.
type helloMessage struct {
	Type         string           `json:"type"`
	InstanceID   string           `json:"instanceId"`
	Services     []helloService   `json:"services"`
	AgentVersion string           `json:"agentVersion"`
	Host         *adsync.HostInfo `json:"host"`
}

type helloService struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Revision uint64 `json:"revision"`
	State    string `json:"state"`
}

// heartbeatMessage is a heartbeat frame and, in long-poll mode, the body of
// every poll request.
type heartbeatMessage struct {
	Type            string             `json:"type"`
	InstanceID      string             `json:"instanceId"`
	Host            *adsync.HostInfo   `json:"host,omitempty"`
	Seq             uint64             `json:"seq"`
	Services        []heartbeatService `json:"services"`
	Liveness        string             `json:"liveness"`
	Fallback        fallbackMessage    `json:"fallback"`
	AgentVersion    string             `json:"agentVersion"`
	Transport       string             `json:"transport"`
	DownstreamCount *uint64            `json:"downstreamCount,omitempty"`
}

type heartbeatService struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Revision   uint64 `json:"revision"`
	State      string `json:"state"`
	LagSeconds uint64 `json:"lagSeconds"`
}

type fallbackMessage struct {
	Active      bool     `json:"active"`
	Services    []string `json:"services"`
	Since       *int64   `json:"since"`
	RoutedCalls uint64   `json:"routedCalls"`
}

// ackMessage confirms a heartbeat and carries the authoritative service set.
type ackMessage struct {
	Type              string           `json:"type"`
	ServicesVersion   int64            `json:"servicesVersion"`
	Services          []ackService     `json:"services"`
	ServerTime        int64            `json:"serverTime"`
	HeartbeatInterval string           `json:"heartbeatInterval"`
	StaleAfter        string           `json:"staleAfter"`
	StopAfter         string           `json:"stopAfter"`
	Commands          []commandMessage `json:"commands"`
}

type ackService struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Head          uint64      `json:"head"`
	OriginHead    uint64      `json:"originHead"`
	Epoch         string      `json:"epoch"`
	HeadStateHash adsync.Hash `json:"headStateHash"`
}

// commandMessage is part of the ack wire format. A downstream relay never
// sends commands.
type commandMessage struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	ServiceID string `json:"serviceId,omitempty"`
	ExpiresAt int64  `json:"expiresAt"`
}

type notifyMessage struct {
	Type      string `json:"type"`
	ServiceID string `json:"serviceId"`
	Head      uint64 `json:"head"`
}

func (s *Server) ack() ackMessage {
	// The service list and the version it belongs to must come from one read,
	// or a downstream that records the version could skip a refresh that was
	// already reflected in a different list.
	services, version := s.mirror.ServicesVersion()
	entries := make([]ackService, 0, len(services))
	for _, service := range services {
		entries = append(entries, ackService{
			ID:            service.ID,
			Name:          service.Name,
			Head:          service.Revision,
			OriginHead:    service.Revision,
			Epoch:         service.Epoch,
			HeadStateHash: service.HeadStateHash,
		})
	}
	return ackMessage{
		Type:              "ack",
		ServicesVersion:   version,
		Services:          entries,
		ServerTime:        s.now().UnixMilli(),
		HeartbeatInterval: formatChannelDuration(s.heartbeat),
		StaleAfter:        formatChannelDuration(s.stale),
		StopAfter:         formatChannelDuration(s.stop),
		Commands:          []commandMessage{},
	}
}

func formatChannelDuration(duration time.Duration) string {
	if duration%time.Second == 0 {
		return strconv.Itoa(int(duration/time.Second)) + "s"
	}
	return strconv.Itoa(int(duration/time.Millisecond)) + "ms"
}

// handleChannel serves one instance channel over WebSocket. The client
// certificate is verified during the TLS handshake; the hello frame supplies
// the self-reported UUID that completes the session identity.
func (s *Server) handleChannel(w http.ResponseWriter, r *http.Request) {
	identity, ok := clientIdentity(r)
	if !ok {
		writeError(w, http.StatusForbidden, "unauthorized", "")
		return
	}
	if _, ok := strictQuery(r.URL.RawQuery); !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// The upgrader already wrote the HTTP error.
		return
	}
	defer conn.Close()
	conn.SetReadLimit(maxChannelFrameBytes)

	_ = conn.SetReadDeadline(time.Now().Add(helloTimeout))
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		return
	}
	if messageType != websocket.TextMessage {
		closeWithReason(conn, websocket.CloseProtocolError, "expected hello")
		return
	}
	var hello helloMessage
	if err := decodeStrict(payload, &hello); err != nil {
		closeWithReason(conn, websocket.CloseProtocolError, "invalid hello")
		return
	}
	if err := validateHello(&hello); err != nil {
		closeWithReason(conn, websocket.CloseProtocolError, "invalid hello")
		return
	}
	identity.uuid = hello.InstanceID
	previous, admitted := s.beginChannelSession(identity, conn)
	if !admitted {
		closeWithReason(conn, websocket.CloseGoingAway, "server shutting down")
		return
	}
	defer s.endChannelSession(identity, conn)
	if previous != nil {
		// A reconnect for the same session identity supersedes the old
		// channel. Closing it keeps one live channel per session.
		closeWithReason(previous, websocket.CloseGoingAway, "session replaced")
		_ = previous.Close()
	}

	subscriber, err := s.hub.Subscribe()
	if err != nil {
		closeWithReason(conn, websocket.CloseGoingAway, "server shutting down")
		return
	}
	defer s.hub.Unsubscribe(subscriber)

	readTimeout := heartbeatFrameSlack * s.heartbeat
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readTimeout))
	})

	acks := make(chan struct{}, 1)
	stop := make(chan struct{})
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		s.channelWriter(conn, subscriber, acks, stop)
	}()
	defer func() {
		close(stop)
		_ = conn.Close()
		writer.Wait()
	}()

	for {
		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			return
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			closeWithReason(conn, websocket.CloseProtocolError, "expected a text frame")
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			closeWithReason(conn, websocket.CloseProtocolError, "invalid frame")
			return
		}
		switch envelope.Type {
		case "heartbeat":
			var heartbeat heartbeatMessage
			if err := decodeStrict(payload, &heartbeat); err != nil {
				closeWithReason(conn, websocket.CloseProtocolError, "invalid heartbeat")
				return
			}
			if heartbeat.InstanceID != identity.uuid {
				// A session identity cannot change on a live channel.
				closeWithReason(conn, websocket.CloseProtocolError, "instance_changed")
				return
			}
			if s.distributionUnavailable() {
				// The availability watcher also closes an established channel;
				// this covers a heartbeat that arrives between its ticks. The
				// downstream reconnects once distribution resumes.
				closeWithReason(conn, websocket.ClosePolicyViolation, "distribution_paused")
				return
			}
			select {
			case acks <- struct{}{}:
			default:
				// An ack is already queued and will carry the current heads.
			}
		case "command_result":
			// A relay never sends commands, so a result is ignored.
		case "hello":
			closeWithReason(conn, websocket.CloseProtocolError, "duplicate hello")
			return
		default:
			closeWithReason(conn, websocket.CloseProtocolError, "unknown frame")
			return
		}
	}
}

// channelWriter is the connection's only writer. It writes the initial ack
// before entering the loop: a downstream initializes its service state from
// that ack, so a notification that raced ahead of it would make the client
// reject an unknown head.
func (s *Server) channelWriter(conn *websocket.Conn, subscriber *Subscriber, acks <-chan struct{}, stop <-chan struct{}) {
	// Distribution can stop while the connection waits for its hello frame;
	// the initial ack must not go out as if it were still active.
	if err := s.writeChannelAck(conn); err != nil {
		if errors.Is(err, errDistributionStopped) {
			closeWithReason(conn, websocket.ClosePolicyViolation, "distribution_paused")
		}
		_ = conn.Close()
		return
	}
	ping := time.NewTicker(s.heartbeat)
	defer ping.Stop()
	for {
		select {
		case <-stop:
			return
		case <-acks:
			// The availability watcher closes the connection when the state
			// flips; the write itself is serialized with the flip, so an ack
			// never confirms distribution while the relay is paused or no
			// longer live.
			if err := s.writeChannelAck(conn); err != nil {
				if errors.Is(err, errDistributionStopped) {
					closeWithReason(conn, websocket.ClosePolicyViolation, "distribution_paused")
				}
				_ = conn.Close()
				return
			}
		case notification := <-subscriber.C():
			if s.distributionUnavailable() {
				closeWithReason(conn, websocket.ClosePolicyViolation, "distribution_paused")
				_ = conn.Close()
				return
			}
			if err := writeChannelJSON(conn, notifyMessage{
				Type:      "notify",
				ServiceID: notification.ServiceID,
				Head:      notification.Head,
			}); err != nil {
				_ = conn.Close()
				return
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(channelWriteTimeout)); err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

func writeChannelJSON(conn *websocket.Conn, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) > maxChannelFrameBytes {
		return fmt.Errorf("%w: channel frame exceeds %d bytes", ErrConfig, maxChannelFrameBytes)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(channelWriteTimeout))
	return conn.WriteMessage(websocket.TextMessage, payload)
}

// handlePoll serves one long-poll request. The request body is a heartbeat; the
// response is an ack. The request is held up to wait so a head advance reaches
// the downstream without a full round trip, and it is answered immediately
// when a notification is available.
func (s *Server) handlePoll(w http.ResponseWriter, r *http.Request) {
	certificate, ok := clientIdentity(r)
	if !ok {
		writeError(w, http.StatusForbidden, "unauthorized", "")
		return
	}
	query, ok := strictQuery(r.URL.RawQuery, "wait")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	waitText := ""
	if query.Has("wait") {
		waitText = query.Get("wait")
		if waitText == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "")
			return
		}
	}
	wait, ok := parsePollWait(waitText)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxChannelFrameBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	if len(body) > maxChannelFrameBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "")
		return
	}
	var heartbeat heartbeatMessage
	if err := decodeStrict(body, &heartbeat); err != nil || heartbeat.Type != "heartbeat" ||
		!adsync.ValidInstanceID(heartbeat.InstanceID) {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	identity := sessionIdentity{fingerprint: certificate.fingerprint, uuid: heartbeat.InstanceID}
	if !s.beginPollSession(identity) {
		writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	defer s.endPollSession(identity)

	subscriber, err := s.hub.Subscribe()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	defer s.hub.Unsubscribe(subscriber)

	if wait > 0 {
		// Captured before the hold: a flip to unavailable closes it, so a
		// paused or no-longer-live relay does not keep the poll held until
		// its timeout.
		unavailable := s.unavailableSignal()
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-subscriber.C():
		case <-timer.C:
		case <-unavailable:
		case <-s.done:
			// The server is closing; do not keep the request held.
			writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
			return
		case <-r.Context().Done():
			return
		}
	}
	// Distribution may have been paused while the request was held, and a poll
	// that was already held must not answer successfully after Close. The
	// availability read lock makes the stop check and the ack atomic against a
	// flip, so a poll that answers was current when it did.
	s.availabilityMu.RLock()
	closed := s.isClosed()
	stopped := s.distributionUnavailable()
	if !closed && !stopped {
		// The availability read lock is held across the write so a flip waits
		// for it; bound the write so a stalled client cannot hold the flip up.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(channelWriteTimeout))
		writeJSON(w, http.StatusOK, s.ack())
	}
	s.availabilityMu.RUnlock()
	if closed {
		writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	if stopped {
		writeError(w, http.StatusServiceUnavailable, "distribution_paused", "")
		return
	}
}

// parsePollWait parses the documented 0..30 second wait. The bound is checked
// before the duration conversion so a value near MaxInt cannot overflow into a
// small or negative duration and slip past the range check. The decimal text
// must be canonical so "01" and "+1" are not silently accepted as numbers.
func parsePollWait(text string) (time.Duration, bool) {
	if text == "" {
		return defaultPollWait, true
	}
	seconds, err := strconv.Atoi(text)
	if err != nil || seconds < 0 || seconds > int(maxPollWait/time.Second) || strconv.Itoa(seconds) != text {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

func validateHello(hello *helloMessage) error {
	if hello.Type != "hello" {
		return fmt.Errorf("%w: expected hello", adsync.ErrProtocol)
	}
	if !adsync.ValidInstanceID(hello.InstanceID) {
		return fmt.Errorf("%w: invalid instance id %q", adsync.ErrProtocol, hello.InstanceID)
	}
	seen := make(map[string]struct{}, len(hello.Services))
	for _, service := range hello.Services {
		if !validEntityID(service.ID) {
			return fmt.Errorf("%w: invalid service id %q", adsync.ErrProtocol, service.ID)
		}
		if _, duplicate := seen[service.ID]; duplicate {
			return fmt.Errorf("%w: duplicate service id %q", adsync.ErrProtocol, service.ID)
		}
		seen[service.ID] = struct{}{}
	}
	return nil
}

func closeWithReason(conn *websocket.Conn, code int, reason string) {
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason), time.Now().Add(channelWriteTimeout))
}

// decodeStrict decodes exactly one JSON value and rejects unknown fields, so a
// frame that does not match the contract is a protocol error rather than a
// silently ignored value.
func decodeStrict(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing data after JSON value")
	}
	return nil
}
