// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/teramoby/speedle-plus/pkg/adsproxy"
	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	speedlestore "github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
)

// TestE2EProductionRuntimeSyncsAndServesDownstream uses the registered store
// builder, evaluator and REST router. No test-owned channel, reconciliation or
// mirror loop is involved: the production store runtime must do all three.
func TestE2EProductionRuntimeSyncsAndServesDownstream(t *testing.T) {
	cloud := newE2ECloud(t)
	_, downstreamToken := e2eDownstreamCredential("B", e2eInstanceB, 2)
	digest := sha256.Sum256([]byte(downstreamToken))
	downstreamFile := filepath.Join(t.TempDir(), "downstream-credentials")
	writeDownstreamCredentials(t, downstreamFile, "B:"+hex.EncodeToString(digest[:])+":*")

	ads, server := startProductionADS(t, cloud, map[string]interface{}{
		ServeDownstreamKey:       true,
		DownstreamCredentialsKey: downstreamFile,
		MaxDownstreamKey:         50,
		MaxConnectionsKey:        2,
	})
	client := server.Client()

	get := func(path, token string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 6<<20))
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, body
	}

	e2eWaitAllInSync(t, &e2eAgent{store: ads}, "books", "orders")
	e2eWaitFor(t, "the production downstream mirror to serve both services", func() bool {
		status, body := get(adsync.APIPrefix+"/services", downstreamToken)
		if status != http.StatusOK {
			return false
		}
		var services adsync.ServicesResponse
		return json.Unmarshal(body, &services) == nil && len(services.Services) == 2
	})

	// A Cloud ack advances the head and triggers the production reconciliation
	// loop. The local evaluator and downstream mirror must both reach revision 2.
	cloud.publish(e2eServiceBooks, e2eUpsertAuthz("p-books-dave-read", "grant user dave read /books/1"))
	cloud.pushAck()
	dave := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("dave")}
	e2eWaitFor(t, "the production evaluator and downstream mirror to install revision 2", func() bool {
		status, _ := ads.Status("books")
		if status.Service.State != adsync.SyncStateInSync || status.Service.Revision != 2 {
			return false
		}
		request, err := http.NewRequest(http.MethodPost, server.URL+svcs.PolicyAtzPath+"is-allowed", strings.NewReader(string(dave.body(t))))
		if err != nil {
			return false
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return false
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || !e2eDecodeResponse(t, body).Allowed {
			return false
		}
		statusCode, servicesBody := get(adsync.APIPrefix+"/services", downstreamToken)
		if statusCode != http.StatusOK {
			return false
		}
		var services adsync.ServicesResponse
		if json.Unmarshal(servicesBody, &services) != nil {
			return false
		}
		for _, service := range services.Services {
			if service.ID == e2eServiceBooks {
				return service.Revision == 2
			}
		}
		return false
	})

	// Open a real downstream channel through the mounted production route.
	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = client.Transport.(*http.Transport).TLSClientConfig
	channelURL := "wss" + strings.TrimPrefix(server.URL, "https") + adsync.APIPrefix + "/channel"
	conn, _, err := dialer.Dial(channelURL, http.Header{"Authorization": []string{"Bearer " + downstreamToken}})
	if err != nil {
		t.Fatalf("dial production downstream channel: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]interface{}{
		"type": "hello", "instanceId": e2eInstanceB, "services": []interface{}{},
		"chain": []string{e2eInstanceA, e2eInstanceB}, "agentVersion": "e2e-production",
	}); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var initialAck struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&initialAck); err != nil || initialAck.Type != "ack" {
		t.Fatalf("initial downstream ack = %+v, %v", initialAck, err)
	}

	// Removing the grant and reloading must close the already-established
	// channel as well as rejecting new HTTP requests.
	replacementToken := e2eCredential(e2eInstanceC, 3)
	replacementDigest := sha256.Sum256([]byte(replacementToken))
	writeDownstreamCredentials(t, downstreamFile, "C:"+hex.EncodeToString(replacementDigest[:])+":*")
	if err := ads.ReloadDownstreamCredentials(); err != nil {
		t.Fatalf("reload downstream credentials: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseTryAgainLater) {
		t.Fatalf("established channel survived credential removal: %v", err)
	}
	if status, _ := get(adsync.APIPrefix+"/services", downstreamToken); status != http.StatusUnauthorized {
		t.Fatalf("removed downstream credential HTTP status = %d", status)
	}
}

