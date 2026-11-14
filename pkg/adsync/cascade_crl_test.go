// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func (p *cascadePKI) crl(t *testing.T, serial int64, number int64, start, end time.Time, delta bool) []byte {
	t.Helper()
	template := &x509.RevocationList{Number: big.NewInt(number), ThisUpdate: start, NextUpdate: end}
	if serial != 0 {
		template.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(serial), RevocationTime: time.Now().Add(-time.Minute)}}
	}
	if delta {
		template.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 27}, Critical: true, Value: []byte{2, 1, 1}}}
	}
	data, err := x509.CreateRevocationList(rand.Reader, template, p.ca, p.key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: data})
}

// newPinnedRelayPKI returns a self-signed certificate that is both the relay's
// TLS certificate and the CAFile trust anchor, so Verify produces a
// one-element peer chain.
func newPinnedRelayPKI(t *testing.T) *cascadePKI {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(11),
		Subject:               pkix.Name{CommonName: "pinned relay"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &cascadePKI{ca: ca, key: private, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// TestCascadePinnedRelayLeafEnforcesCRL covers a CAFile that pins the relay's
// own certificate: the verified chain is the leaf alone, and the leaf is also
// its own CRL issuer. Configuring CRLs must not skip that check.
func TestCascadePinnedRelayLeafEnforcesCRL(t *testing.T) {
	pki := newPinnedRelayPKI(t)
	key, err := x509.MarshalPKCS8PrivateKey(pki.key)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(pki.pem, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	if err != nil {
		t.Fatal(err)
	}
	server := cascadeServer(t, pki, pair, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"servicesVersion":1,"services":[]}`)
	}))
	config := cascadeConfig(t, pki, server.URL)
	now := time.Now().Truncate(time.Second)
	path := writeTestFile(t, t.TempDir(), "relay.crl", pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)
	config.CRLFiles = []string{path}
	client := mustCascadeClient(t, config)
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatalf("fresh CRL for the pinned relay: %v", err)
	}
	revoked := pki.crl(t, 11, 2, now, now.Add(time.Hour), false)
	writeTestFile(t, filepath.Dir(path), "relay.crl", revoked, 0600)
	if err := client.ReloadTLS(); err != nil {
		t.Fatalf("reload CRL revoking the pinned relay: %v", err)
	}
	if _, err := client.Services(context.Background()); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("pinned revoked relay accepted: %v", err)
	}
}

// TestCascadePinnedLeafWithoutIssuerFailsClosed pins a relay leaf whose issuer
// is not in CAFile while a CRL for another trusted CA is configured. No
// configured CRL can cover the pinned leaf, so the connection fails closed
// instead of silently skipping revocation.
func TestCascadePinnedLeafWithoutIssuerFailsClosed(t *testing.T) {
	pki := newCascadePKI(t)
	other := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	serverCert, serverPEM, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	server := cascadeServer(t, pki, serverCert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"servicesVersion":1,"services":[]}`)
	}))
	config := cascadeConfig(t, pki, server.URL)
	bundle := append(append([]byte{}, serverPEM...), other.pem...)
	config.CAFile = writeTestFile(t, t.TempDir(), "pinned.pem", bundle, 0600)
	config.CRLFiles = []string{writeTestFile(t, t.TempDir(), "other.crl", other.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)}
	client := mustCascadeClient(t, config)
	if _, err := client.Services(context.Background()); err == nil || !strings.Contains(err.Error(), "CRL coverage") {
		t.Fatalf("pinned relay leaf without issuer coverage accepted: %v", err)
	}
}

