// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// DefaultDownstreamReloadInterval is how often the listener re-reads its
	// certificate, key, client CA and CRL files."
	DefaultDownstreamReloadInterval = 30 * time.Second
	// DefaultCRLRefreshInterval is how often HTTPS CRL sources are polled.
	DefaultCRLRefreshInterval = time.Hour
	// DefaultCRLMaxAge bounds a CRL's validity when it carries no NextUpdate.
	DefaultCRLMaxAge = 24 * time.Hour
	// crlClockSkew is how far a CRL's ThisUpdate may be in the future.
	crlClockSkew = 5 * time.Minute
)

// crlStallTimeout is the longest a CRL fetch may go without receiving bytes.
// It is a variable only so tests can shorten it.
var crlStallTimeout = 10 * time.Second

var (
	// deltaCRLIndicatorOID marks a delta CRL. The loader keeps only full
	// CRLs, so applying one would treat a partial update as the complete
	// revocation state of the issuer.
	deltaCRLIndicatorOID = asn1.ObjectIdentifier{2, 5, 29, 27}
	// issuingDistributionPointOID scopes a CRL to part of an issuer's
	// certificates: CA certificates only, user certificates only, some
	// revocation reasons, or one distribution point. Scope matching is not
	// implemented, so such a CRL must not count as fresh coverage for every
	// certificate the issuer signed.
	issuingDistributionPointOID = asn1.ObjectIdentifier{2, 5, 29, 28}
	// certificateIssuerEntryOID is the per-entry extension of an indirect CRL.
	// It lets one CRL list revocations for certificates issued by another CA;
	// matching such an entry by serial alone would attribute it to this CRL's
	// issuer.
	certificateIssuerEntryOID = asn1.ObjectIdentifier{2, 5, 29, 29}
	// reasonCodeEntryOID and invalidityDateEntryOID are the standard per-entry
	// annotations. Neither changes which certificate a serial belongs to.
	reasonCodeEntryOID     = asn1.ObjectIdentifier{2, 5, 29, 21}
	invalidityDateEntryOID = asn1.ObjectIdentifier{2, 5, 29, 24}
)

// errRedirect is returned for any redirect during a CRL fetch: the configured
// URL is the only permitted location.
var errRedirect = errors.New("adsproxy: CRL fetch redirected")

// ListenerConfig configures the dedicated downstream TLS listener.
type ListenerConfig struct {
	// Endpoint is the TCP address the listener binds, for example
	// "0.0.0.0:7443".
	Endpoint string
	// CertPath and KeyPath are the relay's TLS server certificate and key.
	CertPath string
	KeyPath  string
	// ClientCAPath is the PEM bundle of CAs trusted to issue downstream client
	// certificates. Every CA in it needs fresh CRL coverage.
	ClientCAPath string
	// CRLFiles and CRLURLs are the full X.509 CRL sources (PEM or DER files,
	// and HTTPS URLs).
	CRLFiles []string
	CRLURLs  []string
	// CRLRefreshInterval is how often HTTPS sources are fetched. Zero uses
	// DefaultCRLRefreshInterval.
	CRLRefreshInterval time.Duration
	// CRLMaxAge bounds a CRL without NextUpdate. Zero uses DefaultCRLMaxAge.
	CRLMaxAge time.Duration
	// ReloadInterval is how often the TLS files are re-read. Zero uses
	// DefaultDownstreamReloadInterval.
	ReloadInterval time.Duration
	// Now supplies the clock for certificate and CRL validity checks. Nil uses
	// time.Now.
	Now func() time.Time
}

// Listener serves one handler on a dedicated TLS listener that requires and
// verifies a downstream client certificate. Its certificate, key, client CA
// and CRLs reload without restarting: a poll runs every ReloadInterval, HTTPS
// CRLs are fetched every CRLRefreshInterval, and Reload is the SIGHUP entry
// point. A reload that changes the client CA or the CRL set closes every
// established connection, so keep-alive connections, WebSockets and held polls
// cannot outlive revoked or expired material.
type Listener struct {
	config   ListenerConfig
	handler  http.Handler
	now      func() time.Time
	reloadMu sync.Mutex

	mu          sync.Mutex
	generation  *tlsGeneration
	connections map[*listenerConn]struct{}
	sources     map[string]*crlSource
	closed      bool
	httpServer  *http.Server
}

// crlSource is one configured CRL origin and its last good bytes. Keeping the
// last good bytes is what lets an unreachable URL or a transiently unreadable
// file keep a fresh cached CRL in force.
type crlSource struct {
	address      string
	isURL        bool
	etag         string
	lastModified string
	data         []byte
}

// tlsGeneration is one complete, immutable set of listener TLS material.
type tlsGeneration struct {
	certificate tls.Certificate
	caPEM       []byte
	roots       *x509.CertPool
	crls        []*crlEntry
}

