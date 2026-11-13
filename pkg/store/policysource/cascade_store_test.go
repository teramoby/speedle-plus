// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func cascadeStoreOptions(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(dir, "client.pem")
	keyFile := filepath.Join(dir, "client.key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatal(err)
	}
	return Options{UpstreamURL: "https://relay.example", ClientCertFile: certFile, ClientKeyFile: keyFile, CAFile: certFile, CacheDir: filepath.Join(dir, "cache"), StartRuntime: true}
}
func TestCascadeStoreRequiresCompleteCredentialsAndCache(t *testing.T) {
	valid := cascadeStoreOptions(t)
	for _, setting := range []string{ClientCertFileKey, ClientKeyFileKey, CAFileKey, CacheDirKey, "InstanceID"} {
		options := valid
		switch setting {
		case ClientCertFileKey:
			options.ClientCertFile = ""
		case ClientKeyFileKey:
			options.ClientKeyFile = ""
		case CAFileKey:
			options.CAFile = ""
		case CacheDirKey:
			options.CacheDir = ""
		case "InstanceID":
			options.InstanceID = testInstanceID
		}
		if _, err := Open(context.Background(), options); err == nil || !strings.Contains(err.Error(), setting) {
			t.Fatalf("missing %s: %v", setting, err)
		}
	}
	first, err := Open(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	id, err := first.client.InstanceID()
	if err != nil || !adsync.ValidInstanceID(id) {
		t.Fatalf("generated ID: %s, %v", id, err)
	}
	if err := first.ReloadDownstreamCredentials(); err != nil {
		t.Fatal(err)
	}
	first.Close()
	first.Close()
	second, err := Open(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
	again, err := second.client.InstanceID()
	if err != nil || again != id {
		t.Fatalf("restart ID: %s, %v", again, err)
	}
	if err := os.WriteFile(filepath.Join(valid.CacheDir, adsync.InstanceIDFile), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), valid); err == nil {
		t.Fatal("corrupt persisted ID accepted")
	}
}
func TestCascadeStoreFailsOnCorruptRetainedCache(t *testing.T) {
	valid := cascadeStoreOptions(t)
	if err := os.MkdirAll(valid.CacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(valid.CacheDir, "service-"+testServiceID1+".json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), valid); err == nil || !strings.Contains(err.Error(), "cache") {
		t.Fatalf("corrupt retained cache: %v", err)
	}
}
