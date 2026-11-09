// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

func doRequest(t *testing.T, server *testServer, method, path, token string, body []byte) (*http.Response, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, server.httpServer.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := server.client().Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	return response, payload
}

func TestServerRouteAndMethodErrors(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	statePath := adsync.APIPrefix + "/services/" + testServiceID1
	for _, testCase := range []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "outside prefix", method: http.MethodGet, path: "/elsewhere", status: http.StatusNotFound},
		{name: "post services", method: http.MethodPost, path: adsync.APIPrefix + "/services", status: http.StatusMethodNotAllowed},
		{name: "post channel", method: http.MethodPost, path: adsync.APIPrefix + "/channel", status: http.StatusMethodNotAllowed},
		{name: "post action", method: http.MethodPost, path: statePath + "/state", status: http.StatusMethodNotAllowed},
		{name: "missing action", method: http.MethodGet, path: statePath, status: http.StatusNotFound},
		{name: "extra segment", method: http.MethodGet, path: statePath + "/state/extra", status: http.StatusNotFound},
		{name: "empty service", method: http.MethodGet, path: adsync.APIPrefix + "/services//state", status: http.StatusNotFound},
		{name: "unknown action", method: http.MethodGet, path: statePath + "/wat", status: http.StatusNotFound},
	} {
		response, body := doRequest(t, server, testCase.method, testCase.path, token, nil)
		if response.StatusCode != testCase.status {
			t.Errorf("%s: status %d body %s", testCase.name, response.StatusCode, body)
		}
	}
}

func TestServerAccessors(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		chain: []string{testUpstreamID, testProxyID},
	}, testCredential{downstreamID: testDownstreamID, token: testToken(testDownstreamID, 1)})
	if got := server.server.InstanceID(); got != testProxyID {
		t.Errorf("InstanceID = %q", got)
	}
	chain := server.server.Chain()
	if len(chain) != 2 || chain[0] != testUpstreamID || chain[1] != testProxyID {
		t.Errorf("Chain = %v", chain)
	}
	chain[0] = "mutated"
	if server.server.Chain()[0] != testUpstreamID {
		t.Error("Chain exposes the server's slice")
	}
	if server.server.Credentials() == nil {
		t.Error("Credentials is nil")
	}
	if got := server.server.DownstreamCount(); got != 0 {
		t.Errorf("DownstreamCount = %d", got)
	}
}

