// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	log "github.com/sirupsen/logrus"
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

func TestCascadeOldForwardFailureCannotCancelNewSelection(t *testing.T) {
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 51, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	reached, release := make(chan struct{}), make(chan struct{})
	old := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(reached); <-release; w.WriteHeader(503) }))
	replacement := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	config := cascadeConfig(t, pki, old.URL)
	config.UpstreamURLs = []string{old.URL, replacement.URL}
	client := mustCascadeClient(t, config)
	var oldCanceled, newCanceled atomic.Bool
	client.selectEndpoint(client.upstreams[0], func() { oldCanceled.Store(true) })
	client.approveCandidate()
	done := make(chan error, 1)
	go func() {
		_, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`))
		done <- err
	}()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("old forward did not start")
	}
	client.selectEndpoint(client.upstreams[1], func() { newCanceled.Store(true) })
	client.approveCandidate()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("old failure was accepted")
	}
	if oldCanceled.Load() || newCanceled.Load() {
		t.Fatal("old request canceled a replacement attempt")
	}
	if !client.FallbackAvailable() {
		t.Fatal("old Retry-After disabled the healthy replacement")
	}
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); err == nil {
		t.Fatal("current failure accepted")
	}
	if !newCanceled.Load() {
		t.Fatal("current failure did not request failover")
	}
}

func TestCascadeCandidateCannotServeDataOrFallbackBeforeAdmission(t *testing.T) {
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 61, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	var requests atomic.Int32
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	client := mustCascadeClient(t, cascadeConfig(t, pki, server.URL))
	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{})
	client.selectEndpoint(client.upstreams[0], func() {})
	if client.FallbackAvailable() {
		t.Fatal("unadmitted candidate reported fallback-ready")
	}
	if _, err := client.Services(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("candidate data error: %v", err)
	}
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("candidate fallback error: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("unadmitted candidate received data/authorization traffic")
	}
	ack := cascadeAck(t, nil)
	ack.Services = []channelAckService{}
	channel.recordSent(*ack.AckSeq)
	if err := channel.applyAck(&ack, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !client.FallbackAvailable() {
		t.Fatal("valid ack did not admit the candidate")
	}
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatal("admitted endpoint was not used")
	}
	// A later disqualifying ack immediately quarantines the endpoint.
	ack.UpstreamState = "stale"
	if err := channel.applyAck(&ack, nil, nil); err == nil {
		t.Fatal("stale candidate accepted")
	}
	if client.FallbackAvailable() {
		t.Fatal("rejected endpoint stayed eligible")
	}
}

func TestCascadeCallbackWaitCannotExtendFreshness(t *testing.T) {
	pki := newCascadePKI(t)
	client := mustCascadeClient(t, cascadeConfig(t, pki, "https://relay.example"))
	machine := newChannelMachine(t)
	channel := newTestChannel(t, client, machine, ChannelOptions{OnChain: func([]string, time.Time) error { time.Sleep(60 * time.Millisecond); return nil }})
	ack := cascadeAck(t, nil)
	ack.Services = []channelAckService{}
	budget := int64(20)
	ack.UpstreamFreshForMs = &budget
	channel.recordSent(*ack.AckSeq)
	if err := channel.applyAck(&ack, nil, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expired callback budget: %v", err)
	}
	if !machine.Instance().LastConfirmedAckAt.IsZero() {
		t.Fatal("expired ack confirmed liveness")
	}
	if machine.upstreamDeadline.After(time.Now()) {
		t.Fatal("callback wait renewed the deadline")
	}
}

func TestCascadeInvalidCadenceCannotCommitTopology(t *testing.T) {
	pki := newCascadePKI(t)
	client := mustCascadeClient(t, cascadeConfig(t, pki, "https://relay.example"))
	machine := newChannelMachine(t)
	var topologyCalls int
	channel := newTestChannel(t, client, machine, ChannelOptions{OnChain: func([]string, time.Time) error { topologyCalls++; return nil }})
	ack := cascadeAck(t, nil)
	ack.StaleAfter = "15s"
	channel.recordSent(*ack.AckSeq)
	if err := channel.applyAck(&ack, nil, nil); !errors.Is(err, ErrProtocol) {
		t.Fatalf("invalid cadence: %v", err)
	}
	if topologyCalls != 0 || machine.upstreamState != "" || !machine.Instance().LastConfirmedAckAt.IsZero() {
		t.Fatal("rejected ack committed topology, freshness or liveness")
	}
}

func TestCascadeServiceErrorsDoNotTriggerFailover(t *testing.T) {
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 62, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	var status atomic.Int32
	status.Store(404)
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(int(status.Load())) }))
	client := mustCascadeClient(t, cascadeConfig(t, pki, server.URL))
	var canceled atomic.Bool
	client.selectEndpoint(client.upstreams[0], func() { canceled.Store(true) })
	client.approveCandidate()
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); !errors.Is(err, ErrServiceNotFound) {
		t.Fatal(err)
	}
	if canceled.Load() {
		t.Fatal("service-level error canceled a healthy node")
	}
	status.Store(503)
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); err == nil {
		t.Fatal("503 accepted")
	}
	if !canceled.Load() {
		t.Fatal("node-level failure did not trigger failover")
	}
}

func TestCascadeExpiredAckCannotCommitTopology(t *testing.T) {
	pki := newCascadePKI(t)
	client := mustCascadeClient(t, cascadeConfig(t, pki, "https://relay.example"))
	var topologyCalls int
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{OnChain: func([]string, time.Time) error { topologyCalls++; return nil }})
	ack := cascadeAck(t, nil)
	channel.recordSent(*ack.AckSeq)
	channel.mu.Lock()
	channel.sentAt[*ack.AckSeq] = time.Now().Add(-time.Minute)
	channel.mu.Unlock()
	if err := channel.applyAck(&ack, nil, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expired ack: %v", err)
	}
	if topologyCalls != 0 {
		t.Fatal("expired live ack committed topology")
	}
}

func TestCascadeRetryAfterSurvivesReturnToSameUpstream(t *testing.T) {
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 71, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "60"); w.WriteHeader(429) }))
	config := cascadeConfig(t, pki, server.URL)
	config.UpstreamURLs = []string{server.URL, "https://alternate.example"}
	client := mustCascadeClient(t, config)
	client.selectEndpoint(client.upstreams[0], func() {})
	client.approveCandidate()
	if _, err := client.ForwardFallback(context.Background(), FallbackIsAllowed, []byte(`{}`)); !errors.Is(err, ErrRateLimited) {
		t.Fatal(err)
	}
	if client.FallbackAvailable() {
		t.Fatal("Retry-After was not retained")
	}
	// A single-upstream reconnect must honor its own hint.
	client.selectEndpoint(client.upstreams[0], func() {})
	client.approveCandidate()
	if client.FallbackAvailable() {
		t.Fatal("same-address reconnect discarded Retry-After")
	}
	// Another endpoint gets its own readiness; returning restores the first hint.
	client.selectEndpoint(client.upstreams[1], func() {})
	client.approveCandidate()
	if !client.FallbackAvailable() {
		t.Fatal("first endpoint's hint suppressed the alternate")
	}
	client.selectEndpoint(client.upstreams[0], func() {})
	client.approveCandidate()
	if client.FallbackAvailable() {
		t.Fatal("returning to an address discarded its hint")
	}
}

func TestCascadeReconnectBackoffResetsAfterHealthyPeriod(t *testing.T) {
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 81, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	ack := cascadeAck(t, nil)
	ack.Services = []channelAckService{}
	var requests atomic.Int32
	admitted, next := make(chan time.Time, 1), make(chan time.Time, 1)
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requests.Add(1)
		if count == 5 {
			next <- time.Now()
		}
		if count != 4 {
			w.WriteHeader(503)
			return
		}
		conn, err := channelUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var frame struct {
			Seq uint64 `json:"seq"`
		}
		if json.Unmarshal(payload, &frame) != nil {
			return
		}
		ack.AckSeq = &frame.Seq
		if err := conn.WriteJSON(ack); err != nil {
			return
		}
		time.Sleep(80 * time.Millisecond)
		admitted <- time.Now()
	}))
	client := mustCascadeClient(t, cascadeConfig(t, pki, server.URL))
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportWebSocket})
	channel.backoff = Backoff{Initial: 100 * time.Millisecond, Max: 800 * time.Millisecond, Jitter: 0.2, Rand: func() float64 { return 0.5 }}
	runChannel(t, channel)
	var start, finish time.Time
	select {
	case start = <-admitted:
	case <-time.After(5 * time.Second):
		t.Fatal("source did not admit after startup failures")
	}
	select {
	case finish = <-next:
	case <-time.After(2 * time.Second):
		t.Fatal("no reconnect after admitted channel ended")
	}
	if gap := finish.Sub(start); gap >= 600*time.Millisecond {
		t.Fatalf("successful admission did not reset startup backoff: %s", gap)
	}
}

func TestCascadeDownstreamSyncingDoesNotChangeHealthyLocalService(t *testing.T) {
	machine := newChannelMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 1), testAckService(testServiceID2, "orders", 1)))
	mustApply(t, machine, testServiceState(testServiceID1, "books", 1))
	if got := machine.Upstream(); got.State != "syncing" {
		t.Fatalf("partial relay state: %+v", got)
	}
	status, known := machine.Status("books")
	if !known || status.Service.State != SyncStateInSync || status.Instance.Liveness != LivenessLive {
		t.Fatalf("pending sibling changed healthy local service: %+v", status)
	}
	mustApply(t, machine, testServiceState(testServiceID2, "orders", 1))
	if got := machine.Upstream(); got.State != "live" {
		t.Fatalf("complete relay state: %+v", got)
	}
}

func TestCascadeGETAddressAndGenerationAreOneSelection(t *testing.T) {
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 91, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	old := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	replacement := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ServicesResponse{ServicesVersion: 1, Services: []Service{}})
	}))
	config := cascadeConfig(t, pki, old.URL)
	config.UpstreamURLs = []string{old.URL, replacement.URL}
	client := mustCascadeClient(t, config)
	newTestChannel(t, client, newChannelMachine(t), ChannelOptions{})
	client.selectEndpoint(client.upstreams[0], func() {})
	client.approveCandidate()
	endpoint := client.endpoint("services")
	var replacementCanceled atomic.Bool
	client.selectEndpoint(client.upstreams[1], func() { replacementCanceled.Store(true) })
	client.approveCandidate()
	var response ServicesResponse
	if _, err := client.get(context.Background(), endpoint, nil, nil, false, &response); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("old GET: %v", err)
	}
	if replacementCanceled.Load() {
		t.Fatal("old GET was attributed to the replacement generation")
	}
	if _, err := client.Services(context.Background()); err != nil {
		t.Fatalf("replacement GET: %v", err)
	}
}

func TestCascadeFlappingAckDoesNotResetBackoff(t *testing.T) {
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 101, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	template := cascadeAck(t, nil)
	template.Services = []channelAckService{}
	attempts := make(chan time.Time, 20)
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case attempts <- time.Now():
		default:
		}
		conn, err := channelUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var frame struct {
			Seq uint64 `json:"seq"`
		}
		if json.Unmarshal(payload, &frame) != nil {
			return
		}
		ack := template
		ack.AckSeq = &frame.Seq
		_ = conn.WriteJSON(ack)
	}))
	client := mustCascadeClient(t, cascadeConfig(t, pki, server.URL))
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportWebSocket})
	channel.heartbeatOverride = time.Second
	channel.backoff = Backoff{Initial: 40 * time.Millisecond, Max: 160 * time.Millisecond, Jitter: 0.2, Rand: func() float64 { return 0.5 }}
	runChannel(t, channel)
	var times []time.Time
	for len(times) < 3 {
		select {
		case at := <-attempts:
			times = append(times, at)
		case <-time.After(3 * time.Second):
			t.Fatal("flapping reconnects stopped")
		}
	}
	if gap := times[2].Sub(times[1]); gap < 70*time.Millisecond {
		t.Fatalf("one ACK reset flapping backoff: %s", gap)
	}
}

func TestCascadeSentReferencesExpireByFreshnessWindow(t *testing.T) {
	pki := newCascadePKI(t)
	client := mustCascadeClient(t, cascadeConfig(t, pki, "https://relay.example"))
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{})
	channel.recordSent(1)
	channel.mu.Lock()
	channel.sentAt[1] = time.Now().Add(-maxStopAfter - time.Second)
	channel.mu.Unlock()
	channel.recordSent(2)
	channel.mu.Lock()
	_, old := channel.sentAt[1]
	_, current := channel.sentAt[2]
	channel.mu.Unlock()
	if old || !current {
		t.Fatal("expired reference retained, or current reference discarded")
	}
	ack := cascadeAck(t, nil)
	if _, err := channel.inheritedDeadline(&ack); !errors.Is(err, ErrProtocol) {
		t.Fatalf("expired old ack reference accepted: %v", err)
	}
}

func TestLegacyCascadeDecoderRejectsExtendedAck(t *testing.T) {
	// This is the b29088be channelAck wire shape. Its decodeStrict helper is
	// unchanged: unknown fields are rejected before an ack can reach Machine.
	type legacyAck struct {
		Type              string              `json:"type"`
		ServicesVersion   int64               `json:"servicesVersion"`
		Services          []channelAckService `json:"services"`
		ServerTime        int64               `json:"serverTime"`
		HeartbeatInterval string              `json:"heartbeatInterval"`
		StaleAfter        string              `json:"staleAfter"`
		StopAfter         string              `json:"stopAfter"`
		Commands          []channelCommand    `json:"commands"`
	}
	old := validChannelAck()
	legacyBytes, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyAck
	if err := decodeStrict(legacyBytes, &legacy); err != nil {
		t.Fatalf("legacy control ACK: %v", err)
	}
	for _, state := range []string{"live", "syncing", "stale"} {
		ack := cascadeAck(t, nil)
		ack.UpstreamState = state
		payload, err := json.Marshal(ack)
		if err != nil {
			t.Fatal(err)
		}
		if err := decodeStrict(payload, &legacy); !errors.Is(err, ErrProtocol) {
			t.Fatalf("legacy decoder accepted %s cascade metadata: %v", state, err)
		}
	}
}

func TestCascadeCanonicalUpstreamDuplicates(t *testing.T) {
	pki := newCascadePKI(t)
	for _, urls := range [][]string{{"https://Relay.example", "https://relay.example:443/"}, {"https://[::1]", "https://[::1]:443/"}} {
		config := cascadeConfig(t, pki, urls[0])
		config.UpstreamURLs = urls
		if _, err := NewClient(config); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("accepted equivalent upstreams %v: %v", urls, err)
		}
	}
	config := cascadeConfig(t, pki, "https://Relay.example:443/")
	config.UpstreamURLs = []string{config.BaseURL}
	client := mustCascadeClient(t, config)
	if client.base.String() != "https://relay.example" {
		t.Fatalf("canonical base: %s", client.base)
	}
}

func TestCascadeFailureWarningsAreRateLimitedAndRedacted(t *testing.T) {
	var output bytes.Buffer
	logger := log.StandardLogger()
	previous, level := logger.Out, logger.GetLevel()
	logger.SetOutput(&output)
	logger.SetLevel(log.WarnLevel)
	defer func() { logger.SetOutput(previous); logger.SetLevel(level) }()
	pki := newCascadePKI(t)
	cert, _, _ := pki.issue(t, 111, x509.ExtKeyUsageServerAuth, "127.0.0.1", time.Now().Add(time.Hour))
	var requests atomic.Int32
	server := cascadeServer(t, pki, cert, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(503) }))
	config := cascadeConfig(t, pki, server.URL+"/private-prefix-must-not-be-logged")
	client := mustCascadeClient(t, config)
	channel := newTestChannel(t, client, newChannelMachine(t), ChannelOptions{Transport: TransportWebSocket})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- channel.Run(ctx) }()
	waitFor(t, 3*time.Second, "repeated failed endpoint attempts", func() bool { return requests.Load() >= 3 })
	cancel()
	<-done
	message := output.String()
	if strings.Count(message, "policy-source cascade upstream attempt failed") != 1 || !strings.Contains(message, "failure_class") || !strings.Contains(message, "unavailable") || !strings.Contains(message, server.URL) {
		t.Fatalf("failure warning: %s", message)
	}
	if strings.Contains(message, "private-prefix") || strings.Contains(message, testAPIKey) {
		t.Fatal("warning exposed a path or credential")
	}
}
