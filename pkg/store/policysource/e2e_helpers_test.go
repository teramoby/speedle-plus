// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

// Helpers for the end-to-end tests that run the production runtime against
// the in-process Cloud fixture.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"
)

const e2eWaitTimeout = 5 * time.Second

// e2eAgent wraps a store under test for the wait helpers.
type e2eAgent struct {
	store *Store
}

func (a *e2eAgent) status(serviceName string) (adsync.Status, bool) {
	return a.store.Status(serviceName)
}

func (a *e2eAgent) liveness() adsync.Liveness {
	return a.store.Instance().Liveness
}

func e2eWaitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	e2eWaitForWithin(t, e2eWaitTimeout, what, condition)
}

func e2eWaitForWithin(t *testing.T, timeout time.Duration, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func e2eWaitLiveness(t *testing.T, agent *e2eAgent, liveness adsync.Liveness) {
	t.Helper()
	e2eWaitFor(t, fmt.Sprintf("instance to be %s", liveness), func() bool {
		return agent.liveness() == liveness
	})
}

func e2eWaitState(t *testing.T, agent *e2eAgent, serviceName string, state adsync.SyncState, reason adsync.SyncReason) {
	t.Helper()
	what := fmt.Sprintf("service %s to be %s", serviceName, state)
	if reason != "" {
		what += " (" + string(reason) + ")"
	}
	e2eWaitFor(t, what, func() bool {
		status, known := agent.status(serviceName)
		return known && status.Service.State == state && status.Service.Reason == reason
	})
}

func e2eWaitAllInSync(t *testing.T, agent *e2eAgent, services ...string) {
	t.Helper()
	e2eWaitLiveness(t, agent, adsync.LivenessLive)
	for _, service := range services {
		e2eWaitState(t, agent, service, adsync.SyncStateInSync, "")
	}
}

func e2eWaitServiceRevision(t *testing.T, agent *e2eAgent, serviceName string, revision uint64) {
	t.Helper()
	e2eWaitFor(t, fmt.Sprintf("service %s to reach revision %d", serviceName, revision), func() bool {
		status, known := agent.status(serviceName)
		return known && status.Service.State == adsync.SyncStateInSync && status.Service.Revision == revision
	})
}

// e2eWaitDecision waits until the instance answers the check with the wanted
// decision, which is a stronger signal than the sync state alone when a source

func e2eDecodeResponse(t *testing.T, body []byte) adsrest.IsAllowedResponse {
	t.Helper()
	var response adsrest.IsAllowedResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode authorization response %q: %v", body, err)
	}
	return response
}

type e2eCheck struct {
	Service    string
	Resource   string
	Action     string
	Principals []*adsrest.JsonPrincipal
	Attributes []*adsrest.JsonAttribute
}

func (c e2eCheck) body(t *testing.T) []byte {
	t.Helper()
	request := adsrest.JsonContext{
		Subject:     &adsrest.JsonSubject{Principals: c.Principals},
		ServiceName: c.Service,
		Resource:    c.Resource,
		Action:      c.Action,
		Attributes:  c.Attributes,
	}
	encoded, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("encode check: %v", err)
	}
	return encoded
}

func e2eUser(name string) []*adsrest.JsonPrincipal {
	return []*adsrest.JsonPrincipal{{Type: "user", Name: name}}
}
