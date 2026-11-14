// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type crlCommitContext struct {
	context.Context
	check func() error
}

func (c crlCommitContext) Err() error { return c.check() }

func TestCascadeCRLUnpublishedGenerationDoesNotAdvanceFloor(t *testing.T) {
	for _, failure := range []string{"cancellation", "expiry"} {
		t.Run(failure, func(t *testing.T) {
			pki := newCascadePKI(t)
			now := time.Now().Truncate(time.Second)
			config := cascadeConfig(t, pki, "https://relay.example")
			original := pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false)
			path := writeTestFile(t, t.TempDir(), "relay.crl", original, 0600)
			config.CRLFiles = []string{path}
			client := mustCascadeClient(t, config)
			manager := client.tlsMaterial
			old := manager.material
			floorsPath := filepath.Join(config.InstanceIDDir, crlWatermarksFile)
			retained, err := os.ReadFile(floorsPath)
			if err != nil {
				t.Fatal(err)
			}
			expires := time.Now().Truncate(time.Second).Add(2 * time.Second)
			writeTestFile(t, filepath.Dir(path), "relay.crl", pki.crl(t, 2, 2, now, expires, false), 0600)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			staged := false
			// Force the failure after the new floor has been flushed to a
			// temporary file, but before either publication or its rename.
			commitContext := crlCommitContext{Context: ctx, check: func() error {
				files, err := filepath.Glob(filepath.Join(config.InstanceIDDir, ".adsync-cache-*"))
				if err != nil {
					t.Fatal(err)
				}
				if len(files) > 0 {
					staged = true
					if failure == "cancellation" {
						cancel()
					} else {
						time.Sleep(time.Until(expires))
					}
				}
				return ctx.Err()
			}}
			err = manager.reload(commitContext)
			if failure == "cancellation" && !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation at commit: %v", err)
			}
			if failure == "expiry" && (err == nil || !strings.Contains(err.Error(), "validity window")) {
				t.Fatalf("expected expiry at commit: %v", err)
			}
			if !staged || manager.material != old {
				t.Fatal("did not retain the exact generation after staged failure")
			}
			actual, err := os.ReadFile(floorsPath)
			if err != nil || !bytes.Equal(actual, retained) {
				t.Fatalf("unpublished generation advanced the durable floor: %v", err)
			}
			files, err := filepath.Glob(filepath.Join(config.InstanceIDDir, ".adsync-cache-*"))
			if err != nil || len(files) != 0 {
				t.Fatalf("staged watermark leaked: %v, %v", files, err)
			}
			writeTestFile(t, filepath.Dir(path), "relay.crl", original, 0600)
			if err := client.ReloadTLS(); err != nil {
				t.Fatalf("unpublished generation advanced the in-memory floor: %v", err)
			}
			client.Close()
			mustCascadeClient(t, config)
		})
	}
}

// Removing every CRL source must not drop a retained floor while its issuer
// is still in CAFile: the previous generation stays active and restart fails
// closed until a covering CRL is configured again.
func TestCascadeCRLSourceRemovalFailsClosed(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	config := cascadeConfig(t, pki, "https://relay.example")
	path := writeTestFile(t, t.TempDir(), "relay.crl", pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)
	config.CRLFiles = []string{path}
	client := mustCascadeClient(t, config)
	old := client.tlsMaterial.material
	client.tlsMaterial.configFiles.CRLFiles = nil
	client.tlsMaterial.configFiles.CRLURLs = nil
	if err := client.ReloadTLS(); err == nil || !strings.Contains(err.Error(), "retained floor") {
		t.Fatalf("reload without CRL sources: %v", err)
	}
	client.tlsMaterial.mu.Lock()
	retained := client.tlsMaterial.material == old
	client.tlsMaterial.mu.Unlock()
	if !retained {
		t.Fatal("removed CRL source displaced the valid generation")
	}
	client.tlsMaterial.configFiles.CRLFiles = []string{path}
	if err := client.ReloadTLS(); err != nil {
		t.Fatalf("restored CRL source rejected: %v", err)
	}
	client.Close()
	config.CRLFiles = nil
	if restarted, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "retained floor") {
		if restarted != nil {
			restarted.Close()
		}
		t.Fatalf("restart without CRL sources: %v", err)
	}
	config.CRLFiles = []string{path}
	mustCascadeClient(t, config)
}