func TestWriteJSONMarshalError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusOK, map[string]any{"bad": make(chan int)})
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "internal_error") {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestTrimChangesPage(t *testing.T) {
	large := strings.Repeat("x", 600<<10)
	huge := strings.Repeat("x", 1200<<10)
	page := adsync.ChangesResponse{
		ServiceID: testServiceID1,
		SourceID:  testServiceID1,
		Epoch:     testServiceID1,
		From:      2,
		To:        3,
		Head:      3,
		Commits: []adsync.Commit{
			{Revision: 2, Changes: []adsync.Change{{Op: "upsert", Kind: "authz", ID: "A", SPDL: &large}}},
			{Revision: 3, Changes: []adsync.Change{{Op: "upsert", Kind: "authz", ID: "B", SPDL: &large}}},
		},
		NextSince: 4,
	}
	encoded, err := trimChangesPage(&page)
	if err != nil {
		t.Fatalf("trim page: %v", err)
	}
	if len(encoded) > maxChangesPageBytes {
		t.Errorf("trimmed page is %d bytes", len(encoded))
	}
	if len(page.Commits) != 1 || page.To != 2 || page.NextSince != 3 {
		t.Errorf("trimmed page = commits %d to %d next %d", len(page.Commits), page.To, page.NextSince)
	}

	single := adsync.ChangesResponse{
		Commits: []adsync.Commit{
			{Revision: 2, Changes: []adsync.Change{{Op: "upsert", Kind: "authz", ID: "A", SPDL: &huge}}},
		},
	}
	if _, err := trimChangesPage(&single); !errors.Is(err, ErrHistoryUnavailable) {
		t.Errorf("single oversized commit: %v, want ErrHistoryUnavailable", err)
	}
}

func TestWriteMirrorErrorMapping(t *testing.T) {
	for _, testCase := range []struct {
		err    error
		status int
		body   string
	}{
		{fmt.Errorf("%w: x", ErrUnknownService), http.StatusNotFound, `{"error":"service_not_found"}`},
		{fmt.Errorf("%w: x", ErrServiceRemoved), http.StatusGone, `{"error":"snapshot_required","reason":"source_gone"}`},
		{fmt.Errorf("%w: x", ErrHistoryUnavailable), http.StatusGone, `{"error":"snapshot_required","reason":"history_unavailable"}`},
		{fmt.Errorf("%w: x", ErrIdentityChanged), http.StatusGone, `{"error":"snapshot_required","reason":"epoch_mismatch"}`},
		{fmt.Errorf("%w: x", adsync.ErrIntegrity), http.StatusGone, `{"error":"snapshot_required","reason":"epoch_mismatch"}`},
		{fmt.Errorf("%w: x", ErrSnapshotRequired), http.StatusServiceUnavailable, `{"error":"source_unavailable"}`},
		{errors.New("boom"), http.StatusInternalServerError, `{"error":"internal_error"}`},
	} {
		recorder := httptest.NewRecorder()
		(&Server{}).writeMirrorError(recorder, testCase.err)
		if recorder.Code != testCase.status || recorder.Body.String() != testCase.body {
			t.Errorf("%v: status %d body %s", testCase.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestResolveServiceSourceUnavailable(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), func(options *MirrorOptions) {
		options.MaxLogCommits = 1
	})
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	for index := 0; index < 2; index++ {
		service.publish(t, signer, upsertAuthz(testServiceID1+string(rune('A'+index)), "policy", "grant read"))
		if err := mirror.ApplyChanges(pageFor(t, service, service.Revision-1, 100)); err != nil {
			t.Fatalf("apply changes: %v", err)
		}
	}
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	for _, action := range []string{"state", "snapshot", "changes?since=1"} {
		response, body := server.get(adsync.APIPrefix+"/services/"+testServiceID1+"/"+action, token, nil)
		if response.StatusCode != http.StatusServiceUnavailable || string(body) != `{"error":"source_unavailable"}` {
			t.Errorf("%s: status %d body %s", action, response.StatusCode, body)
		}
	}
}

func TestNormalizeThresholdsErrors(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	credentials, err := NewCredentialSet([]Credential{{DownstreamID: "edge", Token: "token", Services: []string{"*"}}})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	for _, testCase := range []struct {
		name      string
		heartbeat time.Duration
		stale     time.Duration
		stop      time.Duration
	}{
		{name: "heartbeat below minimum", heartbeat: time.Second},
		{name: "stale not above heartbeat", heartbeat: 30 * time.Second, stale: 30 * time.Second},
		{name: "stop below minimum", stop: time.Second},
		{name: "stop not above stale", stale: 60 * time.Second, stop: 60 * time.Second},
	} {
		_, err := NewServer(Config{
			InstanceID:        testProxyID,
			Mirror:            mirror,
			Credentials:       credentials,
			HeartbeatInterval: testCase.heartbeat,
			StaleAfter:        testCase.stale,
			StopAfter:         testCase.stop,
		})
		if !errors.Is(err, ErrConfig) {
			t.Errorf("%s: %v, want ErrConfig", testCase.name, err)
		}
	}
}

func TestNewServerCredentialFileError(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	_, err := NewServer(Config{
		InstanceID:     testProxyID,
		Mirror:         mirror,
		CredentialFile: filepath.Join(t.TempDir(), "missing"),
	})
	if !errors.Is(err, ErrCredentialFile) {
		t.Errorf("missing credential file: %v, want ErrCredentialFile", err)
	}
}

func TestFormatChannelDuration(t *testing.T) {
	if got := formatChannelDuration(30 * time.Second); got != "30s" {
		t.Errorf("30s = %q", got)
	}
	if got := formatChannelDuration(1500 * time.Millisecond); got != "1500ms" {
		t.Errorf("1500ms = %q", got)
	}
}

func TestCredentialAccessors(t *testing.T) {
	var absent *Credential
	if absent.Allows(testServiceID1) {
		t.Error("nil credential allows a service")
	}
	if absent.ServicesList() != nil {
		t.Error("nil credential has a service list")
	}
	all := &Credential{all: true}
	if !all.Allows(testServiceID1) {
		t.Error("wildcard credential does not allow a service")
	}
	if list := all.ServicesList(); len(list) != 1 || list[0] != "*" {
		t.Errorf("wildcard list = %v", list)
	}
	subset := &Credential{allowed: map[string]struct{}{testServiceID2: {}, testServiceID1: {}}}
	list := subset.ServicesList()
	if len(list) != 2 || list[0] != testServiceID1 || list[1] != testServiceID2 {
		t.Errorf("subset list = %v", list)
	}

	set, err := NewCredentialSet([]Credential{{DownstreamID: "edge", Token: "token", Services: []string{"*"}}})
	if err != nil {
		t.Fatalf("credential set: %v", err)
	}
	if set.Path() != "" {
		t.Errorf("in-memory path = %q", set.Path())
	}
	if set.Len() != 1 {
		t.Errorf("Len = %d", set.Len())
	}
	var nilSet *CredentialSet
	if nilSet.Path() != "" || nilSet.Len() != 0 {
		t.Error("nil set has a path or credentials")
	}
	if nilSet.Current(nil) || set.Current(nil) {
		t.Error("nil credential is current")
	}
	if _, ok := nilSet.Authenticate("token"); ok {
		t.Error("nil set authenticated a token")
	}
	if _, ok := set.Authenticate(""); ok {
		t.Error("empty token authenticated")
	}
	if _, ok := set.Authenticate(strings.Repeat("x", 513)); ok {
		t.Error("oversized token authenticated")
	}
}

func TestCredentialFileEdgeCases(t *testing.T) {
	if _, err := LoadCredentialFile(""); !errors.Is(err, ErrCredentialFile) {
		t.Errorf("empty path: %v", err)
	}
	if _, err := LoadCredentialFile(t.TempDir()); !errors.Is(err, ErrCredentialFile) {
		t.Errorf("directory: %v", err)
	}

	permissive := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(permissive, []byte("edge:"+hashToken("token")+":*\n"), 0o644); err != nil {
		t.Fatalf("write permissive file: %v", err)
	}
	if err := os.Chmod(permissive, 0o644); err != nil {
		t.Fatalf("chmod permissive file: %v", err)
	}
	if _, err := LoadCredentialFile(permissive); !errors.Is(err, ErrCredentialFile) {
		t.Errorf("permissive mode: %v", err)
	}

	large := filepath.Join(t.TempDir(), "large")
	if err := os.WriteFile(large, bytes.Repeat([]byte("#"), maxCredentialFileBytes+1), 0o600); err != nil {
		t.Fatalf("write large file: %v", err)
	}
	if _, err := LoadCredentialFile(large); !errors.Is(err, ErrCredentialFile) {
		t.Errorf("large file: %v", err)
	}

	var many strings.Builder
	for index := 0; index <= maxCredentialLines; index++ {
		fmt.Fprintf(&many, "edge%d:%s:*\n", index, hashToken(fmt.Sprintf("token-%d", index)))
	}
	manyPath := filepath.Join(t.TempDir(), "many")
	if err := os.WriteFile(manyPath, []byte(many.String()), 0o600); err != nil {
		t.Fatalf("write many lines: %v", err)
	}
	if _, err := LoadCredentialFile(manyPath); !errors.Is(err, ErrCredentialFile) {
		t.Errorf("too many lines: %v", err)
	}

	invalidHash := filepath.Join(t.TempDir(), "bad-hash")
	if err := os.WriteFile(invalidHash, []byte("edge:"+strings.Repeat("z", 64)+":*\n"), 0o600); err != nil {
		t.Fatalf("write bad hash: %v", err)
	}
	if _, err := LoadCredentialFile(invalidHash); !errors.Is(err, ErrCredentialFile) {
		t.Errorf("non-hexadecimal hash: %v", err)
	}
}

func TestNewCredentialSetValidation(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		credentials []Credential
	}{
		{name: "no downstream id", credentials: []Credential{{Token: "token"}}},
		{name: "invalid downstream id", credentials: []Credential{{DownstreamID: "bad id", Token: "token"}}},
		{name: "no token", credentials: []Credential{{DownstreamID: "edge"}}},
		{name: "whitespace token", credentials: []Credential{{DownstreamID: "edge", Token: "two words"}}},
		{name: "invalid service", credentials: []Credential{{DownstreamID: "edge", Token: "token", Services: []string{"bad"}}}},
		{name: "duplicate service", credentials: []Credential{{DownstreamID: "edge", Token: "token", Services: []string{testServiceID1, testServiceID1}}}},
		{name: "duplicate id", credentials: []Credential{
			{DownstreamID: "edge", Token: "token-one"},
			{DownstreamID: "edge", Token: "token-two"},
		}},
	} {
		if _, err := NewCredentialSet(testCase.credentials); !errors.Is(err, ErrCredentialFile) {
			t.Errorf("%s: %v, want ErrCredentialFile", testCase.name, err)
		}
	}
}

func TestWatchReloadEdgeCases(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	// A nil signal channel returns immediately.
	server.server.WatchReload(context.Background(), nil)
	// A closed channel ends the loop.
	closed := make(chan os.Signal)
	close(closed)
	server.server.WatchReload(context.Background(), closed)

	// A failed reload is logged and the loop keeps serving the previous set.
	signals := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.server.WatchReload(ctx, signals)
	}()
	server.writeCredentialFile("broken")
	signals <- os.Interrupt
	time.Sleep(50 * time.Millisecond)
	response, _ := server.get(adsync.APIPrefix+"/services", token, nil)
	if response.StatusCode != http.StatusOK {
		t.Errorf("previous credential after failed reload: status %d", response.StatusCode)
	}
	cancel()
	<-done
}

func TestHubEdgeCases(t *testing.T) {
	if _, err := NewHub(1, -time.Second); !errors.Is(err, ErrConfig) {
		t.Errorf("negative window: %v, want ErrConfig", err)
	}
	if _, err := NewHub(-1, 0); !errors.Is(err, ErrConfig) {
		t.Errorf("negative downstreams: %v, want ErrConfig", err)
	}
	hub, err := NewHub(0, time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if err := hub.Allow(""); !errors.Is(err, ErrConfig) {
		t.Errorf("empty downstream: %v, want ErrConfig", err)
	}
	subscriber, err := hub.Subscribe("edge-1", nil)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if got := subscriber.Downstream(); got != "edge-1" {
		t.Errorf("Downstream = %q", got)
	}
	if hub.Subscribers() != 1 {
		t.Errorf("Subscribers = %d", hub.Subscribers())
	}
	var absent *Subscriber
	if absent.C() != nil || absent.Downstream() != "" {
		t.Error("nil subscriber returns a channel or id")
	}
	hub.Unsubscribe(nil)
	hub.Publish("", 1)
	hub.Publish("edge-1", 0)
	hub.Close()
	if err := hub.Allow("edge-1"); !errors.Is(err, ErrConfig) {
		t.Errorf("closed hub: %v, want ErrConfig", err)
	}
	hub.Publish("edge-1", 2)
}

func TestChannelWebSocketFrameErrors(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	validHello := func() helloMessage {
		return helloMessage{
			Type:         "hello",
			InstanceID:   testDownstreamID,
			Services:     []helloService{},
			Chain:        []string{testProxyID, testDownstreamID},
			AgentVersion: "test",
		}
	}
	for _, testCase := range []struct {
		name   string
		open   func(t *testing.T, conn *websocket.Conn)
		code   int
		reason string
	}{
		{
			name: "binary first frame",
			open: func(t *testing.T, conn *websocket.Conn) {
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
					t.Errorf("write binary frame: %v", err)
				}
			},
			code:   websocket.CloseProtocolError,
			reason: "expected hello",
		},
		{
			name: "invalid hello json",
			open: func(t *testing.T, conn *websocket.Conn) {
				if err := conn.WriteMessage(websocket.TextMessage, []byte("{")); err != nil {
					t.Errorf("write invalid json: %v", err)
				}
			},
			code:   websocket.CloseProtocolError,
			reason: "invalid hello",
		},
		{
			name: "wrong hello type",
			open: func(t *testing.T, conn *websocket.Conn) {
				hello := validHello()
				hello.Type = "heartbeat"
				writeJSONFrame(t, conn, hello)
			},
			code:   websocket.CloseProtocolError,
			reason: "protocol_error",
		},
		{
			name: "invalid instance id",
			open: func(t *testing.T, conn *websocket.Conn) {
				hello := validHello()
				hello.InstanceID = "bad"
				writeJSONFrame(t, conn, hello)
			},
			code:   websocket.CloseProtocolError,
			reason: "protocol_error",
		},
		{
			name: "own instance id",
			open: func(t *testing.T, conn *websocket.Conn) {
				hello := validHello()
				hello.InstanceID = testProxyID
				writeJSONFrame(t, conn, hello)
			},
			code:   websocket.ClosePolicyViolation,
			reason: "chain_loop",
		},
		{
			name: "invalid service id",
			open: func(t *testing.T, conn *websocket.Conn) {
				hello := validHello()
				hello.Services = []helloService{{ID: "bad"}}
				writeJSONFrame(t, conn, hello)
			},
			code:   websocket.CloseProtocolError,
			reason: "protocol_error",
		},
		{
			name: "duplicate service id",
			open: func(t *testing.T, conn *websocket.Conn) {
				hello := validHello()
				hello.Services = []helloService{{ID: testServiceID1}, {ID: testServiceID1}}
				writeJSONFrame(t, conn, hello)
			},
			code:   websocket.CloseProtocolError,
			reason: "protocol_error",
		},
		{
			name: "service outside subset",
			open: func(t *testing.T, conn *websocket.Conn) {
				hello := validHello()
				hello.Services = []helloService{{ID: testServiceID2}}
				writeJSONFrame(t, conn, hello)
			},
			code:   websocket.ClosePolicyViolation,
			reason: "forbidden",
		},
		{
			name: "binary after hello",
			open: func(t *testing.T, conn *websocket.Conn) {
				writeJSONFrame(t, conn, validHello())
				if kind, _ := readChannelFrame(t, conn); kind != "ack" {
					t.Fatalf("frame = %q", kind)
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
					t.Errorf("write binary frame: %v", err)
				}
			},
			code:   websocket.CloseProtocolError,
			reason: "expected a text frame",
		},
		{
			name: "invalid frame json",
			open: func(t *testing.T, conn *websocket.Conn) {
				writeJSONFrame(t, conn, validHello())
				if kind, _ := readChannelFrame(t, conn); kind != "ack" {
					t.Fatalf("frame = %q", kind)
				}
				if err := conn.WriteMessage(websocket.TextMessage, []byte("{")); err != nil {
					t.Errorf("write invalid json: %v", err)
				}
			},
			code:   websocket.CloseProtocolError,
			reason: "invalid frame",
		},
		{
			name: "invalid heartbeat",
			open: func(t *testing.T, conn *websocket.Conn) {
				writeJSONFrame(t, conn, validHello())
				if kind, _ := readChannelFrame(t, conn); kind != "ack" {
					t.Fatalf("frame = %q", kind)
				}
				writeJSONFrame(t, conn, map[string]any{"type": "heartbeat", "unknown": true})
			},
			code:   websocket.CloseProtocolError,
			reason: "invalid heartbeat",
		},
		{
			name: "heartbeat chain loop",
			open: func(t *testing.T, conn *websocket.Conn) {
				writeJSONFrame(t, conn, validHello())
				if kind, _ := readChannelFrame(t, conn); kind != "ack" {
					t.Fatalf("frame = %q", kind)
				}
				writeJSONFrame(t, conn, heartbeatMessage{
					Type:     "heartbeat",
					Services: []heartbeatService{},
					Liveness: "live",
					Chain:    []string{testProxyID, testProxyID},
					Fallback: fallbackMessage{Services: []string{}},
				})
			},
			code:   websocket.ClosePolicyViolation,
			reason: "chain_loop",
		},
		{
			name: "chain does not end at sender",
			open: func(t *testing.T, conn *websocket.Conn) {
				hello := validHello()
				hello.Chain = []string{testProxyID, testDownstreamB}
				writeJSONFrame(t, conn, hello)
			},
			code:   websocket.ClosePolicyViolation,
			reason: "chain_loop",
		},
		{
			name: "proxy is not the immediate upstream",
			open: func(t *testing.T, conn *websocket.Conn) {
				hello := validHello()
				hello.Chain = []string{testProxyID, testDownstreamB, testDownstreamID}
				writeJSONFrame(t, conn, hello)
			},
			code:   websocket.ClosePolicyViolation,
			reason: "chain_loop",
		},
		{
			name: "duplicate hello",
			open: func(t *testing.T, conn *websocket.Conn) {
				writeJSONFrame(t, conn, validHello())
				if kind, _ := readChannelFrame(t, conn); kind != "ack" {
					t.Fatalf("frame = %q", kind)
				}
				writeJSONFrame(t, conn, validHello())
			},
			code:   websocket.CloseProtocolError,
			reason: "duplicate hello",
		},
		{
			name: "unknown frame",
			open: func(t *testing.T, conn *websocket.Conn) {
				writeJSONFrame(t, conn, validHello())
				if kind, _ := readChannelFrame(t, conn); kind != "ack" {
					t.Fatalf("frame = %q", kind)
				}
				writeJSONFrame(t, conn, map[string]any{"type": "unknown"})
			},
			code:   websocket.CloseProtocolError,
			reason: "unknown frame",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			credential := testCredential{downstreamID: testDownstreamID, token: token}
			if testCase.name == "service outside subset" {
				credential.services = []string{testServiceID1}
			}
			server := newTestServer(t, mirror, signer.publicKeys(t), nil, credential)
			conn, _, err := server.dial(token)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close()
			testCase.open(t, conn)
			code, reason := readClose(t, conn)
			if code != testCase.code || reason != testCase.reason {
				t.Fatalf("close = %d %q, want %d %q", code, reason, testCase.code, testCase.reason)
			}
		})
	}
}

