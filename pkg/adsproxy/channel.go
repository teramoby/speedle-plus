// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

const (
	// maxChannelFrameBytes is the application-level bound on one channel frame.
	maxChannelFrameBytes = 64 << 10
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
	Type         string         `json:"type"`
	InstanceID   string         `json:"instanceId"`
	CredentialID string         `json:"credentialId,omitempty"`
	Services     []helloService `json:"services"`
	Chain        []string       `json:"chain"`
	AgentVersion string         `json:"agentVersion"`
	StartedAt    int64          `json:"startedAt,omitempty"`
	Transport    string         `json:"transport,omitempty"`
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
	Type               string             `json:"type"`
	Seq                uint64             `json:"seq"`
	Services           []heartbeatService `json:"services"`
	Liveness           string             `json:"liveness"`
	Chain              []string           `json:"chain"`
	Fallback           fallbackMessage    `json:"fallback"`
	AgentVersion       string             `json:"agentVersion"`
	StartedAt          int64              `json:"startedAt"`
	Transport          string             `json:"transport"`
	UpstreamInstanceID *string            `json:"upstreamInstanceId,omitempty"`
	DownstreamCount    *uint64            `json:"downstreamCount,omitempty"`
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

// commandMessage is part of the ack wire format. A proxy never sends commands.
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

func (s *Server) ack(credential *Credential) ackMessage {
	// The service list and the version it belongs to must come from one read,
	// or a downstream that records the version could skip a refresh that was
	// already reflected in a different list.
	services, version := s.mirror.ServicesVersion()
	entries := make([]ackService, 0, len(services))
	for _, service := range services {
		if !credential.Allows(service.ID) {
			continue
		}
		entries = append(entries, ackService{
			ID:            service.ID,
			Name:          service.Name,
			Head:          service.Revision,
			OriginHead:    service.Revision,
			Epoch:         service.Epoch,
			HeadStateHash: service.HeadStateHash,
		})
		if len(entries) == MaxServices {
			break
		}
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

// handleChannel serves one instance channel over WebSocket. The credential and
// the fan-out limit are checked before the upgrade so the downstream sees an
// HTTP error there; chain problems can only be seen after the hello frame and
// close the connection with a policy-violation code.
func (s *Server) handleChannel(w http.ResponseWriter, r *http.Request, credential *Credential) {
	if err := s.hub.Allow(credential.DownstreamID); err != nil {
		writeHubError(w, err)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// The upgrader already wrote the HTTP error.
		return
	}
	if !s.track(conn, credential) {
		closeWithReason(conn, websocket.CloseGoingAway, "server shutting down")
		_ = conn.Close()
		return
	}
	defer s.untrack(conn)
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
	if err := s.validateHello(credential, &hello); err != nil {
		closeWithReason(conn, closeCode(err), closeReason(err))
		return
	}

	subscriber, err := s.hub.Subscribe(credential.DownstreamID, credential.Allows)
	if err != nil {
		closeWithReason(conn, websocket.CloseTryAgainLater, "fanout_limit")
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
		s.channelWriter(conn, credential, subscriber, acks, stop)
	}()
	defer func() {
		close(stop)
		_ = conn.Close()
		writer.Wait()
	}()

	// The first ack confirms the service set without waiting for a heartbeat.
	select {
	case acks <- struct{}{}:
	default:
	}

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
			if s.distributionPaused() {
				// An existing channel ends at the next message after a pause;
				// the downstream reconnects once distribution resumes.
				closeWithReason(conn, websocket.ClosePolicyViolation, "distribution_paused")
				return
			}
			if err := s.validateChain(heartbeat.Chain, ""); err != nil {
				closeWithReason(conn, closeCode(err), closeReason(err))
				return
			}
			select {
			case acks <- struct{}{}:
			default:
				// An ack is already queued and will carry the current heads.
			}
		case "command_result":
			// A proxy never sends commands, so a result is ignored.
		case "hello":
			closeWithReason(conn, websocket.CloseProtocolError, "duplicate hello")
			return
		default:
			closeWithReason(conn, websocket.CloseProtocolError, "unknown frame")
			return
		}
	}
}

// channelWriter is the connection's only writer.
func (s *Server) channelWriter(conn *websocket.Conn, credential *Credential, subscriber *Subscriber, acks <-chan struct{}, stop <-chan struct{}) {
	ping := time.NewTicker(s.heartbeat)
	defer ping.Stop()
	for {
		select {
		case <-stop:
			return
		case <-acks:
			if err := writeChannelJSON(conn, s.ack(credential)); err != nil {
				_ = conn.Close()
				return
			}
		case notification := <-subscriber.C():
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
func (s *Server) handlePoll(w http.ResponseWriter, r *http.Request, credential *Credential) {
	if err := s.hub.Allow(credential.DownstreamID); err != nil {
		writeHubError(w, err)
		return
	}
	wait, ok := parsePollWait(r.URL.Query().Get("wait"))
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
	if err := decodeStrict(body, &heartbeat); err != nil || heartbeat.Type != "heartbeat" {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	if err := s.validateChain(heartbeat.Chain, ""); err != nil {
		writeChainError(w, err)
		return
	}

	subscriber, err := s.hub.Subscribe(credential.DownstreamID, credential.Allows)
	if err != nil {
		writeHubError(w, err)
		return
	}
	defer s.hub.Unsubscribe(subscriber)

	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-subscriber.C():
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
	}
	// A reload may have replaced the grant while the request was held. The ack
	// would otherwise reveal the new service set to a revoked or narrowed one.
	if !s.credentials.Current(credential) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	writeJSON(w, http.StatusOK, s.ack(credential))
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

func (s *Server) validateHello(credential *Credential, hello *helloMessage) error {
	if hello.Type != "hello" {
		return fmt.Errorf("%w: expected hello", adsync.ErrProtocol)
	}
	if !validEntityID(hello.InstanceID) {
		return fmt.Errorf("%w: invalid instance id %q", adsync.ErrProtocol, hello.InstanceID)
	}
	if hello.InstanceID == s.instanceID {
		// A downstream claiming to be this instance is a loop or an
		// impersonation; either way it must not be served.
		return fmt.Errorf("%w: downstream claims this proxy's instance id", ErrChainLoop)
	}
	if err := s.validateChain(hello.Chain, hello.InstanceID); err != nil {
		return err
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
		if !credential.Allows(service.ID) {
			return fmt.Errorf("%w: downstream subscribes to a service outside its subset", ErrConfig)
		}
	}
	return nil
}

// validateChain enforces the cooperative depth and loop rules. The chain lists
// the instances from Cloud to the sender, inclusive of the sender when it is
// known. A repeated instance, this proxy placed anywhere except as the sender's
// immediate upstream, or one of this proxy's own ancestors claiming to be a
// downstream is a loop.
func (s *Server) validateChain(chain []string, senderID string) error {
	if len(chain) > s.maxDepth+1 {
		return fmt.Errorf("%w: %d entries", ErrChainTooDeep, len(chain))
	}
	seen := make(map[string]struct{}, len(chain))
	for _, instanceID := range chain {
		if !validEntityID(instanceID) {
			return fmt.Errorf("%w: invalid chain entry %q", ErrChainLoop, instanceID)
		}
		if _, duplicate := seen[instanceID]; duplicate {
			return fmt.Errorf("%w: repeated instance %q", ErrChainLoop, instanceID)
		}
		seen[instanceID] = struct{}{}
	}

	position := -1
	for index, instanceID := range chain {
		if instanceID == s.instanceID {
			position = index
			break
		}
	}
	if position == len(chain)-2 && senderID == "" {
		// A heartbeat carries no separate instance id, but the contract's
		// inclusive chain ends with the sender, so the last entry names it.
		senderID = chain[len(chain)-1]
	}
	if senderID != "" {
		// The sender cannot be one of the instances this proxy itself
		// synchronizes from: that would be a cycle.
		for _, ancestor := range s.chain[:len(s.chain)-1] {
			if ancestor == senderID {
				return fmt.Errorf("%w: downstream %q is one of this proxy's ancestors", ErrChainLoop, senderID)
			}
		}
	}

	if position < 0 {
		// The sender's chain does not mention this proxy. Without a claimed
		// position there is nothing to check, and an empty chain is legitimate
		// for a locally keyed proxy.
		return nil
	}
	switch {
	case position == len(chain)-1:
		// The sender listed its upstream, excluding itself.
		return nil
	case position == len(chain)-2:
		if senderID == "" || chain[len(chain)-1] == senderID {
			return nil
		}
		return fmt.Errorf("%w: chain does not end at the sender", ErrChainLoop)
	default:
		return fmt.Errorf("%w: this proxy is not the sender's immediate upstream", ErrChainLoop)
	}
}

func closeCode(err error) int {
	switch {
	case errors.Is(err, ErrChainTooDeep), errors.Is(err, ErrChainLoop), errors.Is(err, ErrConfig):
		return websocket.ClosePolicyViolation
	default:
		return websocket.CloseProtocolError
	}
}

func closeReason(err error) string {
	switch {
	case errors.Is(err, ErrChainTooDeep):
		return "chain_too_deep"
	case errors.Is(err, ErrChainLoop):
		return "chain_loop"
	case errors.Is(err, ErrConfig):
		return "forbidden"
	default:
		return "protocol_error"
	}
}

func closeWithReason(conn *websocket.Conn, code int, reason string) {
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason), time.Now().Add(channelWriteTimeout))
}

func writeHubError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrFanoutLimit) {
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusServiceUnavailable, "fanout_limit", "")
		return
	}
	writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
}

func writeChainError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrChainTooDeep) {
		writeError(w, http.StatusForbidden, "chain_too_deep", "")
		return
	}
	writeError(w, http.StatusForbidden, "chain_loop", "")
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

// WatchReload reloads the downstream credential file for every signal received.
// The process wires signal.Notify for SIGHUP (and any other reload signal) and
// runs this in a goroutine; a failed reload is logged and the previous
// credentials stay in force.
func (s *Server) WatchReload(ctx context.Context, signals <-chan os.Signal) {
	if signals == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-signals:
			if !ok {
				return
			}
			if err := s.ReloadCredentials(); err != nil {
				log.Warnf("adsproxy: reload downstream credentials: %v", err)
				continue
			}
			log.Info("adsproxy: downstream credentials reloaded")
		}
	}
}
