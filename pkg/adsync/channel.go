// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// maxChainDepth is the number of proxies allowed between Cloud and a leaf.
	maxChainDepth = 3

	defaultPollWait      = 25 * time.Second
	maxPollWait          = 30 * time.Second
	defaultAckTimeout    = 10 * time.Second
	defaultProbeInterval = 5 * time.Minute
	defaultPingInterval  = 30 * time.Second
)

// Transport selects how the instance channel is carried. The channel always
// uses HTTPS on the configured port; only the framing differs.
type Transport string

const (
	// TransportAuto tries WebSocket first and falls back to long polling when
	// the upgrade fails, the first ack does not arrive in time or two
	// consecutive connections drop abnormally. It retries WebSocket after the
	// probe interval.
	TransportAuto Transport = "auto"
	// TransportWebSocket uses only the WebSocket channel and reconnects with
	// backoff when it drops.
	TransportWebSocket Transport = "websocket"
	// TransportLongPoll uses only HTTPS long polling.
	TransportLongPoll Transport = "longpoll"
)

// ServiceSetChange reports how a confirming ack changed the authoritative
// service set. A service that was removed must have its local copy deleted; a
// service that was added starts syncing.
type ServiceSetChange struct {
	// Added lists services that appeared in the ack and were not known before,
	// sorted by id.
	Added []Service
	// Removed lists services that were known and are absent from the ack,
	// sorted by id.
	Removed []Service
}

// FallbackState is the instance's fallback accounting, reported in every
// heartbeat.
type FallbackState struct {
	// Active reports whether this instance currently forwards any service to
	// Cloud.
	Active bool
	// Services lists the service ids currently routed to Cloud.
	Services []string
	// Since is when the current fallback episode started. The zero value is
	// reported as null.
	Since time.Time
	// RoutedCalls counts the fallback calls of the current episode.
	RoutedCalls uint64
}

// Notify announces that one service's head advanced. The notification only
// accelerates synchronization; a lost one is harmless because every ack
// repeats the heads.
type Notify struct {
	ServiceID string
	Head      uint64
}

// Command is a management command delivered on the instance channel. The
// transport only delivers commands; applying them is the caller's job.
type Command struct {
	ID string
	// Action is resync, pause, resume, set_source or stop.
	Action string
	// ServiceID scopes the command to one service; empty means the instance.
	ServiceID string
	// ExpiresAt is when the source stops caring about the command. The
	// transport does not deliver a command past its expiry.
	ExpiresAt time.Time
	// SourceURL and CredentialProfile are set for set_source only. The command
	// never carries a credential; CredentialProfile names a local file.
	SourceURL         string
	CredentialProfile string
}

// CommandResult is the outcome of one command. Status is acked or rejected; an
// empty status is reported as acked. Result is a small JSON value and must
// never contain secrets or policy content.
type CommandResult struct {
	Status string
	Result any
}

// ChannelOptions configures a Channel.
type ChannelOptions struct {
	// InstanceID is this instance's ULID. It is required.
	InstanceID string
	// AgentVersion is reported in hello and heartbeats. It is required.
	AgentVersion string
	// Chain lists the instance ids from Cloud to this instance, inclusive.
	// The last entry should be InstanceID.
	Chain []string
	// CredentialID is the public credential id from registration, when known.
	CredentialID string
	// Machine receives confirming acks and head notifications. It is required.
	Machine *Machine

	// Transport selects the channel transport. The default is auto.
	Transport Transport
	// HeartbeatInterval overrides the cadence derived from the machine's
	// effective thresholds. Zero follows the thresholds.
	HeartbeatInterval time.Duration
	// PollWait is the long-poll hold, from 0 to 30s. Zero means 25s, which is
	// also the protocol default.
	PollWait time.Duration
	// AckTimeout bounds how long the first ack may take after an upgrade.
	// Zero means 10s.
	AckTimeout time.Duration
	// ProbeInterval is how long auto mode waits before retrying WebSocket
	// after a fallback. Zero means 5 minutes.
	ProbeInterval time.Duration
	// Backoff configures reconnect delays.
	Backoff Backoff

	// Fallback reports the fallback state for heartbeats. Nil reports inactive.
	Fallback func() FallbackState
	// UpstreamInstanceID and DownstreamCount are optional heartbeat fields.
	UpstreamInstanceID string
	DownstreamCount    *uint64

	// OnNotify is called after a head notification was applied to the machine,
	// and also when a confirming ack shows a service's head advanced or its
	// epoch changed. A notification frame is only an accelerator; the ack is
	// what makes a lost notification harmless. It should start a sync and
	// return quickly; the channel calls it on its read loop.
	OnNotify func(Notify)
	// OnServiceSetChange is called when an ack changes the authoritative
	// service set. It should return quickly; the channel calls it on its read
	// loop.
	OnServiceSetChange func(ServiceSetChange)
	// OnCommand is called for every delivered command. A source may repeat a
	// command in several acks, so the callback must ignore an id it has already
	// executed. A nil result means no result is sent. In long-poll mode the
	// result cannot be reported: the poll request body is a heartbeat only.
	OnCommand func(Command) *CommandResult
}

