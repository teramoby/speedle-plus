// Licensed under the Universal Permissive License (UPL) Version 1.0.
package adsrest

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/svcs/gate"
)

type metadataGate struct{}

func (metadataGate) Route(string) gate.Route { return gate.Route{Kind: gate.Fallback} }
func (metadataGate) Forward(context.Context, gate.Operation, *ads.RequestContext) (*gate.Response, error) {
	return &gate.Response{Metadata: map[string][]string{"X-Adapter-Owner": {"upstream"}, "X-Quota": {"7", "8"}}}, nil
}

func TestGateMetadataPreservesMiddlewareHeaders(t *testing.T) {
	service := &RESTService{Gate: metadataGate{}}
	recorder := httptest.NewRecorder()
	recorder.Header().Set("X-Adapter-Owner", "local")
	if !service.dispatch(recorder, httptest.NewRequest("POST", "/", nil), gate.IsAllowed, &ads.RequestContext{}) {
		t.Fatal("gate did not handle request")
	}
	if got := recorder.Header().Values("X-Adapter-Owner"); len(got) != 1 || got[0] != "local" {
		t.Fatalf("middleware header overwritten: %v", got)
	}
	if got := recorder.Header().Values("X-Quota"); len(got) != 2 || got[0] != "7" || got[1] != "8" {
		t.Fatalf("metadata values: %v", got)
	}
}
