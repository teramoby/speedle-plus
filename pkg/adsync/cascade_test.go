// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cascadeAck(t *testing.T, state *ServiceState) channelAck {
	t.Helper()
	ack := validChannelAck()
	id, err := NewInstanceID()
	if err != nil {
		t.Fatal(err)
	}
	ack.Chain = []string{id}
	ack.UpstreamState = "live"
	budget := int64(60000)
	ack.UpstreamFreshForMs = &budget
	seq := uint64(1)
	ack.AckSeq = &seq
	if state != nil {
		ack.Services[0].Head = state.Revision
		ack.Services[0].OriginHead = state.Revision
		ack.Services[0].HeadStateHash = state.StateHash
	}
	return ack
}

// This source speaks both transports using real mTLS and never accepts bearer
// credentials. Each candidate is independently observable in the failover test.
func failoverSource(t *testing.T, pki *cascadePKI, host string, serve func() (int, channelAck), delay ...time.Duration) *httptest.Server {
	t.Helper()
	cert, _, _ := pki.issue(t, 42, x509.ExtKeyUsageServerAuth, host, time.Now().Add(time.Hour))
	return cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("cascade sent bearer credentials")
		}
		status, ack := serve()
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		if r.URL.Path == APIPrefix+"/channel" {
			conn, err := channelUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			for {
				_, payload, readErr := conn.ReadMessage()
				if readErr != nil {
					return
				}
				var frame struct {
					Seq uint64 `json:"seq"`
				}
				if json.Unmarshal(payload, &frame) != nil {
					return
				}
				_, ack = serve()
				ack.AckSeq = &frame.Seq
				if len(delay) > 0 {
					time.Sleep(delay[0])
				}
				if err = conn.WriteJSON(ack); err != nil {
					return
				}
			}
		}
		if r.URL.Path == APIPrefix+"/channel/poll" {
			var frame struct {
				Seq uint64 `json:"seq"`
			}
			_ = json.NewDecoder(r.Body).Decode(&frame)
			ack.AckSeq = &frame.Seq
			if len(delay) > 0 {
				time.Sleep(delay[0])
			}
			_ = json.NewEncoder(w).Encode(ack)
			return
		}
		_ = json.NewEncoder(w).Encode(ServicesResponse{ServicesVersion: 1, Services: []Service{}})
	}))
}

func TestCascadeOrderedFailoverAndPreferredRecovery(t *testing.T) {
	for _, mode := range []Transport{TransportWebSocket, TransportLongPoll, TransportAuto} {
		t.Run(string(mode), func(t *testing.T) {
			pki := newCascadePKI(t)
			retained := testServiceState(testServiceID1, "books", 2)
			good := cascadeAck(t, retained)
			var recovered atomic.Bool
			var mu sync.Mutex
			var visited []string
			mark := func(name string) {
				mu.Lock()
				defer mu.Unlock()
				if len(visited) == 0 || visited[len(visited)-1] != name {
					visited = append(visited, name)
				}
			}
			primary := failoverSource(t, pki, "127.0.0.1", func() (int, channelAck) {
				mark("primary")
				if !recovered.Load() {
					return 503, good
				}
				return 200, good
			})
			wrongTLS := failoverSource(t, pki, "elsewhere.invalid", func() (int, channelAck) { t.Error("wrong SAN reached HTTP"); return 200, good })
			denied := failoverSource(t, pki, "127.0.0.1", func() (int, channelAck) { mark("denied"); return 403, good })
			staleAck := good
			staleAck.UpstreamState = "stale"
			stale := failoverSource(t, pki, "127.0.0.1", func() (int, channelAck) { mark("stale"); return 200, staleAck })
			syncingAck := good
			syncingAck.UpstreamState = "syncing"
			syncingAck.Services = []channelAckService{}
			syncing := failoverSource(t, pki, "127.0.0.1", func() (int, channelAck) { mark("syncing"); return 200, syncingAck })
			olderAck := cascadeAck(t, testServiceState(testServiceID1, "books", 1))
			older := failoverSource(t, pki, "127.0.0.1", func() (int, channelAck) { mark("older"); return 200, olderAck })
			// The alternate has only a DNS SAN, unlike the primary's IP SAN. Each
			// transport must verify the selected URL's host rather than the first host.
			alternate := failoverSource(t, pki, "localhost", func() (int, channelAck) { mark("alternate"); return 200, good })
			alternateURL := strings.Replace(alternate.URL, "127.0.0.1", "localhost", 1)
			config := cascadeConfig(t, pki, primary.URL)
			config.UpstreamURLs = []string{primary.URL, wrongTLS.URL, denied.URL, stale.URL, syncing.URL, older.URL, alternateURL}
			client := mustCascadeClient(t, config)
			machine := newChannelMachine(t)
			mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 2)))
			mustApply(t, machine, retained)
			channel := newTestChannel(t, client, machine, ChannelOptions{Transport: mode})
			channel.failbackInterval = 200 * time.Millisecond
			runChannel(t, channel)
			waitFor(t, 5*time.Second, "healthy alternate", func() bool {
				return client.selectedBase().String() == alternateURL && machine.Instance().Liveness == LivenessLive
			})
			if _, err := client.Services(context.Background()); err != nil {
				t.Fatalf("alternate HTTPS host verification: %v", err)
			}
			if got := machine.Services(); len(got) != 1 || got[0].Revision != 2 || got[0].State != SyncStateInSync {
				t.Fatalf("candidate changed retained membership/data: %+v", got)
			}
			mu.Lock()
			seen := strings.Join(visited, ",")
			mu.Unlock()
			if seen != "primary,denied,stale,syncing,older,alternate" {
				t.Fatalf("failover order: %s", seen)
			}
			recovered.Store(true)
			waitFor(t, 5*time.Second, "preferred upstream recovery", func() bool {
				return client.selectedBase().String() == primary.URL && machine.Instance().Liveness == LivenessLive
			})
		})
	}
}

