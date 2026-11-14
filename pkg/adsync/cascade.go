// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// UpstreamStatus is the remaining end-to-end freshness budget of a relay.
// FreshFor is relative to the observation time, never a renewed per-hop lease.
type UpstreamStatus struct {
	State    string
	FreshFor time.Duration
}

func parseUpstreams(config Config, base *url.URL) ([]*url.URL, error) {
	entries := config.UpstreamURLs
	if len(entries) == 0 {
		return []*url.URL{base}, nil
	}
	if config.APIKey != "" && len(entries) != 1 {
		return nil, fmt.Errorf("adsync: direct mode requires one upstream")
	}
	result := make([]*url.URL, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		candidate, err := url.Parse(entry)
		if err != nil || candidate.Scheme != "https" || candidate.Hostname() == "" || candidate.User != nil || candidate.RawQuery != "" || candidate.ForceQuery || candidate.Fragment != "" || strings.Contains(entry, "#") {
			return nil, fmt.Errorf("adsync: invalid upstream URL")
		}
		candidate.Path = strings.TrimSuffix(candidate.Path, "/")
		if seen[candidate.String()] {
			return nil, fmt.Errorf("adsync: duplicate upstream URL")
		}
		seen[candidate.String()] = true
		result = append(result, candidate)
	}
	if result[0].String() != base.String() {
		return nil, fmt.Errorf("adsync: BaseURL must be the first upstream")
	}
	return result, nil
}

func (c *Client) cascade() bool { return c != nil && c.apiKey == "" && len(c.upstreams) > 0 }
func (c *Client) selectedBase() *url.URL {
	if active := c.activeBase.Load(); active != nil {
		return active
	}
	return c.base
}

