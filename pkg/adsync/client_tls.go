// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// clientTLS owns immutable material generations and every underlying connection,
// including channels and held polls. A reload closes the old generation before
// another connection can be registered. The tls.Config itself is never mutated.
type clientTLS struct {
	mu          sync.Mutex
	reloadMu    sync.Mutex
	configFiles Config
	host        string
	material    *clientTLSGeneration
	connections map[*clientTLSConn]struct{}
	closed      bool
}

type clientTLSGeneration struct {
	certificate tls.Certificate
	roots       *x509.CertPool
	caPEM       []byte
	crls        []*x509.RevocationList
	crlIssuers  map[*x509.RevocationList]crlIssuer
}

// The signing key distinguishes a CA rollover from an older CRL under the
// same subject. Do not rely on optional, untrusted CRL identity metadata.
type crlIssuer struct{ subject, publicKey string }

type clientTLSConn struct {
	net.Conn
	owner   *clientTLS
	timer   *time.Timer
	expires time.Time
}

func (c *clientTLSConn) Close() error {
	c.owner.mu.Lock()
	delete(c.owner.connections, c)
	if c.timer != nil {
		c.timer.Stop()
	}
	c.owner.mu.Unlock()
	return c.Conn.Close()
}

func newClientTLS(config Config, host string) (*clientTLS, error) {
	config.CRLFiles = append([]string(nil), config.CRLFiles...)
	config.CRLURLs = append([]string(nil), config.CRLURLs...)
	manager := &clientTLS{configFiles: config, host: host, connections: make(map[*clientTLSConn]struct{})}
	if err := manager.reload(context.Background()); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *clientTLS) load(ctx context.Context) (*clientTLSGeneration, error) {
	certificate, err := tls.LoadX509KeyPair(m.configFiles.ClientCertFile, m.configFiles.ClientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("adsync: load ClientCertFile/ClientKeyFile: %w", err)
	}
	now := time.Now()
	leaf := certificate.Leaf
	if leaf == nil {
		leaf, err = x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return nil, err
		}
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, fmt.Errorf("adsync: ClientCertFile is outside its validity window")
	}
	if len(leaf.ExtKeyUsage) > 0 {
		allowed := false
		for _, usage := range leaf.ExtKeyUsage {
			allowed = allowed || usage == x509.ExtKeyUsageClientAuth || usage == x509.ExtKeyUsageAny
		}
		if !allowed {
			return nil, fmt.Errorf("adsync: ClientCertFile requires clientAuth usage")
		}
	}
	certificate.Leaf = leaf
	caPEM, err := os.ReadFile(m.configFiles.CAFile)
	if err != nil {
		return nil, fmt.Errorf("adsync: read CAFile: %w", err)
	}
	roots := x509.NewCertPool() // Cascade trusts only the explicit CAFile.
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("adsync: CAFile contains no certificates")
	}
	generation := &clientTLSGeneration{certificate: certificate, roots: roots, caPEM: caPEM}
	for _, path := range m.configFiles.CRLFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("adsync: read CRLFiles: %w", err)
		}
		if err := generation.addCRL(data, now, m.configFiles.CRLMaxAge); err != nil {
			return nil, err
		}
	}
	for _, address := range m.configFiles.CRLURLs {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
			return nil, fmt.Errorf("adsync: CRLURLs must contain HTTPS URLs without credentials, query or fragment")
		}

		data, err := fetchRelayCRL(ctx, address, caPEM)
		if err != nil {
			return nil, err
		}

		if err := generation.addCRL(data, now, m.configFiles.CRLMaxAge); err != nil {
			return nil, err
		}
	}
	return generation, nil
}

func (g *clientTLSGeneration) addCRL(data []byte, now time.Time, maxAge time.Duration) error {
	if block, _ := pem.Decode(data); block != nil {
		data = block.Bytes
	}
	crl, err := x509.ParseRevocationList(data)
	if err != nil {
		return fmt.Errorf("adsync: parse relay CRL: %w", err)
	}
	// A delta CRL cannot provide complete revocation coverage.
	for _, extension := range crl.Extensions {
		if extension.Id.String() == "2.5.29.27" {
			return fmt.Errorf("adsync: delta CRLs are not supported")
		}
	}
	if err := freshCRL(crl, now, maxAge); err != nil {
		return err
	}
	trusted := false
	var identity crlIssuer
	remaining := g.caPEM
	for len(remaining) > 0 {
		block, rest := pem.Decode(remaining)
		remaining = rest
		if block == nil {
			break
		}
		issuer, err := x509.ParseCertificate(block.Bytes)
		if err == nil && bytes.Equal(crl.RawIssuer, issuer.RawSubject) && crl.CheckSignatureFrom(issuer) == nil {
			trusted = true
			identity = crlIssuer{subject: string(issuer.RawSubject), publicKey: string(issuer.RawSubjectPublicKeyInfo)}
			break
		}
	}
	if !trusted {
		return fmt.Errorf("adsync: relay CRL issuer/signature is not trusted by CAFile")
	}
	if g.crlIssuers == nil {
		g.crlIssuers = make(map[*x509.RevocationList]crlIssuer)
	}
	g.crlIssuers[crl] = identity
	g.crls = append(g.crls, crl)
	return nil
}

