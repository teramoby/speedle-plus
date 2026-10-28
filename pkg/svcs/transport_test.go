package svcs_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc"
	adsPB "github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsgrpc"
	pmsPB "github.com/teramoby/speedle-plus/pkg/svcs/pmsgrpc/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Exercise real generated clients and server registration over TCP, rather than
// calling implementations directly (which cannot catch namespace/TLS failures).
func TestGRPCTransportAndPublishedClients(t *testing.T) {
	dir := t.TempDir()
	ca, caKey, caPEM := certificate(t, nil, nil, true)
	serverCert, serverKey := signedFiles(t, dir, "server", ca, caKey)
	clientCert, clientKey := signedFiles(t, dir, "client", ca, caKey)
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	client, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		t.Fatal(err)
	}
	wrongIssuer, wrongKey, wrongCA := certificate(t, nil, nil, true)
	wrongCertFile, wrongKeyFile := signedFiles(t, dir, "untrusted-client", wrongIssuer, wrongKey)
	wrongClient, err := tls.LoadX509KeyPair(wrongCertFile, wrongKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	wrongPool := x509.NewCertPool()
	wrongPool.AppendCertsFromPEM(wrongCA)
	for _, mode := range []string{"tls", "optional-mtls", "mtls", "insecure"} {
		t.Run(mode, func(t *testing.T) {
			params := &flags.Parameters{Insecure: flags.StrParamDetail{Value: "false"}, CertPath: flags.StrParamDetail{Value: serverCert}, KeyPath: flags.StrParamDetail{Value: serverKey}}
			if mode == "mtls" || mode == "optional-mtls" {
				params.ClientCertPath.Value = caFile
			}
			if mode == "mtls" {
				params.ForceClientCert.Value = "true"
			}
			if mode == "insecure" {
				params.Insecure.Value = "true"
			}
			server, err := params.NewGRPCServer()
			if err != nil {
				t.Fatal(err)
			}
			loc := filepath.Join(t.TempDir(), "policies.json")
			if err := os.WriteFile(loc, []byte(`{"services":[{"name":"docs","policies":[{"id":"p1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"book","actions":["read"]}]}]}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			ps, err := store.NewStore("file", map[string]interface{}{"FileLocation": loc})
			if err != nil {
				t.Fatal(err)
			}
			evaluator, err := eval.NewWithStore(&cfg.Config{}, ps)
			if err != nil {
				t.Fatal(err)
			}
			defer evaluator.Close()
			pmsPB.RegisterPolicyManagerServer(server, pmsgrpc.NewServiceImpl(ps))
			service, err := adsgrpc.NewGRPCService(evaluator)
			if err != nil {
				t.Fatal(err)
			}
			adsPB.RegisterEvaluatorServer(server, service)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			go server.Serve(listener)
			defer server.Stop()
			for _, transport := range []struct {
				name    string
				creds   credentials.TransportCredentials
				allowed bool
			}{
				{"plaintext", insecure.NewCredentials(), mode == "insecure"},
				{"trusted server without client cert", credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12}), mode == "tls" || mode == "optional-mtls"},
				{"trusted client", credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: "localhost", Certificates: []tls.Certificate{client}, MinVersion: tls.VersionTLS12}), mode != "insecure"},
				{"untrusted client CA", credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: "localhost", GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &wrongClient, nil }, MinVersion: tls.VersionTLS12}), mode == "tls"},
				{"untrusted server CA", credentials.NewTLS(&tls.Config{RootCAs: wrongPool, ServerName: "localhost", Certificates: []tls.Certificate{client}, MinVersion: tls.VersionTLS12}), false},
			} {
				t.Run(transport.name, func(t *testing.T) {
					conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(transport.creds))
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					services, err := pmsPB.NewPolicyManagerClient(conn).QueryServices(ctx, &pmsPB.ServiceQueryRequest{Name: "docs"})
					if !transport.allowed {
						if err == nil {
							t.Fatal("untrusted transport reached PMS")
						}
						return
					}
					if err != nil || len(services.GetServices()) != 1 {
						t.Fatalf("PMS: response=%v error=%v", services, err)
					}
					result, err := adsPB.NewEvaluatorClient(conn).IsAllowed(ctx, &adsPB.ContextRequest{Subject: &adsPB.Subject{Principals: []*adsPB.Principal{{Type: "user", Name: "alice"}}}, ServiceName: "docs", Resource: "book", Action: "read"})
					if err != nil || !result.GetAllowed() {
						t.Fatalf("ADS: response=%v error=%v", result, err)
					}
				})
			}
		})
	}
}

func certificate(t *testing.T, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, ca bool) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, BasicConstraintsValid: true, IsCA: ca, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	if ca {
		template.KeyUsage |= x509.KeyUsageCertSign
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func signedFiles(t *testing.T, dir, name string, ca *x509.Certificate, key *ecdsa.PrivateKey) (string, string) {
	t.Helper()
	_, generated, data := certificate(t, ca, key, false)
	der, err := x509.MarshalECPrivateKey(generated)
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")
	if err := os.WriteFile(certPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestGRPCConfigurationFailsClosed(t *testing.T) {
	dir := t.TempDir()
	ca, key, caPEM := certificate(t, nil, nil, true)
	certFile, keyFile := signedFiles(t, dir, "valid", ca, key)
	_, otherKey := signedFiles(t, dir, "other", ca, key)
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	badCA := filepath.Join(dir, "bad-ca.crt")
	if err := os.WriteFile(badCA, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, insecure, cert, key, clientCA, force string
		valid                                      bool
	}{
		{"TLS false alias", "FALSE", certFile, keyFile, "", "", true},
		{"TLS zero alias", "0", certFile, keyFile, "", "", true},
		{"insecure true alias", "TRUE", "", "", "", "", true},
		{"insecure one alias", "1", "", "", "", "", true},
		{"invalid insecure", "yes", certFile, keyFile, "", "", false},
		{"missing server key", "false", certFile, "", "", "", false},
		{"mismatched key", "false", certFile, otherKey, "", "", false},
		{"invalid CA", "false", certFile, keyFile, badCA, "true", false},
		{"missing CA", "false", certFile, keyFile, "", "true", false},
		{"invalid force", "false", certFile, keyFile, caFile, "yes", false},
		{"force true alias", "false", certFile, keyFile, caFile, "TRUE", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &flags.Parameters{Insecure: flags.StrParamDetail{Value: tc.insecure}, CertPath: flags.StrParamDetail{Value: tc.cert}, KeyPath: flags.StrParamDetail{Value: tc.key}, ClientCertPath: flags.StrParamDetail{Value: tc.clientCA}, ForceClientCert: flags.StrParamDetail{Value: tc.force}}
			server, err := p.NewGRPCServer()
			if server != nil {
				server.Stop()
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestGRPCListenAddress(t *testing.T) {
	for _, tc := range []struct {
		rest, grpc, want string
		valid            bool
	}{
		{"127.0.0.1:6733", "", "127.0.0.1:50001", true},
		{"0.0.0.0:6733", "", "0.0.0.0:50001", true},
		{"[::1]:6733", "", "[::1]:50001", true},
		{"[fe80::1%lo0]:6733", "", "[fe80::1%lo0]:50001", true},
		{":6733", "", ":50001", true},
		{"127.0.0.1:6733", "127.0.0.1:0", "127.0.0.1:0", true},
		{"127.0.0.1:6733", "[::]:50005", "[::]:50005", true},
		{"127.0.0.1:6733", "malformed", "", false},
		{"malformed", "", "", false},
	} {
		p := &flags.Parameters{Endpoint: flags.StrParamDetail{Value: tc.rest}, GRPCEndpoint: flags.StrParamDetail{Value: tc.grpc}}
		got, err := p.GRPCListenAddress("50001")
		if (err == nil) != tc.valid || got != tc.want {
			t.Errorf("rest=%q grpc=%q got=%q err=%v", tc.rest, tc.grpc, got, err)
		}
	}
}
