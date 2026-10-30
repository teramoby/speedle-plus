//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/store"
)

func TestPutGetLastRequest(t *testing.T) {
	s, err := store.NewStore(etcdStoreConfig(t).StoreType, etcdStoreConfig(t).StoreProps)
	if err != nil {
		t.Fatal("fail to new etcd store")
	}
	store.MaxDiscoverRequestNum = int64(100)
	store.DeleteNumWhenReachMaxDiscoverRequest = int64(10)
	discover := s.(store.DiscoverRequestManager)

	i := 0
	for i < 700 {
		user := ads.Principal{Type: "user", Name: "user" + strconv.Itoa(i%10)}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		serviceName := "erp" + strconv.Itoa(i%10)
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "read", Attributes: map[string]interface{}{}}
		count, err := s.(*Store).PutRequest(&request)
		if err != nil {
			t.Error("fail to put request in store")
		}
		if count > store.MaxDiscoverRequestNum+1 {
			t.Error("should not exceed max number")
		}

		req, _, err := discover.GetLastDiscoverRequest(serviceName)
		if err != nil {
			t.Error("fail to get last request for service")
		}
		if req.Resource != resName {
			t.Error("the last request for service is incorrect")
		}

		i++
	}

}

func TestGetLastRequestContinously(t *testing.T) {
	s, err := store.NewStore(etcdStoreConfig(t).StoreType, etcdStoreConfig(t).StoreProps)
	if err != nil {
		t.Fatal("fail to new etcd store")
	}
	discover := s.(store.DiscoverRequestManager)
	i := 0
	for i < 5 {
		user := ads.Principal{Type: "user", Name: "user" + strconv.Itoa(i%10)}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		serviceName := "erp"
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "read", Attributes: map[string]interface{}{}}
		err := discover.SaveDiscoverRequest(&request)
		if err != nil {
			t.Fatal("fail to put request in store")
		}
		i++
	}
	request, revision, err := discover.GetLastDiscoverRequest("erp")
	if err != nil {
		t.Errorf("failed to GetLastDiscoverRequest: %v", err)
	}
	if request.Resource != "/res4" {
		t.Error("last request is incorrect.")
	}
	for i < 10 {
		user := ads.Principal{Type: "user", Name: "user" + strconv.Itoa(i%10)}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		serviceName := "erp"
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "read", Attributes: map[string]interface{}{}}
		err := discover.SaveDiscoverRequest(&request)
		if err != nil {
			t.Fatal("fail to put request in store")
		}
		i++
	}
	requests, _, err := discover.GetDiscoverRequestsSinceRevision("erp", revision)
	if err != nil {
		t.Errorf("failed to GetDiscoverRequestsSinceRevision: %v", err)
	}
	if len(requests) != 5 {
		t.Error("requests number should be 5")
	}
	if requests[0].Resource != "/res5" ||
		requests[1].Resource != "/res6" ||
		requests[2].Resource != "/res7" ||
		requests[3].Resource != "/res8" ||
		requests[4].Resource != "/res9" {
		t.Error("requests sequence or content is incorrect")
	}
}

func TestResetDiscoverRequests(t *testing.T) {
	s, err := store.NewStore(etcdStoreConfig(t).StoreType, etcdStoreConfig(t).StoreProps)
	if err != nil {
		t.Fatal("fail to new etcd store")
	}
	discover := s.(store.DiscoverRequestManager)
	i := 0
	for i < 100 {
		user := ads.Principal{Type: "user", Name: "user" + strconv.Itoa(i%10)}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		serviceName := "erp" + strconv.Itoa(i%10)
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "read", Attributes: map[string]interface{}{}}
		err := discover.SaveDiscoverRequest(&request)
		if err != nil {
			t.Error("fail to put request.")
		}
		i++
	}
	testRequests, _, err := discover.GetDiscoverRequests("erp0")
	if err != nil {
		t.Errorf("failed to GetDiscoverRequests: %v", err)
	}
	fmt.Println("testRequest len:", len(testRequests))

	err = discover.ResetDiscoverRequests("erp0")
	if err != nil {
		t.Fatal("Fail to reset all requests")
	}
	zeroRequests, _, err := discover.GetDiscoverRequests("erp0")
	if err != nil {
		t.Errorf("failed to GetDiscoverRequests: %v", err)
	}
	if len(zeroRequests) != 0 {
		t.Fatal("Should have no requests now, as requests are reset")
	}

	err = discover.ResetDiscoverRequests("")
	if err != nil {
		t.Fatal("Fail to reset all requests")
	}
	zeroRequests, _, _ = discover.GetDiscoverRequests("")
	if len(zeroRequests) != 0 {
		t.Fatal("Should have no requests now, as requests are reset")
	}

}