// crlEntry is one parsed, issuer-verified CRL.
type crlEntry struct {
	raw        []byte
	issuer     issuerKey
	source     string
	number     *big.Int
	thisUpdate time.Time
	nextUpdate time.Time
	revoked    map[string]struct{}
}

type issuerKey struct {
	subject   string
	publicKey string
	name      string
}

func issuerKeyOf(certificate *x509.Certificate) issuerKey {
	return issuerKey{
		subject:   string(certificate.RawSubject),
		publicKey: string(certificate.RawSubjectPublicKeyInfo),
		name:      subjectName(certificate),
	}
}

// NewListener validates the configuration, loads the initial material and
// fails closed when a required certificate or CRL is unusable.
func NewListener(config ListenerConfig, handler http.Handler) (*Listener, error) {
	if handler == nil {
		return nil, fmt.Errorf("%w: a handler is required", ErrConfig)
	}
	if config.Endpoint == "" {
		return nil, fmt.Errorf("%w: downstreamEndpoint is required", ErrConfig)
	}
	if config.CertPath == "" || config.KeyPath == "" {
		return nil, fmt.Errorf("%w: downstreamCertPath and downstreamKeyPath are required", ErrTLSMaterial)
	}
	if config.ClientCAPath == "" {
		return nil, fmt.Errorf("%w: downstreamClientCAPath is required", ErrTLSMaterial)
	}
	if config.ReloadInterval < 0 {
		return nil, fmt.Errorf("%w: downstreamReloadInterval must not be negative", ErrConfig)
	}
	if config.ReloadInterval == 0 {
		config.ReloadInterval = DefaultDownstreamReloadInterval
	}
	if config.CRLRefreshInterval < 0 {
		return nil, fmt.Errorf("%w: downstreamCRLRefreshInterval must not be negative", ErrConfig)
	}
	if config.CRLRefreshInterval == 0 {
		config.CRLRefreshInterval = DefaultCRLRefreshInterval
	}
	if config.CRLMaxAge < 0 {
		return nil, fmt.Errorf("%w: downstreamCRLMaxAge must not be negative", ErrConfig)
	}
	if config.CRLMaxAge == 0 {
		config.CRLMaxAge = DefaultCRLMaxAge
	}
	config.CRLFiles = append([]string(nil), config.CRLFiles...)
	config.CRLURLs = append([]string(nil), config.CRLURLs...)
	if len(config.CRLFiles) == 0 && len(config.CRLURLs) == 0 {
		return nil, fmt.Errorf("%w: at least one of downstreamCRLFiles and downstreamCRLURLs is required",
			ErrCRL)
	}

	sources := make(map[string]*crlSource, len(config.CRLFiles)+len(config.CRLURLs))
	for _, path := range config.CRLFiles {
		if path == "" {
			return nil, fmt.Errorf("%w: downstreamCRLFiles contains an empty path", ErrCRL)
		}
		address := "file:" + path
		if _, duplicate := sources[address]; duplicate {
			return nil, fmt.Errorf("%w: duplicate CRL source %s", ErrCRL, path)
		}
		sources[address] = &crlSource{address: path}
	}
	for _, address := range config.CRLURLs {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
			return nil, fmt.Errorf("%w: downstreamCRLURLs must contain HTTPS URLs without credentials, query or fragment", ErrCRL)
		}
		key := "url:" + address
		if _, duplicate := sources[key]; duplicate {
			return nil, fmt.Errorf("%w: duplicate CRL source %s", ErrCRL, address)
		}
		sources[key] = &crlSource{address: address, isURL: true}
	}

	now := config.Now
	if now == nil {
		now = time.Now
	}
	listener := &Listener{
		config:      config,
		handler:     handler,
		now:         now,
		connections: make(map[*listenerConn]struct{}),
		sources:     sources,
	}
	if err := listener.reload(true); err != nil {
		return nil, err
	}
	return listener, nil
}

// Reload re-reads every local file and fetches HTTPS CRL sources. Invalid
// replacement material keeps the last valid generation and returns the error.
func (l *Listener) Reload() error {
	return l.reload(true)
}

func (l *Listener) reload(fetchRemote bool) error {
	// Serialize reloads, so a slower poll cannot overwrite a newer SIGHUP
	// generation.
	l.reloadMu.Lock()
	defer l.reloadMu.Unlock()
	next, err := l.load(fetchRemote)
	if err != nil {
		return err
	}
	return l.install(next)
}

