// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testPKI struct {
	caPEM             []byte
	caPool            *x509.CertPool
	serverCertificate tls.Certificate
	clientCertPEM     []byte
	clientKeyPEM      []byte
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()
	return newTestPKIForHosts(t, nil, []net.IP{net.ParseIP("127.0.0.1")})
}

// newTestPKIForHosts issues the same test hierarchy with a server certificate
// for the given names. A proxy test uses it so the source URL host is not a
// loopback address that the environment proxy function would bypass.
func newTestPKIForHosts(t *testing.T, dnsNames []string, addresses []net.IP) *testPKI {
	t.Helper()
	now := time.Now()
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "adsync test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	caPool := x509.NewCertPool()
	caPool.AddCert(caCertificate)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	issue := func(serial int64, usage x509.ExtKeyUsage, dnsNames []string, addresses []net.IP) (tls.Certificate, []byte, []byte) {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate leaf key: %v", err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: "adsync test leaf"},
			NotBefore:    now.Add(-time.Hour),
			NotAfter:     now.Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
			DNSNames:     dnsNames,
			IPAddresses:  addresses,
		}
		der, err := x509.CreateCertificate(rand.Reader, template, caCertificate, public, caPrivate)
		if err != nil {
			t.Fatalf("create leaf certificate: %v", err)
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(private)
		if err != nil {
			t.Fatalf("marshal leaf key: %v", err)
		}
		certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
		if err != nil {
			t.Fatalf("build leaf key pair: %v", err)
		}
		return certificate, certificatePEM, keyPEM
	}

	serverCertificate, _, _ := issue(2, x509.ExtKeyUsageServerAuth, dnsNames, addresses)
	_, clientCertPEM, clientKeyPEM := issue(3, x509.ExtKeyUsageClientAuth, nil, nil)
	return &testPKI{
		caPEM:             caPEM,
		caPool:            caPool,
		serverCertificate: serverCertificate,
		clientCertPEM:     clientCertPEM,
		clientKeyPEM:      clientKeyPEM,
	}
}

func writeTestFile(t *testing.T, directory, name string, data []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestClientCertificate(t *testing.T) {
	vectors := loadGolden(t)
	signer := &fixtureSigner{keyID: vectors.TestKey.KeyID, private: goldenPrivateKey(t, vectors)}
	source := newFixtureSource(testCredential, signer, newFixtureService(testServiceID1, "books"))
	pki := newTestPKI(t)

	server := httptest.NewUnstartedServer(source)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{pki.serverCertificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pki.caPool,
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	directory := t.TempDir()
	caFile := writeTestFile(t, directory, "ca.pem", pki.caPEM, 0o644)
	certFile := writeTestFile(t, directory, "client.pem", pki.clientCertPEM, 0o600)
	keyFile := writeTestFile(t, directory, "client.key", pki.clientKeyPEM, 0o600)

	config := Config{
		BaseURL:        server.URL,
		Credential:     testCredential,
		PublicKeys:     goldenPublicKeys(t, vectors),
		CAFile:         caFile,
		ClientCertFile: certFile,
		ClientKeyFile:  keyFile,
		RequestTimeout: time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
		},
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	response, err := client.Services(context.Background())
	if err != nil {
		t.Fatalf("Services with a client certificate: %v", err)
	}
	if len(response.Services) != 1 {
		t.Fatalf("got %d services", len(response.Services))
	}

	// The same server rejects a client without a certificate.
	config.ClientCertFile = ""
	config.ClientKeyFile = ""
	withoutCertificate, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient without a certificate: %v", err)
	}
	if _, err := withoutCertificate.Services(context.Background()); err == nil {
		t.Fatal("Services succeeded without the required client certificate")
	}
}