// TestE2EProductionRuntimeSyncFailureDoesNotServeStaleDecisions keeps one
// service from synchronizing after bootstrap. The production runtime must keep
// trying on its own, the request gate must stop answering that service from
// its outdated local copy, and every other service keeps evaluating locally.
func TestE2EProductionRuntimeSyncFailureDoesNotServeStaleDecisions(t *testing.T) {
	cloud := newE2ECloud(t)
	ads, server := startProductionADS(t, cloud, nil)
	client := server.Client()
	e2eWaitAllInSync(t, &e2eAgent{store: ads}, "books", "orders")

	isAllowed := func(check e2eCheck) (string, adsrest.IsAllowedResponse, bool) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+svcs.PolicyAtzPath+"is-allowed",
			strings.NewReader(string(check.body(t))))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("is-allowed: %v", err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			return response.Header.Get(svcs.DecisionSourceHeader), adsrest.IsAllowedResponse{}, false
		}
		return response.Header.Get(svcs.DecisionSourceHeader), e2eDecodeResponse(t, body), true
	}
	alice := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}
	carol := e2eCheck{Service: "orders", Resource: "/orders/1", Action: "approve", Principals: e2eUser("carol")}
	if source, decision, ok := isAllowed(alice); !ok || source != "" || !decision.Allowed {
		t.Fatalf("initial local books decision = %+v (source %q, ok %v)", decision, source, ok)
	}

	// Cloud revokes alice's grant, but books can no longer be synchronized.
	booksData := "/services/" + e2eServiceBooks + "/"
	attemptsBefore := cloud.countRequests(booksData+"snapshot") + cloud.countRequests(booksData+"changes")
	cloud.setDataUnavailable(e2eServiceBooks, true)
	cloud.publish(e2eServiceBooks, adsync.Change{Op: e2eChangeDelete, Kind: e2eKindAuthz, ID: "p-books-alice-read"})
	cloud.pushAck()

	e2eWaitFor(t, "the production runtime to attempt and fail the books synchronization", func() bool {
		attempts := cloud.countRequests(booksData+"snapshot") + cloud.countRequests(booksData+"changes")
		status, _ := ads.Status("books")
		return attempts > attemptsBefore && status.Service.State == adsync.SyncStateSyncing && status.Service.Revision == 1
	})
	// The outdated local copy still grants alice. The request must go to Cloud,
	// which already applies the revocation.
	source, decision, ok := isAllowed(alice)
	if !ok || source != svcs.DecisionSourceCloudFallback || decision.Allowed {
		t.Fatalf("books decision during the failure = %+v (source %q, ok %v), want a Cloud denial", decision, source, ok)
	}
	if source, decision, ok := isAllowed(carol); !ok || source != "" || !decision.Allowed {
		t.Fatalf("orders decision during the books failure = %+v (source %q, ok %v), want local", decision, source, ok)
	}

	// Once the source recovers, a notification is enough for the runtime to
	// finish the synchronization and evaluate books locally again.
	cloud.setDataUnavailable(e2eServiceBooks, false)
	cloud.notify(e2eServiceBooks, 2)
	e2eWaitFor(t, "books to return to local evaluation at revision 2", func() bool {
		status, _ := ads.Status("books")
		if status.Service.State != adsync.SyncStateInSync || status.Service.Revision != 2 {
			return false
		}
		source, decision, ok := isAllowed(alice)
		return ok && source == "" && !decision.Allowed
	})
}

