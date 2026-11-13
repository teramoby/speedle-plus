// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCascadeCRLRollbackRejectedAfterRestart(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	config := cascadeConfig(t, pki, "https://relay.example")
	path := writeTestFile(t, t.TempDir(), "relay.crl", pki.crl(t, 2, 2, now, now.Add(time.Hour), false), 0600)
	config.CRLFiles = []string{path}
	first := mustCascadeClient(t, config)
	first.Close()
	floorsPath := filepath.Join(config.InstanceIDDir, crlWatermarksFile)
	info, err := os.Stat(floorsPath)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatalf("persisted watermark permissions: %v, %v", info, err)
	}
	// Both an older number and an older timestamp remain forbidden after restart.
	for _, data := range [][]byte{
		pki.crl(t, 0, 1, now, now.Add(time.Hour), false),
		pki.crl(t, 0, 3, now.Add(-time.Minute), now.Add(time.Hour), false),
	} {
		writeTestFile(t, filepath.Dir(path), "relay.crl", data, 0600)
		if client, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "older relay CRL") {
			if client != nil {
				client.Close()
			}
			t.Fatalf("restart accepted rollback: %v", err)
		}
	}
	writeTestFile(t, filepath.Dir(path), "relay.crl", pki.crl(t, 0, 3, now, now.Add(time.Hour), false), 0600)
	mustCascadeClient(t, config)
	// Damage to retained rollback state is not silently treated as a new cache.
	if err := os.WriteFile(floorsPath, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "watermark") {
		t.Fatalf("corrupt CRL watermark accepted: %v", err)
	}
}

func TestCascadeCRLWatermarkWriteFailureDoesNotPublish(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"servicesVersion":1,"services":[]}`)) }))
	config := cascadeConfig(t, pki, server.URL)
	path := writeTestFile(t, t.TempDir(), "relay.crl", pki.crl(t, 0, 1, now, now.Add(time.Hour), false), 0600)
	config.CRLFiles = []string{path}
	client := mustCascadeClient(t, config)
	floorsPath := filepath.Join(config.InstanceIDDir, crlWatermarksFile)
	retained, err := os.ReadFile(floorsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(floorsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(floorsPath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient(config); err == nil {
		t.Fatal("non-regular CRL watermark file accepted")
	}
	writeTestFile(t, filepath.Dir(path), "relay.crl", pki.crl(t, 2, 2, now, now.Add(time.Hour), false), 0600)
	if err := client.ReloadTLS(); err == nil || !strings.Contains(err.Error(), "persist CRL") {
		t.Fatalf("failed persistence published a generation: %v", err)
	}
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatalf("last good generation lost: %v", err)
	}
	if err := os.Remove(floorsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(floorsPath, retained, 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("revoked relay accepted after durable reload")
	}
}

func TestCascadeCRLPEMBundleLoadsEveryBlock(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"servicesVersion":1,"services":[]}`)) }))
	config := cascadeConfig(t, pki, server.URL)
	good := pki.crl(t, 0, 1, now, now.Add(time.Hour), false)
	bundle := append(append([]byte(nil), good...), pki.crl(t, 2, 1, now, now.Add(time.Hour), false)...)
	path := writeTestFile(t, t.TempDir(), "bundle.pem", bundle, 0600)
	config.CRLFiles = []string{path}
	client := mustCascadeClient(t, config)
	if len(client.tlsMaterial.material.crls) != 2 {
		t.Fatal("CRL bundle was truncated")
	}
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("revocation in second PEM block ignored")
	}
	client.Close()
	writeTestFile(t, filepath.Dir(path), "bundle.pem", good, 0600)
	if replay, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "older relay CRL") {
		if replay != nil {
			replay.Close()
		}
		t.Fatalf("equal-watermark non-revoking CRL accepted after restart: %v", err)
	}
	writeTestFile(t, filepath.Dir(path), "bundle.pem", bundle, 0600)
	mustCascadeClient(t, config)
	for _, invalid := range [][]byte{append(append([]byte(nil), good...), []byte("garbage")...), append(append([]byte(nil), good...), pki.crl(t, 0, 2, now.Add(-2*time.Hour), now.Add(-time.Hour), false)...)} {
		writeTestFile(t, filepath.Dir(path), "bundle.pem", invalid, 0600)
		if _, err := NewClient(config); err == nil {
			t.Fatal("invalid later PEM data accepted")
		}
	}
}

