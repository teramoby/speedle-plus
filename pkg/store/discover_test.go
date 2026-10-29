package store

import (
	"testing"

	"github.com/teramoby/speedle-plus/api/ads"
)

func TestGeneratePoliciesForAnonymousRequest(t *testing.T) {
	requests := []*ads.RequestContext{{ServiceName: "svc", Resource: "/r", Action: "get"}}
	services, err := GeneratePoliciesFromDiscoverRequests(requests, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	service := services["svc"]
	if service == nil || len(service.Policies) != 1 {
		t.Fatalf("got %+v, want one policy for svc", service)
	}
	if got := service.Policies[0].Principals; len(got) != 1 || len(got[0]) != 1 || got[0][0] != "role:"+ads.BuiltIn_Role_Anonymous {
		t.Errorf("got principals %v, want the anonymous role", got)
	}
}
