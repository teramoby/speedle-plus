//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/svcs"
)

// fakeADS answers is-allowed with reply and counts the calls.
func fakeADS(t *testing.T, reply func(w http.ResponseWriter, req isAllowedRequest)) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != svcs.PolicyAtzPath+"is-allowed" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req isAllowedRequest
		json.NewDecoder(r.Body).Decode(&req)
		reply(w, req)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestADSAuthorizerAsksAndCaches(t *testing.T) {
	server, calls := fakeADS(t, func(w http.ResponseWriter, req isAllowedRequest) {
		if req.ServiceName != "speedle-console" || req.Resource != Resource || len(req.Subject.Principals) != 2 {
			t.Errorf("request %+v", req)
		}
		fmt.Fprintf(w, `{"allowed":%v}`, req.Action == ActionRead)
	})
	now := time.Now()
	a := &ADSAuthorizer{Client: server.Client(), Endpoint: server.URL + "/", Service: "speedle-console", TTL: time.Minute,
		now: func() time.Time { return now }}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if ok, err := a.Allowed(ctx, alice, ActionRead); !ok || err != nil {
			t.Fatalf("read: %v, %v", ok, err)
		}
	}
	if ok, err := a.Allowed(ctx, alice, ActionWrite); ok || err != nil {
		t.Errorf("write: %v, %v", ok, err)
	}
	if *calls != 2 {
		t.Errorf("%d calls, want 2", *calls)
	}
	// The principals' order does not matter.
	a.Allowed(ctx, []*ads.Principal{alice[1], alice[0]}, ActionRead)
	if *calls != 2 {
		t.Errorf("%d calls after reordering, want 2", *calls)
	}
	now = now.Add(2 * time.Minute)
	a.Allowed(ctx, alice, ActionRead)
	if *calls != 3 {
		t.Errorf("%d calls after expiry, want 3", *calls)
	}
}

func TestADSAuthorizerClearsAFullCache(t *testing.T) {
	server, _ := fakeADS(t, func(w http.ResponseWriter, req isAllowedRequest) { fmt.Fprint(w, `{"allowed":true}`) })
	a := &ADSAuthorizer{Client: server.Client(), Endpoint: server.URL, Service: "s", TTL: time.Minute, cache: map[string]decision{}}
	for i := 0; i < maxCacheEntries; i++ {
		a.cache[fmt.Sprint(i)] = decision{}
	}
	a.Allowed(context.Background(), alice, ActionRead)
	if len(a.cache) != 1 {
		t.Errorf("%d cache entries", len(a.cache))
	}
}

func TestADSAuthorizerWithoutCache(t *testing.T) {
	server, calls := fakeADS(t, func(w http.ResponseWriter, req isAllowedRequest) { fmt.Fprint(w, `{"allowed":true}`) })
	a := &ADSAuthorizer{Client: server.Client(), Endpoint: server.URL, Service: "s"}
	a.Allowed(context.Background(), alice, ActionRead)
	a.Allowed(context.Background(), alice, ActionRead)
	if *calls != 2 {
		t.Errorf("%d calls, want 2", *calls)
	}
}

func TestADSAuthorizerFailsClosed(t *testing.T) {
	ctx := context.Background()
	// An evaluation error is a denial.
	server, _ := fakeADS(t, func(w http.ResponseWriter, req isAllowedRequest) {
		fmt.Fprint(w, `{"allowed":true,"errorMessage":"service not found"}`)
	})
	if ok, err := (&ADSAuthorizer{Client: server.Client(), Endpoint: server.URL}).Allowed(ctx, alice, ActionRead); ok || err != nil {
		t.Errorf("evaluation error: %v, %v", ok, err)
	}
	// Anything else is an error.
	for name, reply := range map[string]func(http.ResponseWriter, isAllowedRequest){
		"status":   func(w http.ResponseWriter, _ isAllowedRequest) { http.Error(w, "down", http.StatusInternalServerError) },
		"not JSON": func(w http.ResponseWriter, _ isAllowedRequest) { fmt.Fprint(w, "{") },
		"truncated": func(w http.ResponseWriter, _ isAllowedRequest) {
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "{")
		},
	} {
		server, _ := fakeADS(t, reply)
		if ok, err := (&ADSAuthorizer{Client: server.Client(), Endpoint: server.URL}).Allowed(ctx, alice, ActionRead); ok || err == nil {
			t.Errorf("%s: %v, %v", name, ok, err)
		}
	}
	server.Close()
	if _, err := (&ADSAuthorizer{Client: server.Client(), Endpoint: server.URL}).Allowed(ctx, alice, ActionRead); err == nil {
		t.Error("unreachable ADS: no error")
	}
	if _, err := (&ADSAuthorizer{Client: http.DefaultClient, Endpoint: "http://bad host"}).Allowed(ctx, alice, ActionRead); err == nil {
		t.Error("bad URL: no error")
	}
}