// Dropping one issuer's source while another source remains is rejected too:
// its retained floor is still inside the CAFile trust domain.
func TestCascadeCRLDroppedIssuerFailsClosed(t *testing.T) {
	pki, other := newCascadePKI(t), newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	config := cascadeConfig(t, pki, "https://relay.example")
	config.CAFile = writeTestFile(t, filepath.Dir(config.CAFile), "ca-both.pem", append(append([]byte(nil), pki.pem...), other.pem...), 0600)
	first := writeTestFile(t, t.TempDir(), "first.crl", pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)
	second := writeTestFile(t, filepath.Dir(first), "second.crl", other.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)
	config.CRLFiles = []string{first, second}
	client := mustCascadeClient(t, config)
	old := client.tlsMaterial.material
	client.tlsMaterial.configFiles.CRLFiles = []string{first}
	if err := client.ReloadTLS(); err == nil || !strings.Contains(err.Error(), "retained floor") {
		t.Fatalf("dropped issuer was accepted: %v", err)
	}
	client.tlsMaterial.mu.Lock()
	retained := client.tlsMaterial.material == old
	client.tlsMaterial.mu.Unlock()
	if !retained {
		t.Fatal("dropped issuer displaced the valid generation")
	}
	client.tlsMaterial.configFiles.CRLFiles = []string{first, second}
	if err := client.ReloadTLS(); err != nil {
		t.Fatalf("restored issuer source rejected: %v", err)
	}
	client.Close()
	config.CRLFiles = []string{first}
	if restarted, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "retained floor") {
		if restarted != nil {
			restarted.Close()
		}
		t.Fatalf("restart without the issuer source: %v", err)
	}
	config.CRLFiles = []string{first, second}
	mustCascadeClient(t, config)
}

// A retained floor keeps applying when its issuer is no longer listed in
// CAFile but still chains to a trusted CA: the relay can supply that
// intermediate, so the verified chain decides.
func TestCascadeCRLFloorAppliesToUnlistedIntermediate(t *testing.T) {
	root := newCascadePKI(t)
	intermediate := newIntermediatePKI(t, root)
	now := time.Now().Truncate(time.Second)
	leaf, _, _ := intermediate.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	leaf.Certificate = append(leaf.Certificate, intermediate.ca.Raw)
	relay := cascadeServer(t, root, leaf, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"servicesVersion":1,"services":[]}`))
	}))
	config := cascadeConfig(t, root, relay.URL)
	directory := filepath.Dir(config.CAFile)
	config.CAFile = writeTestFile(t, directory, "intermediate.pem", intermediate.pem, 0600)
	crlPath := writeTestFile(t, t.TempDir(), "intermediate.crl", intermediate.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)
	config.CRLFiles = []string{crlPath}
	client := mustCascadeClient(t, config)
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The intermediate now chains to a CAFile root but is not listed.
	client.tlsMaterial.configFiles.CAFile = writeTestFile(t, directory, "root.pem", root.pem, 0600)
	client.tlsMaterial.configFiles.CRLFiles = nil
	if err := client.ReloadTLS(); err != nil {
		t.Fatalf("trust-domain change without the listed issuer: %v", err)
	}
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("retained floor lapsed for an unlisted intermediate")
	}
	// Restoring the issuer and a fresh CRL at or above the floor recovers.
	client.tlsMaterial.configFiles.CAFile = config.CAFile
	writeTestFile(t, filepath.Dir(crlPath), "intermediate.crl", intermediate.crl(t, 0, 2, now, now.Add(time.Hour), false), 0600)
	client.tlsMaterial.configFiles.CRLFiles = []string{crlPath}
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Revocation through the same issuer is enforced again.
	writeTestFile(t, filepath.Dir(crlPath), "intermediate.crl", intermediate.crl(t, 2, 3, now, now.Add(time.Hour), false), 0600)
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("revoked leaf accepted through the intermediate")
	}
}

// A source that stops answering must keep both the last valid generation and
// its revocation decision; restoring the source publishes a newer CRL.
func TestCascadeCRLSourceFailureRetainsGeneration(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	relayCert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	relay := cascadeServer(t, pki, relayCert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"servicesVersion":1,"services":[]}`))
	}))
	var unavailable atomic.Bool
	var crl atomic.Value
	crl.Store(pki.crl(t, 2, 1, now.Add(-time.Minute), now.Add(time.Hour), false))
	publisher := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Write(crl.Load().([]byte))
	}))
	cert, _, _ := pki.issue(t, 5, x509.ExtKeyUsageServerAuth, "127.0.0.1", now.Add(time.Hour))
	publisher.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	publisher.StartTLS()
	defer publisher.Close()
	config := cascadeConfig(t, pki, relay.URL)
	config.CRLURLs = []string{publisher.URL}
	client := mustCascadeClient(t, config)
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("revoked relay accepted")
	}
	old := client.tlsMaterial.material
	unavailable.Store(true)
	if err := client.ReloadTLS(); err == nil {
		t.Fatal("unavailable CRL source replaced the generation")
	}
	client.tlsMaterial.mu.Lock()
	retained := client.tlsMaterial.material == old
	client.tlsMaterial.mu.Unlock()
	if !retained {
		t.Fatal("unavailable CRL source displaced the valid generation")
	}
	if _, err := client.Services(context.Background()); err == nil {
		t.Fatal("revocation dropped when the CRL source failed")
	}
	unavailable.Store(false)
	crl.Store(pki.crl(t, 0, 2, now, now.Add(time.Hour), false))
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatal(err)
	}
}

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