func crlExpiry(crl *x509.RevocationList, maxAge time.Duration) time.Time {
	if !crl.NextUpdate.IsZero() {
		return crl.NextUpdate
	}
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}
	return crl.ThisUpdate.Add(maxAge)
}

func freshCRL(crl *x509.RevocationList, now time.Time, maxAge time.Duration) error {
	if crl.ThisUpdate.After(now.Add(5*time.Minute)) || !now.Before(crlExpiry(crl, maxAge)) {
		return fmt.Errorf("adsync: relay CRL is outside its validity window")
	}
	return nil
}

func (m *clientTLS) reload(ctx context.Context) error {
	// Serialize readers of the files too, so a slower poll cannot overwrite a
	// newer SIGHUP generation. Invalid replacements keep the last good one.
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	next, err := m.load(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return net.ErrClosed
	}

	if m.material != nil {
		old := m.material
		// At least one CRL for each retained signing issuer must cover its old
		// watermark. Other fresh sources may still contain older CRLs for union
		// revocation coverage; comparing every pair would reject even a no-op.
		for _, oldCRL := range old.crls {
			sameIssuer, current := false, false
			for _, newCRL := range next.crls {
				if old.crlIssuers[oldCRL] != next.crlIssuers[newCRL] {
					continue
				}
				sameIssuer = true
				if !newCRL.ThisUpdate.Before(oldCRL.ThisUpdate) &&
					(oldCRL.Number == nil || (newCRL.Number != nil && newCRL.Number.Cmp(oldCRL.Number) >= 0)) {
					current = true
				}
			}
			if sameIssuer && !current {
				return fmt.Errorf("adsync: refusing an older relay CRL")
			}
		}

		if sameTLSMaterial(old, next) {
			return nil
		}
	}
	m.material = next
	for connection := range m.connections {
		connection.Conn.Close()
		if connection.timer != nil {
			connection.timer.Stop()
		}
		delete(m.connections, connection)
	}
	return nil
}

func sameTLSMaterial(a, b *clientTLSGeneration) bool {
	if !bytes.Equal(a.caPEM, b.caPEM) || len(a.certificate.Certificate) != len(b.certificate.Certificate) || len(a.crls) != len(b.crls) {
		return false
	}
	for i := range a.certificate.Certificate {
		if !bytes.Equal(a.certificate.Certificate[i], b.certificate.Certificate[i]) {
			return false
		}
	}
	for i := range a.crls {
		if !bytes.Equal(a.crls[i].Raw, b.crls[i].Raw) {
			return false
		}
	}
	return true
}

func (m *clientTLS) config() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Verification below uses the current explicit roots, host SAN and
		// serverAuth on EVERY handshake, including any resumed TLS connection.
		InsecureSkipVerify: true, // Verified by VerifyConnection against the URL host.
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.closed {
				return nil, net.ErrClosed
			}
			return &m.material.certificate, nil
		},
		VerifyConnection: m.verifyConnection,
	}
}