func (l *Listener) load(fetchRemote bool) (*tlsGeneration, error) {
	now := l.now()
	certificate, err := loadServerCertificate(l.config.CertPath, l.config.KeyPath, now)
	if err != nil {
		return nil, err
	}
	caPEM, caCerts, err := loadClientCA(l.config.ClientCAPath)
	if err != nil {
		return nil, err
	}
	// The CRL endpoints are trusted through the current client CA bundle, so a
	// rotated bundle must apply to this reload's fetches, not only after a
	// restart.
	crlHTTP := newCRLClient(caPEM)
	caKeys := make(map[issuerKey]struct{}, len(caCerts))
	for _, ca := range caCerts {
		caKeys[issuerKeyOf(ca)] = struct{}{}
	}

	var candidates []*crlEntry
	for _, source := range l.sortedSources() {
		// A URL is fetched on the CRL cadence. It is also fetched by a local
		// poll when it has never succeeded, so a startup that failed to reach
		// it can recover without waiting a full interval.
		fetch := !source.isURL || fetchRemote || source.data == nil
		if !fetch {
			continue
		}
		fetched, err := l.sourceData(source, fetch, crlHTTP)
		if err != nil {
			return nil, err
		}
		entries, err := parseCRLs(fetched.data, source.address, now, caCerts)
		if err != nil {
			return nil, err
		}
		// Only a usable response replaces the cached copy, so a malformed
		// refresh cannot displace the last good CRL.
		source.data = fetched.data
		source.etag = fetched.etag
		source.lastModified = fetched.lastModified
		candidates = append(candidates, entries...)
	}

	l.mu.Lock()
	previous := l.generation
	l.mu.Unlock()
	crls := mergeCRLs(previous, candidates, caKeys)
	generation := &tlsGeneration{
		certificate: certificate,
		caPEM:       caPEM,
		roots:       rootsFor(caPEM),
		crls:        crls,
	}
	for _, ca := range caCerts {
		if generation.freshCRLs(issuerKeyOf(ca), now, l.config.CRLMaxAge) > 0 {
			continue
		}
		description := l.describeSources()
		if previous == nil {
			return nil, fmt.Errorf("%w: no fresh CRL covers issuer %q from %s",
				ErrCRL, subjectName(ca), description)
		}
		log.Warnf("adsproxy: no fresh CRL covers issuer %q from %s; new connections for it are rejected",
			subjectName(ca), description)
	}
	return generation, nil
}

func (l *Listener) sortedSources() []*crlSource {
	keys := make([]string, 0, len(l.sources))
	for key := range l.sources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sources := make([]*crlSource, 0, len(keys))
	for _, key := range keys {
		sources = append(sources, l.sources[key])
	}
	return sources
}

func (l *Listener) describeSources() string {
	files := make([]string, 0, len(l.config.CRLFiles))
	files = append(files, l.config.CRLFiles...)
	urls := make([]string, 0, len(l.config.CRLURLs))
	urls = append(urls, l.config.CRLURLs...)
	parts := make([]string, 0, len(files)+len(urls))
	for _, path := range files {
		parts = append(parts, "file "+path)
	}
	for _, address := range urls {
		parts = append(parts, "URL "+address)
	}
	return fmt.Sprintf("configured sources (%s)", joinComma(parts))
}

// crlSourceData is one source's current bytes and the validators for them.
type crlSourceData struct {
	data         []byte
	etag         string
	lastModified string
}

// sourceData returns one source's current bytes. Files are re-read on every
// call; URLs use a conditional fetch. A source that cannot be read keeps its
// last good bytes, so a transient failure does not disturb a fresh cached CRL.
func (l *Listener) sourceData(source *crlSource, fetchRemote bool, client *http.Client) (crlSourceData, error) {
	cached := crlSourceData{data: source.data, etag: source.etag, lastModified: source.lastModified}
	if !source.isURL {
		data, err := os.ReadFile(source.address)
		if err == nil {
			return crlSourceData{data: data}, nil
		}
		if source.data != nil {
			log.Warnf("adsproxy: CRL file %s is unreadable; keeping the last good copy", source.address)
			return cached, nil
		}
		return crlSourceData{}, fmt.Errorf("%w: read CRL file %s: %v", ErrCRL, source.address, err)
	}
	if !fetchRemote {
		if source.data == nil {
			return crlSourceData{}, fmt.Errorf("%w: CRL URL %s has not been fetched yet", ErrCRL, source.address)
		}
		return cached, nil
	}
	data, etag, lastModified, err := l.fetchCRL(source, client)
	if err != nil {
		if source.data != nil {
			log.Warnf("adsproxy: CRL URL %s is unavailable; keeping the last good copy", source.address)
			return cached, nil
		}
		return crlSourceData{}, err
	}
	return crlSourceData{data: data, etag: etag, lastModified: lastModified}, nil
}