// Backoff configures the reconnect delay: the initial delay doubles per
// attempt up to the maximum, then ±Jitter is applied. A successful connection
// resets the sequence.
type Backoff struct {
	// Initial is the delay before the second attempt. Zero means 1s.
	Initial time.Duration
	// Max caps the delay. Zero means 60s.
	Max time.Duration
	// Jitter is the fraction added or removed, between 0 and 1. Zero means
	// 0.3.
	Jitter float64
	// Rand returns a value in [0,1) for jitter. Tests may replace it.
	Rand func() float64
}

func (b Backoff) normalized() (Backoff, error) {
	if b.Initial == 0 {
		b.Initial = defaultInitialBackoff
	}
	if b.Initial < 0 {
		return b, fmt.Errorf("adsync: Backoff.Initial must not be negative")
	}
	if b.Max == 0 {
		b.Max = defaultMaxBackoff
	}
	if b.Max < b.Initial {
		return b, fmt.Errorf("adsync: Backoff.Max must not be smaller than Backoff.Initial")
	}
	if b.Jitter == 0 {
		b.Jitter = defaultJitter
	}
	if b.Jitter < 0 || b.Jitter > 1 {
		return b, fmt.Errorf("adsync: Backoff.Jitter must be between 0 and 1")
	}
	if b.Rand == nil {
		b.Rand = rand.Float64
	}
	return b, nil
}

func (b Backoff) delay(attempt int) time.Duration {
	return exponentialDelay(b.Initial, b.Max, b.Jitter, b.Rand, attempt)
}

// Channel carries the instance-level notification channel: hello, heartbeats,
// acks, head notifications and commands for every service of one instance. It
// drives a Machine and never carries policy content. Start it with Run; cancel
// the context to stop it.
//
// A Channel is safe to query while it runs.
type Channel struct {
	client  *Client
	machine *Machine

	instanceID   string
	credentialID string
	agentVersion string
	chain        []string
	mode         Transport

	heartbeatOverride time.Duration
	pollWait          time.Duration
	ackTimeout        time.Duration
	probeInterval     time.Duration
	backoff           Backoff
	dialer            *websocket.Dialer

	upstreamInstanceID string
	downstreamCount    *uint64
	fallback           func() FallbackState
	onNotify           func(Notify)
	onServiceSetChange func(ServiceSetChange)
	onCommand          func(Command) *CommandResult

	startedAt time.Time

	sequence atomic.Uint64
	running  atomic.Bool

	mu       sync.Mutex
	current  Transport
	services []Service
}