func TestCascadeCRLValidationAndReload(t *testing.T) {
	pki := newCascadePKI(t)
	certificate, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	server := cascadeServer(t, pki, certificate, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"servicesVersion":1,"services":[]}`) }))
	config := cascadeConfig(t, pki, server.URL)
	now := time.Now().Truncate(time.Second)
	fresh := pki.crl(t, 0, 2, now.Add(-time.Minute), now.Add(time.Hour), false)
	path := writeTestFile(t, t.TempDir(), "relay.crl", fresh, 0600)
	config.CRLFiles = []string{path}
	client := mustCascadeClient(t, config)
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A revoked certificate is rejected after refresh, including pooled HTTP.
	revoked := pki.crl(t, 2, 3, now, now.Add(time.Hour), false)
	writeTestFile(t, filepath.Dir(path), "relay.crl", revoked, 0600)
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("revoked relay accepted")
	}
	writeTestFile(t, filepath.Dir(path), "relay.crl", fresh, 0600)
	if err := client.ReloadTLS(); err == nil {
		t.Fatal("older CRL replaced newer one")
	}
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("rollback removed revocation")
	}
	recovered := pki.crl(t, 0, 4, now, now.Add(time.Hour), false)
	writeTestFile(t, filepath.Dir(path), "relay.crl", recovered, 0600)
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Invalid material at startup never produces a usable client.
	for _, data := range [][]byte{
		[]byte("bad CRL"), pki.crl(t, 0, 5, now.Add(-2*time.Hour), now.Add(-time.Hour), false),
		pki.crl(t, 0, 5, now.Add(10*time.Minute), now.Add(time.Hour), false),
		pki.crl(t, 0, 5, now, now.Add(time.Hour), true),
		newCascadePKI(t).crl(t, 0, 5, now, now.Add(time.Hour), false),
	} {
		writeTestFile(t, filepath.Dir(path), "relay.crl", data, 0600)
		if _, err := NewClient(config); err == nil {
			t.Fatal("invalid CRL accepted")
		}
	}
	config.CRLFiles = []string{path + ".missing"}
	if _, err := NewClient(config); err == nil {
		t.Fatal("unreadable CRL accepted")
	}

	// Go's CRL generator requires NextUpdate. Exercise the absent-field
	// validity rule directly instead of altering signed fixture DER.
	withoutNext := &x509.RevocationList{ThisUpdate: now.Add(-2 * time.Hour)}
	if err := freshCRL(withoutNext, now, time.Hour); err == nil {
		t.Fatal("expired CRL without NextUpdate accepted")
	}
	if err := freshCRL(withoutNext, now, 0); err != nil {
		t.Fatal(err)
	}

}

func TestCascadeRejectsUnsupportedCriticalCRLExtensions(t *testing.T) {
	pki := newCascadePKI(t)
	certificate, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	server := cascadeServer(t, pki, certificate, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"servicesVersion":1,"services":[]}`) }))
	for _, extension := range []pkix.Extension{
		// issuingDistributionPoint: SEQUENCE { onlyContainsCACerts [2] TRUE }.
		{Id: asn1.ObjectIdentifier{2, 5, 29, 28}, Critical: true, Value: []byte{0x30, 3, 0x82, 1, 0xff}},
		{Id: asn1.ObjectIdentifier{2, 5, 29, 28}, Value: []byte{0x30, 3, 0x82, 1, 0xff}},
		{Id: asn1.ObjectIdentifier{1, 2, 3, 4}, Critical: true, Value: []byte{5, 0}},
	} {
		t.Run(extension.Id.String(), func(t *testing.T) {
			config := cascadeConfig(t, pki, server.URL)
			now := time.Now()
			path := writeTestFile(t, t.TempDir(), "relay.crl", pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)
			config.CRLFiles = []string{path}
			client := mustCascadeClient(t, config)
			if _, err := client.Services(context.Background()); err != nil {
				t.Fatal(err)
			}
			data, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(2), ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour), ExtraExtensions: []pkix.Extension{extension}}, pki.ca, pki.key)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Dir(path), "relay.crl", data, 0600)
			if _, err := NewClient(config); err == nil || !strings.Contains(err.Error(), extension.Id.String()) {
				t.Fatalf("unsupported CRL accepted at startup: %v", err)
			}
			if err := client.ReloadTLS(); err == nil {
				t.Fatal("unsupported CRL replaced full coverage")
			}
			if _, err := client.Services(context.Background()); err != nil {
				t.Fatalf("invalid replacement discarded previous full CRL: %v", err)
			}
		})
	}
}

