// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"fmt"
	"sync"
	"time"
)

const (
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
// The hub has no downstream or connection limit: cascade deployments have no
// engine-side fan-out ceiling.
type Hub struct {
	window time.Duration

	mu          sync.Mutex
	subscribers map[*Subscriber]struct{}
	pending     map[string]uint64
	timer       *time.Timer
	closed      bool
}

// Subscriber is one downstream session's notification sink.
type Subscriber struct {
	notify chan Notify

	once sync.Once
}

// NewHub creates a hub. A zero window uses DefaultCoalesceWindow.
func NewHub(window time.Duration) (*Hub, error) {
	if window == 0 {
		window = DefaultCoalesceWindow
	}
	if window < 0 {
		return nil, fmt.Errorf("%w: CoalesceWindow must not be negative", ErrConfig)
	}
	return &Hub{
		window:      window,
		subscribers: make(map[*Subscriber]struct{}),
		pending:     make(map[string]uint64),
	}, nil
}

// Subscribe registers one downstream session.
func (h *Hub) Subscribe() (*Subscriber, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, fmt.Errorf("%w: hub is closed", ErrConfig)
	}
	subscriber := &Subscriber{notify: make(chan Notify, subscriberQueue)}
	h.subscribers[subscriber] = struct{}{}
	return subscriber, nil
}

// Unsubscribe removes a subscriber. It is safe to call more than once.
func (h *Hub) Unsubscribe(subscriber *Subscriber) {
	if subscriber == nil {
		return
	}
	subscriber.once.Do(func() {
		h.mu.Lock()
		delete(h.subscribers, subscriber)
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

// Subscribers returns the number of active sessions, including long-poll
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
