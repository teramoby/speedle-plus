// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

const (
	// Channel protocol defaults. They match the distribution contract.
	defaultHeartbeatInterval = 30 * time.Second
	defaultStaleAfter        = 60 * time.Second
	defaultStopAfter         = 120 * time.Second

	minHeartbeatInterval = 15 * time.Second
	maxHeartbeatInterval = 5 * time.Minute
	minStopAfter         = 60 * time.Second
	maxStopAfter         = 15 * time.Minute
)

// Config configures a Server.
type Config struct {
	// InstanceID is this proxy's stable UUIDv7. The relay reports it in its own
	// heartbeats; downstream instances never need to match it.
	InstanceID string
	// Mirror holds the verified copies served downstream. It is required.
	Mirror *Mirror
	// CoalesceWindow is how long notifications are collected per service.
	// Zero uses DefaultCoalesceWindow. It is ignored when Hub is set.
	CoalesceWindow time.Duration
	// Hub overrides the fan-out hub, for a caller that shares one hub or a
	// test that inspects it.
	Hub *Hub
	// HeartbeatInterval, StaleAfter and StopAfter are the thresholds announced
	// in every ack. Zero values use 30s, 60s and 120s.
	HeartbeatInterval time.Duration
	StaleAfter        time.Duration
	StopAfter         time.Duration
	// Now supplies the clock for serverTime. Nil uses time.Now.
	Now func() time.Time
	// Paused reports whether downstream distribution must stop: a pause
	// command from Cloud, or an instance that is no longer live and therefore
	// cannot vouch for the copies it would serve. It closes distribution to
	// downstream instances only; synchronization and local evaluation are
	// unaffected. Nil means never paused.
	Paused func() bool
}

// sessionIdentity is the authenticated identity of one downstream session: the
// verified client certificate fingerprint and the UUIDv7 the downstream
// reports in its hello or heartbeat. Neither value grants extra access: the
// certificate authenticates, and the UUID only names the session for
// diagnostics and de-duplication.
type sessionIdentity struct {
	fingerprint string
	uuid        string
}

// Server serves the /policy-source/v1 protocol to downstream ADS instances. It
// is an http.Handler meant to be mounted only on the dedicated downstream
// listener, whose TLS configuration requires and verifies a client certificate.
type Server struct {
	instanceID string
	mirror     *Mirror
	hub        *Hub
	ownsHub    bool

	heartbeat time.Duration
	stale     time.Duration
	stop      time.Duration

	now         func() time.Time
	paused      func() bool
	upgrader    websocket.Upgrader
	removeWatch func()

	mu sync.Mutex
	// channels holds the one live WebSocket channel of each session identity.
	// A reconnect replaces its predecessor.
	channels map[sessionIdentity]*websocket.Conn
	// sessions counts active channels and held polls per identity. Its length
	// is the number of distinct direct downstream sessions, which is the count
	// the relay reports in its own heartbeat.
	sessions map[sessionIdentity]int
	// fingerprintsByUUID maps a reported UUID to the certificate fingerprints
	// currently using it, so a collision under another certificate is logged
	// without affecting either session.
	fingerprintsByUUID map[string]map[string]struct{}
	// unavailable mirrors the paused callback. It is true while distribution
	// must stop, and unavailableCh is closed in that state so a held poll
	// answers immediately instead of waiting out its hold.
	unavailable   bool
	unavailableCh chan struct{}
	// availabilityMu serializes confirming writes with an availability flip.
	// A writer holds the read lock across its check and response; the watcher
	// holds the write lock while it flips the state and closes connections.
	availabilityMu sync.RWMutex

	closeOnce sync.Once
	closed    bool
	done      chan struct{}
}