func TestPollBodyTooLarge(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	body := bytes.Repeat([]byte("x"), maxChannelFrameBytes+1)
	response, payload := server.post(adsync.APIPrefix+"/channel/poll?wait=0", token, body)
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status %d body %s", response.StatusCode, payload)
	}
}

func TestPollRevokedWhileHeld(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	type result struct {
		status int
		body   []byte
	}
	done := make(chan result, 1)
	go func() {
		response, body := server.post(adsync.APIPrefix+"/channel/poll?wait=1", token,
			heartbeatFrame([]string{testProxyID, testDownstreamID}))
		done <- result{status: response.StatusCode, body: body}
	}()
	time.Sleep(50 * time.Millisecond)
	// The credential is gone after the reload. A poll that already
	// authenticated fails the revalidation; one that starts afterwards fails
	// authentication, so either ordering answers 401.
	server.writeCredentialFile(testDownstreamB + ":" + hashToken(testToken(testDownstreamB, 2)) + ":*")
	if err := server.server.ReloadCredentials(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := <-done
	if got.status != http.StatusUnauthorized {
		t.Errorf("status %d body %s", got.status, got.body)
	}
}

func TestMirrorOptionValidation(t *testing.T) {
	signer := newTestSigner(t)
	for name, mutate := range map[string]func(*MirrorOptions){
		"commits": func(options *MirrorOptions) { options.MaxLogCommits = -1 },
		"bytes":   func(options *MirrorOptions) { options.MaxLogBytes = -1 },
	} {
		options := MirrorOptions{PublicKeys: signer.publicKeys(t)}
		mutate(&options)
		if _, err := NewMirror(options); !errors.Is(err, ErrConfig) {
			t.Errorf("%s: %v, want ErrConfig", name, err)
		}
	}
}

func TestMirrorChangesEdgeCases(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "policy", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	low, err := mirror.Changes(testServiceID1, 1, 0)
	if err != nil || len(low.Commits) != 1 {
		t.Errorf("limit below one: %v %+v", err, low.Commits)
	}
	high, err := mirror.Changes(testServiceID1, 1, 1000)
	if err != nil || len(high.Commits) != 1 {
		t.Errorf("limit above the maximum: %v %+v", err, high.Commits)
	}
	if _, err := mirror.Changes(testServiceID1, 1, 100); err != nil {
		t.Fatalf("changes: %v", err)
	}

	overflowed := newTestMirror(t, signer.publicKeys(t), func(options *MirrorOptions) {
		options.MaxLogCommits = 1
	})
	other := newFixtureService(testServiceID2, "orders")
	if err := overflowed.ApplySnapshot(snapshotAt(t, other, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	for index := 0; index < 2; index++ {
		other.publish(t, signer, upsertAuthz(testServiceID2+string(rune('A'+index)), "policy", "grant read"))
		if err := overflowed.ApplyChanges(pageFor(t, other, other.Revision-1, 100)); err != nil {
			t.Fatalf("apply changes: %v", err)
		}
	}
	if _, err := overflowed.Changes(testServiceID2, 1, 100); !errors.Is(err, ErrSnapshotRequired) {
		t.Errorf("overflowed changes: %v, want ErrSnapshotRequired", err)
	}
}

func TestMirrorNilInputs(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	if err := mirror.ApplySnapshot(nil); !errors.Is(err, ErrConfig) {
		t.Errorf("nil snapshot: %v, want ErrConfig", err)
	}
	if err := mirror.ApplyChanges(nil); !errors.Is(err, ErrConfig) {
		t.Errorf("nil page: %v, want ErrConfig", err)
	}
	if err := mirror.ApplyChanges(&adsync.ChangesResponse{ServiceID: testUnknownID}); !errors.Is(err, ErrUnknownService) {
		t.Errorf("unknown service: %v, want ErrUnknownService", err)
	}
}

func TestMirrorSnapshotValidation(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	for _, testCase := range []struct {
		name   string
		mutate func(*adsync.SnapshotResponse)
	}{
		{name: "invalid service id", mutate: func(response *adsync.SnapshotResponse) { response.ServiceID = "bad" }},
		{name: "invalid source id", mutate: func(response *adsync.SnapshotResponse) { response.SourceID = "bad" }},
		{name: "invalid epoch", mutate: func(response *adsync.SnapshotResponse) { response.Epoch = "bad" }},
		{name: "invalid revision", mutate: func(response *adsync.SnapshotResponse) { response.Revision = 1<<53 + 1 }},
		{name: "invalid name", mutate: func(response *adsync.SnapshotResponse) { response.Snapshot.Name = "-bad" }},
		{name: "missing hashes", mutate: func(response *adsync.SnapshotResponse) { response.StateHash = adsync.Hash{} }},
	} {
		response := service.snapshotResponse(t, signer)
		testCase.mutate(response)
		if err := mirror.ApplySnapshot(response); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("%s: %v, want ErrProtocol", testCase.name, err)
		}
	}
}

func TestMirrorChangePageValidation(t *testing.T) {
	signer := newTestSigner(t)
	newMirrorWithCommit := func(t *testing.T) (*Mirror, *fixtureService) {
		t.Helper()
		mirror := newTestMirror(t, signer.publicKeys(t), nil)
		service := newFixtureService(testServiceID1, "books")
		if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
			t.Fatalf("apply snapshot: %v", err)
		}
		service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "policy", "grant read"))
		return mirror, service
	}
	valid := func(t *testing.T, service *fixtureService) *adsync.ChangesResponse {
		t.Helper()
		return pageFor(t, service, 1, 100)
	}

	t.Run("already applied", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		if err := mirror.ApplyChanges(page); err != nil {
			t.Fatalf("first delivery: %v", err)
		}
		if err := mirror.ApplyChanges(page); err != nil {
			t.Errorf("duplicate delivery: %v", err)
		}
	})

	t.Run("identity changed", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.Epoch = testServiceID2
		if err := mirror.ApplyChanges(page); !errors.Is(err, ErrIdentityChanged) {
			t.Errorf("epoch change: %v, want ErrIdentityChanged", err)
		}
	})

	t.Run("revision out of range", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.From = 1<<53 + 1
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("from out of range: %v, want ErrProtocol", err)
		}
	})

	t.Run("ends after head", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.To = page.Head + 1
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("to after head: %v, want ErrProtocol", err)
		}
	})

	t.Run("no commits", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.Commits = nil
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("empty page: %v, want ErrProtocol", err)
		}
	})

	t.Run("not contiguous", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		service.publish(t, signer, upsertAuthz(testServiceID1+"A2", "policy", "grant read"))
		page := pageFor(t, service, 1, 100)
		if len(page.Commits) != 2 {
			t.Fatalf("fixture commits = %d", len(page.Commits))
		}
		page.Commits[1].Revision = page.Commits[0].Revision
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("non-contiguous page: %v, want ErrProtocol", err)
		}
	})

	t.Run("broken chain", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.Commits[0].PrevHash = adsync.Hash{}
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrIntegrity) {
			t.Errorf("broken chain: %v, want ErrIntegrity", err)
		}
	})

	t.Run("hash mismatch", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		name := "tampered"
		page.Commits[0].Changes = []adsync.Change{upsertAuthz(testServiceID1+"A9", name, "grant read")}
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrIntegrity) {
			t.Errorf("hash mismatch: %v, want ErrIntegrity", err)
		}
	})

	t.Run("bad signature", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.Commits[0].Signature.Value = signer.sign([]byte("other")).Value
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrIntegrity) {
			t.Errorf("bad signature: %v, want ErrIntegrity", err)
		}
	})

	t.Run("service change targets another service", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		service.publish(t, signer, renameService(testServiceID2, "other"))
		page := pageFor(t, service, 1, 100)
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("foreign service change: %v, want ErrProtocol", err)
		}
	})

	t.Run("last commit before page end", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		service.publish(t, signer, upsertAuthz(testServiceID1+"A2", "policy", "grant read"))
		page := pageFor(t, service, 1, 1)
		page.To = page.Head
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
			t.Errorf("short page: %v, want ErrProtocol", err)
		}
	})

	t.Run("head state hash mismatch", func(t *testing.T) {
		mirror, service := newMirrorWithCommit(t)
		page := valid(t, service)
		page.HeadStateHash = adsync.StateHash(adsync.Snapshot{Name: "other"})
		if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrIntegrity) {
			t.Errorf("head hash: %v, want ErrIntegrity", err)
		}
	})
}