func (l *Listener) fetchCRL(source *crlSource, client *http.Client) ([]byte, string, string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.address, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: invalid CRL URL %s", ErrCRL, source.address)
	}
	if source.etag != "" {
		request.Header.Set("If-None-Match", source.etag)
	}
	if source.lastModified != "" {
		request.Header.Set("If-Modified-Since", source.lastModified)
	}
	watch := newStallWatch(crlStallTimeout, cancel)
	defer watch.stop()
	response, err := client.Do(request)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: fetch CRL URL %s: %v", ErrCRL, source.address, err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotModified:
		if source.data == nil {
			return nil, "", "", fmt.Errorf("%w: CRL URL %s answered 304 without a cached copy", ErrCRL, source.address)
		}
		return source.data, source.etag, source.lastModified, nil
	case http.StatusOK:
	default:
		return nil, "", "", fmt.Errorf("%w: CRL URL %s answered %d", ErrCRL, source.address, response.StatusCode)
	}
	data, err := io.ReadAll(&progressReader{reader: response.Body, watch: watch})
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: read CRL URL %s: %v", ErrCRL, source.address, err)
	}
	watch.progress()
	return data, response.Header.Get("ETag"), response.Header.Get("Last-Modified"), nil
}

// install atomically switches to a new generation. A changed client CA or CRL
// set invalidates every established connection, because a keep-alive
// connection, WebSocket or held poll must not outlive revoked or expired
// client material.
func (l *Listener) install(next *tlsGeneration) error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return net.ErrClosed
	}
	previous := l.generation
	if previous != nil && sameMaterial(previous, next) {
		// Installing an identical generation would make an in-flight
		// handshake look stale for no reason.
		l.mu.Unlock()
		return nil
	}
	l.generation = next
	var stale []*listenerConn
	if previous != nil && (!bytes.Equal(previous.caPEM, next.caPEM) || !sameCRLs(previous.crls, next.crls)) {
		stale = make([]*listenerConn, 0, len(l.connections))
		for connection := range l.connections {
			stale = append(stale, connection)
		}
	}
	l.mu.Unlock()
	for _, connection := range stale {
		_ = connection.Close()
	}
	return nil
}

// sameMaterial reports whether a reload found exactly the material already in
// force. Server certificate bytes, the client CA bundle and the CRL set must
// all match.
func sameMaterial(left, right *tlsGeneration) bool {
	return sameCertificate(left.certificate, right.certificate) &&
		bytes.Equal(left.caPEM, right.caPEM) &&
		sameCRLs(left.crls, right.crls)
}

func sameCertificate(left, right tls.Certificate) bool {
	if len(left.Certificate) != len(right.Certificate) {
		return false
	}
	for index := range left.Certificate {
		if !bytes.Equal(left.Certificate[index], right.Certificate[index]) {
			return false
		}
	}
	return true
}

// Serve accepts TLS connections on listener and serves them until Close or the
// listener fails. It never returns http.ErrServerClosed.
func (l *Listener) Serve(listener net.Listener) error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		_ = listener.Close()
		return net.ErrClosed
	}
	server := &http.Server{
		Handler:           l.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
	}
	l.httpServer = server
	l.mu.Unlock()
	tracked := &trackingListener{Listener: listener, owner: l}
	tlsListener := tls.NewListener(tracked, l.tlsConfig())
	err := server.Serve(tlsListener)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// ListenAndServe binds the configured endpoint and serves it.
func (l *Listener) ListenAndServe() error {
	listener, err := net.Listen("tcp", l.config.Endpoint)
	if err != nil {
		return fmt.Errorf("%w: listen on %s: %v", ErrConfig, l.config.Endpoint, err)
	}
	return l.Serve(listener)
}

// Watch polls local TLS material every ReloadInterval, HTTPS CRLs every
// CRLRefreshInterval, and reloads both immediately for each signal. A failed
// reload keeps the last valid generation.
func (l *Listener) Watch(ctx context.Context, signals <-chan os.Signal) {
	tlsTicker := time.NewTicker(l.config.ReloadInterval)
	defer tlsTicker.Stop()
	crlTicker := time.NewTicker(l.config.CRLRefreshInterval)
	defer crlTicker.Stop()
	for {
		fetchRemote := false
		select {
		case <-ctx.Done():
			return
		case _, ok := <-signals:
			if !ok {
				signals = nil
				continue
			}
			fetchRemote = true
		case <-tlsTicker.C:
		case <-crlTicker.C:
			fetchRemote = true
		}
		if err := l.reload(fetchRemote); err != nil {
			log.Warnf("adsproxy: downstream TLS reload failed; keeping the last valid generation")
		}
	}
}

// Close stops serving and closes every established connection. It is safe to
// call more than once.
func (l *Listener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	server := l.httpServer
	connections := make([]*listenerConn, 0, len(l.connections))
	for connection := range l.connections {
		connections = append(connections, connection)
	}
	l.mu.Unlock()
	if server != nil {
		_ = server.Close()
	}
	for _, connection := range connections {
		_ = connection.Close()
	}
	return nil
}