// runCascade owns endpoint selection. A transport loop returns after its first
// failure in cascade mode; automatic WS-to-poll fallback remains per endpoint.
func (c *Channel) runCascade(ctx context.Context) error {
	index, rounds := 0, 0
	for ctx.Err() == nil {
		attemptCtx, cancel := context.WithCancel(ctx)
		selectEndpoint := func() {
			c.client.selectEndpoint(c.client.upstreams[index], cancel)
			c.mu.Lock()
			c.acked = false
			c.healthySince = time.Time{}
			c.sentAt = make(map[uint64]time.Time)
			c.lastAckSeq = 0
			c.mu.Unlock()
		}
		if c.selectUpstream != nil {
			c.selectUpstream(selectEndpoint)
		} else {
			selectEndpoint()
		}
		preferred := false
		monitorDone := make(chan struct{})
		go func() {
			defer close(monitorDone)
			tick := time.NewTicker(100 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-attemptCtx.Done():
					return
				case <-tick.C:
					c.mu.Lock()
					since := c.healthySince
					c.mu.Unlock()
					if since.IsZero() {
						continue
					}
					if c.machine.Instance().Liveness != LivenessLive {
						cancel()
						return
					}
					if index != 0 && time.Since(since) >= c.failbackInterval {
						preferred = true
						cancel()
						return
					}
				}
			}
		}()
		_ = c.runTransport(attemptCtx)
		c.client.dataReady.Store(false)
		cancel()
		<-monitorDone
		c.client.upstreamMu.Lock()
		c.client.upstreamCancel = nil
		c.client.upstreamMu.Unlock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.mu.Lock()
		admitted := c.acked
		c.mu.Unlock()
		if admitted {
			rounds = 0
		}
		if preferred {
			index = 0
			continue
		}
		index = (index + 1) % len(c.client.upstreams)
		if index == 0 {
			rounds++
			if err := sleepContext(ctx, c.backoff.delay(rounds)); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

func validateCascadeMetadata(ack *channelAck) error {
	if len(ack.Chain) > 0 {
		seen := map[string]bool{}
		for _, id := range ack.Chain {
			if !ValidInstanceID(id) || seen[id] {
				return fmt.Errorf("%w: invalid ancestor chain", ErrProtocol)
			}
			seen[id] = true
		}
	}
	if ack.UpstreamState == "" && ack.UpstreamFreshForMs == nil {
		return nil
	}
	if ack.UpstreamState != "live" && ack.UpstreamState != "syncing" && ack.UpstreamState != "stale" {
		return fmt.Errorf("%w: invalid upstreamState", ErrProtocol)
	}
	if ack.UpstreamFreshForMs == nil || *ack.UpstreamFreshForMs < 0 || *ack.UpstreamFreshForMs > int64(maxStopAfter/time.Millisecond) {
		return fmt.Errorf("%w: invalid upstreamFreshForMs", ErrProtocol)
	}
	return nil
}

func (c *Channel) applyCascadeMetadata(ack *channelAck) error {
	if err := validateCascadeMetadata(ack); err != nil {
		return err
	}
	self, err := c.client.InstanceID()
	if err != nil {
		return err
	}
	for _, id := range ack.Chain {
		if id == self {
			return fmt.Errorf("%w: chain_loop", ErrProtocol)
		}
	}
	deadline := time.Time{}
	if ack.UpstreamFreshForMs != nil {
		deadline = time.Now().Add(time.Duration(*ack.UpstreamFreshForMs) * time.Millisecond)
	}
	if c.client.cascade() {
		var err error
		deadline, err = c.inheritedDeadline(ack)
		if err != nil {
			return err
		}
		if len(ack.Chain) == 0 || ack.UpstreamState == "" || ack.UpstreamFreshForMs == nil {
			return fmt.Errorf("%w: cascade ack requires chain and freshness", ErrProtocol)
		}
		services := make([]Service, len(ack.Services))
		for i, entry := range ack.Services {
			services[i] = entry.service()
		}
		if ack.UpstreamState == "live" && *ack.UpstreamFreshForMs > 0 {
			if err := c.machine.validateCandidate(services); err != nil {
				return err
			}
		}
	}
	if ack.UpstreamState == "live" && !time.Now().Before(deadline) {
		return fmt.Errorf("%w: upstream_stale", ErrUnavailable)
	}
	if c.onChain != nil {
		if err := c.onChain(append(append([]string(nil), ack.Chain...), self), deadline); err != nil {
			return err
		}
	}
	if ack.UpstreamState != "" {
		remaining := c.machine.observeUpstreamUntil(ack.UpstreamState, deadline)
		if ack.UpstreamState != "live" || remaining <= 0 {
			return fmt.Errorf("%w: upstream_%s", ErrUnavailable, ack.UpstreamState)
		}
	}
	c.mu.Lock()
	if c.healthySince.IsZero() {
		c.healthySince = time.Now()
	}
	c.mu.Unlock()
	return nil
}

func (m *Machine) observeUpstream(state string, freshFor time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upstreamState = state
	m.upstreamDeadline = m.clock.Now().Add(min(freshFor, m.effectiveThresholdsLocked().StaleAfter))
}

// Upstream reports the minimum direct and inherited freshness window and
// includes local synchronization state. No ack means syncing, even with no data.
func (m *Machine) Upstream() UpstreamStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	if m.lastConfirmedAckAt.IsZero() {
		return UpstreamStatus{State: "syncing"}
	}
	deadline := m.lastConfirmedAckAt.Add(m.effectiveThresholdsLocked().StaleAfter)
	if !m.upstreamDeadline.IsZero() && m.upstreamDeadline.Before(deadline) {
		deadline = m.upstreamDeadline
	}
	remaining := max(time.Duration(0), deadline.Sub(now))
	if remaining == 0 || m.livenessLocked(now) != LivenessLive || m.upstreamState == "stale" {
		return UpstreamStatus{State: "stale"}
	}
	if m.upstreamState == "syncing" {
		return UpstreamStatus{State: "syncing", FreshFor: remaining}
	}
	for _, record := range m.records {
		if record.setMember && record.state != SyncStateInSync {
			return UpstreamStatus{State: "syncing", FreshFor: remaining}
		}
	}
	return UpstreamStatus{State: "live", FreshFor: remaining}
}

// validateCandidate checks before any membership change, including removals.
func (m *Machine) validateCandidate(services []Service) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, service := range services {
		r := m.records[service.ID]
		if r == nil {
			continue
		}
		if r.headKnown && service.Epoch == r.epoch && (service.Revision < r.headRevision || (service.Revision == r.headRevision && !r.headStateHash.IsZero() && service.HeadStateHash != r.headStateHash)) {
			return fmt.Errorf("%w: upstream head regressed or conflicts", ErrIntegrity)
		}
		if r.hasLocal && service.Epoch == r.localEpoch && (service.Revision < r.localRevision || (service.Revision == r.localRevision && service.HeadStateHash != r.localStateHash)) {
			return fmt.Errorf("%w: upstream below retained copy", ErrIntegrity)
		}
	}
	return nil
}