// NewServer validates the configuration and wires the mirror's head events into
// the fan-out hub.
func NewServer(config Config) (*Server, error) {
	if !adsync.ValidInstanceID(config.InstanceID) {
		return nil, fmt.Errorf("%w: InstanceID must be a UUIDv7", ErrConfig)
	}
	if config.Mirror == nil {
		return nil, fmt.Errorf("%w: Mirror is required", ErrConfig)
	}
	heartbeat, stale, stop, err := normalizeThresholds(
		config.HeartbeatInterval, config.StaleAfter, config.StopAfter)
	if err != nil {
		return nil, err
	}

	hub := config.Hub
	ownsHub := hub == nil
	if hub == nil {
		hub, err = NewHub(config.CoalesceWindow)
		if err != nil {
			return nil, err
		}
	}

	now := config.Now
	if now == nil {
		now = time.Now
	}
	server := &Server{
		instanceID:         config.InstanceID,
		mirror:             config.Mirror,
		hub:                hub,
		ownsHub:            ownsHub,
		heartbeat:          heartbeat,
		stale:              stale,
		stop:               stop,
		now:                now,
		paused:             config.Paused,
		channels:           make(map[sessionIdentity]*websocket.Conn),
		sessions:           make(map[sessionIdentity]int),
		fingerprintsByUUID: make(map[string]map[string]struct{}),
		unavailableCh:      make(chan struct{}),
		done:               make(chan struct{}),
	}
	server.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(*http.Request) bool {
			// The downstream listener authenticates every connection with a
			// verified client certificate, which a browser cannot present
			// cross-origin. An origin check adds nothing.
			return true
		},
	}
	server.removeWatch = config.Mirror.AddObserver(func(event HeadEvent) {
		hub.Publish(event.ServiceID, event.Revision)
	})
	server.refreshAvailability()
	go server.watchAvailability()
	return server, nil
}

// distributionWatchInterval bounds how long a state change that no request
// drives, such as a heartbeat deadline passing while a channel is idle, can
// leave downstream channels open.
const distributionWatchInterval = 500 * time.Millisecond

// watchAvailability re-evaluates the paused callback until the server closes.
// The callback also depends on time (an instance stops when its last
// confirming ack ages out), so there is no event to subscribe to.
func (s *Server) watchAvailability() {
	ticker := time.NewTicker(distributionWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.refreshAvailability()
		}
	}
}

// errDistributionStopped reports that a confirming response was abandoned
// because distribution stopped before it could be written.
var errDistributionStopped = errors.New("adsproxy: distribution stopped")

// refreshAvailability closes every downstream channel and wakes held polls
// when distribution flips to unavailable, so a paused or no-longer-live relay
// stops confirming stale policy instead of waiting for the next heartbeat. It
// takes the availability write lock, so a flip waits for an in-flight ack or
// poll response to finish and cannot land between that response's check and
// its write.
func (s *Server) refreshAvailability() {
	s.availabilityMu.Lock()
	defer s.availabilityMu.Unlock()
	unavailable := s.distributionUnavailable()
	s.mu.Lock()
	if s.closed || unavailable == s.unavailable {
		s.mu.Unlock()
		return
	}
	s.unavailable = unavailable
	var connections []*websocket.Conn
	if unavailable {
		close(s.unavailableCh)
		connections = make([]*websocket.Conn, 0, len(s.channels))
		for _, conn := range s.channels {
			connections = append(connections, conn)
		}
	} else {
		s.unavailableCh = make(chan struct{})
	}
	s.mu.Unlock()
	for _, conn := range connections {
		closeWithReason(conn, websocket.ClosePolicyViolation, "distribution_paused")
		_ = conn.Close()
	}
}

// writeChannelAck writes one confirming ack unless distribution has stopped.
// The availability read lock makes the check and the write atomic against a
// flip: a pause or liveness change cannot complete between them, so an ack
// that reaches a downstream was current when it was written.
func (s *Server) writeChannelAck(conn *websocket.Conn) error {
	s.availabilityMu.RLock()
	defer s.availabilityMu.RUnlock()
	if s.distributionUnavailable() {
		return errDistributionStopped
	}
	return writeChannelJSON(conn, s.ack())
}

// unavailableSignal returns a channel that is closed while distribution is
// unavailable. A caller that captured it before a flip sees the flip.
func (s *Server) unavailableSignal() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unavailableCh
}

// InstanceID returns this proxy's instance id.
func (s *Server) InstanceID() string { return s.instanceID }

