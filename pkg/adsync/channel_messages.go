// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"time"
)

// Channel wire types from the shared policy-source contract. Client frames are
// hello, heartbeat and command_result; server frames are ack and notify. A
// frame is capped at 64 KiB, carries no policy content and never carries the
// credential: the channel is authenticated by the Authorization header of the
// upgrade or poll request.
const (
	channelTypeHello         = "hello"
	channelTypeHeartbeat     = "heartbeat"
	channelTypeAck           = "ack"
	channelTypeNotify        = "notify"
	channelTypeCommandResult = "command_result"

	commandActionResync    = "resync"
	commandActionPause     = "pause"
	commandActionResume    = "resume"
	commandActionSetSource = "set_source"
	commandActionStop      = "stop"

	commandStatusAcked    = "acked"
	commandStatusRejected = "rejected"

	maxChannelFrameBytes = 64 << 10
	// maxChainEntries is the contract bound: one entry per proxy plus this
	// instance.
	maxChainEntries = maxChainDepth + 1
)

// channelDurationPattern is the contract's duration syntax. time.ParseDuration
// alone would also accept hours and signed values.
var channelDurationPattern = regexp.MustCompile(`^[1-9][0-9]*(ms|s|m)$`)

// channelHelloService is one entry of the hello service list.
type channelHelloService struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Revision uint64 `json:"revision"`
	State    string `json:"state"`
}

// channelHello is the first client frame after a WebSocket upgrade.
type channelHello struct {
	Type         string                `json:"type"`
	InstanceID   string                `json:"instanceId"`
	CredentialID string                `json:"credentialId,omitempty"`
	Services     []channelHelloService `json:"services"`
	Chain        []string              `json:"chain"`
	AgentVersion string                `json:"agentVersion"`
	StartedAt    int64                 `json:"startedAt,omitempty"`
	Transport    string                `json:"transport,omitempty"`
}

// channelHeartbeatService is one entry of the heartbeat service list.
type channelHeartbeatService struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Revision   uint64 `json:"revision"`
	State      string `json:"state"`
	LagSeconds uint64 `json:"lagSeconds"`
}

// channelFallback describes whether this instance routes any service to Cloud.
type channelFallback struct {
	Active      bool     `json:"active"`
	Services    []string `json:"services"`
	Since       *int64   `json:"since"`
	RoutedCalls uint64   `json:"routedCalls"`
}

// channelHeartbeat covers every service in one message. In long-poll mode it is
// the body of each poll request.
type channelHeartbeat struct {
	Type               string                    `json:"type"`
	Seq                uint64                    `json:"seq"`
	Services           []channelHeartbeatService `json:"services"`
	Liveness           string                    `json:"liveness"`
	Chain              []string                  `json:"chain"`
	Fallback           channelFallback           `json:"fallback"`
	AgentVersion       string                    `json:"agentVersion"`
	StartedAt          int64                     `json:"startedAt"`
	Transport          string                    `json:"transport"`
	UpstreamInstanceID *string                   `json:"upstreamInstanceId,omitempty"`
	DownstreamCount    *uint64                   `json:"downstreamCount,omitempty"`
}

// channelAckService is one service entry of an ack. Head is the source head;
// originHead is the head the source last confirmed upstream.
type channelAckService struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Head          uint64 `json:"head"`
	OriginHead    uint64 `json:"originHead"`
	Epoch         string `json:"epoch"`
	HeadStateHash Hash   `json:"headStateHash"`
}

// channelAck confirms a heartbeat and carries the authoritative service set.
type channelAck struct {
	Type              string              `json:"type"`
	ServicesVersion   int64               `json:"servicesVersion"`
	Services          []channelAckService `json:"services"`
	ServerTime        int64               `json:"serverTime"`
	HeartbeatInterval string              `json:"heartbeatInterval"`
	StaleAfter        string              `json:"staleAfter"`
	StopAfter         string              `json:"stopAfter"`
	Commands          []channelCommand    `json:"commands"`
}

// channelNotify announces that one service's head advanced.
type channelNotify struct {
	Type      string `json:"type"`
	ServiceID string `json:"serviceId"`
	Head      uint64 `json:"head"`
}

// channelCommandParams carries the set_source parameters. The command never
// carries a credential: credentialProfile names a local file.
type channelCommandParams struct {
	URL               string `json:"url"`
	CredentialProfile string `json:"credentialProfile"`
}

// channelCommand is one management command from the ack.
type channelCommand struct {
	ID        string                `json:"id"`
	Action    string                `json:"action"`
	ServiceID string                `json:"serviceId,omitempty"`
	Params    *channelCommandParams `json:"params,omitempty"`
	ExpiresAt int64                 `json:"expiresAt"`
}

// channelCommandResult reports a delivered command back to the source.
type channelCommandResult struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Status string `json:"status"`
	Result any    `json:"result,omitempty"`
	At     int64  `json:"at"`
}

func (s channelAckService) service() Service {
	return Service{
		ID:            s.ID,
		Name:          s.Name,
		Revision:      s.Head,
		Epoch:         s.Epoch,
		HeadStateHash: s.HeadStateHash,
	}
}

func (s channelAckService) validate() error {
	if err := s.service().validate(); err != nil {
		return err
	}
	if s.OriginHead > s.Head {
		return fmt.Errorf("%w: service %q originHead %d is ahead of head %d", ErrProtocol, s.ID, s.OriginHead, s.Head)
	}
	return nil
}