func TestMirrorRemovedAndOverflowedPages(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "policy", "grant read"))
	page := pageFor(t, service, 1, 100)
	mirror.RemoveService(testServiceID1)
	if err := mirror.ApplyChanges(page); !errors.Is(err, ErrServiceRemoved) {
		t.Errorf("removed service: %v, want ErrServiceRemoved", err)
	}

	overflowed := newTestMirror(t, signer.publicKeys(t), func(options *MirrorOptions) {
		options.MaxLogCommits = 1
	})
	other := newFixtureService(testServiceID2, "orders")
	if err := overflowed.ApplySnapshot(snapshotAt(t, other, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	for index := 0; index < 2; index++ {
		other.publish(t, signer, upsertAuthz(testServiceID2+string(rune('A'+index)), "policy", "grant read"))
		if err := overflowed.ApplyChanges(pageFor(t, other, other.Revision-1, 100)); err != nil {
			t.Fatalf("apply changes: %v", err)
		}
	}
	next := other.changesPage(other.Revision, 100)
	if err := overflowed.ApplyChanges(&next); !errors.Is(err, ErrSnapshotRequired) {
		t.Errorf("overflowed service: %v, want ErrSnapshotRequired", err)
	}
}

func TestCredentialSetReloadWithoutFile(t *testing.T) {
	if err := (&CredentialSet{}).Reload(); !errors.Is(err, ErrCredentialFile) {
		t.Errorf("reload without a file: %v, want ErrCredentialFile", err)
	}
}

func TestRemoveServiceIgnoresUnknownAndRemoved(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	version := mirror.Version()
	mirror.RemoveService(testUnknownID)
	if mirror.Version() != version {
		t.Error("removing an unknown service advanced the version")
	}
	mirror.RemoveService(testServiceID1)
	removedVersion := mirror.Version()
	mirror.RemoveService(testServiceID1)
	if mirror.Version() != removedVersion {
		t.Error("removing a removed service advanced the version")
	}
}

func TestMirrorSnapshotConflictAndRename(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	first := newFixtureService(testServiceID1, "books")
	first.publish(t, signer, upsertAuthz(testServiceID1+"A1", "one", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, first, signer)); err != nil {
		t.Fatalf("apply first snapshot: %v", err)
	}
	second := newFixtureService(testServiceID1, "books")
	second.publish(t, signer, upsertAuthz(testServiceID1+"B1", "two", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, second, signer)); !errors.Is(err, adsync.ErrIntegrity) {
		t.Errorf("same revision, different state hash: %v, want ErrIntegrity", err)
	}

	renamed := newTestMirror(t, signer.publicKeys(t), nil)
	base := newFixtureService(testServiceID1, "books")
	if err := renamed.ApplySnapshot(snapshotAt(t, base, signer)); err != nil {
		t.Fatalf("apply base snapshot: %v", err)
	}
	version := renamed.Version()
	updated := newFixtureService(testServiceID1, "books")
	updated.publish(t, signer, renameService(testServiceID1, "books-v2"))
	if err := renamed.ApplySnapshot(snapshotAt(t, updated, signer)); err != nil {
		t.Fatalf("apply renamed snapshot: %v", err)
	}
	services := renamed.Services()
	if len(services) != 1 || services[0].Name != "books-v2" {
		t.Fatalf("services after rename = %+v", services)
	}
	if renamed.Version() <= version {
		t.Error("a rename through a snapshot did not advance the version")
	}
}

func TestServerInvalidChainEntry(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	credentials, err := NewCredentialSet([]Credential{{DownstreamID: "edge", Token: "token", Services: []string{"*"}}})
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	_, err = NewServer(Config{
		InstanceID:  testProxyID,
		Mirror:      mirror,
		Credentials: credentials,
		Chain:       []string{"bad", testProxyID},
	})
	if !errors.Is(err, ErrConfig) {
		t.Errorf("invalid chain entry: %v, want ErrConfig", err)
	}
}

func TestWriteChannelJSONBounds(t *testing.T) {
	if err := writeChannelJSON(nil, make(chan int)); err == nil {
		t.Error("a non-JSON value was accepted")
	}
	if err := writeChannelJSON(nil, strings.Repeat("x", maxChannelFrameBytes+1)); !errors.Is(err, ErrConfig) {
		t.Errorf("oversized frame: %v, want ErrConfig", err)
	}
}

func TestChainValidationAcceptsUnrelatedChains(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	for _, chain := range [][]string{
		{testDownstreamID},
		{testDownstreamID, testProxyID},
	} {
		response, body := server.post(adsync.APIPrefix+"/channel/poll?wait=0", token, heartbeatFrame(chain))
		if response.StatusCode != http.StatusOK {
			t.Errorf("chain %v: status %d body %s", chain, response.StatusCode, body)
		}
	}
}

func TestPollRejectsTrailingJSON(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	body := append(heartbeatFrame([]string{testProxyID, testDownstreamID}), []byte("{}")...)
	response, payload := server.post(adsync.APIPrefix+"/channel/poll?wait=0", token, body)
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d body %s", response.StatusCode, payload)
	}
}

