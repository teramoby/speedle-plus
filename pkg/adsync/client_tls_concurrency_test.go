// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countedTLSConn struct {
	net.Conn
	closes     atomic.Int32
	entered    chan struct{}
	release    chan struct{}
	afterClose func()
}

func (c *countedTLSConn) Close() error {
	if c.closes.Add(1) == 1 && c.entered != nil {
		close(c.entered)
		<-c.release
	}
	if c.afterClose != nil {
		c.afterClose()
	}
	return c.Conn.Close()
}

func TestCascadeConcurrentCloseClosesUnderlyingOnce(t *testing.T) {
	for _, ownerFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "connection first", true: "owner first"}[ownerFirst], func(t *testing.T) {
			raw, peer := net.Pipe()
			defer peer.Close()
			counted := &countedTLSConn{Conn: raw, entered: make(chan struct{}), release: make(chan struct{})}
			manager := &clientTLS{connections: make(map[*clientTLSConn]struct{})}
			conn := &clientTLSConn{Conn: counted, owner: manager}
			manager.connections[conn] = struct{}{}
			client := &Client{tlsMaterial: manager, http: &http.Client{}}
			ownerDone, connDone := make(chan struct{}), make(chan struct{})
			closeOwner := func() { defer close(ownerDone); client.Close() }
			closeConn := func() { defer close(connDone); conn.Close() }
			if ownerFirst {
				go closeOwner()
			} else {
				go closeConn()
			}
			<-counted.entered
			if ownerFirst {
				go closeConn()
			} else {
				go closeOwner()
			}
			// The wrapper must remain owned until its underlying Close completes.
			select {
			case <-ownerDone:
				t.Error("owner returned while the underlying connection was still open")
			case <-time.After(50 * time.Millisecond):
			}
			close(counted.release)
			<-ownerDone
			<-connDone
			conn.Close()
			client.Close()
			if counted.closes.Load() != 1 {
				t.Fatalf("underlying closed %d times", counted.closes.Load())
			}
			if len(manager.connections) != 0 {
				t.Fatal("closed connection still registered")
			}
		})
	}
}

func TestCascadeVerificationDoesNotBlockLifecycle(t *testing.T) {
	for _, action := range []string{"close", "reload"} {
		t.Run(action, func(t *testing.T) {
			pki := newCascadePKI(t)
			client := mustCascadeClient(t, cascadeConfig(t, pki, "https://relay.example"))
			manager := client.tlsMaterial
			cert, _, _ := pki.issue(t, 2, x509.ExtKeyUsageServerAuth, "relay.example", time.Now().Add(time.Hour))
			leaf, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			roots := x509.NewCertPool()
			roots.AddCertWithConstraint(pki.ca, func([]*x509.Certificate) error { close(entered); <-release; return nil })
			manager.material.roots = roots // Before any concurrent access to this generation.
			verified := make(chan error, 1)
			go func() {
				verified <- manager.verifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}})
			}()
			<-entered
			if action == "reload" {
				_, certPEM, keyPEM := pki.issue(t, 4, x509.ExtKeyUsageClientAuth, "", time.Now().Add(time.Hour))
				if err := os.WriteFile(manager.configFiles.ClientCertFile, certPEM, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(manager.configFiles.ClientKeyFile, keyPEM, 0600); err != nil {
					t.Fatal(err)
				}
			}
			completed := make(chan error, 1)
			go func() {
				if action == "close" {
					completed <- client.Close()
				} else {
					completed <- client.ReloadTLS()
				}
			}()
			select {
			case err := <-completed:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(time.Second):
				t.Error("certificate verification blocked the lifecycle mutex")
				unblock()
				<-completed
			}
			unblock()
			if err := <-verified; err == nil {
				t.Fatal("verification accepted a closed or replaced generation")
			}
			if action == "close" {
				if _, err := manager.dialContext(context.Background(), "tcp", "127.0.0.1:1"); !errors.Is(err, net.ErrClosed) {
					t.Fatalf("dial after close: %v", err)
				}
			}
		})
	}
}

// Publication must finish both durable and in-memory state before disconnecting
// users of the old generation. Restart must see the same revocation decision.
func TestCascadeCRLPublicationCommitsBeforeDisconnect(t *testing.T) {
	pki := newCascadePKI(t)
	now := time.Now().Truncate(time.Second)
	config := cascadeConfig(t, pki, "https://relay.example")
	path := writeTestFile(t, t.TempDir(), "relay.crl", pki.crl(t, 0, 1, now.Add(-time.Minute), now.Add(time.Hour), false), 0600)
	config.CRLFiles = []string{path}
	client := mustCascadeClient(t, config)
	manager := client.tlsMaterial
	old := manager.material
	replacement := pki.crl(t, 2, 2, now, now.Add(time.Hour), false)
	writeTestFile(t, filepath.Dir(path), "relay.crl", replacement, 0600)
	raw, peer := net.Pipe()
	defer peer.Close()
	observed := false
	counted := &countedTLSConn{Conn: raw, afterClose: func() {
		observed = true
		if manager.material == old || manager.material.crls[0].Number.Int64() != 2 {
			t.Error("disconnected before generation publication")
		}
		data, err := os.ReadFile(filepath.Join(config.InstanceIDDir, crlWatermarksFile))
		var envelope cacheEnvelope
		if err != nil {
			t.Error(err)
			return
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Error(err)
			return
		}
		if !bytes.Equal(envelope.Payload, manager.floorPayload) {
			t.Error("durable and active floors differ at disconnect")
		}
		var floors []crlWatermark
		if err := json.Unmarshal(envelope.Payload, &floors); err != nil || len(floors) != 1 || floors[0].Number.Int64() != 2 {
			t.Errorf("durable floor did not advance: %v", err)
		}
	}}
	conn := &clientTLSConn{Conn: counted, owner: manager}
	manager.connections[conn] = struct{}{}
	if err := client.ReloadTLS(); err != nil {
		t.Fatal(err)
	}
	if !observed {
		t.Fatal("old connection was not disconnected")
	}
	conn.Close()
	if counted.closes.Load() != 1 {
		t.Fatal("reload and wrapper both closed the connection")
	}
	client.Close()
	restarted := mustCascadeClient(t, config)
	if restarted.tlsMaterial.material.crls[0].Number.Int64() != 2 {
		t.Fatal("restart lost the committed generation's floor")
	}
}