// DownstreamCount returns the number of distinct direct downstream sessions
// with an active channel or held poll. The process reports it in its own
// heartbeat.
func (s *Server) DownstreamCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// Close closes every active downstream channel, ends held polls, releases the
// mirror observer and closes the hub. The server must not be used afterwards.
// It is safe to call more than once.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		connections := make([]*websocket.Conn, 0, len(s.channels))
		for _, conn := range s.channels {
			connections = append(connections, conn)
		}
		close(s.done)
		s.mu.Unlock()
		for _, conn := range connections {
			_ = conn.Close()
		}
		if s.removeWatch != nil {
			s.removeWatch()
			s.removeWatch = nil
		}
		// A hub supplied through Config is shared with its owner; only a hub
		// this server created may be closed here.
		if s.ownsHub {
			s.hub.Close()
		}
	})
}

// isClosed reports whether Close has run.
func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// beginChannelSession registers a live channel for one session identity. When
// the identity already has a channel, the previous connection is returned so
// the caller can close it after the new one is admitted; a reconnect never
// counts twice and never leaves two live channels behind.
func (s *Server) beginChannelSession(identity sessionIdentity, conn *websocket.Conn) (*websocket.Conn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, false
	}
	previous := s.channels[identity]
	s.channels[identity] = conn
	s.retainLocked(identity)
	return previous, true
}

func (s *Server) endChannelSession(identity sessionIdentity, conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channels[identity] == conn {
		delete(s.channels, identity)
	}
	s.releaseLocked(identity)
}

// beginPollSession registers one held poll. A poll has no connection to
// de-duplicate; the identity is counted once while any poll is held.
func (s *Server) beginPollSession(identity sessionIdentity) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.retainLocked(identity)
	return true
}

func (s *Server) endPollSession(identity sessionIdentity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseLocked(identity)
}

func (s *Server) retainLocked(identity sessionIdentity) {
	if s.sessions[identity] == 0 {
		fingerprints := s.fingerprintsByUUID[identity.uuid]
		if fingerprints == nil {
			fingerprints = make(map[string]struct{})
			s.fingerprintsByUUID[identity.uuid] = fingerprints
		}
		for fingerprint := range fingerprints {
			if fingerprint != identity.fingerprint {
				log.Warnf("adsproxy: downstream UUID %s is active under certificates %s and %s; both sessions are kept",
					identity.uuid, fingerprint, identity.fingerprint)
				break
			}
		}
		fingerprints[identity.fingerprint] = struct{}{}
	}
	s.sessions[identity]++
}

func (s *Server) releaseLocked(identity sessionIdentity) {
	if s.sessions[identity] <= 1 {
		delete(s.sessions, identity)
		fingerprints := s.fingerprintsByUUID[identity.uuid]
		delete(fingerprints, identity.fingerprint)
		if len(fingerprints) == 0 {
			delete(s.fingerprintsByUUID, identity.uuid)
		}
		return
	}
	s.sessions[identity]--
}

// clientIdentity returns the verified client certificate of one connection.
// The listener's TLS configuration rejects a missing, untrusted, expired or
// revoked certificate during the handshake; this check keeps the handler fail
// closed if it is ever mounted somewhere else.
func clientIdentity(r *http.Request) (sessionIdentity, bool) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 ||
		len(r.TLS.PeerCertificates) == 0 {
		return sessionIdentity{}, false
	}
	leaf := r.TLS.VerifiedChains[0][0]
	digest := sha256.Sum256(leaf.Raw)
	return sessionIdentity{fingerprint: hex.EncodeToString(digest[:])}, true
}

// ServeHTTP implements the /policy-source/v1 data endpoints and the instance
// channel. All paths are authenticated by the listener's verified client
// certificate; this handler performs no token authentication.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	path := r.URL.Path
	if path != adsync.APIPrefix && !strings.HasPrefix(path, adsync.APIPrefix+"/") {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	if _, ok := clientIdentity(r); !ok {
		writeError(w, http.StatusForbidden, "unauthorized", "")
		return
	}
	if s.isClosed() {
		writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	if s.distributionUnavailable() {
		writeError(w, http.StatusServiceUnavailable, "distribution_paused", "")
		return
	}
	rest := strings.TrimPrefix(path, adsync.APIPrefix)
	switch {
	case rest == "/services":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
			return
		}
		s.handleServices(w, r)
	case rest == "/channel":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
			return
		}
		s.handleChannel(w, r)
	case rest == "/channel/poll":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
			return
		}
		s.handlePoll(w, r)
	case strings.HasPrefix(rest, "/services/"):
		s.handleService(w, r, strings.TrimPrefix(rest, "/services/"))
	default:
		writeError(w, http.StatusNotFound, "not_found", "")
	}
}