// TestE2EProductionRuntimeWithholdsStaleDownstreamPolicies covers a source head
// that advances while the downstream mirror cannot fetch the new state. The
// proxy must withhold the service from its advertised set and from channel acks
// instead of letting downstream instances keep evaluating the older copy. After
// the source recovers the service is served again; a stopped instance stops
// answering downstream requests and ends established channels.
func TestE2EProductionRuntimeWithholdsStaleDownstreamPolicies(t *testing.T) {
	cloud := newE2ECloud(t)
	_, downstreamToken := e2eDownstreamCredential("B", e2eInstanceB, 2)
	digest := sha256.Sum256([]byte(downstreamToken))
	downstreamFile := filepath.Join(t.TempDir(), "downstream-credentials")
	writeDownstreamCredentials(t, downstreamFile, "B:"+hex.EncodeToString(digest[:])+":*")

	ads, server := startProductionADS(t, cloud, map[string]interface{}{
		ServeDownstreamKey:       true,
		DownstreamCredentialsKey: downstreamFile,
	})
	client := server.Client()
	get := func(path string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+downstreamToken)
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, body
	}
	downstreamServices := func() []adsync.Service {
		status, body := get(adsync.APIPrefix + "/services")
		if status != http.StatusOK {
			return nil
		}
		var response adsync.ServicesResponse
		if json.Unmarshal(body, &response) != nil {
			return nil
		}
		return response.Services
	}

	e2eWaitAllInSync(t, &e2eAgent{store: ads}, "books", "orders")
	e2eWaitFor(t, "the downstream service set to carry both services", func() bool {
		return len(downstreamServices()) == 2
	})

	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = client.Transport.(*http.Transport).TLSClientConfig
	channelURL := "wss" + strings.TrimPrefix(server.URL, "https") + adsync.APIPrefix + "/channel"
	conn, _, err := dialer.Dial(channelURL, http.Header{"Authorization": []string{"Bearer " + downstreamToken}})
	if err != nil {
		t.Fatalf("dial production downstream channel: %v", err)
	}
	defer conn.Close()
	writeHello := func() {
		t.Helper()
		if err := conn.WriteJSON(map[string]interface{}{
			"type": "hello", "instanceId": e2eInstanceB, "services": []interface{}{},
			"chain": []string{e2eInstanceA, e2eInstanceB}, "agentVersion": "e2e-production",
		}); err != nil {
			t.Fatal(err)
		}
	}
	readAck := func() map[string]uint64 {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			if err := conn.SetReadDeadline(deadline); err != nil {
				t.Fatal(err)
			}
			var frame struct {
				Type     string `json:"type"`
				Services []struct {
					ID   string `json:"id"`
					Head uint64 `json:"head"`
				} `json:"services"`
			}
			if err := conn.ReadJSON(&frame); err != nil {
				t.Fatalf("read channel frame: %v", err)
			}
			if frame.Type != "ack" {
				continue
			}
			heads := make(map[string]uint64, len(frame.Services))
			for _, service := range frame.Services {
				heads[service.ID] = service.Head
			}
			return heads
		}
	}
	heartbeat := func() map[string]uint64 {
		t.Helper()
		if err := conn.WriteJSON(map[string]interface{}{
			"type": "heartbeat", "chain": []string{e2eInstanceA, e2eInstanceB},
		}); err != nil {
			t.Fatal(err)
		}
		return readAck()
	}

	writeHello()
	if heads := readAck(); heads[e2eServiceBooks] != 1 || heads[e2eServiceOrders] != 1 {
		t.Fatalf("initial downstream heads = %v", heads)
	}

	// Cloud advances the books head to revoke a grant, but books data requests
	// fail from now on. The local store fails closed; the mirror must stop
	// advertising and acknowledging the older copy too.
	cloud.setDataUnavailable(e2eServiceBooks, true)
	cloud.publish(e2eServiceBooks, adsync.Change{Op: e2eChangeDelete, Kind: e2eKindAuthz, ID: "p-books-alice-read"})
	cloud.pushAck()
	e2eWaitFor(t, "the runtime to fail the books synchronization", func() bool {
		status, _ := ads.Status("books")
		return status.Service.State == adsync.SyncStateSyncing && status.Service.Revision == 1
	})
	e2eWaitForWithin(t, 20*time.Second, "the downstream service set to withhold books", func() bool {
		services := downstreamServices()
		return len(services) == 1 && services[0].ID == e2eServiceOrders
	})
	if status, body := get(adsync.APIPrefix + "/services/" + e2eServiceBooks + "/snapshot"); status != http.StatusServiceUnavailable || string(body) != `{"error":"source_unavailable"}` {
		t.Fatalf("withheld books snapshot: status %d body %s", status, body)
	}
	if heads := heartbeat(); heads[e2eServiceBooks] != 0 || heads[e2eServiceOrders] != 1 {
		t.Fatalf("downstream heads while books data is unavailable = %v", heads)
	}

	// A notification that the source recovered is enough to verify the new head
	// and serve the service downstream again.
	cloud.setDataUnavailable(e2eServiceBooks, false)
	cloud.notify(e2eServiceBooks, 2)
	e2eWaitForWithin(t, 20*time.Second, "the downstream service set to serve books at revision 2", func() bool {
		for _, service := range downstreamServices() {
			if service.ID == e2eServiceBooks {
				return service.Revision == 2
			}
		}
		return false
	})
	if heads := heartbeat(); heads[e2eServiceBooks] != 2 || heads[e2eServiceOrders] != 1 {
		t.Fatalf("downstream heads after recovery = %v", heads)
	}

	// A stopped instance must stop answering downstream data requests and end
	// established channels at their next heartbeat.
	stopID := cloud.sendCommand("stop", "", nil)
	e2eWaitFor(t, "the stop command to be acknowledged", func() bool {
		status, found := cloud.commandResult(stopID)
		return found && status == "acked"
	})
	if status, body := get(adsync.APIPrefix + "/services"); status != http.StatusServiceUnavailable || string(body) != `{"error":"distribution_paused"}` {
		t.Fatalf("stopped instance downstream services: status %d body %s", status, body)
	}
	if err := conn.WriteJSON(map[string]interface{}{
		"type": "heartbeat", "chain": []string{e2eInstanceA, e2eInstanceB},
	}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			closeErr, ok := err.(*websocket.CloseError)
			if !ok || closeErr.Code != websocket.ClosePolicyViolation || closeErr.Text != "distribution_paused" {
				t.Fatalf("stopped channel error = %v", err)
			}
			break
		}
	}
}

