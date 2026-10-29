//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package adsgrpc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
)

const testPolicies = `{"services":[{"name":"app","type":"application",
 "policies":[
  {"id":"p1","name":"p1","effect":"grant","principals":[["user:alice"]],"permissions":[{"resource":"res1","actions":["read"]}]},
  {"id":"p2","name":"p2","effect":"grant","principals":[["role:admin"]],"permissions":[{"resource":"res2","actions":["write"]}],"condition":"level == 'high'"}],
 "rolePolicies":[
  {"id":"rp1","name":"rp1","effect":"grant","roles":["admin"],"principals":["user:alice"]}]}]}`

func newService(t *testing.T) *GRPCService {
	t.Helper()
	dir := t.TempDir()
	location := filepath.Join(dir, "policies.json")
	if err := os.WriteFile(location, []byte(testPolicies), 0600); err != nil {
		t.Fatal(err)
	}
	// The file store writes discover requests to the working directory.
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	conf := &cfg.Config{StoreConfig: &cfg.StoreConfig{
		StoreType:  cfg.StorageTypeFile,
		StoreProps: map[string]interface{}{"FileLocation": location},
	}}
	evaluator, err := eval.NewFromConfig(conf)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewGRPCService(evaluator)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func request(user, resource, action string, attributes map[string]string) *pb.ContextRequest {
	return &pb.ContextRequest{
		Subject:     &pb.Subject{Principals: []*pb.Principal{{Type: "user", Name: user}}},
		ServiceName: "app",
		Resource:    resource,
		Action:      action,
		Attributes:  attributes,
	}
}

func TestIsAllowed(t *testing.T) {
	service := newService(t)
	cases := []struct {
		name string
		in   *pb.ContextRequest
		want bool
	}{
		{"direct grant", request("alice", "res1", "read", nil), true},
		{"other action", request("alice", "res1", "write", nil), false},
		{"other user", request("bob", "res1", "read", nil), false},
		{"role grant with condition", request("alice", "res2", "write", map[string]string{"level": "high"}), true},
		{"role grant, condition false", request("alice", "res2", "write", map[string]string{"level": "low"}), false},
		{"no subject", &pb.ContextRequest{ServiceName: "app", Resource: "res1", Action: "read"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := service.IsAllowed(context.Background(), c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got.Allowed != c.want {
				t.Fatalf("allowed = %v, want %v (reason %d)", got.Allowed, c.want, got.Reason)
			}
		})
	}
}

func TestGetAllGrantedRolesAndPermissions(t *testing.T) {
	service := newService(t)
	roles, err := service.GetAllGrantedRoles(context.Background(), request("alice", "", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roles.Roles, []string{"admin"}) {
		t.Fatalf("roles = %v", roles.Roles)
	}

	perms, err := service.GetAllPermissions(context.Background(), request("alice", "", "", map[string]string{"level": "high"}))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range perms.Permissions {
		for _, a := range p.Actions {
			got = append(got, p.Resource+":"+a)
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"res1:read", "res2:write"}) {
		t.Fatalf("permissions = %v", got)
	}
}

func TestDiscover(t *testing.T) {
	service := newService(t)
	got, err := service.Discover(context.Background(), request("carol", "res9", "read", map[string]string{"k": "v"}))
	if err != nil {
		t.Fatal(err)
	}
	// Discover mode allows every request and records it.
	if !got.Allowed {
		t.Fatalf("discover denied the request: %+v", got)
	}
}

func TestDiagnose(t *testing.T) {
	service := newService(t)
	in := request("alice", "res2", "write", map[string]string{"level": "high"})
	got, err := service.Diagnose(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Allowed || got.RequestContext != in {
		t.Fatalf("unexpected response %+v", got)
	}
	if !reflect.DeepEqual(got.GrantedRoles, []string{"admin"}) {
		t.Fatalf("granted roles = %v", got.GrantedRoles)
	}
	if len(got.RolePolicies) != 1 || got.RolePolicies[0].Name != "rp1" {
		t.Fatalf("role policies = %+v", got.RolePolicies)
	}
	var conditioned *pb.EvaluatedPolicy
	for _, p := range got.Policies {
		if p.Name == "p2" {
			conditioned = p
		}
	}
	if conditioned == nil || conditioned.Condition == nil || conditioned.Condition.ConditionExpression != "level == 'high'" {
		t.Fatalf("policy p2 missing or without its condition: %+v", got.Policies)
	}
	if len(conditioned.Principals) != 1 || conditioned.Principals[0] != "role:admin" {
		t.Fatalf("p2 principals = %v", conditioned.Principals)
	}
}

// failingEvaluator fails every evaluation, to cover the error paths.
type failingEvaluator struct {
	eval.InternalEvaluator
}

var errEvaluation = errors.New("evaluation failed")

func (failingEvaluator) AssertToken(*adsapi.RequestContext) error { return nil }
func (failingEvaluator) IsAllowed(adsapi.RequestContext) (bool, adsapi.Reason, error) {
	return false, adsapi.ERROR_IN_EVALUATION, errEvaluation
}
func (failingEvaluator) GetAllGrantedRoles(adsapi.RequestContext) ([]string, error) {
	return nil, errEvaluation
}
func (failingEvaluator) GetAllGrantedPermissions(adsapi.RequestContext) ([]pms.Permission, error) {
	return nil, errEvaluation
}
func (failingEvaluator) Discover(adsapi.RequestContext) (bool, adsapi.Reason, error) {
	return false, adsapi.ERROR_IN_EVALUATION, errEvaluation
}
func (failingEvaluator) Diagnose(adsapi.RequestContext) (*adsapi.EvaluationResult, error) {
	return nil, errEvaluation
}

func TestEvaluationErrors(t *testing.T) {
	service, _ := NewGRPCService(failingEvaluator{})
	in := request("alice", "res1", "read", map[string]string{"a": "b"})
	ctx := context.Background()
	if _, err := service.IsAllowed(ctx, in); err != errEvaluation {
		t.Errorf("IsAllowed error = %v", err)
	}
	if _, err := service.GetAllGrantedRoles(ctx, in); err != errEvaluation {
		t.Errorf("GetAllGrantedRoles error = %v", err)
	}
	if _, err := service.GetAllPermissions(ctx, in); err != errEvaluation {
		t.Errorf("GetAllPermissions error = %v", err)
	}
	if _, err := service.Discover(ctx, in); err != errEvaluation {
		t.Errorf("Discover error = %v", err)
	}
	if _, err := service.Diagnose(ctx, in); err != errEvaluation {
		t.Errorf("Diagnose error = %v", err)
	}
}

func TestConvertAPIRequestContext(t *testing.T) {
	req := &adsapi.RequestContext{
		Subject: &adsapi.Subject{
			Principals: []*adsapi.Principal{{Type: "user", Name: "alice", IDD: "idd"}},
			TokenType:  "bearer",
			Token:      "t",
		},
		ServiceName: "app",
		Resource:    "res",
		Action:      "read",
		Attributes:  map[string]interface{}{"n": 1, "s": "x"},
	}
	got := convertAPIRequestContext(req)
	want := &pb.ContextRequest{
		Subject: &pb.Subject{
			Principals: []*pb.Principal{{Type: "user", Name: "alice", Idd: "idd"}},
			TokenType:  "bearer",
			Token:      "t",
		},
		ServiceName: "app",
		Resource:    "res",
		Action:      "read",
		Attributes:  map[string]string{"n": "1", "s": "x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// And back again.
	back := convertGRPCContextRequest(got)
	if back.Subject.Principals[0].IDD != "idd" || back.Attributes["s"] != "x" {
		t.Fatalf("round trip lost data: %+v", back)
	}
	if convertAPISubject(nil) != nil || convertAPIPrincipals(nil) != nil {
		t.Fatal("nil subject or principals must convert to nil")
	}
	if convertGRPCSubject(nil) != nil || convertGRPCPrincipals(nil) != nil {
		t.Fatal("nil gRPC subject or principals must convert to nil")
	}
	if len(convertAttributes(nil)) != 0 {
		t.Fatal("nil attributes must convert to an empty map")
	}
}

func TestConvertAPIPolicies(t *testing.T) {
	policy := &pms.Policy{
		ID: "id", Name: "p", Effect: "grant", Condition: "a > 1",
		Principals:  [][]string{{"user:alice", "group:g"}},
		Permissions: []*pms.Permission{{Resource: "r", Actions: []string{"read"}}},
	}
	var policyResp pb.Policy
	convertAPIPolicy2PolicyResponse(policy, &policyResp)
	if policyResp.ID != "id" || policyResp.Condition != "a > 1" ||
		!reflect.DeepEqual(policyResp.Principals[0].Principals, []string{"user:alice", "group:g"}) ||
		policyResp.Permissions[0].Resource != "r" {
		t.Fatalf("policy response = %+v", policyResp)
	}
	convertAPIPolicy2PolicyResponse(nil, &policyResp) // must not panic

	rolePolicy := &pms.RolePolicy{ID: "rid", Name: "rp", Effect: "deny", Roles: []string{"r"},
		Principals: []string{"user:bob"}, Resources: []string{"x"}, ResourceExpressions: []string{"y.*"}, Condition: "c"}
	var rolePolicyResp pb.RolePolicy
	convertAPIRolePolicy2RolePolicyResponse(rolePolicy, &rolePolicyResp)
	if rolePolicyResp.ID != "rid" || rolePolicyResp.Effect != "deny" || rolePolicyResp.ResourceExpressions[0] != "y.*" {
		t.Fatalf("role policy response = %+v", rolePolicyResp)
	}
	convertAPIRolePolicy2RolePolicyResponse(nil, &rolePolicyResp)
	convertAPIPolicy2EvaluatedPolicyResponse(nil, nil)
	convertAPIRolePolicy2EvaluatedRolePolicyResponse(nil, nil)
}
