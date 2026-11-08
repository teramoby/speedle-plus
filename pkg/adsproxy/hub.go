// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"fmt"
	"sync"
	"time"
)

const (
	// DefaultMaxDownstream is the default number of distinct downstream
	// instances one proxy serves.
	DefaultMaxDownstream = 50
	// MaxDownstreamLimit is the protocol's upper bound for that number.
	MaxDownstreamLimit = 100
	// DefaultMaxConnectionsPerDownstream bounds simultaneous WebSockets and
	// held long polls from one credential. Two permits a reconnect to overlap
	// the previous connection's cleanup without allowing unbounded fan-out.
	DefaultMaxConnectionsPerDownstream = 2
	// DefaultCoalesceWindow is how long the hub collects head advances before
	// it broadcasts. One service produces at most one notification per window,
	// carrying the latest head.
	DefaultCoalesceWindow = 100 * time.Millisecond

	// subscriberQueue is how many notifications one subscriber may have
	// outstanding. Notifications are accelerators; a slow subscriber loses
	// some rather than blocking the hub.
	subscriberQueue = 64
)

// Notify announces that one service's head advanced.
type Notify struct {
	ServiceID string
	Head      uint64
}

// Hub fans notifications out to the downstream channel connections. It
// coalesces per service over a short window, so a burst of revisions produces
// one notification with the latest head instead of one per revision.
//
// The hub tracks distinct downstream ids for the instance limit and active
// registrations for the global and per-credential connection limits. A
// long-poll waiter and a WebSocket therefore each occupy a connection slot.
type Hub struct {
	maxDownstream               int
	maxConnectionsPerDownstream int
	window                      time.Duration

	mu          sync.Mutex
	subscribers map[*Subscriber]struct{}
	counts      map[string]int
	pending     map[string]uint64
	timer       *time.Timer
	closed      bool
}

// Subscriber is one downstream's notification sink.
type Subscriber struct {
	downstreamID string
	allowed      func(string) bool
	notify       chan Notify

	once sync.Once
}

// NewHub creates a hub. maxDownstream zero uses DefaultMaxDownstream and is
// capped at MaxDownstreamLimit. window zero uses DefaultCoalesceWindow. The
// optional per-downstream bound defaults to two active channels and waiters.
func NewHub(maxDownstream int, window time.Duration, perDownstream ...int) (*Hub, error) {
	if maxDownstream == 0 {
		maxDownstream = DefaultMaxDownstream
	}
	if maxDownstream < 1 || maxDownstream > MaxDownstreamLimit {
		return nil, fmt.Errorf("%w: MaxDownstream must be between 1 and %d", ErrConfig, MaxDownstreamLimit)
	}
	if len(perDownstream) > 1 {
		return nil, fmt.Errorf("%w: NewHub accepts at most one per-downstream connection bound", ErrConfig)
	}
	maxConnections := DefaultMaxConnectionsPerDownstream
	if len(perDownstream) == 1 && perDownstream[0] != 0 {
		maxConnections = perDownstream[0]
	}
	if maxConnections < 1 || maxConnections > 16 {
		return nil, fmt.Errorf("%w: per-downstream connection bound must be between 1 and 16", ErrConfig)
	}
	if window == 0 {
		window = DefaultCoalesceWindow
	}
	if window < 0 {
		return nil, fmt.Errorf("%w: CoalesceWindow must not be negative", ErrConfig)
	}
	return &Hub{
		maxDownstream:               maxDownstream,
		maxConnectionsPerDownstream: maxConnections,
		window:                      window,
		subscribers:                 make(map[*Subscriber]struct{}),
		counts:                      make(map[string]int),
		pending:                     make(map[string]uint64),
	}, nil
}

// Allow reports whether the downstream may register, without registering it.
// A server uses it before accepting a channel so the limit can be reported as
// an HTTP 503 instead of a WebSocket close.
func (h *Hub) Allow(downstreamID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.allowLocked(downstreamID)
}

