package etcd

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"go.etcd.io/etcd/client/pkg/v3/transport"
	"go.uber.org/zap"
)

func TestTLSAllowedCN(t *testing.T) {
	cert, err := transport.SelfCert(zap.NewNop(), t.TempDir(), []string{"localhost"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, allowedCN := range []string{"", "etcd-peer"} {
		t.Run("allowed="+allowedCN, func(t *testing.T) {
			manager, err := (Etcd3StoreBuilder{}).NewStore(map[string]interface{}{
				EtcdEndpointKey:          "https://127.0.0.1:1",
				EtcdTLSClientCertFileKey: cert.CertFile,
				EtcdTLSClientKeyFileKey:  cert.KeyFile,
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
