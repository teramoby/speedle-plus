package etcd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTLSAllowedCN(t *testing.T) {
	certFile, keyFile := tlsClientFixture(t)
	for _, allowedCN := range []string{"", "etcd-peer"} {
		t.Run("allowed="+allowedCN, func(t *testing.T) {
			manager, err := (Etcd3StoreBuilder{}).NewStore(map[string]interface{}{
				EtcdEndpointKey:          "https://127.0.0.1:1",
				EtcdTLSClientCertFileKey: certFile,
				EtcdTLSClientKeyFileKey:  keyFile,
				EtcdTLSAllowedCNKey:      allowedCN,
			})
			if err != nil {
				t.Fatal(err)
			}
			s := manager.(*Store)
			defer s.destroy()
			verify := s.Config.TLS.VerifyPeerCertificate
			if allowedCN == "" {
				if verify != nil {
					t.Fatal("an empty allowed CN must not add a peer-name restriction")
				}
				return
			}
			if verify == nil {
				t.Fatal("the configured peer-name restriction was lost")
			}
			for _, peerCN := range []string{"etcd-peer", "other-peer", ""} {
				// TLS has already verified these chains before this callback.
				chains := [][]*x509.Certificate{{{Subject: pkix.Name{CommonName: peerCN}}}}
				err := verify(nil, chains)
				if (err == nil) != (peerCN == allowedCN) {
					t.Errorf("allowed=%q peer=%q: %v", allowedCN, peerCN, err)
				}
			}
		})
	}
}

// A real client certificate fixture, independent of etcd's zap-based generator.
func tlsClientFixture(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "client.pem"), filepath.Join(dir, "client-key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