// TestE2EProductionRuntimeRemovalAppliesWhileAnotherServiceIsHeld covers a
// removal that arrives while the data request for another service is still in
// flight. The request gate must stop evaluating the removed service as soon as
// the removal is known, not after the held fetch finishes.
func TestE2EProductionRuntimeRemovalAppliesWhileAnotherServiceIsHeld(t *testing.T) {
	cloud := newE2ECloud(t)
	ads, server := startProductionADS(t, cloud, nil)
	client := server.Client()
	alice := e2eCheck{Service: "books", Resource: "/books/1", Action: "read", Principals: e2eUser("alice")}
	isAllowed := func() (int, adsrest.IsAllowedResponse) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.URL+svcs.PolicyAtzPath+"is-allowed",
			strings.NewReader(string(alice.body(t))))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("is-allowed: %v", err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			return response.StatusCode, adsrest.IsAllowedResponse{}
		}
		return response.StatusCode, e2eDecodeResponse(t, body)
	}

	e2eWaitAllInSync(t, &e2eAgent{store: ads}, "books", "orders")
	if status, decision := isAllowed(); status != http.StatusOK || !decision.Allowed {
		t.Fatalf("baseline books decision = %+v (status %d)", decision, status)
	}

	ordersData := "/services/" + e2eServiceOrders + "/"
	heldBefore := cloud.countRequests(ordersData+"snapshot") + cloud.countRequests(ordersData+"changes")
	// Advance the orders head and hold its data so the next pass must fetch it.
	cloud.publish(e2eServiceOrders, e2eUpsertAuthz("p-orders-dave-approve", "grant user dave approve /orders/1"))
	cloud.hold(e2eServiceOrders)
	defer cloud.release(e2eServiceOrders)
	cloud.removeService(e2eServiceBooks)
	cloud.pushAck()

	// Wait until the next pass has reached the held orders data request. The
	// books removal must already be visible to the gate at that point.
	e2eWaitFor(t, "the held orders data request to start", func() bool {
		return cloud.countRequests(ordersData+"snapshot")+cloud.countRequests(ordersData+"changes") > heldBefore
	})
	status, decision := isAllowed()
	if status == http.StatusOK && decision.Allowed {
		t.Fatalf("the removed service was authorized locally while another fetch was held: %+v", decision)
	}
	if status != http.StatusOK && status != http.StatusServiceUnavailable {
		t.Fatalf("removed service decision = status %d %+v", status, decision)
	}
}

