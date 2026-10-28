// Generate short-lived certificates for local documentation examples.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	out := flag.String("out", "./tls", "New directory for demonstration certificates")
	dnsNames := flag.String("dns-names", "", "Additional comma-separated DNS names for the server certificate")
	flag.Parse()
	if err := generate(*out, *dnsNames); err != nil {
		log.Fatal(err)
	}
}
func generate(out, dnsNames string) error {
	if err := os.MkdirAll(out, 0700); err != nil {
		return err
	}
	for _, name := range []string{"ca.crt", "server.crt", "server.key", "client.crt", "client.key"} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			return fmt.Errorf("refusing to overwrite %s; choose a new directory", name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	now := time.Now()
	issue := func(name string, ca bool, issuer *x509.Certificate, issuerKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, nil, err
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			return nil, nil, nil, err
		}
		template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Speedle local example " + name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), BasicConstraintsValid: true, IsCA: ca, KeyUsage: x509.KeyUsageDigitalSignature}
		if ca {
			template.KeyUsage |= x509.KeyUsageCertSign
			issuer, issuerKey = template, key
		} else if name == "server" {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			template.DNSNames = []string{"localhost"}
			for _, name := range strings.Split(dnsNames, ",") {
				if name = strings.TrimSpace(name); name != "" {
					template.DNSNames = append(template.DNSNames, name)
				}
			}
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		} else {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
		if err != nil {
			return nil, nil, nil, err
		}
		cert, err := x509.ParseCertificate(der)
		return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), err
	}
	ca, key, encoded, err := issue("CA", true, nil, nil)
	if err != nil {
		return err
	}
	write := func(name string, data []byte, mode os.FileMode) error {
		f, err := os.OpenFile(filepath.Join(out, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	if err := write("ca.crt", encoded, 0644); err != nil {
		return err
	}
	for _, name := range []string{"server", "client"} {
		_, private, encoded, err := issue(name, false, ca, key)
		if err != nil {
			return err
		}
		if err := write(name+".crt", encoded, 0644); err != nil {
			return err
		}
		raw, err := x509.MarshalPKCS8PrivateKey(private)
		if err != nil {
			return err
		}
		if err := write(name+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), 0600); err != nil {
			return err
		}
	}
	absolute, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	fmt.Printf("Local demonstration certificates valid for 24 hours: %s\n", absolute)
	return nil
}
