//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package assertion

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePEM(t *testing.T, name, kind string, der []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func clientCertificate(t *testing.T) (cert, key string) {
	t.Helper()
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ads"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	keyDER, _ := x509.MarshalECPrivateKey(privateKey)
	return writePEM(t, "client.pem", "CERTIFICATE", der), writePEM(t, "client-key.pem", "EC PRIVATE KEY", keyDER)
}

func TestNewAsserterConfigurations(t *testing.T) {
	if _, err := NewAsserter(nil, nil); err == nil {
		t.Error("a nil configuration must fail")
	}
	if _, err := NewAsserter(&AsserterConfig{}, nil); err == nil {
		t.Error("an empty endpoint must fail")
	}
	conf := &AsserterConfig{Endpoint: "http://asserter"}
	if _, err := NewAsserter(conf, nil); err != nil || conf.HTTPTimeout != 10 {
		t.Errorf("default timeout %d, %v", conf.HTTPTimeout, err)
	}

	cert, key := clientCertificate(t)
	if _, err := NewAsserter(&AsserterConfig{Endpoint: "https://asserter", CACert: cert, ClientCert: cert, ClientKey: key}, nil); err != nil {
		t.Errorf("https with a CA and a client certificate: %v", err)
	}
	if _, err := NewAsserter(&AsserterConfig{Endpoint: "https://asserter", CACert: "/nonexistent/ca.pem"}, nil); err == nil {
		t.Error("a missing CA file must fail")
	}
	if _, err := NewAsserter(&AsserterConfig{Endpoint: "https://asserter", ClientCert: cert, ClientKey: cert}, nil); err == nil {
		t.Error("an invalid client key must fail")
	}
}

func TestAssertTokenOverTLS(t *testing.T) {
	var headers http.Header
	var path string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers, path = r.Header, r.URL.Path
		fmt.Fprint(w, `{"principals":[{"type":"user","name":"alice"}]}`)
	}))
	defer server.Close()
	ca := writePEM(t, "ca.pem", "CERTIFICATE", server.Certificate().Raw)
	tenant := "/tenant1"
	asserter, err := NewAsserter(&AsserterConfig{Endpoint: server.URL + "/assert", CACert: ca}, &tenant)
	if err != nil {
		t.Fatal(err)
	}
	got, err := asserter.AssertToken("tok", "oidc", "corp", map[string]string{"X-A": "1"})
	if err != nil || len(got.Principals) != 1 || got.Principals[0].Name != "alice" {
		t.Fatalf("AssertToken = %+v, %v", got, err)
	}
	if path != "/assert/tenant1" || headers.Get(TokenKey) != "tok" || headers.Get(IdpTypeKey) != "oidc" ||
		headers.Get(AllowedIDDKey) != "corp" || headers.Get("X-A") != "1" || headers.Get(RequestHeaderKey) != "X-A" {
		t.Fatalf("request %s %v", path, headers)
	}
}

func TestAssertTokenFailures(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"status":     func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		"not json":   func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "nope") },
		"error code": func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"errCode":7,"errMessage":"expired"}`) },
	}
	for name, handler := range cases {
		server := httptest.NewServer(handler)
		asserter, _ := NewAsserter(&AsserterConfig{Endpoint: server.URL}, nil)
		if got, err := asserter.AssertToken("tok", "oidc", "", nil); err == nil {
			t.Errorf("%s: asserted %+v", name, got)
		}
		server.Close()
	}

	asserter, _ := NewAsserter(&AsserterConfig{Endpoint: "http://127.0.0.1:1"}, nil)
	if _, err := asserter.AssertToken("", "oidc", "", nil); err == nil {
		t.Error("an empty token must fail")
	}
	if _, err := asserter.AssertToken("tok", "oidc", "", nil); err == nil {
		t.Error("an unreachable asserter must fail")
	}
	bad, _ := NewAsserter(&AsserterConfig{Endpoint: "http://a b"}, nil)
	if _, err := bad.AssertToken("tok", "oidc", "", nil); err == nil {
		t.Error("an invalid endpoint must fail")
	}
}