func TestPollOnClosedHub(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	hub, err := NewHub(0, 0)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{hub: hub}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	hub.Close()
	response, payload := server.post(adsync.APIPrefix+"/channel/poll?wait=0", token,
		heartbeatFrame([]string{testProxyID, testDownstreamID}))
	if response.StatusCode != http.StatusServiceUnavailable || string(payload) != `{"error":"source_unavailable"}` {
		t.Errorf("status %d body %s", response.StatusCode, payload)
	}
}

func TestChannelRequiresUpgrade(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), nil, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	response, body := server.get(adsync.APIPrefix+"/channel", token, nil)
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("plain GET: status %d body %s", response.StatusCode, body)
	}
	conn, _, err := server.dial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
}

func TestMirrorOversizedCommitOverflows(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "policy", strings.Repeat("x", 1200<<10)))
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); err != nil {
		t.Fatalf("apply oversized commit: %v", err)
	}
	if services := mirror.Services(); len(services) != 0 {
		t.Errorf("service with an unsendable commit is advertised: %+v", services)
	}
	if needs := mirror.NeedsSnapshot(); len(needs) != 1 || needs[0] != testServiceID1 {
		t.Errorf("NeedsSnapshot = %v", needs)
	}
}

func TestMirrorRejectsCommitThatCannotApply(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	changes := []adsync.Change{{Op: "upsert", Kind: "service", ID: testServiceID1}}
	hash, err := adsync.CommitHash(service.CommitHash, changes)
	if err != nil {
		t.Fatalf("commit hash: %v", err)
	}
	commit := adsync.Commit{
		Revision: service.Revision + 1,
		PrevHash: service.CommitHash,
		Hash:     hash,
		At:       1762500000000,
		Changes:  changes,
	}
	commit.Signature = signer.sign(adsync.CommitSignedBytes(service.ID, service.Epoch, commit.Revision, hash))
	page := &adsync.ChangesResponse{
		ServiceID: service.ID,
		SourceID:  service.ID,
		Epoch:     service.Epoch,
		From:      service.Revision + 1,
		To:        service.Revision + 1,
		Head:      service.Revision + 1,
		Commits:   []adsync.Commit{commit},
	}
	if err := mirror.ApplyChanges(page); !errors.Is(err, adsync.ErrProtocol) {
		t.Errorf("commit without a service name: %v, want ErrProtocol", err)
	}
}

