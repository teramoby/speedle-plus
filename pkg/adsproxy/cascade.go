// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"slices"
)

// SetChain installs the runtime equivalent of Config.Chain. It also closes
// existing channels which became ancestors after an upstream change.
func (s *Server) SetChain(chain []string) error {
	guard, err := validateChain(chain, s.instanceID)
	if err != nil {
		return err
	}
	s.availabilityMu.Lock()
	s.mu.Lock()
	if !slices.Equal(s.chain, chain) {
		close(s.chainChangedCh)
		s.chainChangedCh = make(chan struct{})
	}
	s.chain = append([]string(nil), chain...)
	s.loopGuard = guard
	var connections []*websocket.Conn
	for identity, conn := range s.channels {
		if _, loop := guard[identity.uuid]; loop {
			connections = append(connections, conn)
		}
	}
	s.mu.Unlock()
	s.availabilityMu.Unlock()
	for _, conn := range connections {
		// Close immediately; a peer which is not reading cannot delay topology.
		_ = conn.Close()
	}
	return nil
}

func validateChain(chain []string, self string) (map[string]struct{}, error) {
	if len(chain) == 0 || chain[len(chain)-1] != self {
		return nil, fmt.Errorf("%w: Chain must end with InstanceID", ErrConfig)
	}
	guard := make(map[string]struct{}, len(chain))
	for _, id := range chain {
		if !adsync.ValidInstanceID(id) {
			return nil, fmt.Errorf("%w: Chain entries must be UUIDv7 values", ErrConfig)
		}
		if _, duplicate := guard[id]; duplicate {
			return nil, fmt.Errorf("%w: Chain repeats an instance", ErrConfig)
		}
		guard[id] = struct{}{}
	}
	return guard, nil
}

func (s *Server) upstreamStatus() adsync.UpstreamStatus {
	if s.upstream != nil {
		return s.upstream()
	}
	return adsync.UpstreamStatus{State: "live", FreshFor: s.stale}
}

func (s *Server) chainChangeSignal() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chainChangedCh
}