func TestGetRequests(t *testing.T) {
	s, err := store.NewStore(etcdStoreConfig(t).StoreType, etcdStoreConfig(t).StoreProps)
	if err != nil {
		t.Fatal("fail to new etcd store")
	}
	store.MaxDiscoverRequestNum = int64(1000)
	store.DeleteNumWhenReachMaxDiscoverRequest = int64(100)
	discover := s.(store.DiscoverRequestManager)
	err = discover.ResetDiscoverRequests("")
	if err != nil {
		t.Fatal("Fail to reset all requests")
	}

	requestNum := 550
	i := 0
	for i < requestNum {
		user := ads.Principal{Type: "user", Name: "user" + strconv.Itoa(i%10)}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		serviceName := "erp" + strconv.Itoa(i%10)
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "read", Attributes: map[string]interface{}{}}
		err := discover.SaveDiscoverRequest(&request)
		if err != nil {
			t.Error("fail to put request.")
		}
		i++
	}
	fmt.Println("starting get requests, time is:", time.Now())
	requests, _, err := s.(*Store).GetRequests(s.(*Store).discoverPrefix(), 100)
	fmt.Println("finish get requests, time is:", time.Now())
	if err != nil {
		t.Errorf("failed to GetRequests: %v", err)
	}
	if len(requests) != requestNum {
		t.Error("number incorrect, expected is:", requestNum, ",but is:", len(requests))
	}
	for index, req := range requests {
		if req.Resource != "/res"+strconv.Itoa(index) {
			t.Error("sequence is incorrect,", "expected is:", "/res"+strconv.Itoa(index), "but is:", req.Resource)
		}
	}
}

func TestGeneratePolicies(t *testing.T) {
	s, err := store.NewStore(etcdStoreConfig(t).StoreType, etcdStoreConfig(t).StoreProps)
	if err != nil {
		t.Fatal("fail to new etcd store")
	}
	discover := s.(store.DiscoverRequestManager)
	err = discover.ResetDiscoverRequests("")
	if err != nil {
		t.Fatal("Fail to reset all requests")
	}
	serviceName := "erp"
	i := 0
	for i < 5 {
		user := ads.Principal{Type: "user", Name: "user1"}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "read", Attributes: map[string]interface{}{}}
		err := discover.SaveDiscoverRequest(&request)
		if err != nil {
			t.Error("fail to put request.")
		}
		i++
	}
	for i < 10 {
		user := ads.Principal{Type: "user", Name: "user2"}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "write", Attributes: map[string]interface{}{}}
		err := discover.SaveDiscoverRequest(&request)
		if err != nil {
			t.Error("fail to put request.")
		}
		i++
	}
	serviceName = "erp1"
	i = 0
	for i < 5 {
		user := ads.Principal{Type: "user", Name: "user1"}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "write", Attributes: map[string]interface{}{}}
		err := discover.SaveDiscoverRequest(&request)
		if err != nil {
			t.Error("fail to put request.")
		}
		i++
	}
	for i < 10 {
		user := ads.Principal{Type: "user", Name: "user2"}
		subj := ads.Subject{Principals: []*ads.Principal{&user}}
		resName := "/res" + strconv.Itoa(i)
		request := ads.RequestContext{Subject: &subj, ServiceName: serviceName, Resource: resName, Action: "read", Attributes: map[string]interface{}{}}
		err := discover.SaveDiscoverRequest(&request)
		if err != nil {
			t.Error("fail to put request.")
		}
		i++
	}
	serviceMap, _, err := discover.GeneratePolicies("erp", "", "user1", "")
	if err != nil {
		t.Error("fail to generate policy for user:", err)
	}
	if serviceMap["erp"] == nil {
		t.Error("service should exist.")
	} else {
		if len(serviceMap["erp"].Policies) != 5 {
			t.Error("should have generated 5 policies")
		} else {
			for index, policy := range serviceMap["erp"].Policies {
				if (policy.Permissions[0].Resource != "/res"+strconv.Itoa(index)) && (policy.Permissions[0].Actions[0] != "read") {
					t.Error("policy error")
				}
			}
		}
	}
	serviceMap, _, err = discover.GeneratePolicies("erp1", "", "user1", "")
	if err != nil {
		t.Error("fail to generate policy for user:", err)
	}
	if serviceMap["erp1"] == nil {
		t.Error("service should exist")
	} else {
		if len(serviceMap["erp1"].Policies) != 5 {
			t.Error("should have generated 5 policies")
		} else {
			for index, policy := range serviceMap["erp1"].Policies {
				if (policy.Permissions[0].Resource != "/res"+strconv.Itoa(index)) && (policy.Permissions[0].Actions[0] != "write") {
					t.Error("policy error")
				}
			}
		}
	}
	serviceMap, _, err = discover.GeneratePolicies("erp", "", "user2", "")
	if err != nil {
		t.Error("fail to generate policy for user:", err)
	}
	if serviceMap["erp"] == nil {
		t.Error("policy should not be nil")
	} else {
		if len(serviceMap["erp"].Policies) != 5 {
			t.Error("should have generated 5 policies")
		} else {
			for index, policy := range serviceMap["erp"].Policies {
				if (policy.Permissions[0].Resource != "/res"+strconv.Itoa(5+index)) && (policy.Permissions[0].Actions[0] != "write") {
					t.Error("policy error")
				}
			}
		}
	}
	serviceMap, _, err = discover.GeneratePolicies("erp1", "", "user2", "")
	if err != nil {
		t.Error("fail to generate policy for user:", err)
	}
	if serviceMap["erp1"] == nil {
		t.Error("policy should not be nil")
	} else {
		if len(serviceMap["erp1"].Policies) != 5 {
			t.Error("should have generated 5 policies")
		} else {
			for index, policy := range serviceMap["erp1"].Policies {
				if (policy.Permissions[0].Resource != "/res"+strconv.Itoa(5+index)) && (policy.Permissions[0].Actions[0] != "read") {
					t.Error("policy error")
				}
			}
		}
	}

	serviceMap, _, err = discover.GeneratePolicies("erp", "", "", "")
	if err != nil {
		t.Error("fail to generate policies:", err)
	} else {
		if len(serviceMap["erp"].RolePolicies) != 2 {
			t.Error("should have generated 2 role policies")
		}

		if len(serviceMap["erp"].Policies) != 10 {
			t.Error("should have generated 10 policies")
		}
	}

	serviceMap, _, err = discover.GeneratePolicies("erp1", "", "", "")
	if err != nil {
		t.Error("fail to generate policies:", err)
	} else {
		if len(serviceMap["erp1"].RolePolicies) != 2 {
			t.Error("should have generated 2 role policies")
		}

		if len(serviceMap["erp1"].Policies) != 10 {
			t.Error("should have generated 10 policies")
		}
	}

}