func TestMirrorDelayedSnapshotCannotReviveASignedDelete(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	before := snapshotAt(t, service, signer)
	service.publish(t, signer, deleteChange("service", testServiceID1))
	if err := mirror.ApplyChanges(pageFor(t, service, 1, 100)); !errors.Is(err, adsync.ErrSourceGone) {
		t.Fatalf("signed delete: %v, want ErrSourceGone", err)
	}

	// The pre-deletion snapshot was fetched before the delete reached the
	// source. It must not bring the deleted policies back.
	if err := mirror.ApplySnapshot(before); err != nil {
		t.Fatalf("delayed snapshot: %v", err)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceRemoved {
		t.Errorf("availability after delayed snapshot = %v, want ServiceRemoved", availability)
	}
	if services := mirror.Services(); len(services) != 0 {
		t.Errorf("delayed snapshot revived the service: %+v", services)
	}

	// Newer source state does revive it: a later revision is past the delete.
	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("newer snapshot: %v", err)
	}
	if services := mirror.Services(); len(services) != 1 || services[0].Revision != service.Revision {
		t.Errorf("newer snapshot did not revive the service: %+v", services)
	}
}

func TestMirrorDelayedSnapshotCannotReviveAnAdministrativeRemoval(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	service := newFixtureService(testServiceID1, "books")
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	before := snapshotAt(t, service, signer)
	mirror.RemoveService(testServiceID1)
	if err := mirror.ApplySnapshot(before); err != nil {
		t.Fatalf("delayed snapshot: %v", err)
	}
	if _, availability := mirror.Lookup(testServiceID1); availability != ServiceRemoved {
		t.Errorf("availability after delayed snapshot = %v, want ServiceRemoved", availability)
	}

	service.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
		t.Fatalf("newer snapshot: %v", err)
	}
	if services := mirror.Services(); len(services) != 1 {
		t.Errorf("newer snapshot did not revive the service: %+v", services)
	}
}