// TestE2EProductionRuntimeRejectsAConflictingAdvertisedHash covers the
// equal-revision case: a mirror copy at the advertised revision with a
// different state hash must not be treated as current.
func TestE2EProductionRuntimeRejectsAConflictingAdvertisedHash(t *testing.T) {
	cloud := newE2ECloud(t)
	ads, _ := startProductionADS(t, cloud, nil)
	e2eWaitAllInSync(t, &e2eAgent{store: ads}, "books")

	fixture := cloud.service(e2eServiceBooks)
	mirror, err := adsproxy.NewMirror(adsproxy.MirrorOptions{PublicKeys: cloud.keys})
	if err != nil {
		t.Fatalf("new mirror: %v", err)
	}
	snapshot := adsync.SnapshotResponse{
		ServiceID:  fixture.ID,
		SourceID:   fixture.ID,
		Epoch:      fixture.Epoch,
		Revision:   fixture.Revision,
		StateHash:  fixture.StateHash,
		CommitHash: fixture.CommitHash,
		Snapshot:   fixture.State,
	}
	snapshot.Signature = cloud.signer.sign(adsync.SnapshotSignedBytes(
		snapshot.SourceID, snapshot.Epoch, snapshot.Revision, snapshot.StateHash, snapshot.CommitHash))
	if err := mirror.ApplySnapshot(&snapshot); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}

	service := adsync.Service{
		ID: fixture.ID, Name: fixture.Name, Revision: fixture.Revision,
		Epoch: fixture.Epoch, HeadStateHash: fixture.StateHash,
	}
	if err := ads.reachMirrorHead(context.Background(), mirror, service); err != nil {
		t.Fatalf("reachMirrorHead at the verified head: %v", err)
	}
	if !ads.mirrorReached(mirror, service) {
		t.Fatal("mirrorReached rejected the verified head")
	}

	conflict := service
	conflict.HeadStateHash = adsync.StateHash(adsync.Snapshot{Name: "conflicting-head"})
	if ads.mirrorReached(mirror, conflict) {
		t.Fatal("mirrorReached accepted a conflicting hash at the advertised revision")
	}
	if err := ads.reachMirrorHead(context.Background(), mirror, conflict); !errors.Is(err, adsync.ErrIntegrity) {
		t.Fatalf("reachMirrorHead conflicting hash = %v, want ErrIntegrity", err)
	}
}

// startProductionADS builds an ADS the way the shipped binary does: the
// registered store builder, the evaluator with watching enabled and the REST
// router. extra adds store properties to the common policy-source
// configuration.
func startProductionADS(t *testing.T, cloud *e2eCloud, extra map[string]interface{}) (*Store, *httptest.Server) {
	t.Helper()
	source := cloud.source()
	fallback := cloud.fallbackSource(e2eInstanceA, 201)
	credentialFile := writeRuntimeCredential(t, "source-credential", source.Credential)
	fallbackCredentialFile := writeRuntimeCredential(t, "fallback-credential", fallback.Credential)

	keyDocument, err := json.Marshal(adsync.PublicKeyFile{Keys: []adsync.PublicKeyEntry{{
		KeyID:     cloud.signer.keyID,
		PublicKey: base64.StdEncoding.EncodeToString(cloud.signer.private.Public().(ed25519.PublicKey)),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "policy-keys.json")
	if err := os.WriteFile(keyFile, keyDocument, 0o644); err != nil {
		t.Fatal(err)
	}

	properties := map[string]interface{}{
		PolicySourceURLKey:     source.URL,
		CredentialFileKey:      credentialFile,
		PublicKeysKey:          keyFile,
		CAFileKey:              source.CAFile,
		CloudFallbackURLKey:    fallback.URL,
		CloudCredentialFileKey: fallbackCredentialFile,
		TransportKey:           string(adsync.TransportWebSocket),
		HeartbeatIntervalKey:   "15s",
		StaleAfterKey:          "30s",
		StopAfterKey:           "60s",
	}
	for key, value := range extra {
		properties[key] = value
	}
	manager, err := speedlestore.NewStore(StoreType, properties)
	if err != nil {
		t.Fatalf("build registered policy-source store: %v", err)
	}
	ads := manager.(*Store)
	engine, err := eval.NewWithStore(&cfg.Config{EnableWatch: true}, ads)
	if err != nil {
		t.Fatalf("build evaluator: %v", err)
	}
	evaluator := engine.(*eval.PolicyEvalImpl)
	t.Cleanup(func() { _ = evaluator.Close() })
	router, err := adsrest.NewRouter(evaluator)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}
	server := httptest.NewTLSServer(router)
	t.Cleanup(server.Close)
	return ads, server
}

func writeRuntimeCredential(t *testing.T, name, credential string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(credential+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeDownstreamCredentials(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