func (s *Server) handleService(w http.ResponseWriter, r *http.Request, remainder string) {
	segments := strings.Split(remainder, "/")
	if len(segments) != 2 || segments[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	serviceID, action := segments[0], segments[1]
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
		return
	}
	switch action {
	case "state":
		if _, ok := strictQuery(r.URL.RawQuery); !ok {
			writeError(w, http.StatusBadRequest, "invalid_request", "")
			return
		}
		s.handleState(w, serviceID)
	case "snapshot":
		if _, ok := strictQuery(r.URL.RawQuery); !ok {
			writeError(w, http.StatusBadRequest, "invalid_request", "")
			return
		}
		s.handleSnapshot(w, r, serviceID)
	case "changes":
		s.handleChanges(w, r, serviceID)
	default:
		writeError(w, http.StatusNotFound, "not_found", "")
	}
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	if _, ok := strictQuery(r.URL.RawQuery); !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	services, version := s.mirror.ServicesVersion()
	writeJSON(w, http.StatusOK, adsync.ServicesResponse{
		ServicesVersion: version,
		Services:        services,
	})
}

type stateResponse struct {
	ServiceID     string      `json:"serviceId"`
	SourceID      string      `json:"sourceId"`
	Epoch         string      `json:"epoch"`
	Head          uint64      `json:"head"`
	HeadStateHash adsync.Hash `json:"headStateHash"`
	ServerTime    int64       `json:"serverTime"`
}

func (s *Server) handleState(w http.ResponseWriter, serviceID string) {
	info, ok := s.resolveService(w, serviceID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, stateResponse{
		ServiceID:     info.ID,
		SourceID:      info.SourceID,
		Epoch:         info.Epoch,
		Head:          info.Revision,
		HeadStateHash: info.StateHash,
		ServerTime:    s.now().UnixMilli(),
	})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request, serviceID string) {
	if _, ok := s.resolveService(w, serviceID); !ok {
		return
	}
	response, found := s.mirror.Snapshot(serviceID)
	if !found {
		writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
		return
	}
	etag := snapshotETag(response)
	w.Header().Set("ETag", etag)
	if ifNoneMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request, serviceID string) {
	if _, ok := s.resolveService(w, serviceID); !ok {
		return
	}
	query, ok := strictQuery(r.URL.RawQuery, "since", "limit")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	sinceText := query.Get("since")
	since, err := strconv.ParseUint(sinceText, 10, 64)
	if err != nil || strconv.FormatUint(since, 10) != sinceText {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	limit := maxCommitsPerPage
	if query.Has("limit") {
		limitText := query.Get("limit")
		parsed, err := strconv.Atoi(limitText)
		if err != nil || parsed < 1 || parsed > maxCommitsPerPage || strconv.Itoa(parsed) != limitText {
			writeError(w, http.StatusBadRequest, "invalid_request", "")
			return
		}
		limit = parsed
	}

	page, err := s.mirror.Changes(serviceID, since, limit)
	if err != nil {
		s.writeMirrorError(w, err)
		return
	}
	encoded, err := trimChangesPage(&page)
	if err != nil {
		writeError(w, http.StatusGone, "snapshot_required", "history_unavailable")
		return
	}
	writeEncodedJSON(w, http.StatusOK, encoded)
}

// resolveService reports why a service cannot be served. Every valid client
// certificate may read every service the relay holds; there are no per-
// certificate subsets.
func (s *Server) resolveService(w http.ResponseWriter, serviceID string) (ServiceInfo, bool) {
	info, availability := s.mirror.Lookup(serviceID)
	switch availability {
	case ServiceAvailable:
		return info, true
	case ServiceRemoved:
		writeError(w, http.StatusGone, "snapshot_required", "source_gone")
	case ServiceNeedsSnapshot:
		writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
	default:
		writeError(w, http.StatusNotFound, "service_not_found", "")
	}
	return ServiceInfo{}, false
}

func (s *Server) writeMirrorError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnknownService):
		writeError(w, http.StatusNotFound, "service_not_found", "")
	case errors.Is(err, ErrServiceRemoved):
		writeError(w, http.StatusGone, "snapshot_required", "source_gone")
	case errors.Is(err, ErrHistoryUnavailable):
		writeError(w, http.StatusGone, "snapshot_required", "history_unavailable")
	case errors.Is(err, ErrIdentityChanged), errors.Is(err, adsync.ErrIntegrity):
		writeError(w, http.StatusGone, "snapshot_required", "epoch_mismatch")
	case errors.Is(err, ErrSnapshotRequired):
		writeError(w, http.StatusServiceUnavailable, "source_unavailable", "")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "")
	}
}

