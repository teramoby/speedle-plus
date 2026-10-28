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

func TestGeneratePoliciesFiltersPrincipals(t *testing.T) {
	alice := &ads.Principal{Type: ads.PRINCIPAL_TYPE_USER, Name: "alice", IDD: "corp"}
	devs := &ads.Principal{Type: ads.PRINCIPAL_TYPE_GROUP, Name: "devs"}
	requests := []*ads.RequestContext{
		// alice is not the first principal of the request.
		{ServiceName: "svc", Resource: "/a", Action: "get", Subject: &ads.Subject{Principals: []*ads.Principal{devs, alice}}},
		{ServiceName: "svc", Resource: "/a", Action: "put", Subject: &ads.Subject{Principals: []*ads.Principal{alice}}},
		{ServiceName: "svc", Resource: "/a", Action: "put", Subject: &ads.Subject{Principals: []*ads.Principal{alice}}},
		{ServiceName: "svc", Resource: "/b", Action: "get"},
		{ServiceName: "svc", Resource: "/b", Action: "put", Subject: &ads.Subject{}},
		{ServiceName: "svc", Resource: "/b", Action: "put"},
	}
	services, err := GeneratePoliciesFromDiscoverRequests(requests, ads.PRINCIPAL_TYPE_USER, "alice", "corp")
	if err != nil {
		t.Fatal(err)
	}
	svc := services["svc"]
	if len(svc.RolePolicies) != 1 || svc.RolePolicies[0].Principals[0] != "idd=corp:user:alice" {
		t.Fatalf("role policies %+v", svc.RolePolicies)
	}
	var aliceActions, anonymousActions []string
	for _, policy := range svc.Policies {
		switch policy.Principals[0][0] {
		case "role:role_idd=corp:user:alice":
			aliceActions = policy.Permissions[0].Actions
		case "role:" + ads.BuiltIn_Role_Anonymous:
			anonymousActions = policy.Permissions[0].Actions
		}
	}
	if len(aliceActions) != 2 || aliceActions[0] != "get" || aliceActions[1] != "put" {
		t.Errorf("alice may %v on /a, want get and put", aliceActions)
	}
	if len(anonymousActions) != 2 {
		t.Errorf("anonymous may %v on /b, want get and put", anonymousActions)
	}

	all, _ := GeneratePoliciesFromDiscoverRequests(requests, "", "", "")
	if len(all["svc"].RolePolicies) != 2 {
		t.Errorf("without a filter: role policies %+v", all["svc"].RolePolicies)
	}
}