func (m *clientTLS) verifyConnection(state tls.ConnectionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return net.ErrClosed
	}
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("adsync: relay sent no certificate")
	}
	generation := m.material
	intermediates := x509.NewCertPool()
	for _, certificate := range state.PeerCertificates[1:] {
		intermediates.AddCert(certificate)
	}
	now := time.Now()
	chains, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
		Roots: generation.roots, Intermediates: intermediates, DNSName: m.host,
		CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return err
	}
	expiry := generation.certificate.Leaf.NotAfter
	for _, chain := range chains {
		chainExpiry := expiry
		valid := true
		for i, certificate := range chain {
			if certificate.NotAfter.Before(chainExpiry) {
				chainExpiry = certificate.NotAfter
			}
			if len(generation.crls) == 0 || i == len(chain)-1 {
				continue
			}
			covered := false
			for _, crl := range generation.crls {
				if !bytes.Equal(crl.RawIssuer, chain[i+1].RawSubject) || crl.CheckSignatureFrom(chain[i+1]) != nil || freshCRL(crl, now, m.configFiles.CRLMaxAge) != nil {
					continue
				}
				covered = true
				if end := crlExpiry(crl, m.configFiles.CRLMaxAge); end.Before(chainExpiry) {
					chainExpiry = end
				}
				for _, entry := range crl.RevokedCertificateEntries {
					if entry.SerialNumber.Cmp(certificate.SerialNumber) == 0 {
						return fmt.Errorf("adsync: relay certificate is revoked")
					}
				}
			}
			valid = valid && covered
		}
		if !valid {
			continue
		}
		// Conservatively expire connections at the earliest authenticated peer
		// deadline. Reload closes all of them, including handshakes in progress.
		for connection := range m.connections {
			if !chainExpiry.Before(connection.expires) {
				continue
			}
			connection.expires = chainExpiry
			if connection.timer != nil {
				connection.timer.Stop()
			}
			connection.timer = time.AfterFunc(time.Until(chainExpiry), func() { connection.Close() })
		}
		return nil
	}
	return fmt.Errorf("adsync: relay certificate lacks fresh CRL coverage")
}

func (m *clientTLS) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	m.mu.Lock()
	generation := m.material
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, net.ErrClosed
	}
	raw, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || generation != m.material {
		raw.Close()
		return nil, fmt.Errorf("adsync: TLS material changed while connecting")
	}
	connection := &clientTLSConn{Conn: raw, owner: m, expires: generation.certificate.Leaf.NotAfter}
	m.connections[connection] = struct{}{}
	connection.timer = time.AfterFunc(time.Until(generation.certificate.Leaf.NotAfter), func() { connection.Close() })
	return connection, nil
}

// ReloadTLS applies a complete valid generation and reconnects every upstream
// connection on change. It is also the explicit SIGHUP entry point.
func (c *Client) ReloadTLS() error {
	if c.tlsMaterial == nil {
		return nil
	}
	return c.tlsMaterial.reload(context.Background())
}

// WatchTLS polls every 30 seconds by default and listens for reload signals.
// The owner cancels ctx and joins this method before closing the client.
func (c *Client) WatchTLS(ctx context.Context, signals <-chan os.Signal) {
	if c.tlsMaterial == nil {
		return
	}
	interval := c.tlsMaterial.configFiles.TLSReloadInterval
	if interval == 0 {
		interval = 30 * time.Second
	}
	if refresh := c.tlsMaterial.configFiles.CRLRefreshInterval; refresh > 0 && refresh < interval {
		interval = refresh
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-signals:
			if !ok {
				signals = nil
				continue
			}
		case <-ticker.C:
		}
		if err := c.tlsMaterial.reload(ctx); err != nil {
			log.Warn("policy-source TLS reload failed; retaining the last valid generation")
		}
	}
}

// Close closes the owned upstream transports. The owner must stop WatchTLS first.
func (c *Client) Close() error {
	if c.tlsMaterial != nil {
		m := c.tlsMaterial
		m.mu.Lock()
		m.closed = true
		for connection := range m.connections {
			connection.Conn.Close()
			if connection.timer != nil {
				connection.timer.Stop()
			}
			delete(m.connections, connection)
		}
		m.mu.Unlock()
	}
	c.http.CloseIdleConnections()
	return nil
}

// fetchRelayCRL has a stall deadline, no response-size or total-transfer cap.
func fetchRelayCRL(parent context.Context, address string, caPEM []byte) ([]byte, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	watch := newStallWatch(defaultRequestTimeout, cancel)
	defer watch.stop()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("adsync: invalid CRLURLs request")
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	roots.AppendCertsFromPEM(caPEM)
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   defaultRequestTimeout,
		ResponseHeaderTimeout: defaultRequestTimeout,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{CheckRedirect: rejectRedirect, Transport: transport}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("adsync: fetch CRLURLs failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("adsync: fetch CRLURLs failed")
	}
	data, err := io.ReadAll(&progressReader{Reader: response.Body, watch: watch})
	if err != nil {
		return nil, fmt.Errorf("adsync: read CRLURLs failed")
	}
	return data, nil
}