func (l *Listener) track(connection *listenerConn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	l.connections[connection] = struct{}{}
	return true
}

func (l *Listener) untrack(connection *listenerConn) {
	l.mu.Lock()
	delete(l.connections, connection)
	l.mu.Unlock()
}

// tlsConfig is the base configuration. The per-handshake configuration from
// GetConfigForClient supplies the current client CA, server certificate and
// CRL verification, so a live tls.Config is never mutated.
func (l *Listener) tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion:             tls.VersionTLS12,
		SessionTicketsDisabled: true,
		ClientAuth:             tls.RequireAndVerifyClientCert,
		GetConfigForClient:     l.configForClient,
	}
}

func (l *Listener) configForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, net.ErrClosed
	}
	generation := l.generation
	connection := trackedConnection(hello.Conn)
	l.mu.Unlock()
	return &tls.Config{
		MinVersion:             tls.VersionTLS12,
		SessionTicketsDisabled: true,
		ClientAuth:             tls.RequireAndVerifyClientCert,
		ClientCAs:              generation.roots,
		Certificates:           []tls.Certificate{generation.certificate},
		VerifyConnection: func(state tls.ConnectionState) error {
			return l.verifyConnection(generation, connection, state)
		},
	}, nil
}

// trackedConnection returns the tracked connection behind a handshake. A
// caller using the tls.Config without this listener gets nil and normal
// verification.
func trackedConnection(connection net.Conn) *listenerConn {
	tracked, ok := connection.(*listenerConn)
	if !ok {
		return nil
	}
	return tracked
}

// verifyConnection re-checks the generation, verifies fresh CRL coverage for
// every non-root certificate in the chain and schedules the connection to
// close when any certificate or CRL covering it expires. A handshake that
// started under material a reload just replaced is rejected rather than
// admitted.
func (l *Listener) verifyConnection(generation *tlsGeneration, connection *listenerConn, state tls.ConnectionState) error {
	l.mu.Lock()
	current := l.generation
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return net.ErrClosed
	}
	if current != generation {
		return ErrStaleTLSGeneration
	}
	if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 {
		return fmt.Errorf("%w: client certificate has no verified chain", ErrTLSMaterial)
	}
	deadline, err := generation.verifyClient(state.VerifiedChains[0], l.now(), l.config.CRLMaxAge)
	if err != nil {
		return err
	}
	if connection != nil {
		connection.setExpiry(deadline)
	}
	return nil
}

// verifyClient enforces fresh revocation coverage for each non-root
// certificate in the verified chain and returns the earliest expiry of the
// client certificate, the server certificate and every covering CRL.
func (g *tlsGeneration) verifyClient(chain []*x509.Certificate, now time.Time, maxAge time.Duration) (time.Time, error) {
	if len(chain) == 0 {
		return time.Time{}, fmt.Errorf("%w: empty chain", ErrTLSMaterial)
	}
	deadline := chain[0].NotAfter
	if g.certificate.Leaf != nil && g.certificate.Leaf.NotAfter.Before(deadline) {
		deadline = g.certificate.Leaf.NotAfter
	}
	for index := 0; index+1 < len(chain); index++ {
		leaf, issuer := chain[index], chain[index+1]
		if leaf.NotAfter.Before(deadline) {
			deadline = leaf.NotAfter
		}
		key := issuerKeyOf(issuer)
		fresh := 0
		for _, crl := range g.crls {
			if crl.issuer != key || !crl.fresh(now, maxAge) {
				continue
			}
			fresh++
			if expiry := crl.expiry(maxAge); expiry.Before(deadline) {
				deadline = expiry
			}
			if crl.revokes(leaf) {
				return time.Time{}, fmt.Errorf("%w: serial %s under issuer %q",
					ErrRevoked, leaf.SerialNumber, subjectName(issuer))
			}
		}
		if fresh == 0 {
			return time.Time{}, fmt.Errorf("%w: issuer %q", ErrNoFreshCRL, subjectName(issuer))
		}
	}
	return deadline, nil
}

func (g *tlsGeneration) freshCRLs(issuer issuerKey, now time.Time, maxAge time.Duration) int {
	fresh := 0
	for _, crl := range g.crls {
		if crl.issuer == issuer && crl.fresh(now, maxAge) {
			fresh++
		}
	}
	return fresh
}

func (c *crlEntry) expiry(maxAge time.Duration) time.Time {
	if !c.nextUpdate.IsZero() {
		return c.nextUpdate
	}
	return c.thisUpdate.Add(maxAge)
}