func TestCascadeCRLPEMRejectsSkippedBlocks(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	first := pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false)
	last := pki.crl(t, 0, 2, now, now.Add(time.Hour), false)
	bad := []byte("-----BEGIN X509 CRL-----\nnot base64!\n-----END X509 CRL-----\n")
	config := cascadeConfig(t, pki, "https://relay.example")
	path := writeTestFile(t, t.TempDir(), "bundle.pem", first, 0600)
	config.CRLFiles = []string{path}
	client := mustCascadeClient(t, config)
	old := client.tlsMaterial.material
	for _, data := range [][]byte{
		append(append(append([]byte(nil), first...), bad...), last...),
		append(append([]byte(nil), bad...), last...),
		append([]byte("garbage prefix\n"), last...),
	} {
		writeTestFile(t, filepath.Dir(path), "bundle.pem", data, 0600)
		startup := config
		startup.InstanceIDDir = filepath.Join(t.TempDir(), "fresh-cache")
		if accepted, err := NewClient(startup); err == nil || !strings.Contains(err.Error(), "PEM") {
			if accepted != nil {
				accepted.Close()
			}
			t.Fatalf("skipped malformed CRL data at startup: %v", err)
		}
		if err := client.ReloadTLS(); err == nil || !strings.Contains(err.Error(), "PEM") {
			t.Fatalf("skipped malformed CRL data on reload: %v", err)
		}
		if client.tlsMaterial.material != old {
			t.Fatal("malformed bundle displaced the valid generation")
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

// TestRetainedFloorNameMatch covers the pinned-leaf fallback: without the
// issuer certificate the floor key cannot be checked, so only the issuer name
// can rule a floor out.
func TestRetainedFloorNameMatch(t *testing.T) {
	manager := &clientTLS{crlFloors: []crlWatermark{{Subject: []byte("issuer-a"), PublicKey: []byte("key")}}}
	if !manager.mayRetainFloorForName([]byte("issuer-a")) {
		t.Fatal("a floor with the peer's issuer name was not treated as applicable")
	}
	if manager.mayRetainFloorForName([]byte("issuer-b")) {
		t.Fatal("a floor for another issuer name was treated as applicable")
	}
	if manager.mayRetainFloorForName(nil) {
		t.Fatal("an empty issuer name matched a retained floor")
	}
}