func (a *channelAck) validate() error {
	if a.Type != channelTypeAck {
		return fmt.Errorf("%w: unexpected channel message %q", ErrProtocol, a.Type)
	}
	if a.ServicesVersion <= 0 || a.ServicesVersion > maxSafeInteger {
		return fmt.Errorf("%w: invalid servicesVersion %d", ErrProtocol, a.ServicesVersion)
	}
	if len(a.Services) > maxServicesPerSource {
		return fmt.Errorf("%w: %d services exceed the protocol bound", ErrProtocol, len(a.Services))
	}
	seen := make(map[string]struct{}, len(a.Services))
	for index := range a.Services {
		if err := a.Services[index].validate(); err != nil {
			return err
		}
		if _, duplicate := seen[a.Services[index].ID]; duplicate {
			return fmt.Errorf("%w: duplicate service id %q", ErrProtocol, a.Services[index].ID)
		}
		seen[a.Services[index].ID] = struct{}{}
	}
	if a.ServerTime < 0 || a.ServerTime > maxSafeInteger {
		return fmt.Errorf("%w: invalid serverTime", ErrProtocol)
	}
	if _, err := parseChannelDuration(a.HeartbeatInterval); err != nil {
		return err
	}
	if _, err := parseChannelDuration(a.StaleAfter); err != nil {
		return err
	}
	if _, err := parseChannelDuration(a.StopAfter); err != nil {
		return err
	}
	for index := range a.Commands {
		if err := a.Commands[index].validate(); err != nil {
			return err
		}
	}
	return nil
}

func (n *channelNotify) validate() error {
	if n.Type != channelTypeNotify {
		return fmt.Errorf("%w: unexpected channel message %q", ErrProtocol, n.Type)
	}
	if !validEntityID(n.ServiceID) {
		return fmt.Errorf("%w: invalid service id %q", ErrProtocol, n.ServiceID)
	}
	if !validRevision(n.Head) {
		return fmt.Errorf("%w: invalid head %d", ErrProtocol, n.Head)
	}
	return nil
}

func (c *channelCommand) validate() error {
	if !validULID(c.ID) {
		return fmt.Errorf("%w: invalid command id %q", ErrProtocol, c.ID)
	}
	if c.ExpiresAt < 0 || c.ExpiresAt > maxSafeInteger {
		return fmt.Errorf("%w: command %q has an invalid expiry", ErrProtocol, c.ID)
	}
	switch c.Action {
	case commandActionResync, commandActionPause, commandActionResume, commandActionStop:
		if c.Params != nil {
			return fmt.Errorf("%w: command %q must not carry params", ErrProtocol, c.ID)
		}
		if c.ServiceID != "" && !validEntityID(c.ServiceID) {
			return fmt.Errorf("%w: command %q has an invalid service id", ErrProtocol, c.ID)
		}
	case commandActionSetSource:
		if c.ServiceID != "" {
			return fmt.Errorf("%w: set_source command %q must not name a service", ErrProtocol, c.ID)
		}
		if c.Params == nil {
			return fmt.Errorf("%w: set_source command %q has no params", ErrProtocol, c.ID)
		}
		target, err := url.Parse(c.Params.URL)
		if err != nil || target.Scheme != "https" || target.Host == "" || target.User != nil {
			return fmt.Errorf("%w: set_source command %q has an invalid url", ErrProtocol, c.ID)
		}
		if c.Params.CredentialProfile == "" || len(c.Params.CredentialProfile) > 128 {
			return fmt.Errorf("%w: set_source command %q has an invalid credential profile", ErrProtocol, c.ID)
		}
	default:
		return fmt.Errorf("%w: unknown command action %q", ErrProtocol, c.Action)
	}
	return nil
}

func (c channelCommand) command() Command {
	command := Command{
		ID:        c.ID,
		Action:    c.Action,
		ServiceID: c.ServiceID,
		ExpiresAt: time.UnixMilli(c.ExpiresAt).UTC(),
	}
	if c.Params != nil {
		command.SourceURL = c.Params.URL
		command.CredentialProfile = c.Params.CredentialProfile
	}
	return command
}

func parseChannelDuration(value string) (time.Duration, error) {
	if !channelDurationPattern.MatchString(value) {
		return 0, fmt.Errorf("%w: invalid duration %q", ErrProtocol, value)
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%w: invalid duration %q", ErrProtocol, value)
	}
	return duration, nil
}

func validULID(value string) bool {
	return ulidPattern.MatchString(value)
}

// decodeServerMessage decodes one ack or notify frame. The tag is read first,
// then the concrete message is decoded strictly so an unknown field is a
// protocol violation rather than a value silently ignored.
func decodeServerMessage(payload []byte) (any, error) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("%w: decode channel message: %v", ErrProtocol, err)
	}
	switch envelope.Type {
	case channelTypeAck:
		var ack channelAck
		if err := decodeStrict(payload, &ack); err != nil {
			return nil, err
		}
		if err := ack.validate(); err != nil {
			return nil, err
		}
		return &ack, nil
	case channelTypeNotify:
		var notify channelNotify
		if err := decodeStrict(payload, &notify); err != nil {
			return nil, err
		}
		if err := notify.validate(); err != nil {
			return nil, err
		}
		return &notify, nil
	default:
		return nil, fmt.Errorf("%w: unknown channel message %q", ErrProtocol, envelope.Type)
	}
}
