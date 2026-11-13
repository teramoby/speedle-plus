// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Package adsyncfake provides an in-memory adsync.RequestGate and an evaluator
// wrapper for entry-point tests. It never talks to the network.
package adsyncfake

import (
	"context"
	"net/http"
	"sync"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/eval"
)

// Forwarded is one fallback call the source received.
type Forwarded struct {
	Operation adsync.FallbackOperation
	Body      []byte
}

// Source is a controllable RequestGate.
type Source struct {
	mu       sync.Mutex
	instance adsync.InstanceStatus
	services []adsync.ServiceStatus
	order    map[string]int

	fallbackAvailable bool
	fallbackStatus    int
	fallbackHeader    http.Header
	fallbackBody      []byte
	fallbackErr       error
	forwarded         []Forwarded

	// staleUntrusted models a store whose stale instance never evaluates
	// locally.
	staleUntrusted bool

	// pendingRemovals holds service names whose old copies are still installed
	// in the evaluator after a removal or rename.
	pendingRemovals map[string]struct{}
}

// New returns a live instance with no services and an available fallback.
func New() *Source {
	return &Source{
		instance:          adsync.InstanceStatus{Liveness: adsync.LivenessLive},
		order:             make(map[string]int),
		pendingRemovals:   make(map[string]struct{}),
		fallbackAvailable: true,
	}
}

// SetRemovalPending records whether a removed or renamed service name is still
// installed in the evaluator.
func (s *Source) SetRemovalPending(name string, pending bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending {
		s.pendingRemovals[name] = struct{}{}
		return
	}
	delete(s.pendingRemovals, name)
}

// RemovalInstalled implements the store's removal guard: a name is installed
// until it is recorded as pending.
func (s *Source) RemovalInstalled(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, pending := s.pendingRemovals[name]
	return !pending
}

// SetInstance replaces the instance status.
func (s *Source) SetInstance(instance adsync.InstanceStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instance = instance
}

// SetService adds or replaces one service.
func (s *Source) SetService(service adsync.ServiceStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index, found := s.order[service.Name]; found {
		s.services[index] = service
		return
	}
	s.order[service.Name] = len(s.services)
	s.services = append(s.services, service)
}

// SetServiceState sets the state of one service, creating it when needed.
func (s *Source) SetServiceState(name string, state adsync.SyncState) {
	s.mu.Lock()
	service := s.serviceLocked(name)
	service.State = state
	s.mu.Unlock()
}

// SetStaleIsUntrusted makes a stale instance of the source forward or deny
// every request, like the policy-source store.
func (s *Source) SetStaleIsUntrusted(untrusted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staleUntrusted = untrusted
}

// StaleIsUntrusted implements the stale routing hook.
func (s *Source) StaleIsUntrusted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.staleUntrusted
}

// SetFallbackAvailable controls whether a syncing service may be forwarded.
func (s *Source) SetFallbackAvailable(available bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallbackAvailable = available
}

// SetFallbackResponse sets the response a forwarded call returns.
func (s *Source) SetFallbackResponse(status int, header http.Header, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallbackStatus = status
	s.fallbackHeader = header
	s.fallbackBody = body
}

// SetFallbackError makes every forwarded call fail.
func (s *Source) SetFallbackError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallbackErr = err
}

// Forwarded returns a copy of every forwarded call.
func (s *Source) Forwarded() []Forwarded {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Forwarded(nil), s.forwarded...)
}

// Status implements adsync.StatusProvider.
func (s *Source) Status(name string) (adsync.Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found := s.order[name]; !found {
		return adsync.Status{Instance: s.instance}, false
	}
	return adsync.Status{Instance: s.instance, Service: s.serviceLocked(name)}, true
}

// Instance implements adsync.RequestGate.
func (s *Source) Instance() adsync.InstanceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.instance
}

// Services implements adsync.RequestGate.
func (s *Source) Services() []adsync.ServiceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]adsync.ServiceStatus(nil), s.services...)
}

// ForwardFallback implements adsync.RequestGate.
func (s *Source) ForwardFallback(_ context.Context, operation adsync.FallbackOperation, body []byte) (*adsync.FallbackResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forwarded = append(s.forwarded, Forwarded{Operation: operation, Body: append([]byte(nil), body...)})
	if s.fallbackErr != nil {
		return nil, s.fallbackErr
	}
	status := s.fallbackStatus
	if status == 0 {
		status = http.StatusOK
	}
	header := s.fallbackHeader
	if header == nil {
		header = http.Header{"Content-Type": []string{"application/json"}}
	}
	response := &adsync.FallbackResponse{
		StatusCode:     status,
		Header:         header.Clone(),
		Body:           append([]byte(nil), s.fallbackBody...),
		DecisionSource: adsync.DecisionSourceUpstream,
	}
	return response, nil
}

// FallbackAvailable implements adsync.RequestGate.
func (s *Source) FallbackAvailable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fallbackAvailable
}

func (s *Source) serviceLocked(name string) adsync.ServiceStatus {
	index, found := s.order[name]
	if !found {
		s.order[name] = len(s.services)
		s.services = append(s.services, adsync.ServiceStatus{Name: name})
		return s.services[len(s.services)-1]
	}
	return s.services[index]
}

var _ adsync.RequestGate = (*Source)(nil)

// Evaluator wraps a real evaluator and attaches a fake distribution source, so
// entry-point tests exercise the real local path together with a controlled
// distribution state.
type Evaluator struct {
	eval.InternalEvaluator
	source adsync.RequestGate
}

// WithGate returns an evaluator that reports the given gate source.
func WithGate(inner eval.InternalEvaluator, source adsync.RequestGate) *Evaluator {
	return &Evaluator{InternalEvaluator: inner, source: source}
}

// DistributionStore implements the optional interface the entry points use.
func (e *Evaluator) DistributionStore() any { return e.source }