func TestCascadeCRLURL(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now()
	data := pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false)
	var fail atomic.Bool
	var redirect atomic.Bool
	publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if redirect.Load() {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		w.Write(data)
	}))
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	publisher.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	publisher.StartTLS()
	defer publisher.Close()
	config := cascadeConfig(t, pki, "https://relay.example")
	config.CRLURLs = []string{publisher.URL}
	client := mustCascadeClient(t, config)
	fail.Store(true)
	if err := client.ReloadTLS(); err == nil {
		t.Fatal("failed CRL fetch accepted")
	}
	fail.Store(false)
	redirect.Store(true)
	if _, err := NewClient(config); err == nil {
		t.Fatal("CRL redirect followed")
	}
	for _, address := range []string{"http://pki.example/crl", "https://user@pki.example/crl", "https://pki.example/crl?query", "https://pki.example/crl#fragment", "://"} {
		config.CRLURLs = []string{address}
		if _, err := NewClient(config); err == nil {
			t.Fatalf("bad CRL URL accepted: %s", address)
		}
	}
}

func TestCascadeCAReplacementAndExpiry(t *testing.T) {
	pki := newCascadePKI(t)
	// The held response must be disconnected at certificate expiry; progress
	// and HTTP keep-alive cannot extend certificate validity.
	expiry := time.Now().Add(2 * time.Second).Truncate(time.Second)
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", expiry)
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	config := cascadeConfig(t, pki, server.URL)
	client := mustCascadeClient(t, config)
	response, err := client.http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(response.Body); response.Body.Close(); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("held response completed normally")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("connection survived peer certificate expiry")
	}
	// Invalid CA replacements retain the old roots; a valid changed bundle
	// becomes effective immediately, without accepting the previous CA.
	writeTestFile(t, filepath.Dir(config.CAFile), "ca.pem", []byte("invalid"), 0600)
	if err := client.ReloadTLS(); err == nil {
		t.Fatal("invalid replacement CA accepted")
	}
	nextPKI := newCascadePKI(t)
	if err := os.WriteFile(config.CAFile, nextPKI.pem, 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	oldCert, _, _ := pki.issue(t, 9, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	if err := client.tlsMaterial.verifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{oldCert.Leaf}}); err == nil {
		t.Fatal("removed relay CA still trusted")
	}
	client.Close()
	if err := client.ReloadTLS(); err == nil {
		t.Fatal("closed client reloaded")
	}
	if _, err := client.tlsMaterial.config().GetClientCertificate(nil); err == nil {
		t.Fatal("closed client returned credential")
	}
}

func TestCascadeCRLReloadWithMultipleSourcesAndIssuerRotation(t *testing.T) {
	pki := newCascadePKI(t)
	config := cascadeConfig(t, pki, "https://relay.example")
	now := time.Now().Truncate(time.Second)
	directory := t.TempDir()
	first := writeTestFile(t, directory, "one.crl", pki.crl(t, 0, 1, now.Add(-2*time.Minute), now.Add(time.Hour), false), 0600)
	second := writeTestFile(t, directory, "two.crl", pki.crl(t, 0, 2, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)
	config.CRLFiles = []string{first, second}
	client := mustCascadeClient(t, config)
	if err := client.ReloadTLS(); err != nil {
		t.Fatalf("unchanged CRLs rejected: %v", err)
	}
	writeTestFile(t, directory, "two.crl", pki.crl(t, 0, 3, now, now.Add(time.Hour), false), 0600)
	if err := client.ReloadTLS(); err != nil {
		t.Fatalf("newer CRL rejected: %v", err)
	}
	// A new issuing key may reuse the same subject name and restart numbering.
	replacement := newCascadePKI(t)
	if err := os.WriteFile(config.CAFile, replacement.pem, 0600); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, directory, "one.crl", replacement.crl(t, 0, 1, now, now.Add(time.Hour), false), 0600)
	writeTestFile(t, directory, "two.crl", replacement.crl(t, 0, 1, now, now.Add(time.Hour), false), 0600)
	if err := client.ReloadTLS(); err != nil {
		t.Fatalf("CA key rotation rejected: %v", err)
	}
	if err := os.WriteFile(config.CAFile, pki.pem, 0600); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, directory, "one.crl", pki.crl(t, 0, 1, now, now.Add(time.Hour), false), 0600)
	writeTestFile(t, directory, "two.crl", pki.crl(t, 0, 2, now, now.Add(time.Hour), false), 0600)
	if err := client.ReloadTLS(); err == nil {
		t.Fatal("issuer removal erased its retained CRL floor")
	}
	if _, err := NewClient(config); err == nil {
		t.Fatal("restart erased the removed issuer's CRL floor")
	}
}

