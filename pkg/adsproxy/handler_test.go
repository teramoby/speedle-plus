// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/adsync"
)

// TestDownstreamHandlerRequiresVerifiedClientCertificate proves the composed
// route table fails closed on its own, not only behind the dedicated listener:
// every mounted operation rejects a request whose connection did not present a
// verified client certificate, and no handler runs.
func TestDownstreamHandlerRequiresVerifiedClientCertificate(t *testing.T) {
	authzCalls := 0
	server, err := NewServer(Config{InstanceID: testProxyID, Mirror: newTestMirror(t, nil)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	record := func(w http.ResponseWriter, _ *http.Request) {
		authzCalls++
		w.WriteHeader(http.StatusOK)
	}
	handler, err := DownstreamHandler(
		server,
		AuthzHandlers{
			IsAllowed:             http.HandlerFunc(record),
			AllGrantedRoles:       http.HandlerFunc(record),
			AllGrantedPermissions: http.HandlerFunc(record),
		},
	)
	if err != nil {
		t.Fatalf("downstream handler: %v", err)
	}

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, adsync.APIPrefix + "/services"},
		{http.MethodPost, "/authz-check/v1/is-allowed"},
		{http.MethodPost, "/authz-check/v1/all-granted-roles"},
		{http.MethodPost, "/authz-check/v1/all-granted-permissions"},
	}
	for _, route := range routes {
		for name, state := range map[string]*tls.ConnectionState{
			"no TLS":       nil,
			"no chain":     {},
			"empty chains": {PeerCertificates: []*x509.Certificate{{Raw: []byte("client")}}, VerifiedChains: [][]*x509.Certificate{{}}},
		} {
			request := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			request.TLS = state
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden || recorder.Body.String() != `{"error":"unauthorized"}` {
				t.Errorf("%s %s with %s: status %d body %s",
					route.method, route.path, name, recorder.Code, recorder.Body.String())
			}
		}
	}
	if authzCalls != 0 {
		t.Fatalf("unauthenticated requests reached authorization handlers: %d", authzCalls)
	}

	// The same routes reach their handlers once the connection carries a
	// verified client certificate.
	certificate := &x509.Certificate{Raw: []byte("verified-client")}
	for _, route := range routes {
		request := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		request.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{certificate},
			VerifiedChains:   [][]*x509.Certificate{{certificate}},
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s %s with a verified certificate: status %d", route.method, route.path, recorder.Code)
		}
	}
	if authzCalls != 3 {
		t.Errorf("verified requests reached authorization handlers %d times, want 3", authzCalls)
	}
}