func TestMirrorServiceChurnDoesNotExhaustCapacity(t *testing.T) {
	signer := newTestSigner(t)
	mirror := newTestMirror(t, signer.publicKeys(t), nil)
	ids := make([]string, 0, MaxServices)
	for index := 0; index < MaxServices; index++ {
		id := fmt.Sprintf("01HF7YAT%018d", 100+index)
		ids = append(ids, id)
		service := newFixtureService(id, fmt.Sprintf("service-%d", index))
		if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
			t.Fatalf("service %d: %v", index, err)
		}
	}
	for _, id := range ids {
		mirror.RemoveService(id)
	}
	if services := mirror.Services(); len(services) != 0 {
		t.Fatalf("removed services are advertised: %+v", services)
	}

	// Tombstones must not hold the service slots: a fresh set of ids replaces
	// the removed ones.
	for index := 0; index < MaxServices; index++ {
		id := fmt.Sprintf("01HF7YAT%018d", 200+index)
		service := newFixtureService(id, fmt.Sprintf("fresh-%d", index))
		if err := mirror.ApplySnapshot(snapshotAt(t, service, signer)); err != nil {
			t.Fatalf("fresh service %d: %v", index, err)
		}
	}
	if services := mirror.Services(); len(services) != MaxServices {
		t.Errorf("advertised services = %d, want %d", len(services), MaxServices)
	}

	// A removed service no longer counts, but a full active set still bounds
	// the mirror.
	extra := newFixtureService("01HF7YAT000000000000000999", "extra")
	if err := mirror.ApplySnapshot(snapshotAt(t, extra, signer)); !errors.Is(err, ErrTooManyServices) {
		t.Errorf("eleventh active service: %v, want ErrTooManyServices", err)
	}
}

