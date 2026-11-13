// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Exercise both signed endpoints through real chunked HTTPS downloads. Each
// transfer lasts several request timeouts while continuing to make progress.
func TestSignedDownloadsUseStallTimeout(t *testing.T) {
	service := newFixtureService(testServiceID1, "books")
	fixture := newFixture(t, service)
	original := fixture.newClient(nil)
	snapshot, _, err := original.Snapshot(context.Background(), service.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	service.publish(t, Change{Op: "delete", Kind: "authz", ID: "missing"})
	changes, err := original.Changes(context.Background(), service.ID, snapshot.SourceID, snapshot.Epoch, snapshot.Revision, snapshot.CommitHash, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"snapshot", "changes"} {
		for _, stall := range []bool{false, true} {
			t.Run(endpoint+"/stall="+strconv.FormatBool(stall), func(t *testing.T) {
				var payload []byte
				if endpoint == "snapshot" {
					payload, _ = json.Marshal(snapshot)
				} else {
					payload, _ = json.Marshal(changes)
				}
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					for i := 0; i < 8; i++ {
						start, end := len(payload)*i/8, len(payload)*(i+1)/8
						if _, err := w.Write(payload[start:end]); err != nil {
							return
						}
						w.(http.Flusher).Flush()
						delay := 25 * time.Millisecond
						if stall && i == 0 {
							delay = 400 * time.Millisecond
						}
						select {
						case <-r.Context().Done():
							return
						case <-time.After(delay):
						}
					}
				}))
				defer server.Close()
				httpClient := server.Client()
				httpClient.Timeout = 10 * time.Millisecond
				client, err := NewClient(Config{BaseURL: server.URL, APIKey: testAPIKey, InstanceID: testInstanceID, HTTPClient: httpClient, RequestTimeout: 150 * time.Millisecond, SnapshotTimeout: time.Nanosecond, Retry: RetryPolicy{MaxAttempts: 1}})
				if err != nil {
					t.Fatal(err)
				}
				if endpoint == "snapshot" {
					_, _, err = client.Snapshot(context.Background(), service.ID, "")
				} else {
					_, err = client.Changes(context.Background(), service.ID, snapshot.SourceID, snapshot.Epoch, snapshot.Revision, snapshot.CommitHash, 100)
				}
				if stall {
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("stall: %v", err)
					}
				} else if err != nil {
					t.Fatalf("active download: %v", err)
				}
				if httpClient.Timeout != 10*time.Millisecond {
					t.Fatal("caller client mutated")
				}
			})
		}
	}
}

func TestStreamRejectsTrailingData(t *testing.T) {
	for _, suffix := range []string{" {}", " garbage"} {
		var response ServicesResponse
		if err := decodeStrictReader(strings.NewReader(`{"servicesVersion":1,"services":[]}`+suffix), &response); !errors.Is(err, ErrProtocol) {
			t.Fatalf("trailing data: %v", err)
		}
	}
}

type failingBody struct{ io.Reader }

func (r failingBody) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		err = io.ErrClosedPipe
	}
	return n, err
}

func TestStreamReadFailureRetriesWithFreshResponse(t *testing.T) {
	fixture := newFixture(t, newFixtureService(testServiceID1, "books"))
	transport := &scriptedTransport{results: []func(*http.Request) (*http.Response, error){
		func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(failingBody{strings.NewReader(`{"servicesVersion":9,"services":[`)})}, nil
		},
		func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"servicesVersion":1,"services":[]}`))}, nil
		},
	}}
	client := fixture.newClient(func(config *Config) {
		config.CAFile = ""
		config.Proxy = nil
		config.HTTPClient = &http.Client{Transport: transport}
		config.Retry = RetryPolicy{MaxAttempts: 2, Sleep: func(context.Context, time.Duration) error { return nil }}
	})
	response, err := client.Services(context.Background())
	if err != nil || response.ServicesVersion != 1 || len(response.Services) != 0 || transport.calls != 2 {
		t.Fatalf("stream retry: %+v, %v, calls=%d", response, err, transport.calls)
	}
}
