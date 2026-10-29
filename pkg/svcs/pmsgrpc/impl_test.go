//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pmsgrpc

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
	_ "github.com/teramoby/speedle-plus/pkg/store/etcd"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsgrpc/pb"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

func newStore(t *testing.T) pms.PolicyStoreManager {
	t.Helper()
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	// The file store keeps discover requests in the working directory.
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	ps := storetest.PolicyStore(t, location)
	return ps
}

func code(err error) codes.Code {
	return status.Code(err)
}

var ctx = context.Background()

// Expose only the required interface, hiding the file store's optional Discover.
type withoutDiscover struct{ pms.PolicyStoreManager }

func TestDiscoverUnsupportedStore(t *testing.T) {
	impl := NewServiceImpl(withoutDiscover{newStore(t)})
	_, err := impl.GetDiscoverRequests(ctx, &pb.DiscoverRequestsRequest{})
	if code(err) != codes.Unimplemented {
		t.Fatalf("GetDiscoverRequests: %v", err)
	}
	_, err = impl.ResetDiscoverRequests(ctx, &pb.ResetRequestsRequest{})
	if code(err) != codes.Unimplemented {
		t.Fatalf("ResetDiscoverRequests: %v", err)
	}
	_, err = impl.GetDiscoverPolicies(ctx, &pb.DiscoverPoliciesRequest{})
	if code(err) != codes.Unimplemented {
		t.Fatalf("GetDiscoverPolicies: %v", err)
	}
}

func TestServices(t *testing.T) {
	impl := NewServiceImpl(newStore(t))
	for _, in := range []*pb.ServiceRequest{
		{Name: "app", Type: pb.ServiceType_APPLICATION},
		{Name: "k8s", Type: pb.ServiceType_K8S_CLUSTER},
	} {
		got, err := impl.CreateService(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != in.Name || got.Type != in.Type {
			t.Fatalf("created %+v, want %+v", got, in)
		}
	}
	if _, err := impl.CreateService(ctx, &pb.ServiceRequest{Name: "app"}); code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate service error = %v", err)
	}

	all, err := impl.QueryServices(ctx, &pb.ServiceQueryRequest{})
	if err != nil || len(all.Services) != 2 {
		t.Fatalf("services = %v, %v", all, err)
	}
	one, err := impl.QueryServices(ctx, &pb.ServiceQueryRequest{Name: "k8s"})
	if err != nil || len(one.Services) != 1 || one.Services[0].Type != pb.ServiceType_K8S_CLUSTER {
		t.Fatalf("service k8s = %v, %v", one, err)
	}
	if _, err := impl.QueryServices(ctx, &pb.ServiceQueryRequest{Name: "missing"}); code(err) != codes.NotFound {
		t.Fatalf("missing service error = %v", err)
	}

	if _, err := impl.DeleteServices(ctx, &pb.ServiceQueryRequest{Name: "k8s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.DeleteServices(ctx, &pb.ServiceQueryRequest{Name: "k8s"}); code(err) != codes.NotFound {
		t.Fatalf("deleting a missing service error = %v", err)
	}
	if _, err := impl.DeleteServices(ctx, &pb.ServiceQueryRequest{}); err != nil {
		t.Fatal(err)
	}
	if all, _ := impl.QueryServices(ctx, &pb.ServiceQueryRequest{}); len(all.Services) != 0 {
		t.Fatalf("services left after deleting all: %v", all.Services)
	}
}

func TestPolicies(t *testing.T) {
	impl := NewServiceImpl(newStore(t))
	if _, err := impl.CreateService(ctx, &pb.ServiceRequest{Name: "app"}); err != nil {
		t.Fatal(err)
	}
	policy := &pb.Policy{
		Name:        "p1",
		Effect:      pb.Effect_DENY,
		Principals:  []*pb.AndPrincipals{{Principals: []string{"user:alice"}}},
		Permissions: []*pb.Policy_Permission{{Resource: "res", Actions: []string{"read"}}, {ResourceExpression: "r.*", Actions: []string{"write"}}},
		Condition:   "a == 'b'",
	}
	created, err := impl.CreatePolicy(ctx, &pb.PolicyRequest{ServiceName: "app", Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	if created.Id == "" || created.Effect != pb.Effect_DENY || created.Condition != policy.Condition ||
		!reflect.DeepEqual(created.Principals[0].Principals, []string{"user:alice"}) ||
		created.Permissions[1].ResourceExpression != "r.*" {
		t.Fatalf("created policy %+v", created)
	}
	grant, err := impl.CreatePolicy(ctx, &pb.PolicyRequest{ServiceName: "app", Policy: &pb.Policy{Name: "p2", Effect: pb.Effect_GRANT}})
	if err != nil || grant.Effect != pb.Effect_GRANT || len(grant.Permissions) != 0 {
		t.Fatalf("grant policy %+v, %v", grant, err)
	}

	for name, in := range map[string]*pb.PolicyRequest{
		"no service":    {Policy: policy},
		"no policy":     {ServiceName: "app"},
		"global policy": {ServiceName: pms.GlobalService, Policy: policy},
	} {
		if _, err := impl.CreatePolicy(ctx, in); code(err) != codes.InvalidArgument {
			t.Errorf("%s: error = %v", name, err)
		}
	}
	if _, err := impl.CreatePolicy(ctx, &pb.PolicyRequest{ServiceName: "missing", Policy: policy}); code(err) != codes.NotFound {
		t.Errorf("policy in a missing service: error = %v", err)
	}

	all, err := impl.QueryPolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "app"})
	if err != nil || len(all.Policies) != 2 {
		t.Fatalf("policies = %v, %v", all, err)
	}
	filtered, err := impl.QueryPolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "app", Filters: "name eq p1"})
	if err != nil || len(filtered.Policies) != 1 || filtered.Policies[0].Name != "p1" {
		t.Fatalf("filtered policies = %v, %v", filtered, err)
	}
	one, err := impl.QueryPolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "app", PolicyID: created.Id})
	if err != nil || len(one.Policies) != 1 || one.Policies[0].Name != "p1" {
		t.Fatalf("policy by id = %v, %v", one, err)
	}
	if _, err := impl.QueryPolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "app", PolicyID: "nope"}); err == nil {
		t.Error("a missing policy id must fail")
	}
	if _, err := impl.QueryPolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "missing"}); code(err) != codes.NotFound {
		t.Errorf("policies of a missing service: error = %v", err)
	}
	if _, err := impl.QueryPolicies(ctx, &pb.PolicyQueryRequest{}); code(err) != codes.InvalidArgument {
		t.Errorf("query without service: error = %v", err)
	}

	if _, err := impl.DeletePolicies(ctx, &pb.PolicyQueryRequest{}); code(err) != codes.InvalidArgument {
		t.Errorf("delete without service: error = %v", err)
	}
	if _, err := impl.DeletePolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "app", PolicyID: created.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.DeletePolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "app", PolicyID: created.Id}); code(err) != codes.NotFound {
		t.Errorf("deleting a deleted policy: error = %v", err)
	}
	if _, err := impl.DeletePolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.DeletePolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "missing"}); code(err) != codes.NotFound {
		t.Errorf("deleting policies of a missing service: error = %v", err)
	}
	if all, _ := impl.QueryPolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "app"}); len(all.Policies) != 0 {
		t.Fatalf("policies left: %v", all.Policies)
	}
}