func TestCascadeFreshnessExpiresWithoutAnotherAck(t *testing.T) {
	pki := newCascadePKI(t)
	ack := cascadeAck(t, nil)
	ack.Services = []channelAckService{}
	budget := int64(120)
	ack.UpstreamFreshForMs = &budget
	var initial atomic.Bool
	cert, _, _ := pki.issue(t, 7, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	primary := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := channelUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
		if err = conn.WriteJSON(ack); err != nil {
			return
		}
		initial.Store(true)
		// Pongs keep the socket live; only the inherited freshness expires.
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	fresh := cascadeAck(t, nil)
	fresh.Services = []channelAckService{}
	secondary := failoverSource(t, pki, "127.0.0.1", func() (int, channelAck) { return 200, fresh })
	config := cascadeConfig(t, pki, primary.URL)
	config.UpstreamURLs = []string{primary.URL, secondary.URL}
	client := mustCascadeClient(t, config)
	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{Transport: TransportWebSocket})
	runChannel(t, channel)
	waitFor(t, 3*time.Second, "first ack", initial.Load)
	waitFor(t, 3*time.Second, "freshness failover", func() bool {
		return client.selectedBase().String() == secondary.URL && machine.Upstream().State == "live"
	})
}

func TestCascadeRejectsInvalidMetadataBeforeMembershipChanges(t *testing.T) {
	pki := newCascadePKI(t)
	client := mustCascadeClient(t, cascadeConfig(t, pki, "https://relay.example"))
	self, _ := client.InstanceID()
	retained := testServiceState(testServiceID1, "books", 2)
	for _, tc := range []struct {
		name   string
		mutate func(*channelAck)
	}{
		{"missing chain", func(a *channelAck) { a.Chain = nil }},
		{"invalid uuid", func(a *channelAck) { a.Chain = []string{"bad"} }},
		{"duplicate", func(a *channelAck) { a.Chain = append(a.Chain, a.Chain[0]) }},
		{"self loop", func(a *channelAck) { a.Chain = append(a.Chain, self) }},
		{"missing budget", func(a *channelAck) { a.UpstreamFreshForMs = nil }},
		{"negative budget", func(a *channelAck) { v := int64(-1); a.UpstreamFreshForMs = &v }},
		{"excess budget", func(a *channelAck) { v := int64(1 << 62); a.UpstreamFreshForMs = &v }},
		{"unknown state", func(a *channelAck) { a.UpstreamState = "unknown" }},
		{"stale", func(a *channelAck) { a.UpstreamState = "stale"; a.Services = nil }},
		{"syncing", func(a *channelAck) { a.UpstreamState = "syncing"; a.Services = nil }},
		{"zero budget", func(a *channelAck) { v := int64(0); a.UpstreamFreshForMs = &v }},
		{"regression", func(a *channelAck) { a.Services[0].Head = 1; a.Services[0].OriginHead = 1 }},
		{"conflicting hash", func(a *channelAck) { a.Services[0].HeadStateHash = Hash{1} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := newChannelMachine(t)
			mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 2)))
			mustApply(t, machine, retained)
			channel := newTestChannel(t, client, machine, ChannelOptions{})
			ack := cascadeAck(t, retained)
			channel.recordSent(*ack.AckSeq)
			tc.mutate(&ack)
			before := machine.Instance().LastConfirmedAckAt
			if err := channel.applyAck(&ack, nil, nil); err == nil {
				t.Fatal("invalid candidate accepted")
			}
			if got := machine.Services(); len(got) != 1 || got[0].Revision != 2 || got[0].State == SyncStateRemoved {
				t.Fatalf("retained data changed: %+v", got)
			}
			if !machine.Instance().LastConfirmedAckAt.Equal(before) {
				t.Fatal("rejected candidate refreshed liveness")
			}
		})
	}
}