func (c *crlEntry) fresh(now time.Time, maxAge time.Duration) bool {
	if c.thisUpdate.After(now.Add(crlClockSkew)) {
		return false
	}
	return now.Before(c.expiry(maxAge))
}

func (c *crlEntry) revokes(certificate *x509.Certificate) bool {
	if certificate.SerialNumber == nil {
		return false
	}
	_, revoked := c.revoked[certificate.SerialNumber.String()]
	return revoked
}

// listenerConn is one accepted connection. It remembers the TLS generation it
// handshook with and closes itself when the client certificate, the server
// certificate or a covering CRL expires.
type listenerConn struct {
	net.Conn
	listener *Listener

	mu      sync.Mutex
	expires time.Time
	timer   *time.Timer
	closed  bool
}

// setExpiry schedules the connection to close at the earliest authenticated
// deadline. A later call can only move the deadline earlier.
func (c *listenerConn) setExpiry(deadline time.Time) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if !c.timerDeadlineLocked(deadline) {
		c.mu.Unlock()
		return
	}
	wait := time.Until(deadline)
	if wait <= 0 {
		c.mu.Unlock()
		go func() { _ = c.Close() }()
		return
	}
	c.expires = deadline
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(wait, func() { _ = c.Close() })
	c.mu.Unlock()
}

// timerDeadlineLocked reports whether deadline should replace the current
// timer: only an earlier deadline does.
func (c *listenerConn) timerDeadlineLocked(deadline time.Time) bool {
	return c.timer == nil || deadline.Before(c.expires)
}

func (c *listenerConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()
	c.listener.untrack(c)
	return c.Conn.Close()
}

// trackingListener registers every accepted connection so a reload can close
// the ones whose material changed.
type trackingListener struct {
	net.Listener
	owner *Listener
}

func (l *trackingListener) Accept() (net.Conn, error) {
	connection, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tracked := &listenerConn{Conn: connection, listener: l.owner}
	if !l.owner.track(tracked) {
		_ = connection.Close()
		return nil, net.ErrClosed
	}
	return tracked, nil
}

// loadServerCertificate loads and validates the relay's TLS certificate. An
// expired, not-yet-valid or non-serverAuth certificate is invalid replacement
// material.
func loadServerCertificate(certPath, keyPath string, now time.Time) (tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: load %s and %s: %v", ErrTLSMaterial, certPath, keyPath, err)
	}
	if len(certificate.Certificate) == 0 {
		return tls.Certificate{}, fmt.Errorf("%w: %s contains no certificate", ErrTLSMaterial, certPath)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: parse %s: %v", ErrTLSMaterial, certPath, err)
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return tls.Certificate{}, fmt.Errorf("%w: %s is outside its validity window", ErrTLSMaterial, certPath)
	}
	if !hasUsage(leaf, x509.ExtKeyUsageServerAuth) {
		return tls.Certificate{}, fmt.Errorf("%w: %s lacks serverAuth usage", ErrTLSMaterial, certPath)
	}
	certificate.Leaf = leaf
	return certificate, nil
}

func loadClientCA(path string) ([]byte, []*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read %s: %v", ErrTLSMaterial, path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, nil, fmt.Errorf("%w: %s contains no certificates", ErrTLSMaterial, path)
	}
	var certificates []*x509.Certificate
	remaining := data
	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		remaining = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse %s: %v", ErrTLSMaterial, path, err)
		}
		if !certificate.BasicConstraintsValid || !certificate.IsCA {
			return nil, nil, fmt.Errorf("%w: %s contains a certificate that is not a CA", ErrTLSMaterial, path)
		}
		certificates = append(certificates, certificate)
	}
	if len(certificates) == 0 {
		return nil, nil, fmt.Errorf("%w: %s contains no certificates", ErrTLSMaterial, path)
	}
	return data, certificates, nil
}

func rootsFor(caPEM []byte) *x509.CertPool {
	// AppendCertsFromPEM already succeeded when the same bundle was loaded.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	return pool
}

