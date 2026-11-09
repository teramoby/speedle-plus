// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

const (
	// DefaultMaxChainDepth is the number of proxies allowed between Cloud and
	// a leaf.
	DefaultMaxChainDepth = 3
	// MaxChainDepthLimit is the protocol's upper bound for that number.
	MaxChainDepthLimit = 3

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
	// InstanceID is this proxy's stable ULID. It identifies the proxy in chain
	// checks and is the id the process appends to its own upstream chain.
	InstanceID string
	// Chain is this proxy's own chain from Cloud to itself, inclusive. The last
	// entry must be InstanceID. Nil means just this instance, as for a proxy
	// that connects to Cloud directly. The chain is used to reject a
	// downstream that is actually one of this proxy's ancestors.
	Chain []string
	// Mirror holds the verified copies served downstream. It is required.
	Mirror *Mirror
	// Credentials is the downstream credential set. When nil, CredentialFile
	// is loaded instead.
	Credentials *CredentialSet
	// CredentialFile is the 0600 downstream credential file. It is used when
	// Credentials is nil and by ReloadCredentials.
	CredentialFile string
	// MaxDownstream bounds the number of distinct downstream instances. Zero
	// uses DefaultMaxDownstream; the range is 1 to MaxDownstreamLimit. It is
	// ignored when Hub is set.
	MaxDownstream int
	// MaxConnectionsPerDownstream bounds active WebSockets and held long polls
	// for each credential. Zero uses DefaultMaxConnectionsPerDownstream.
	MaxConnectionsPerDownstream int
	// MaxChainDepth bounds the number of proxy hops between Cloud and a leaf.
	// Zero uses DefaultMaxChainDepth; the range is 1 to MaxChainDepthLimit.
	MaxChainDepth int
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
	// Paused reports whether downstream distribution is paused, for example
	// after a pause command from Cloud. Pausing closes distribution to
	// downstream instances only; synchronization and local evaluation are
	// unaffected. Nil means never paused.
	Paused func() bool
}

// Server serves the /policy-source/v1 protocol to downstream ADS instances. It
// is an http.Handler, so the process mounts it next to its authorization
// routes.
type Server struct {
	instanceID  string
	chain       []string
	mirror      *Mirror
	credentials *CredentialSet
	hub         *Hub
	ownsHub     bool

	heartbeat time.Duration
	stale     time.Duration
	stop      time.Duration
	maxDepth  int

	now         func() time.Time
	paused      func() bool
	upgrader    websocket.Upgrader
	removeWatch func()

	mu        sync.Mutex
	sessions  map[*websocket.Conn]struct{}
	closeOnce sync.Once
	closed    bool
}