func TestCascadeCRLFetchIgnoresGlobalTransportAndCancelsProgress(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now()
	data := pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false)
	var stream atomic.Bool
	started := make(chan struct{}, 1)
	publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !stream.Load() {
			w.Write(data)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case started <- struct{}{}:
		default:
		}
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				w.Write([]byte("progress"))
				w.(http.Flusher).Flush()
			}
		}
	}))
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	publisher.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	publisher.StartTLS()
	defer publisher.Close()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) { panic("global default transport must not be used") })
	defer func() { http.DefaultTransport = oldTransport }()
	config := cascadeConfig(t, pki, "https://relay.example")
	config.CRLURLs = []string{publisher.URL}
	config.TLSReloadInterval = 5 * time.Millisecond
	client := mustCascadeClient(t, config)
	if client.tlsMaterial.config().ClientSessionCache != nil {
		t.Fatal("cascade client enables TLS session resumption")
	}
	stream.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); client.WatchTLS(ctx, nil) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("CRL refresh did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("progressing CRL download prevented watcher shutdown")
	}
}

func TestCascadeStartupHonorsCancellationWhileCRLProgresses(t *testing.T) {
	pki := newCascadePKI(t)
	started := make(chan struct{}, 1)
	publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case started <- struct{}{}:
		default:
		}
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				w.Write([]byte("progress"))
				w.(http.Flusher).Flush()
			}
		}
	}))
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	publisher.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	publisher.StartTLS()
	defer publisher.Close()
	config := cascadeConfig(t, pki, "https://relay.example")
	config.CRLURLs = []string{publisher.URL}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		client, err := NewClientContext(ctx, config)
		if client != nil {
			client.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("startup CRL request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup ignored cancellation")
	}
}

func TestCascadeReloadRechecksValidityAfterCRLDownload(t *testing.T) {
	for _, material := range []string{"certificate", "CRL"} {
		t.Run(material, func(t *testing.T) {
			pki := newCascadePKI(t)
			now := time.Now()
			var data atomic.Value
			data.Store(pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false))
			var delay atomic.Bool
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if delay.Load() {
					select {
					case started <- struct{}{}:
					default:
					}
					ticker := time.NewTicker(20 * time.Millisecond)
					defer ticker.Stop()
				wait:
					for {
						select {
						case <-r.Context().Done():
							return
						case <-release:
							break wait
						case <-ticker.C:
							w.Write([]byte("\n"))
							w.(http.Flusher).Flush()
						}
					}
				}
				w.Write(data.Load().([]byte))
			}))
			serverCert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
			publisher.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}, MinVersion: tls.VersionTLS12}
			publisher.StartTLS()
			defer publisher.Close()
			config := cascadeConfig(t, pki, "https://relay.example")
			config.CRLURLs = []string{publisher.URL}
			client := mustCascadeClient(t, config)
			old := client.tlsMaterial.material
			expires := time.Now().Add(2 * time.Second).Truncate(time.Second)
			if material == "certificate" {
				_, cert, key := pki.issue(t, 4, x509.ExtKeyUsageClientAuth, "", expires)
				writeTestFile(t, filepath.Dir(config.ClientCertFile), "client.pem", cert, 0600)
				writeTestFile(t, filepath.Dir(config.ClientKeyFile), "client.key", key, 0600)
			} else {
				data.Store(pki.crl(t, 0, 2, now.Add(-time.Minute), expires, false))
			}
			delay.Store(true)
			done := make(chan error, 1)
			go func() { done <- client.ReloadTLS() }()
			select {
			case <-started:
			case <-time.After(time.Second):
				close(release)
				t.Fatal("reload did not start before replacement expiry")
			}
			time.Sleep(time.Until(expires) + 50*time.Millisecond)
			close(release)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("replacement expired during download but was installed")
				}
			case <-time.After(time.Second):
				t.Fatal("reload did not finish")
			}
			client.tlsMaterial.mu.Lock()
			retained := client.tlsMaterial.material == old
			client.tlsMaterial.mu.Unlock()
			if !retained {
				t.Fatal("expired replacement displaced the last valid generation")
			}
		})
	}
}