func TestRolePolicies(t *testing.T) {
	impl := NewServiceImpl(newStore(t))
	if _, err := impl.CreateService(ctx, &pb.ServiceRequest{Name: "app"}); err != nil {
		t.Fatal(err)
	}
	rolePolicy := &pb.RolePolicy{
		Name: "rp1", Effect: pb.Effect_GRANT, Roles: []string{"admin"}, Principals: []string{"user:alice"},
		Resources: []string{"res"}, ResourceExpressions: []string{"r.*"}, Condition: "a == 'b'",
	}
	created, err := impl.CreateRolePolicy(ctx, &pb.RolePolicyRequest{ServiceName: "app", RolePolicy: rolePolicy})
	if err != nil {
		t.Fatal(err)
	}
	if created.Id == "" || created.Effect != pb.Effect_GRANT || created.Condition != rolePolicy.Condition ||
		!reflect.DeepEqual(created.ResourceExpressions, []string{"r.*"}) {
		t.Fatalf("created role policy %+v", created)
	}
	deny, err := impl.CreateRolePolicy(ctx, &pb.RolePolicyRequest{ServiceName: "app",
		RolePolicy: &pb.RolePolicy{Name: "rp2", Effect: pb.Effect_DENY, Roles: []string{"guest"}, Principals: []string{"user:bob"}}})
	if err != nil || deny.Effect != pb.Effect_DENY {
		t.Fatalf("deny role policy %+v, %v", deny, err)
	}

	if _, err := impl.CreateRolePolicy(ctx, &pb.RolePolicyRequest{RolePolicy: rolePolicy}); code(err) != codes.InvalidArgument {
		t.Errorf("no service: error = %v", err)
	}
	if _, err := impl.CreateRolePolicy(ctx, &pb.RolePolicyRequest{ServiceName: "app"}); code(err) != codes.InvalidArgument {
		t.Errorf("no role policy: error = %v", err)
	}
	if _, err := impl.CreateRolePolicy(ctx, &pb.RolePolicyRequest{ServiceName: "missing", RolePolicy: rolePolicy}); code(err) != codes.NotFound {
		t.Errorf("missing service: error = %v", err)
	}

	all, err := impl.QueryRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "app"})
	if err != nil || len(all.RolePolicies) != 2 {
		t.Fatalf("role policies = %v, %v", all, err)
	}
	filtered, err := impl.QueryRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "app", Filters: "name eq rp2"})
	if err != nil || len(filtered.RolePolicies) != 1 || filtered.RolePolicies[0].Name != "rp2" {
		t.Fatalf("filtered role policies = %v, %v", filtered, err)
	}
	one, err := impl.QueryRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "app", RolePolicyID: created.Id})
	if err != nil || len(one.RolePolicies) != 1 || one.RolePolicies[0].Name != "rp1" {
		t.Fatalf("role policy by id = %v, %v", one, err)
	}
	if _, err := impl.QueryRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "app", RolePolicyID: "nope"}); code(err) != codes.NotFound {
		t.Errorf("missing role policy: error = %v", err)
	}
	if _, err := impl.QueryRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "missing"}); code(err) != codes.NotFound {
		t.Errorf("role policies of a missing service: error = %v", err)
	}
	if _, err := impl.QueryRolePolicies(ctx, &pb.RolePolicyQueryRequest{}); code(err) != codes.InvalidArgument {
		t.Errorf("query without service: error = %v", err)
	}

	counts, err := impl.ListPolicyCounts(ctx, &pb.Empty{})
	if err != nil || counts.CountMap["app"].RolePolicyCount != 2 {
		t.Fatalf("counts = %v, %v", counts, err)
	}

	if _, err := impl.DeleteRolePolicies(ctx, &pb.RolePolicyQueryRequest{}); code(err) != codes.InvalidArgument {
		t.Errorf("delete without service: error = %v", err)
	}
	if _, err := impl.DeleteRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "app", RolePolicyID: created.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.DeleteRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "app", RolePolicyID: created.Id}); code(err) != codes.NotFound {
		t.Errorf("deleting a deleted role policy: error = %v", err)
	}
	if _, err := impl.DeleteRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.DeleteRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "missing"}); code(err) != codes.NotFound {
		t.Errorf("deleting role policies of a missing service: error = %v", err)
	}
}