func hasUsage(certificate *x509.Certificate, usage x509.ExtKeyUsage) bool {
	if len(certificate.ExtKeyUsage) == 0 {
		// An empty EKU list means any usage, per the X.509 rules Go applies.
		return true
	}
	for _, candidate := range certificate.ExtKeyUsage {
		if candidate == usage || candidate == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}

func parseCRLs(data []byte, source string, now time.Time, caCerts []*x509.Certificate) ([]*crlEntry, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: %s is empty", ErrCRL, source)
	}
	var lists []*x509.RevocationList
	sawPEM := false
	remaining := data
	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		sawPEM = true
		remaining = rest
		if block.Type != "X509 CRL" && block.Type != "CRL" {
			continue
		}
		crl, err := x509.ParseRevocationList(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: parse %s: %v", ErrCRL, source, err)
		}
		lists = append(lists, crl)
	}
	if !sawPEM {
		crl, err := x509.ParseRevocationList(data)
		if err != nil {
			return nil, fmt.Errorf("%w: parse %s: %v", ErrCRL, source, err)
		}
		lists = append(lists, crl)
	}
	if len(lists) == 0 {
		return nil, fmt.Errorf("%w: %s contains no CRL", ErrCRL, source)
	}
	entries := make([]*crlEntry, 0, len(lists))
	for _, crl := range lists {
		entry, err := buildCRLEntry(crl, source, now, caCerts)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// checkCRLExtensions rejects a CRL whose extensions would change how its
// revoked serials apply. Go's CRL parser reports critical extensions but does
// not fail on them, so an unsupported one could silently restrict the CRL's
// scope while verifyClient still counts it as full coverage for the issuer.
func checkCRLExtensions(extensions []pkix.Extension, source string) error {
	for _, extension := range extensions {
		switch {
		case extension.Id.Equal(deltaCRLIndicatorOID):
			return fmt.Errorf("%w: %s contains a delta CRL", ErrCRL, source)
		case extension.Id.Equal(issuingDistributionPointOID):
			return fmt.Errorf("%w: %s has an issuing distribution point", ErrCRL, source)
		case extension.Critical:
			// The extensions this loader understands are never required to be
			// critical, so anything marked critical is unsupported.
			return fmt.Errorf("%w: %s has an unsupported critical extension %s",
				ErrCRL, source, extension.Id)
		}
	}
	return nil
}

// checkRevokedEntryExtensions rejects a revoked entry whose extensions would
// change which certificate its serial belongs to. The standard reasonCode and
// invalidityDate annotations are accepted; any other entry extension, notably
// certificateIssuer from an indirect CRL, is rejected rather than attributed
// to the wrong issuer.
func checkRevokedEntryExtensions(extensions []pkix.Extension, source string) error {
	for _, extension := range extensions {
		switch {
		case extension.Id.Equal(reasonCodeEntryOID), extension.Id.Equal(invalidityDateEntryOID):
		case extension.Id.Equal(certificateIssuerEntryOID):
			return fmt.Errorf("%w: %s has an indirect revoked entry", ErrCRL, source)
		default:
			return fmt.Errorf("%w: %s has an unsupported revoked-entry extension %s",
				ErrCRL, source, extension.Id)
		}
	}
	return nil
}

func buildCRLEntry(crl *x509.RevocationList, source string, now time.Time, caCerts []*x509.Certificate) (*crlEntry, error) {
	if err := checkCRLExtensions(crl.Extensions, source); err != nil {
		return nil, err
	}
	if crl.ThisUpdate.After(now.Add(crlClockSkew)) {
		return nil, fmt.Errorf("%w: %s has a ThisUpdate in the future", ErrCRL, source)
	}
	var issuer *x509.Certificate
	for _, candidate := range caCerts {
		if !bytes.Equal(crl.RawIssuer, candidate.RawSubject) {
			continue
		}
		if err := crl.CheckSignatureFrom(candidate); err != nil {
			continue
		}
		issuer = candidate
		break
	}
	if issuer == nil {
		return nil, fmt.Errorf("%w: %s was not signed by a client CA", ErrCRL, source)
	}
	revoked := make(map[string]struct{}, len(crl.RevokedCertificateEntries))
	for _, entry := range crl.RevokedCertificateEntries {
		if err := checkRevokedEntryExtensions(entry.Extensions, source); err != nil {
			return nil, err
		}
		if entry.SerialNumber != nil {
			revoked[entry.SerialNumber.String()] = struct{}{}
		}
	}
	return &crlEntry{
		raw:        append([]byte(nil), crl.Raw...),
		issuer:     issuerKeyOf(issuer),
		source:     source,
		number:     crl.Number,
		thisUpdate: crl.ThisUpdate,
		nextUpdate: crl.NextUpdate,
		revoked:    revoked,
	}, nil
}

// mergeCRLs combines the CRLs already accepted with the ones a reload just
// read. Within one source an older CRL never replaces a newer one; across
// sources every current CRL for an issuer is kept, so their revoked serials are
// unioned. An unreachable source therefore does not drop a still-fresh cached
// CRL, and a corrected CRL from the configured source takes effect.
func mergeCRLs(previous *tlsGeneration, candidates []*crlEntry, caKeys map[issuerKey]struct{}) []*crlEntry {
	type sourceKey struct {
		issuer issuerKey
		source string
	}
	group := func(entries []*crlEntry) map[sourceKey][]*crlEntry {
		grouped := make(map[sourceKey][]*crlEntry)
		for _, entry := range entries {
			key := sourceKey{issuer: entry.issuer, source: entry.source}
			grouped[key] = append(grouped[key], entry)
		}
		return grouped
	}
	var previousEntries []*crlEntry
	if previous != nil {
		for _, entry := range previous.crls {
			if _, trusted := caKeys[entry.issuer]; trusted {
				previousEntries = append(previousEntries, entry)
			}
		}
	}
	previousGroups := group(previousEntries)
	candidateGroups := group(candidates)
	keys := make([]sourceKey, 0, len(previousGroups)+len(candidateGroups))
	seenKey := make(map[sourceKey]struct{}, len(previousGroups)+len(candidateGroups))
	for key := range previousGroups {
		keys = append(keys, key)
		seenKey[key] = struct{}{}
	}
	for key := range candidateGroups {
		if _, duplicate := seenKey[key]; !duplicate {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].source != keys[right].source {
			return keys[left].source < keys[right].source
		}
		if keys[left].issuer.name != keys[right].issuer.name {
			return keys[left].issuer.name < keys[right].issuer.name
		}
		return keys[left].issuer.publicKey < keys[right].issuer.publicKey
	})

	var merged []*crlEntry
	seenRaw := make(map[string]struct{}, len(previousEntries)+len(candidates))
	add := func(entries []*crlEntry) {
		for _, entry := range entries {
			key := string(entry.raw)
			if _, duplicate := seenRaw[key]; duplicate {
				continue
			}
			seenRaw[key] = struct{}{}
			merged = append(merged, entry)
		}
	}
	for _, key := range keys {
		previousGroup, candidateGroup := previousGroups[key], candidateGroups[key]
		switch {
		case len(candidateGroup) == 0:
			add(previousGroup)
		case len(previousGroup) == 0:
			add(candidateGroup)
		case crlNewer(newestCRL(candidateGroup), newestCRL(previousGroup)):
			add(candidateGroup)
		default:
			add(previousGroup)
			log.Warnf("adsproxy: ignoring a CRL that is not newer for issuer %q from %s",
				key.issuer.name, key.source)
		}
	}
	return merged
}