// NewServer validates the configuration and wires the mirror's head events into
// the fan-out hub.
func NewServer(config Config) (*Server, error) {
	if config.InstanceID == "" || !validEntityID(config.InstanceID) {
		return nil, fmt.Errorf("%w: InstanceID must be a ULID", ErrConfig)
	}
	if config.Mirror == nil {
		return nil, fmt.Errorf("%w: Mirror is required", ErrConfig)
	}

	credentials := config.Credentials
	if credentials == nil {
		if config.CredentialFile == "" {
			return nil, fmt.Errorf("%w: Credentials or CredentialFile is required", ErrConfig)
		}
		loaded, err := LoadCredentialFile(config.CredentialFile)
		if err != nil {
			return nil, err
		}
		credentials = loaded
	}

	maxDepth := config.MaxChainDepth
	if maxDepth == 0 {
		maxDepth = DefaultMaxChainDepth
	}
	if maxDepth < 1 || maxDepth > MaxChainDepthLimit {
		return nil, fmt.Errorf("%w: MaxChainDepth must be between 1 and %d", ErrConfig, MaxChainDepthLimit)
	}

	chain := append([]string(nil), config.Chain...)
	if len(chain) == 0 {
		chain = []string{config.InstanceID}
	}
	for _, instanceID := range chain {
		if !validEntityID(instanceID) {
			return nil, fmt.Errorf("%w: invalid chain entry %q", ErrConfig, instanceID)
		}
	}
	if chain[len(chain)-1] != config.InstanceID {
		return nil, fmt.Errorf("%w: Chain must end with InstanceID", ErrConfig)
	}
	if len(chain)-1 > maxDepth {
		return nil, fmt.Errorf("%w: Chain places this proxy %d hops from Cloud, the limit is %d",
			ErrConfig, len(chain)-1, maxDepth)
	}

	heartbeat, stale, stop, err := normalizeThresholds(
		config.HeartbeatInterval, config.StaleAfter, config.StopAfter)
	if err != nil {
		return nil, err
	}

	hub := config.Hub
	ownsHub := hub == nil
	if hub == nil {
		hub, err = NewHub(config.MaxDownstream, config.CoalesceWindow, config.MaxConnectionsPerDownstream)
		if err != nil {
			return nil, err
		}
	}

	now := config.Now
	if now == nil {
		now = time.Now
	}
	server := &Server{
		instanceID:  config.InstanceID,
		chain:       chain,
		mirror:      config.Mirror,
		credentials: credentials,
		hub:         hub,
		ownsHub:     ownsHub,
		heartbeat:   heartbeat,
		stale:       stale,
		stop:        stop,
		maxDepth:    maxDepth,
		now:         now,
		paused:      config.Paused,
		sessions:    make(map[*websocket.Conn]struct{}),
	}
	server.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(*http.Request) bool {
			// Downstream instances authenticate with a bearer token that a
			// browser cannot set cross-origin, so an origin check adds nothing.
			return true
		},
	}
	server.removeWatch = config.Mirror.AddObserver(func(event HeadEvent) {
		hub.Publish(event.ServiceID, event.Revision)
	})
	return server, nil
}

// InstanceID returns this proxy's instance id.
func (s *Server) InstanceID() string { return s.instanceID }

// Chain returns this proxy's own chain from Cloud to itself.
func (s *Server) Chain() []string { return append([]string(nil), s.chain...) }

// Credentials returns the credential set currently in force.
func (s *Server) Credentials() *CredentialSet { return s.credentials }

// DownstreamCount returns the number of distinct downstream instances currently
// registered. The process reports it in its own heartbeat.
func (s *Server) DownstreamCount() int { return s.hub.Downstreams() }

// ReloadCredentials re-reads the credential file and swaps the new set in
// atomically. A failed reload keeps the previous credentials and channels in
// force. Every established channel is then closed so removed credentials and
// changed service subsets take effect immediately; clients reconnect and
// resynchronize with their current grants.
func (s *Server) ReloadCredentials() error {
	if err := s.credentials.Reload(); err != nil {
		return err
	}
	s.closeDownstreamChannels(websocket.CloseTryAgainLater, "credentials_reloaded")
	return nil
}

func (s *Server) closeDownstreamChannels(code int, reason string) {
	s.mu.Lock()
	connections := make([]*websocket.Conn, 0, len(s.sessions))
	for conn := range s.sessions {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		closeWithReason(conn, code, reason)
		_ = conn.Close()
	}
}

// Close closes every active downstream channel, releases the mirror observer
// and closes the hub. The server must not be used afterwards. It is safe to call
// more than once.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		connections := make([]*websocket.Conn, 0, len(s.sessions))
		for conn := range s.sessions {
			connections = append(connections, conn)
		}
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

// track registers an upgraded channel connection so Close and credential
// reloads can terminate it. The grant must still be current, closing the race
// between bearer authentication and a concurrent reload.
func (s *Server) track(conn *websocket.Conn, credential *Credential) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.credentials.Current(credential) {
		return false
	}
	s.sessions[conn] = struct{}{}
	return true
}

func (s *Server) untrack(conn *websocket.Conn) {
	s.mu.Lock()
	delete(s.sessions, conn)
	s.mu.Unlock()
}