func TestFunctions(t *testing.T) {
	impl := NewServiceImpl(newStore(t))
	in := &pb.Function{Name: "f1", Description: "d", FuncUrl: "https://f1", LocalFuncUrl: "http://local",
		Ca: "ca", ResultCachable: true, ResultTTL: 10}
	created, err := impl.CreateFunction(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(created, in) {
		t.Fatalf("created %+v, want %+v", created, in)
	}
	if _, err := impl.CreateFunction(ctx, &pb.Function{Name: "f2", FuncUrl: "https://f2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.CreateFunction(ctx, in); code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate function error = %v", err)
	}

	all, err := impl.QueryFunctions(ctx, &pb.FunctionQueryRequest{})
	if err != nil || len(all.Functions) != 2 {
		t.Fatalf("functions = %v, %v", all, err)
	}
	filtered, err := impl.QueryFunctions(ctx, &pb.FunctionQueryRequest{Filters: "name eq f2"})
	if err != nil || len(filtered.Functions) != 1 || filtered.Functions[0].Name != "f2" {
		t.Fatalf("filtered functions = %v, %v", filtered, err)
	}
	one, err := impl.QueryFunctions(ctx, &pb.FunctionQueryRequest{Name: "f1"})
	if err != nil || len(one.Functions) != 1 || one.Functions[0].Ca != "ca" {
		t.Fatalf("function f1 = %v, %v", one, err)
	}
	if _, err := impl.QueryFunctions(ctx, &pb.FunctionQueryRequest{Name: "nope"}); code(err) != codes.NotFound {
		t.Errorf("missing function: error = %v", err)
	}

	if _, err := impl.DeleteFunctions(ctx, &pb.FunctionQueryRequest{}); code(err) != codes.InvalidArgument {
		t.Errorf("delete without name or filters: error = %v", err)
	}
	if _, err := impl.DeleteFunctions(ctx, &pb.FunctionQueryRequest{Name: "f1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.DeleteFunctions(ctx, &pb.FunctionQueryRequest{Name: "f1"}); code(err) != codes.NotFound {
		t.Errorf("deleting a deleted function: error = %v", err)
	}
	if _, err := impl.DeleteFunctions(ctx, &pb.FunctionQueryRequest{Filters: "all"}); err != nil {
		t.Fatal(err)
	}
	if all, _ := impl.QueryFunctions(ctx, &pb.FunctionQueryRequest{}); len(all.Functions) != 0 {
		t.Fatalf("functions left: %v", all.Functions)
	}
}

func TestDiscover(t *testing.T) {
	ps := newStore(t)
	impl := NewServiceImpl(ps)
	manager, supported := ps.(store.DiscoverRequestManager)
	if !supported {
		_, err := impl.GetDiscoverRequests(ctx, &pb.DiscoverRequestsRequest{})
		if code(err) != codes.Unimplemented {
			t.Fatal(err)
		}
		return
	}
	for _, resource := range []string{"r1", "r2"} {
		err := manager.SaveDiscoverRequest(&ads.RequestContext{
			Subject:     &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice", IDD: "idd"}}, TokenType: "t", Token: "x"},
			ServiceName: "app",
			Resource:    resource,
			Action:      "read",
			Attributes:  map[string]interface{}{"n": 1},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	all, err := impl.GetDiscoverRequests(ctx, &pb.DiscoverRequestsRequest{ServiceName: "app"})
	if err != nil || len(all.Requests) != 2 {
		t.Fatalf("requests = %v, %v", all, err)
	}
	first := all.Requests[0]
	if first.Subject.Principals[0].Idd != "idd" || first.Attributes["n"] != "1" {
		t.Fatalf("request lost data: %+v", first)
	}
	last, err := impl.GetDiscoverRequests(ctx, &pb.DiscoverRequestsRequest{ServiceName: "app", Last: true})
	if err != nil || len(last.Requests) != 1 || last.Requests[0].Resource != "r2" {
		t.Fatalf("last request = %v, %v", last, err)
	}
	since, err := impl.GetDiscoverRequests(ctx, &pb.DiscoverRequestsRequest{ServiceName: "app", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if since.Revision != all.Revision {
		t.Fatalf("revision since 1 = %d, want %d", since.Revision, all.Revision)
	}

	policies, err := impl.GetDiscoverPolicies(ctx, &pb.DiscoverPoliciesRequest{ServiceName: "app"})
	if err != nil || len(policies.Services) != 1 || len(policies.Services[0].Policies) == 0 {
		t.Fatalf("generated policies = %v, %v", policies, err)
	}

	if _, err := impl.ResetDiscoverRequests(ctx, &pb.ResetRequestsRequest{ServiceName: "app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.GetDiscoverRequests(ctx, &pb.DiscoverRequestsRequest{ServiceName: "app", Last: true}); code(err) != codes.NotFound {
		t.Fatalf("last request after reset: error = %v", err)
	}
}

func TestToGRPCStatus(t *testing.T) {
	if toGRPCStatus(nil) != nil {
		t.Fatal("nil error must stay nil")
	}
	for errCode, want := range map[errors.ErrorCode]codes.Code{
		errors.StoreError:          codes.Internal,
		errors.EntityNotFound:      codes.NotFound,
		errors.EntityAlreadyExists: codes.AlreadyExists,
		errors.SerializationError:  codes.Internal,
		errors.ExceedLimit:         codes.ResourceExhausted,
		errors.InvalidRequest:      codes.InvalidArgument,
		errors.ConfigError:         codes.Unknown,
	} {
		if got := code(toGRPCStatus(errors.New(errCode, "x"))); got != want {
			t.Errorf("%s: code %v, want %v", errCode, got, want)
		}
	}
}

func TestConvertRPCService(t *testing.T) {
	if got := convertRPCServiceRequest(&pb.ServiceRequest{Name: "a", Type: pb.ServiceType_K8S_CLUSTER}); got.Type != pms.TypeK8SCluster {
		t.Fatalf("k8s service type = %q", got.Type)
	}
	service := &pms.Service{Name: "s", Type: pms.TypeApplication,
		Policies:     []*pms.Policy{{ID: "p", Effect: pms.Grant}},
		RolePolicies: []*pms.RolePolicy{{ID: "rp", Effect: pms.Deny}}}
	got := convertMetaService(service)
	if got.Type != pb.ServiceType_APPLICATION || got.Policies[0].Id != "p" || got.RolePolicies[0].Effect != pb.Effect_DENY {
		t.Fatalf("converted service %+v", got)
	}
	if convertAPISubject(nil) != nil || convertAPIPrincipals(nil) != nil {
		t.Fatal("nil subject or principals must convert to nil")
	}
}

func TestStoreErrors(t *testing.T) {
	failing := storetest.NewFailingStore()
	failing.CountsSucceed = true
	impl := NewServiceImpl(failing)
	policy := &pb.Policy{Name: "p", Effect: pb.Effect_GRANT}
	rolePolicy := &pb.RolePolicy{Name: "rp", Effect: pb.Effect_GRANT}
	calls := map[string]func() error{
		"CreateFunction": func() error { _, err := impl.CreateFunction(ctx, &pb.Function{Name: "f"}); return err },
		"QueryFunctions": func() error { _, err := impl.QueryFunctions(ctx, &pb.FunctionQueryRequest{}); return err },
		"QueryFunctionsFilter": func() error {
			_, err := impl.QueryFunctions(ctx, &pb.FunctionQueryRequest{Filters: "name eq f"})
			return err
		},
		"DeleteFunctionsAll": func() error {
			_, err := impl.DeleteFunctions(ctx, &pb.FunctionQueryRequest{Filters: "all"})
			return err
		},
		"CreateService":     func() error { _, err := impl.CreateService(ctx, &pb.ServiceRequest{Name: "s"}); return err },
		"QueryServices":     func() error { _, err := impl.QueryServices(ctx, &pb.ServiceQueryRequest{}); return err },
		"DeleteServicesAll": func() error { _, err := impl.DeleteServices(ctx, &pb.ServiceQueryRequest{}); return err },
		"CreatePolicy": func() error {
			_, err := impl.CreatePolicy(ctx, &pb.PolicyRequest{ServiceName: "s", Policy: policy})
			return err
		},
		"QueryPoliciesFilter": func() error {
			_, err := impl.QueryPolicies(ctx, &pb.PolicyQueryRequest{ServiceName: "s", Filters: "name eq p"})
			return err
		},
		"CreateRolePolicy": func() error {
			_, err := impl.CreateRolePolicy(ctx, &pb.RolePolicyRequest{ServiceName: "s", RolePolicy: rolePolicy})
			return err
		},
		"QueryRolePoliciesFilter": func() error {
			_, err := impl.QueryRolePolicies(ctx, &pb.RolePolicyQueryRequest{ServiceName: "s", Filters: "name eq rp"})
			return err
		},
		"ListPolicyCounts":    func() error { _, err := impl.ListPolicyCounts(ctx, &pb.Empty{}); return err },
		"GetDiscoverRequests": func() error { _, err := impl.GetDiscoverRequests(ctx, &pb.DiscoverRequestsRequest{}); return err },
		"GetDiscoverRequestsSince": func() error {
			_, err := impl.GetDiscoverRequests(ctx, &pb.DiscoverRequestsRequest{Revision: 3})
			return err
		},
		"ResetDiscoverRequests": func() error { _, err := impl.ResetDiscoverRequests(ctx, &pb.ResetRequestsRequest{}); return err },
		"GetDiscoverPolicies":   func() error { _, err := impl.GetDiscoverPolicies(ctx, &pb.DiscoverPoliciesRequest{}); return err },
	}
	for name, call := range calls {
		if err := call(); code(err) != codes.Internal {
			t.Errorf("%s: error = %v, want Internal", name, err)
		}
	}

	// Limit checks fail first when the counts fail.
	impl = NewServiceImpl(storetest.NewFailingStore())
	if _, err := impl.CreateService(ctx, &pb.ServiceRequest{Name: "s"}); code(err) != codes.Internal {
		t.Errorf("CreateService with failing counts: %v", err)
	}
	if _, err := impl.CreatePolicy(ctx, &pb.PolicyRequest{ServiceName: "s", Policy: policy}); code(err) != codes.Internal {
		t.Errorf("CreatePolicy with failing counts: %v", err)
	}
	if _, err := impl.CreateRolePolicy(ctx, &pb.RolePolicyRequest{ServiceName: "s", RolePolicy: rolePolicy}); code(err) != codes.Internal {
		t.Errorf("CreateRolePolicy with failing counts: %v", err)
	}
}