// recordSent captures monotonic time before a frame can reach the relay.
func (c *Channel) recordSent(seq uint64) {
	if !c.client.cascade() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sentAt == nil {
		c.sentAt = make(map[uint64]time.Time)
	}
	c.sentAt[seq] = time.Now()
}

// An ack is generated after the echoed client frame arrived. Subtracting its
// entire RTT therefore overestimates transit/queue time, conservatively; it
// cannot extend an ancestor deadline and requires no synchronized wall clocks.
// Repeated unsolicited acks keep aging the same reference until a new frame.
func (c *Channel) inheritedDeadline(ack *channelAck) (time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ack.AckSeq == nil || ack.UpstreamFreshForMs == nil {
		return time.Time{}, fmt.Errorf("%w: cascade ack requires ackSeq and freshness", ErrProtocol)
	}
	seq := *ack.AckSeq
	sent, ok := c.sentAt[seq]
	if !ok || seq < c.lastAckSeq {
		return time.Time{}, fmt.Errorf("%w: unknown or reordered ackSeq", ErrProtocol)
	}
	// Keep the current reference for unsolicited acks and discard older ones.
	c.lastAckSeq = seq
	for previous := range c.sentAt {
		if previous < seq {
			delete(c.sentAt, previous)
		}
	}
	return sent.Add(time.Duration(*ack.UpstreamFreshForMs) * time.Millisecond), nil
}

func (c *Client) selectEndpoint(base *url.URL, cancel context.CancelFunc) {
	c.upstreamMu.Lock()
	defer c.upstreamMu.Unlock()
	previous := c.channelBase()
	c.fallbackMu.Lock()
	if c.fallbackRetryByUpstream == nil {
		c.fallbackRetryByUpstream = make(map[string]time.Time)
	}
	c.fallbackRetryByUpstream[previous.String()] = c.fallbackRetryAt
	c.fallbackRetryAt = c.fallbackRetryByUpstream[base.String()]
	c.fallbackRetrySeq++
	c.fallbackMu.Unlock()
	c.candidateBase.Store(base)
	c.dataReady.Store(false)
	c.upstreamGeneration++
	c.upstreamCancel = cancel
}

func (c *Client) failUpstreamGeneration(generation uint64) {
	c.upstreamMu.Lock()
	defer c.upstreamMu.Unlock()
	if c.cascade() && generation == c.upstreamGeneration && c.upstreamCancel != nil {
		c.upstreamCancel()
	}
}

func (c *Client) channelBase() *url.URL {
	if candidate := c.candidateBase.Load(); candidate != nil {
		return candidate
	}
	return c.base
}
func (c *Client) selectionReady() bool {
	return !c.cascade() || !c.selectedByChannel.Load() || c.dataReady.Load()
}
func (c *Client) approveCandidate() {
	if !c.cascade() {
		return
	}
	c.upstreamMu.Lock()
	defer c.upstreamMu.Unlock()
	if candidate := c.candidateBase.Load(); candidate != nil {
		c.activeBase.Store(candidate)
		c.dataReady.Store(true)
	}
}

func (m *Machine) observeUpstreamUntil(state string, deadline time.Time) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Sample the machine clock first, then the remaining monotonic duration:
	// lock/callback waits can only shorten the inherited absolute deadline.
	now := m.clock.Now()
	remaining := max(time.Duration(0), time.Until(deadline))
	m.upstreamState = state
	m.upstreamDeadline = now.Add(min(remaining, m.effectiveThresholdsLocked().StaleAfter))
	return remaining
}