func TestCascadeFreshnessBudgetPropagatesAndRecovers(t *testing.T) {
	machine, clock := newTestMachine(t)
	if status := machine.Upstream(); status.State != "syncing" {
		t.Fatal(status)
	}
	mustAck(t, machine, ackWith())
	machine.observeUpstream("live", 45*time.Second)
	clock.Advance(10 * time.Second)
	if got := machine.Upstream(); got.State != "live" || got.FreshFor != 35*time.Second {
		t.Fatal(got)
	}
	// A parent heartbeat with the remaining budget must not grant a new window.
	mustAck(t, machine, ackWith())
	machine.observeUpstream("live", 35*time.Second)
	clock.Advance(35 * time.Second)
	if got := machine.Upstream(); got.State != "stale" || got.FreshFor != 0 {
		t.Fatal(got)
	}
	if got := machine.Instance(); got.Liveness != LivenessStale {
		t.Fatal(got)
	}
	mustAck(t, machine, ackWith())
	machine.observeUpstream("syncing", 20*time.Second)
	if got := machine.Upstream(); got.State != "syncing" {
		t.Fatal(got)
	}
	machine.observeUpstream("live", time.Hour)
	if got := machine.Upstream(); got.State != "live" || got.FreshFor != 60*time.Second {
		t.Fatal(got)
	}
}

func TestCascadeUpstreamURLValidation(t *testing.T) {
	base, _ := url.Parse("https://relay")
	for _, urls := range [][]string{{"https://other"}, {"https://relay", "https://relay/"}, {"https://relay", "http://other"}, {"https://relay", "https://user:secret@other"}, {"https://relay", "https://other?q=x"}} {
		_, err := parseUpstreams(Config{UpstreamURLs: urls}, base)
		if err == nil {
			t.Fatalf("accepted %v", urls)
		}
	}
	client := &Client{}
	client.FailUpstream()
}

// A real delayed ACK at each hop must shorten, never extend, the root lease.
// Both transports use the same echoed-sequence RTT accounting; this test fails
// if an implementation simply adds the advertised budget at receipt.
func TestCascadeDelayedAcksNeverExtendAncestorDeadline(t *testing.T) {
	for _, mode := range []Transport{TransportWebSocket, TransportLongPoll} {
		t.Run(string(mode), func(t *testing.T) {
			pki := newCascadePKI(t)
			ancestorDeadline := time.Now().Add(900 * time.Millisecond)
			parentStatus := func() UpstreamStatus {
				remaining := time.Until(ancestorDeadline)
				if remaining <= 0 {
					return UpstreamStatus{State: "stale"}
				}
				return UpstreamStatus{State: "live", FreshFor: remaining}
			}
			var machines []*Machine
			for hop := 0; hop < 3; hop++ {
				upstream := parentStatus
				template := cascadeAck(t, nil)
				template.Services = []channelAckService{}
				server := failoverSource(t, pki, "127.0.0.1", func() (int, channelAck) {
					status := upstream()
					ack := template
					ack.UpstreamState = status.State
					budget := max(int64(0), status.FreshFor.Milliseconds())
					ack.UpstreamFreshForMs = &budget
					return 200, ack
				}, 75*time.Millisecond)
				client := mustCascadeClient(t, cascadeConfig(t, pki, server.URL))
				machine := newChannelMachine(t)
				channel := newTestChannel(t, client, machine, ChannelOptions{Transport: mode})
				channel.heartbeatOverride = time.Hour
				runChannel(t, channel)
				waitFor(t, time.Second, "delayed ack", func() bool { return !machine.Instance().LastConfirmedAckAt.IsZero() })
				observedAt := time.Now()
				status := machine.Upstream()
				if status.State != "live" || observedAt.Add(status.FreshFor).After(ancestorDeadline) {
					t.Fatalf("hop %d extended ancestor deadline: remaining=%s ancestor=%s", hop, status.FreshFor, time.Until(ancestorDeadline))
				}
				machines = append(machines, machine)
				parentStatus = machine.Upstream
			}
			if err := sleepContext(context.Background(), time.Until(ancestorDeadline)+10*time.Millisecond); err != nil {
				t.Fatal(err)
			}
			for hop, machine := range machines {
				if machine.Instance().Liveness == LivenessLive || machine.Upstream().State == "live" {
					t.Fatalf("hop %d remained live beyond the ancestor deadline", hop)
				}
			}
		})
	}
}