// distributionUnavailable reports whether downstream distribution must stop:
// the paused callback fired, for example after a Cloud pause command or
// because the instance is no longer live.
func (s *Server) distributionUnavailable() bool {
	return s.paused != nil && s.paused()
}

func snapshotETag(response adsync.SnapshotResponse) string {
	return `"` + response.SourceID + "-" + response.Epoch + "-" +
		strconv.FormatUint(response.Revision, 10) + "-" + response.StateHash.String() + "-" +
		response.CommitHash.String() + `"`
}

// strictQuery parses a request query against an explicit allowed-key set.
// Duplicate keys, unknown keys and malformed escapes are protocol errors;
// selecting one value with Values.Get would hide all three.
func strictQuery(raw string, allowedKeys ...string) (url.Values, bool) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, false
	}
	allowed := make(map[string]struct{}, len(allowedKeys))
	for _, key := range allowedKeys {
		allowed[key] = struct{}{}
	}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok || len(entries) != 1 {
			return nil, false
		}
	}
	return values, true
}

// ifNoneMatch reports whether the request's If-None-Match header contains the
// exact current entity tag. The wildcard is deliberately not accepted: a 304 is
// permitted only for an exact snapshot identity tuple.
func ifNoneMatch(header, etag string) bool {
	if strings.TrimSpace(header) == "" {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimSpace(candidate) == etag {
			return true
		}
	}
	return false
}

// trimChangesPage enforces the one-megabyte response bound by dropping trailing
// commits and adjusting to/nextSince. It returns the encoded page.
func trimChangesPage(page *adsync.ChangesResponse) ([]byte, error) {
	encoded, err := json.Marshal(page)
	if err != nil {
		return nil, err
	}
	for len(page.Commits) > 1 && len(encoded) > maxChangesPageBytes {
		page.Commits = page.Commits[:len(page.Commits)/2]
		page.To = page.Commits[len(page.Commits)-1].Revision
		page.NextSince = page.To + 1
		encoded, err = json.Marshal(page)
		if err != nil {
			return nil, err
		}
	}
	if len(encoded) > maxChangesPageBytes {
		return nil, fmt.Errorf("%w: one commit exceeds a page", ErrHistoryUnavailable)
	}
	return encoded, nil
}

func normalizeThresholds(heartbeat, stale, stop time.Duration) (time.Duration, time.Duration, time.Duration, error) {
	if heartbeat == 0 {
		heartbeat = defaultHeartbeatInterval
	}
	if stale == 0 {
		stale = defaultStaleAfter
	}
	if stop == 0 {
		stop = defaultStopAfter
	}
	if heartbeat < minHeartbeatInterval || heartbeat > maxHeartbeatInterval {
		return 0, 0, 0, fmt.Errorf("%w: heartbeat interval %s is outside %s..%s",
			ErrConfig, heartbeat, minHeartbeatInterval, maxHeartbeatInterval)
	}
	if stale <= heartbeat {
		return 0, 0, 0, fmt.Errorf("%w: stale threshold %s must be greater than the heartbeat interval %s",
			ErrConfig, stale, heartbeat)
	}
	if stop < minStopAfter || stop > maxStopAfter {
		return 0, 0, 0, fmt.Errorf("%w: stop threshold %s is outside %s..%s",
			ErrConfig, stop, minStopAfter, maxStopAfter)
	}
	if stop <= stale {
		return 0, 0, 0, fmt.Errorf("%w: stop threshold %s must be greater than the stale threshold %s",
			ErrConfig, stop, stale)
	}
	return heartbeat, stale, stop, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}
	writeEncodedJSON(w, status, encoded)
}

func writeEncodedJSON(w http.ResponseWriter, status int, encoded []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

type errorBody struct {
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, reason string) {
	writeJSON(w, status, errorBody{Error: code, Reason: reason})
}