func newDiscoverStore(t *testing.T, keyPrefix string) *Store {
	t.Helper()
	props := map[string]interface{}{EtcdEndpointKey: etcdStoreConfig(t).StoreProps[EtcdEndpointKey]}
	if keyPrefix != "" {
		props[EtcdKeyPrefixKey] = keyPrefix
	}
	s, err := store.NewStore(StoreType, props)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.(*Store).destroy() })
	return s.(*Store)
}

func discoverRequest(serviceName string) *ads.RequestContext {
	subject := ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}}}
	return &ads.RequestContext{Subject: &subject, ServiceName: serviceName, Resource: "res", Action: "read", Attributes: map[string]interface{}{}}
}

func TestDiscoverRequestsStayWithTheirKeyPrefix(t *testing.T) {
	first, second := newDiscoverStore(t, "/discover-first/"), newDiscoverStore(t, "/discover-second/")
	t.Cleanup(func() { first.ResetDiscoverRequests(""); second.ResetDiscoverRequests("") })
	if err := first.SaveDiscoverRequest(discoverRequest("scoped")); err != nil {
		t.Fatal(err)
	}
	if requests, _, err := second.GetDiscoverRequests("scoped"); err != nil || len(requests) != 0 {
		t.Fatalf("another key prefix sees requests = %v, %v", requests, err)
	}
	if err := second.ResetDiscoverRequests(""); err != nil {
		t.Fatal(err)
	}
	if requests, _, err := first.GetDiscoverRequests("scoped"); err != nil || len(requests) != 1 {
		t.Fatalf("own requests after another prefix's reset = %v, %v", requests, err)
	}
}

func TestDefaultKeyPrefixKeepsEarlierDiscoverRequests(t *testing.T) {
	s := newDiscoverStore(t, "")
	if s.KeyPrefix != DefaultKeyPrefix {
		t.Fatalf("key prefix = %q", s.KeyPrefix)
	}
	t.Cleanup(func() { s.ResetDiscoverRequests("legacy") })
	// A request as stores recorded it before discover requests were scoped.
	value, err := json.Marshal(discoverRequest("legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.client.Put(context.Background(), "/isAllowedRequests/legacy/"+time.Now().String(), string(value)); err != nil {
		t.Fatal(err)
	}
	request, _, err := s.GetLastDiscoverRequest("legacy")
	if err != nil || request == nil || request.ServiceName != "legacy" {
		t.Fatalf("earlier request = %v, %v", request, err)
	}
}