// NewChannel validates the options and returns a Channel for the policy source
// of client. The client's base URL, credential, TLS settings and proxy
// configuration are reused, so the channel and the data client always talk to
// the same source with the same transport trust.
func NewChannel(client *Client, options ChannelOptions) (*Channel, error) {
	if client == nil {
		return nil, fmt.Errorf("adsync: a client is required")
	}
	if options.Machine == nil {
		return nil, fmt.Errorf("adsync: ChannelOptions.Machine is required")
	}
	if !validULID(options.InstanceID) {
		return nil, fmt.Errorf("%w: invalid instance id %q", ErrProtocol, options.InstanceID)
	}
	if options.CredentialID != "" && !validULID(options.CredentialID) {
		return nil, fmt.Errorf("%w: invalid credential id %q", ErrProtocol, options.CredentialID)
	}
	if options.AgentVersion == "" || len(options.AgentVersion) > 64 {
		return nil, fmt.Errorf("adsync: AgentVersion must be 1 to 64 characters")
	}
	if len(options.Chain) > maxChainEntries {
		return nil, fmt.Errorf("adsync: chain has %d entries, the protocol allows %d", len(options.Chain), maxChainEntries)
	}
	for _, instanceID := range options.Chain {
		if !validULID(instanceID) {
			return nil, fmt.Errorf("%w: invalid chain entry %q", ErrProtocol, instanceID)
		}
	}
	if options.UpstreamInstanceID != "" && !validULID(options.UpstreamInstanceID) {
		return nil, fmt.Errorf("%w: invalid upstream instance id %q", ErrProtocol, options.UpstreamInstanceID)
	}

	mode := options.Transport
	if mode == "" {
		mode = TransportAuto
	}
	switch mode {
	case TransportAuto, TransportWebSocket, TransportLongPoll:
	default:
		return nil, fmt.Errorf("adsync: unknown transport %q", options.Transport)
	}

	heartbeatOverride := options.HeartbeatInterval
	if heartbeatOverride < 0 {
		return nil, fmt.Errorf("adsync: HeartbeatInterval must not be negative")
	}
	pollWait := options.PollWait
	if pollWait == 0 {
		pollWait = defaultPollWait
	}
	if pollWait < 0 || pollWait > maxPollWait {
		return nil, fmt.Errorf("adsync: PollWait must be between 0 and %s", maxPollWait)
	}
	ackTimeout := options.AckTimeout
	if ackTimeout == 0 {
		ackTimeout = defaultAckTimeout
	}
	if ackTimeout < 0 {
		return nil, fmt.Errorf("adsync: AckTimeout must not be negative")
	}
	probeInterval := options.ProbeInterval
	if probeInterval == 0 {
		probeInterval = defaultProbeInterval
	}
	if probeInterval < 0 {
		return nil, fmt.Errorf("adsync: ProbeInterval must not be negative")
	}
	backoff, err := options.Backoff.normalized()
	if err != nil {
		return nil, err
	}

	chain := append([]string(nil), options.Chain...)
	current := TransportWebSocket
	if mode == TransportLongPoll || client.dialer == nil {
		current = TransportLongPoll
	}
	channel := &Channel{
		client:             client,
		machine:            options.Machine,
		instanceID:         options.InstanceID,
		credentialID:       options.CredentialID,
		agentVersion:       options.AgentVersion,
		chain:              chain,
		mode:               mode,
		heartbeatOverride:  heartbeatOverride,
		pollWait:           pollWait,
		ackTimeout:         ackTimeout,
		probeInterval:      probeInterval,
		backoff:            backoff,
		dialer:             client.dialer,
		upstreamInstanceID: options.UpstreamInstanceID,
		downstreamCount:    options.DownstreamCount,
		fallback:           options.Fallback,
		onNotify:           options.OnNotify,
		onServiceSetChange: options.OnServiceSetChange,
		onCommand:          options.OnCommand,
		startedAt:          time.Now(),
		current:            current,
	}
	return channel, nil
}

// CurrentTransport reports the transport in use, or the one being attempted.
// It is never auto.
func (c *Channel) CurrentTransport() Transport {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

// Services returns the authoritative service set from the most recent ack. It
// is empty before the first ack.
func (c *Channel) Services() []Service {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Service(nil), c.services...)
}

// Run carries the channel until the context is canceled or the source reports a
// terminal condition such as a revoked credential. It reconnects with jittered
// backoff in between.
func (c *Channel) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("adsync: nil context")
	}
	if !c.running.CompareAndSwap(false, true) {
		return fmt.Errorf("adsync: channel is already running")
	}
	defer c.running.Store(false)

	switch c.mode {
	case TransportWebSocket:
		if c.dialer == nil {
			return fmt.Errorf("adsync: the WebSocket transport is not configured")
		}
		return c.runWebSocketLoop(ctx)
	case TransportLongPoll:
		return c.runLongPollLoop(ctx, time.Time{})
	default:
		return c.runAuto(ctx)
	}
}