func TestChannelFirstFrameIsAckEvenWithPendingNotification(t *testing.T) {
	signer := newTestSigner(t)
	mirror, books, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	server := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{
		coalesceWindow: time.Millisecond,
	}, testCredential{
		downstreamID: testDownstreamID,
		token:        token,
	})
	conn, _, err := server.dial(token)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	writeJSONFrame(t, conn, helloMessage{
		Type:         "hello",
		InstanceID:   testDownstreamID,
		Services:     []helloService{},
		Chain:        []string{testProxyID, testDownstreamID},
		AgentVersion: "test",
	})
	// A head advance lands while the channel starts. Its coalesced
	// notification must not overtake the initial ack.
	books.publish(t, signer, upsertAuthz(testServiceID1+"A1", "read", "grant read"))
	if err := mirror.ApplyChanges(pageFor(t, books, 1, 100)); err != nil {
		t.Fatalf("apply changes: %v", err)
	}
	kind, payload := readChannelFrame(t, conn)
	if kind != "ack" {
		t.Fatalf("first frame = %q, payload %s", kind, payload)
	}
	ack := decodeBody[ackMessage](t, payload)
	if len(ack.Services) != 2 {
		t.Errorf("ack services = %+v", ack.Services)
	}
}

func TestServersShareAHubWithoutStealingItsLifetime(t *testing.T) {
	signer := newTestSigner(t)
	mirror, _, _ := newPopulatedMirror(t, signer)
	token := testToken(testDownstreamID, 1)
	hub, err := NewHub(0, time.Millisecond)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	credential := testCredential{downstreamID: testDownstreamID, token: token}
	first := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{hub: hub}, credential)
	second := newTestServer(t, mirror, signer.publicKeys(t), &testServerOptions{hub: hub}, credential)

	first.server.Close()
	response, body := second.post(adsync.APIPrefix+"/channel/poll?wait=0", token,
		heartbeatFrame([]string{testProxyID, testDownstreamID}))
	if response.StatusCode != http.StatusOK {
		t.Errorf("second server after the first closed: status %d body %s", response.StatusCode, body)
	}
	subscriber, err := hub.Subscribe("edge-other", nil)
	if err != nil {
		t.Fatalf("hub after the first server closed: %v", err)
	}
	hub.Unsubscribe(subscriber)
}