func (h *Hub) allowLocked(downstreamID string) error {
	if h.closed {
		return fmt.Errorf("%w: hub is closed", ErrConfig)
	}
	if downstreamID == "" {
		return fmt.Errorf("%w: downstream id is required", ErrConfig)
	}
	if h.counts[downstreamID] >= h.maxConnectionsPerDownstream {
		return fmt.Errorf("%w: downstream %q has %d active connections", ErrFanoutLimit, downstreamID, h.counts[downstreamID])
	}
	if len(h.subscribers) >= h.maxDownstream*h.maxConnectionsPerDownstream {
		return fmt.Errorf("%w: %d active connections", ErrFanoutLimit, len(h.subscribers))
	}
	if h.counts[downstreamID] == 0 && len(h.counts) >= h.maxDownstream {
		return fmt.Errorf("%w: %d downstreams", ErrFanoutLimit, len(h.counts))
	}
	return nil
}

// Subscribe registers one subscriber. allowed filters the services it may
// receive; nil allows every service.
func (h *Hub) Subscribe(downstreamID string, allowed func(string) bool) (*Subscriber, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.allowLocked(downstreamID); err != nil {
		return nil, err
	}
	subscriber := &Subscriber{
		downstreamID: downstreamID,
		allowed:      allowed,
		notify:       make(chan Notify, subscriberQueue),
	}
	h.subscribers[subscriber] = struct{}{}
	h.counts[downstreamID]++
	return subscriber, nil
}

// Unsubscribe removes a subscriber. It is safe to call more than once.
func (h *Hub) Unsubscribe(subscriber *Subscriber) {
	if subscriber == nil {
		return
	}
	subscriber.once.Do(func() {
		h.mu.Lock()
		if _, found := h.subscribers[subscriber]; found {
			delete(h.subscribers, subscriber)
			h.counts[subscriber.downstreamID]--
			if h.counts[subscriber.downstreamID] <= 0 {
				delete(h.counts, subscriber.downstreamID)
			}
		}
		h.mu.Unlock()
		// The notification channel is intentionally not closed: flush may have
		// copied this subscriber and still be sending. A send on a buffered
		// channel that nobody reads is harmless and the channel is collected
		// with the subscriber.
	})
}

// C is the subscriber's notification stream.
func (s *Subscriber) C() <-chan Notify {
	if s == nil {
		return nil
	}
	return s.notify
}

// Downstream returns the downstream id this subscriber belongs to.
func (s *Subscriber) Downstream() string {
	if s == nil {
		return ""
	}
	return s.downstreamID
}

// Downstreams returns the number of distinct downstream instances.
func (h *Hub) Downstreams() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.counts)
}

// Subscribers returns the number of active registrations, including long-poll
// waiters.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}

// Publish records a head advance for one service and schedules a broadcast
// after the coalescing window. Publishing a head that a pending notification
// already covers only updates the pending head.
func (h *Hub) Publish(serviceID string, head uint64) {
	if serviceID == "" || head == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if current, found := h.pending[serviceID]; !found || head > current {
		h.pending[serviceID] = head
	}
	if h.timer == nil {
		h.timer = time.AfterFunc(h.window, h.flush)
	}
}

func (h *Hub) flush() {
	h.mu.Lock()
	pending := h.pending
	h.pending = make(map[string]uint64)
	h.timer = nil
	subscribers := make([]*Subscriber, 0, len(h.subscribers))
	for subscriber := range h.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	h.mu.Unlock()

	for serviceID, head := range pending {
		for _, subscriber := range subscribers {
			if subscriber.allowed != nil && !subscriber.allowed(serviceID) {
				continue
			}
			select {
			case subscriber.notify <- Notify{ServiceID: serviceID, Head: head}:
			default:
				// Notifications are best effort. The next ack repeats the head.
			}
		}
	}
}

// Close stops a pending broadcast and rejects new subscribers. Existing
// subscribers are not closed; callers unsubscribe them.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	h.pending = make(map[string]uint64)
}