func (c *Channel) runAuto(ctx context.Context) error {
	if c.dialer == nil {
		// Without a WebSocket dialer the only usable transport is long
		// polling; the caller supplied a custom HTTP client without one.
		return c.runLongPollLoop(ctx, time.Time{})
	}
	transport := TransportWebSocket
	attempt := 0
	failures := 0
	var probeAt time.Time
	fallBack := func() {
		transport = TransportLongPoll
		probeAt = time.Now().Add(c.probeInterval)
		attempt = 0
		failures = 0
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch transport {
		case TransportWebSocket:
			outcome := c.runWebSocket(ctx)
			if outcome.terminal {
				return outcome.err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if outcome.healthy {
				attempt = 0
				failures = 0
			}
			if outcome.upgradeFailed || outcome.noAck {
				fallBack()
				continue
			}
			// Two consecutive connections that either never became healthy or
			// dropped soon after the ack mean WebSocket is not usable here.
			if !outcome.healthy {
				failures++
				if failures >= 2 {
					fallBack()
					continue
				}
			}
			attempt++
			if err := c.sleepBeforeRetry(ctx, attempt, outcome.err); err != nil {
				return err
			}
			continue
		case TransportLongPoll:
			probe, healthy, err := c.runLongPoll(ctx, probeAt)
			if err != nil {
				if isTerminalError(err) {
					return err
				}
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				if healthy {
					attempt = 0
				}
				attempt++
				if err := c.sleepBeforeRetry(ctx, attempt, err); err != nil {
					return err
				}
			} else if probe {
				transport = TransportWebSocket
				probeAt = time.Time{}
				attempt = 0
			}
		}
	}
}

func (c *Channel) runWebSocketLoop(ctx context.Context) error {
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		outcome := c.runWebSocket(ctx)
		if outcome.terminal {
			return outcome.err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if outcome.healthy {
			attempt = 0
		}
		attempt++
		if err := c.sleepBeforeRetry(ctx, attempt, outcome.err); err != nil {
			return err
		}
	}
}

// sleepBeforeRetry waits with jittered exponential backoff. A source that asked
// for a longer Retry-After is honoured up to the maximum delay.
func (c *Channel) sleepBeforeRetry(ctx context.Context, attempt int, cause error) error {
	delay := c.backoff.delay(attempt)
	if retryAfter := retryAfterOf(cause); retryAfter > delay {
		if retryAfter > c.backoff.Max {
			retryAfter = c.backoff.Max
		}
		delay = retryAfter
	}
	return sleepContext(ctx, delay)
}

func (c *Channel) runLongPollLoop(ctx context.Context, probeAt time.Time) error {
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		probe, healthy, err := c.runLongPoll(ctx, probeAt)
		if err != nil {
			if isTerminalError(err) {
				return err
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if healthy {
				attempt = 0
			}
			attempt++
			if err := c.sleepBeforeRetry(ctx, attempt, err); err != nil {
				return err
			}
		} else if probe {
			// A caller-supplied probe deadline only exists in auto mode; a
			// fixed long-poll channel never returns here.
			return nil
		}
	}
}

// applyAck feeds one confirming ack to the machine, reports a service-set
// change and delivers commands.
func (c *Channel) applyAck(ack *channelAck, results chan<- channelCommandResult) error {
	heartbeat, err := parseChannelDuration(ack.HeartbeatInterval)
	if err != nil {
		return err
	}
	staleAfter, err := parseChannelDuration(ack.StaleAfter)
	if err != nil {
		return err
	}
	stopAfter, err := parseChannelDuration(ack.StopAfter)
	if err != nil {
		return err
	}
	services := make([]Service, len(ack.Services))
	for index := range ack.Services {
		services[index] = ack.Services[index].service()
	}
	extraKnown := c.machineOnlyServices()
	if err := c.machine.Ack(Ack{
		Services:          services,
		HeartbeatInterval: heartbeat,
		StaleAfter:        staleAfter,
		StopAfter:         stopAfter,
	}); err != nil {
		return err
	}
	change, advanced := c.storeServiceSet(extraKnown, services)
	if change != nil && c.onServiceSetChange != nil {
		c.onServiceSetChange(*change)
	}
	if c.onNotify != nil {
		for _, service := range advanced {
			c.onNotify(Notify{ServiceID: service.ID, Head: service.Revision})
		}
	}
	c.deliverCommands(ack.Commands, results)
	return nil
}

// machineOnlyServices returns the non-removed services the machine tracks but
// the previous ack set does not. After a restart a cached service is recorded
// with Applied before the first ack; if the first ack omits it, the source has
// removed it and the change must be reported so the local copy is deleted.
func (c *Channel) machineOnlyServices() []Service {
	c.mu.Lock()
	known := make(map[string]struct{}, len(c.services))
	for _, service := range c.services {
		known[service.ID] = struct{}{}
	}
	c.mu.Unlock()
	var extra []Service
	for _, status := range c.machine.Services() {
		if status.State == SyncStateRemoved {
			continue
		}
		if _, found := known[status.ID]; found {
			continue
		}
		extra = append(extra, Service{ID: status.ID, Name: status.Name, Revision: status.Head})
	}
	return extra
}

// storeServiceSet records the authoritative set, reports how it changed and
// which services advanced since the previous ack. extraKnown lists services the
// machine tracked before the ack outside the previous ack set; those absent
// from the ack count as removals.
func (c *Channel) storeServiceSet(extraKnown, services []Service) (*ServiceSetChange, []Service) {
	c.mu.Lock()
	known := make(map[string]Service, len(c.services))
	for _, service := range c.services {
		known[service.ID] = service
	}
	current := make(map[string]Service, len(services))
	var added, removed, advanced []Service
	for _, service := range services {
		current[service.ID] = service
		previous, found := known[service.ID]
		if !found {
			added = append(added, service)
			continue
		}
		if service.Revision > previous.Revision || service.Epoch != previous.Epoch {
			advanced = append(advanced, service)
		}
	}
	for id, service := range known {
		if _, found := current[id]; !found {
			removed = append(removed, service)
		}
	}
	for _, service := range extraKnown {
		if _, found := current[service.ID]; !found {
			removed = append(removed, service)
		}
	}
	c.services = append([]Service(nil), services...)
	c.mu.Unlock()

	sort.Slice(added, func(left, right int) bool { return added[left].ID < added[right].ID })
	sort.Slice(removed, func(left, right int) bool { return removed[left].ID < removed[right].ID })
	sort.Slice(advanced, func(left, right int) bool { return advanced[left].ID < advanced[right].ID })
	if len(added) == 0 && len(removed) == 0 {
		return nil, advanced
	}
	return &ServiceSetChange{Added: added, Removed: removed}, advanced
}

func (c *Channel) deliverCommands(commands []channelCommand, results chan<- channelCommandResult) {
	if c.onCommand == nil || len(commands) == 0 {
		return
	}
	now := time.Now()
	for _, wire := range commands {
		if !wire.command().ExpiresAt.After(now) {
			continue
		}
		command := wire.command()
		result := c.onCommand(command)
		if result == nil || results == nil {
			continue
		}
		status := result.Status
		if status == "" {
			status = commandStatusAcked
		}
		if status != commandStatusAcked && status != commandStatusRejected {
			status = commandStatusRejected
			result = &CommandResult{Status: status, Result: "invalid result status"}
		}
		frame := channelCommandResult{
			Type:   channelTypeCommandResult,
			ID:     command.ID,
			Status: status,
			Result: result.Result,
			At:     time.Now().UnixMilli(),
		}
		select {
		case results <- frame:
		default:
			// The writer is busy or gone; a command result is best effort.
		}
	}
}

// heartbeatInterval is the current heartbeat cadence. A confirming ack can
// tighten the machine's thresholds, so the value is read on every use.
func (c *Channel) heartbeatInterval() time.Duration {
	if c.heartbeatOverride > 0 {
		return c.heartbeatOverride
	}
	return c.machine.Thresholds().HeartbeatInterval
}

func (c *Channel) pingInterval() time.Duration {
	if heartbeat := c.heartbeatInterval(); heartbeat < defaultPingInterval {
		return heartbeat
	}
	return defaultPingInterval
}

// readTimeout is how long the connection may stay silent before it is treated
// as half open. An ack is due for every heartbeat, and a protocol pong is due
// for every ping, so two heartbeat periods is generous.
func (c *Channel) readTimeout() time.Duration {
	return 2 * c.heartbeatInterval()
}

func (c *Channel) writeTimeout() time.Duration {
	return c.client.requestTimeout
}

// localServices returns the services to announce, excluding removed ones and
// bounding the list by the protocol limit.
func (c *Channel) localServices() []ServiceStatus {
	all := c.machine.Services()
	services := make([]ServiceStatus, 0, len(all))
	for _, service := range all {
		if service.State == SyncStateRemoved {
			continue
		}
		services = append(services, service)
	}
	if len(services) > maxServicesPerSource {
		services = services[:maxServicesPerSource]
	}
	return services
}

func (c *Channel) helloMessage(transport Transport) channelHello {
	statuses := c.localServices()
	services := make([]channelHelloService, 0, len(statuses))
	for _, status := range statuses {
		services = append(services, channelHelloService{
			ID:       status.ID,
			Name:     status.Name,
			Revision: status.Revision,
			State:    string(status.State),
		})
	}
	return channelHello{
		Type:         channelTypeHello,
		InstanceID:   c.instanceID,
		CredentialID: c.credentialID,
		Services:     services,
		Chain:        append([]string(nil), c.chain...),
		AgentVersion: c.agentVersion,
		StartedAt:    c.startedAt.UnixMilli(),
		Transport:    string(transport),
	}
}

func (c *Channel) heartbeatMessage(transport Transport) channelHeartbeat {
	statuses := c.localServices()
	services := make([]channelHeartbeatService, 0, len(statuses))
	for _, status := range statuses {
		services = append(services, channelHeartbeatService{
			ID:       status.ID,
			Name:     status.Name,
			Revision: status.Revision,
			State:    string(status.State),
			// The transport does not track wall-clock lag; a service is either
			// at the confirmed head or catching up on revisions.
			LagSeconds: 0,
		})
	}
	fallback := channelFallback{Services: []string{}}
	if c.fallback != nil {
		state := c.fallback()
		fallback.Active = state.Active
		fallback.RoutedCalls = state.RoutedCalls
		for _, serviceID := range state.Services {
			if validEntityID(serviceID) && len(fallback.Services) < maxServicesPerSource {
				fallback.Services = append(fallback.Services, serviceID)
			}
		}
		if !state.Since.IsZero() {
			since := state.Since.UnixMilli()
			fallback.Since = &since
		}
	}
	message := channelHeartbeat{
		Type:         channelTypeHeartbeat,
		Seq:          c.sequence.Add(1),
		Services:     services,
		Liveness:     string(c.machine.Instance().Liveness),
		Chain:        append([]string(nil), c.chain...),
		Fallback:     fallback,
		AgentVersion: c.agentVersion,
		StartedAt:    c.startedAt.UnixMilli(),
		Transport:    string(transport),
	}
	if c.upstreamInstanceID != "" {
		upstream := c.upstreamInstanceID
		message.UpstreamInstanceID = &upstream
	}
	if c.downstreamCount != nil {
		count := *c.downstreamCount
		message.DownstreamCount = &count
	}
	return message
}

func (c *Channel) channelURL() string {
	target := *c.client.base
	target.Scheme = "wss"
	if c.client.base.Scheme == "http" {
		target.Scheme = "ws"
	}
	target.Path = c.client.base.Path + APIPrefix + "/channel"
	target.RawQuery = ""
	target.Fragment = ""
	return target.String()
}

func (c *Channel) pollURL() string {
	target := *c.client.base
	target.Path = c.client.base.Path + APIPrefix + "/channel/poll"
	seconds := int(c.pollWait / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	if seconds > int(maxPollWait/time.Second) {
		seconds = int(maxPollWait / time.Second)
	}
	query := url.Values{}
	query.Set("wait", strconv.Itoa(seconds))
	target.RawQuery = query.Encode()
	target.Fragment = ""
	return target.String()
}

func (c *Channel) requestHeaders() http.Header {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.client.credential)
	header.Set("Accept", "application/json")
	header.Set("User-Agent", c.client.userAgent)
	return header
}

func (c *Channel) setCurrent(transport Transport) {
	c.mu.Lock()
	c.current = transport
	c.mu.Unlock()
}

// terminalError marks a failure that must not be retried.
type terminalError struct {
	err error
}

func (e *terminalError) Error() string { return e.err.Error() }

func (e *terminalError) Unwrap() error { return e.err }

func isTerminalError(err error) bool {
	var terminal *terminalError
	return errors.As(err, &terminal)
}

// terminalStatusError stops the machine and returns a terminal error for a
// status that retrying cannot fix. It returns nil for a retryable status.
func (c *Channel) terminalStatusError(method, path string, status int, body []byte) error {
	statusErr := newStatusError(method, path, status, body)
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		c.machine.Stop(ReasonRevoked)
		return &terminalError{err: statusErr}
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return nil
	}
	if status >= 500 {
		return nil
	}
	// Any other client error means the endpoint or the request does not match
	// what the source serves; retrying would loop forever.
	c.machine.Stop(ReasonConfigError)
	return &terminalError{err: statusErr}
}