func newestCRL(entries []*crlEntry) *crlEntry {
	newest := entries[0]
	for _, entry := range entries[1:] {
		if crlNewer(entry, newest) {
			newest = entry
		}
	}
	return newest
}

func crlNewer(a, b *crlEntry) bool {
	if a.number != nil && b.number != nil {
		if comparison := a.number.Cmp(b.number); comparison != 0 {
			return comparison > 0
		}
	}
	return a.thisUpdate.After(b.thisUpdate)
}

func sameCRLs(left, right []*crlEntry) bool {
	if len(left) != len(right) {
		return false
	}
	leftRaw := make([][]byte, len(left))
	rightRaw := make([][]byte, len(right))
	for index := range left {
		leftRaw[index] = left[index].raw
		rightRaw[index] = right[index].raw
	}
	sort.Slice(leftRaw, func(i, j int) bool { return bytes.Compare(leftRaw[i], leftRaw[j]) < 0 })
	sort.Slice(rightRaw, func(i, j int) bool { return bytes.Compare(rightRaw[i], rightRaw[j]) < 0 })
	for index := range leftRaw {
		if !bytes.Equal(leftRaw[index], rightRaw[index]) {
			return false
		}
	}
	return true
}

func subjectName(certificate *x509.Certificate) string {
	if len(certificate.Subject.String()) == 0 {
		return "a certificate authority with an empty subject"
	}
	return certificate.Subject.String()
}

func joinComma(parts []string) string {
	result := ""
	for index, part := range parts {
		if index > 0 {
			result += ", "
		}
		result += part
	}
	return result
}

func newCRLClient(caPEM []byte) *http.Client {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	roots.AppendCertsFromPEM(caPEM)
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   crlStallTimeout,
		ResponseHeaderTimeout: crlStallTimeout,
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirect
		},
	}
}

// stallWatch cancels a fetch that stops making progress.
type stallWatch struct {
	timeout  time.Duration
	timer    *time.Timer
	cancel   context.CancelFunc
	stopOnce sync.Once
}

func newStallWatch(timeout time.Duration, cancel context.CancelFunc) *stallWatch {
	watch := &stallWatch{timeout: timeout, cancel: cancel}
	watch.timer = time.AfterFunc(timeout, cancel)
	return watch
}

func (w *stallWatch) progress() {
	w.timer.Reset(w.timeout)
}

func (w *stallWatch) stop() {
	w.stopOnce.Do(func() {
		w.timer.Stop()
	})
}

// progressReader resets the stall watchdog whenever bytes arrive. There is no
// total transfer or response size limit.
type progressReader struct {
	reader io.Reader
	watch  *stallWatch
}

func (r *progressReader) Read(p []byte) (int, error) {
	read, err := r.reader.Read(p)
	if read > 0 {
		r.watch.progress()
	}
	return read, err
}