func TestCRLWatermarkRetention(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	one := crlWatermark{Subject: []byte("issuer"), PublicKey: []byte("key"), ThisUpdate: now, Number: big.NewInt(1), RawHash: hashOf([]byte("one"))}
	two := one
	two.Number = big.NewInt(2)
	if !two.covers(one) || one.covers(two) {
		t.Fatal("number ordering lost")
	}
	withoutNumber := one
	withoutNumber.Number = nil
	if !one.covers(withoutNumber) || withoutNumber.covers(one) {
		t.Fatal("absent-number ordering lost")
	}
	equivocated := one
	equivocated.RawHash = hashOf([]byte("different CRL"))
	if equivocated.covers(one) || one.covers(equivocated) {
		t.Fatal("equal position hid different signed CRL contents")
	}
	if equivocated.covers(withoutNumber) {
		t.Fatal("adding a number without advancing time hid changed contents")
	}
	for _, floors := range [][]crlWatermark{nil, {{Subject: []byte("issuer")}}, {{Subject: []byte("issuer"), PublicKey: []byte("key"), ThisUpdate: now, Number: big.NewInt(-1)}}} {
		payload, err := json.Marshal(floors)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(cacheEnvelope{Format: crlWatermarksFormat, Checksum: hashOf(payload), Payload: payload})
		if err != nil {
			t.Fatal(err)
		}
		manager := &clientTLS{configFiles: Config{InstanceIDDir: t.TempDir()}}
		if err := os.WriteFile(filepath.Join(manager.configFiles.InstanceIDDir, crlWatermarksFile), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := manager.loadCRLFloors(); err == nil {
			t.Fatal("invalid CRL floor accepted")
		}
	}
}

func TestCascadeCRLRetainsIncomparableWatermarks(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	config := cascadeConfig(t, pki, "https://relay.example")
	first := pki.crl(t, 0, 5, now.Add(-2*time.Minute), now.Add(time.Hour), false)
	second := pki.crl(t, 0, 4, now.Add(-time.Minute), now.Add(time.Hour), false)
	path := writeTestFile(t, t.TempDir(), "bundle.pem", append(append([]byte(nil), first...), second...), 0600)
	config.CRLFiles = []string{path}
	client := mustCascadeClient(t, config)
	if len(client.tlsMaterial.crlFloors) != 2 {
		t.Fatal("incomparable watermarks were collapsed")
	}
	if err := client.ReloadTLS(); err != nil {
		t.Fatalf("no-op incomparable reload: %v", err)
	}
	client.Close()
	restarted := mustCascadeClient(t, config)
	writeTestFile(t, filepath.Dir(path), "bundle.pem", first, 0600)
	if err := restarted.ReloadTLS(); err == nil {
		t.Fatal("lost the newer timestamp watermark")
	}
	writeTestFile(t, filepath.Dir(path), "bundle.pem", pki.crl(t, 0, 6, now, now.Add(time.Hour), false), 0600)
	if err := restarted.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	if len(restarted.tlsMaterial.crlFloors) != 1 {
		t.Fatal("dominated watermarks were not compacted")
	}
}

func TestCRLWatermarkEnvelopeValidation(t *testing.T) {
	payload := []byte(`[{"subject":"aXNzdWVy","publicKey":"a2V5","thisUpdate":"2026-01-01T00:00:00Z","number":1}]`)
	for _, envelope := range []cacheEnvelope{
		{Format: "unknown", Checksum: hashOf(payload), Payload: payload},
		{Format: crlWatermarksFormat, Checksum: hashOf([]byte("wrong")), Payload: payload},
		{Format: crlWatermarksFormat, Checksum: hashOf([]byte(`[{"unexpected":true}]`)), Payload: []byte(`[{"unexpected":true}]`)},
	} {
		data, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		manager := &clientTLS{configFiles: Config{InstanceIDDir: t.TempDir()}}
		if err := os.WriteFile(filepath.Join(manager.configFiles.InstanceIDDir, crlWatermarksFile), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := manager.loadCRLFloors(); err == nil {
			t.Fatal("invalid watermark envelope accepted")
		}
	}
	if runtime.GOOS != "windows" {
		manager := &clientTLS{configFiles: Config{InstanceIDDir: t.TempDir()}}
		path := filepath.Join(manager.configFiles.InstanceIDDir, crlWatermarksFile)
		if err := os.WriteFile(path, payload, 0644); err != nil {
			t.Fatal(err)
		}
		if err := manager.loadCRLFloors(); err == nil {
			t.Fatal("unsafe CRL watermark permissions accepted")
		}
	}
}