func TestCascadeCRLFetchRequiresExplicitTrust(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now()
	data := pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false)
	publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	publisher.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	publisher.StartTLS()
	defer publisher.Close()
	roots := x509.NewCertPool()
	roots.AddCert(pki.ca)
	if _, err := fetchRelayCRL(context.Background(), publisher.URL, roots, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := fetchRelayCRL(context.Background(), publisher.URL, x509.NewCertPool(), 0, 0); err == nil {
		t.Fatal("empty explicit trust fell back to system roots")
	}
	if _, err := fetchRelayCRL(context.Background(), publisher.URL, nil, 0, 0); err == nil {
		t.Fatal("missing explicit trust accepted")
	}
}

func TestCascadeCRLFetchHonorsRequestTimeout(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now()
	data := pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false)
	for _, body := range []bool{false, true} {
		t.Run(fmt.Sprintf("body=%v", body), func(t *testing.T) {
			publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if body {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				select {
				case <-time.After(150 * time.Millisecond):
					w.Write(data)
				case <-r.Context().Done():
				}
			}))
			cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
			publisher.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
			publisher.StartTLS()
			defer publisher.Close()
			config := cascadeConfig(t, pki, "https://relay.example")
			config.CRLURLs = []string{publisher.URL}
			config.RequestTimeout = 20 * time.Millisecond
			if client, err := NewClient(config); err == nil {
				client.Close()
				t.Fatal("CRL fetch ignored the short configured stall timeout")
			}
			config.RequestTimeout = 2 * time.Second
			mustCascadeClient(t, config)
		})
	}
}

// A publisher that keeps dribbling bytes must not hold a fetch forever: the
// stall deadline rejects a drip slower than the configured request timeout,
// and the fixed total deadline rejects one that outlasts the transfer window.
func TestCascadeCRLFetchBoundsSlowDrip(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now()
	data := pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false)
	for _, test := range []struct {
		name     string
		stall    time.Duration
		interval time.Duration
		total    time.Duration
		want     error
	}{
		{"stall", 200 * time.Millisecond, 400 * time.Millisecond, 0, errCRLStall},
		{"total", time.Second, 20 * time.Millisecond, 1500 * time.Millisecond, errCRLDownloadTime},
	} {
		t.Run(test.name, func(t *testing.T) {
			publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				for offset := 0; ; offset = (offset + 1) % len(data) {
					if _, err := w.Write(data[offset : offset+1]); err != nil {
						return
					}
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
						return
					case <-time.After(test.interval):
					}
				}
			}))
			cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
			publisher.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
			publisher.StartTLS()
			defer publisher.Close()
			roots := x509.NewCertPool()
			roots.AddCert(pki.ca)
			started := time.Now()
			if _, err := fetchRelayCRL(context.Background(), publisher.URL, roots, test.stall, test.total); !errors.Is(err, test.want) {
				t.Fatalf("slow drip: %v", err)
			}
			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Fatalf("slow drip was not bounded: %s", elapsed)
			}
		})
	}
}

func TestCascadeCRLFetchResetsStallAfterHeaders(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now()
	data := pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false)
	publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Each response gap fits the timeout; the complete transfer does not.
		time.Sleep(600 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(600 * time.Millisecond)
		w.Write(data)
	}))
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	publisher.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	publisher.StartTLS()
	defer publisher.Close()
	config := cascadeConfig(t, pki, "https://relay.example")
	config.CRLURLs = []string{publisher.URL}
	config.RequestTimeout = time.Second
	mustCascadeClient(t, config)
}