// ServeHTTP implements the /policy-source/v1 data endpoints and the instance
// channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	path := r.URL.Path
	if path != adsync.APIPrefix && !strings.HasPrefix(path, adsync.APIPrefix+"/") {
		writeError(w, http.StatusNotFound, "not_found", "")
		return
	}
	credential, authenticated := s.authenticate(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	if s.distributionPaused() {
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
		s.handleServices(w, credential)
	case rest == "/channel":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
			return
		}
		s.handleChannel(w, r, credential)
	case rest == "/channel/poll":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
			return
		}
		s.handlePoll(w, r, credential)
	case strings.HasPrefix(rest, "/services/"):
		s.handleService(w, r, credential, strings.TrimPrefix(rest, "/services/"))
	default:
		writeError(w, http.StatusNotFound, "not_found", "")
	}
}

func (s *Server) handleService(w http.ResponseWriter, r *http.Request, credential *Credential, remainder string) {
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
		s.handleState(w, credential, serviceID)
	case "snapshot":
		s.handleSnapshot(w, r, credential, serviceID)
	case "changes":
		s.handleChanges(w, r, credential, serviceID)
	default:
		writeError(w, http.StatusNotFound, "not_found", "")
	}
}

func (s *Server) handleServices(w http.ResponseWriter, credential *Credential) {
	services, version := s.mirror.ServicesVersion()
	visible := make([]adsync.Service, 0, len(services))
	for _, service := range services {
		if !credential.Allows(service.ID) {
			continue
		}
		visible = append(visible, service)
		if len(visible) == MaxServices {
			break
		}
	}
	writeJSON(w, http.StatusOK, adsync.ServicesResponse{
		ServicesVersion: version,
		Services:        visible,
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

func (s *Server) handleState(w http.ResponseWriter, credential *Credential, serviceID string) {
	info, ok := s.resolveService(w, credential, serviceID)
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

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request, credential *Credential, serviceID string) {
	if _, ok := s.resolveService(w, credential, serviceID); !ok {
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

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request, credential *Credential, serviceID string) {
	if _, ok := s.resolveService(w, credential, serviceID); !ok {
		return
	}
	sinceText := r.URL.Query().Get("since")
	since, err := strconv.ParseUint(sinceText, 10, 64)
	if err != nil || strconv.FormatUint(since, 10) != sinceText {
		writeError(w, http.StatusBadRequest, "invalid_request", "")
		return
	}
	limit := maxCommitsPerPage
	if limitText := r.URL.Query().Get("limit"); limitText != "" {
		parsed, err := strconv.Atoi(limitText)
		if err != nil || parsed < 1 || parsed > maxCommitsPerPage {
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

// resolveService applies the credential's service subset and reports why a
// service cannot be served. A service outside the subset is answered exactly
// like an unknown service, with no detail that would reveal its existence.
func (s *Server) resolveService(w http.ResponseWriter, credential *Credential, serviceID string) (ServiceInfo, bool) {
	if !credential.Allows(serviceID) {
		writeError(w, http.StatusNotFound, "service_not_found", "")
		return ServiceInfo{}, false
	}
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

// distributionPaused reports whether a pause command closed downstream
// distribution.
func (s *Server) distributionPaused() bool {
	return s.paused != nil && s.paused()
}

func (s *Server) authenticate(r *http.Request) (*Credential, bool) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		return nil, false
	}
	return s.credentials.Authenticate(token)
}

func bearerToken(value string) (string, bool) {
	const prefix = "bearer "
	if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(value[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

func snapshotETag(response adsync.SnapshotResponse) string {
	return `"` + response.SourceID + "-" + response.Epoch + "-" +
		strconv.FormatUint(response.Revision, 10) + "-" + response.StateHash.String() + `"`
}

// ifNoneMatch reports whether the request's If-None-Match header matches the
// current entity tag. Only an exact match (or *) counts; the contract forbids
// guessing that an older tag is close enough.
func ifNoneMatch(header, etag string) bool {
	if strings.TrimSpace(header) == "" {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == tagAny || candidate == etag {
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

const tagAny = "*"

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
