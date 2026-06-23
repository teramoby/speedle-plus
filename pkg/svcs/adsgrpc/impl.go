//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package adsgrpc

import (
	"context"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"

	"github.com/teramoby/speedle-plus/pkg/logging"
)

// GRPCService is the ADS GRPC implementation
type GRPCService struct {
	evaluator eval.InternalEvaluator
}

// NewGRPCService constructs a new ADS GRPC service instance
func NewGRPCService(evaluator eval.InternalEvaluator) (*GRPCService, error) {

	return &GRPCService{
		evaluator: evaluator,
	}, nil
}

func convertGRPCContextRequest(context *pb.ContextRequest) *adsapi.RequestContext {
	ret := adsapi.RequestContext{
		Subject:     convertGRPCSubject(context.Subject),
		ServiceName: context.ServiceName,
		Resource:    context.Resource,
		Action:      context.Action,
	}

	if context.Attributes == nil {
		return &ret
	}
	ret.Attributes = make(map[string]interface{})
	for k, v := range context.Attributes {
		ret.Attributes[k] = v
	}
	return &ret
}

func convertGRPCPrincipals(principals []*pb.Principal) []*adsapi.Principal {
	if principals == nil {
		return nil
	}

	ret := make([]*adsapi.Principal, 0, len(principals))
	for _, princ := range principals {
		ret = append(ret, &adsapi.Principal{
			Type: princ.Type,
			Name: princ.Name,
			IDD:  princ.Idd,
		})
	}
	return ret
}

func convertGRPCSubject(subject *pb.Subject) *adsapi.Subject {
	if subject == nil {
		return nil
	}
	ret := adsapi.Subject{
		Principals: convertGRPCPrincipals(subject.Principals),
		TokenType:  subject.TokenType,
		Token:      subject.Token,
	}
	return &ret
}

func (impl *GRPCService) IsAllowed(ctx context.Context, in *pb.ContextRequest) (*pb.IsAllowedResponse, error) {
	reqCtx := convertGRPCContextRequest(in)

	allowed, reason, err := impl.evaluator.IsAllowed(*reqCtx)
	if err != nil {
		// Audit log
		logging.WriteSimpleFailedAuditLog("[gRPC]IsAllowed", reqCtx, err.Error())
		return nil, err
	}

	response := pb.IsAllowedResponse{
		Allowed: allowed,
		Reason:  int32(reason),
	}

	// Audit log
	logging.WriteSimpleSucceededAuditLog("[gRPC]IsAllowed", reqCtx, response)

	return &response, nil
}

func (impl *GRPCService) GetAllGrantedRoles(ctx context.Context, in *pb.ContextRequest) (*pb.AllRoleResponse, error) {
	reqCtx := convertGRPCContextRequest(in)

	roles, err := impl.evaluator.GetAllGrantedRoles(*reqCtx)
	if err != nil {
		// Audit log
		logging.WriteSimpleFailedAuditLog("[gRPC]GetAllGrantedRoles", reqCtx, err.Error())
		return nil, err
	}

	// Audit log
	logging.WriteSimpleSucceededAuditLog("[gRPC]GetAllGrantedRoles", reqCtx, roles)

	return &pb.AllRoleResponse{
		Roles: roles,
	}, nil
}

func (impl *GRPCService) GetAllPermissions(ctx context.Context, in *pb.ContextRequest) (*pb.AllPermissionResponse, error) {
	reqCtx := convertGRPCContextRequest(in)

	perms, err := impl.evaluator.GetAllGrantedPermissions(*reqCtx)
	if err != nil {
		// Audit log
		logging.WriteSimpleFailedAuditLog("[gRPC]GetAllGrantedPermissions", reqCtx, err.Error())
		return nil, err
	}

	ret := pb.AllPermissionResponse{
		Permissions: make([]*pb.AllPermissionResponse_Permission, 0, len(perms)),
	}
	for _, perm := range perms {
		ret.Permissions = append(ret.Permissions, &pb.AllPermissionResponse_Permission{
			Resource: perm.Resource,
			Actions:  perm.Actions,
		})
	}

	// Audit log
	logging.WriteSimpleSucceededAuditLog("[gRPC]GetAllGrantedPermissions", reqCtx, ret.Permissions)

	return &ret, nil
}

func (impl *GRPCService) Discover(ctx context.Context, in *pb.ContextRequest) (*pb.IsAllowedResponse, error) {
	reqCtx := convertGRPCContextRequest(in)

	allowed, reason, err := impl.evaluator.Discover(*reqCtx)
	if err != nil {
		// Audit log
		logging.WriteSimpleFailedAuditLog("[gRPC]Discovery", reqCtx, err.Error())
		return nil, err
	}
	// Audit log
	logging.WriteSimpleSucceededAuditLog("[gRPC]Discovery", reqCtx, nil)

	return &pb.IsAllowedResponse{
		Allowed: allowed,
		Reason:  int32(reason),
	}, nil

}

func convertAPIPolicy2EvaluatedPolicyResponse(apiPolicy *adsapi.EvaluatedPolicy, policyResp *pb.EvaluatedPolicy) {
	if apiPolicy == nil || policyResp == nil {
		// It shouldn't happen
		return
	}

	retPermission := make([]*pb.EvaluatedPolicy_Permission, 0, len(apiPolicy.Permissions))
	for _, permission := range apiPolicy.Permissions {
		retPermission = append(retPermission, &pb.EvaluatedPolicy_Permission{
			Resource:           permission.Resource,
			Actions:            permission.Actions,
			ResourceExpression: permission.ResourceExpression,
		})
	}

	policyResp.Status = apiPolicy.Status
	policyResp.ID = apiPolicy.ID
	policyResp.Name = apiPolicy.Name
	policyResp.Effect = apiPolicy.Effect
	policyResp.Permissions = retPermission
	if apiPolicy.Principals != nil && len(apiPolicy.Principals) > 0 {
		policyResp.Principals = apiPolicy.Principals[0]
	}
	if apiPolicy.Condition != nil {
		policyResp.Condition = &pb.EvaluatedCondition{
			ConditionExpression: apiPolicy.Condition.ConditionExpression,
			EvaluationResult:    apiPolicy.Condition.EvaluationResult,
		}
	}
}

func convertAPIRolePolicy2EvaluatedRolePolicyResponse(apiRolePolicy *adsapi.EvaluatedRolePolicy, rolePolicyResp *pb.EvaluatedRolePolicy) {
	if apiRolePolicy == nil || rolePolicyResp == nil {
		// It shouldn't happen
		return
	}

	rolePolicyResp.Status = apiRolePolicy.Status
	rolePolicyResp.ID = apiRolePolicy.ID
	rolePolicyResp.Name = apiRolePolicy.Name
	rolePolicyResp.Effect = apiRolePolicy.Effect
	rolePolicyResp.Roles = apiRolePolicy.Roles
	if apiRolePolicy.Principals != nil && len(apiRolePolicy.Principals) > 0 {
		rolePolicyResp.Principals = apiRolePolicy.Principals
	}
	rolePolicyResp.Resources = apiRolePolicy.Resources
	rolePolicyResp.ResourceExpressions = apiRolePolicy.ResourceExpressions
	if apiRolePolicy.Condition != nil {
		rolePolicyResp.Condition = &pb.EvaluatedCondition{
			ConditionExpression: apiRolePolicy.Condition.ConditionExpression,
			EvaluationResult:    apiRolePolicy.Condition.EvaluationResult,
		}
	}
}

// WARNING: The Diagnose RPC exposes full policy structure including all policy
// definitions, conditions, and evaluation results. In production, restrict
// access to this RPC to admin users only.
func (impl *GRPCService) Diagnose(ctx context.Context, in *pb.ContextRequest) (*pb.EvaluationDebugResponse, error) {
	reqCtx := convertGRPCContextRequest(in)

	evaResult, err := impl.evaluator.Diagnose(*reqCtx)
	if err != nil {
		// Audit log
		logging.WriteSimpleFailedAuditLog("[gRPC]Diagnose", reqCtx, err.Error())
		return nil, err
	}

	// convert all the role policies
	retRolePolicies := make([]*pb.EvaluatedRolePolicy, 0, len(evaResult.RolePolicies))
	for _, rolePolicy := range evaResult.RolePolicies {
		var rolePolicyResp pb.EvaluatedRolePolicy
		convertAPIRolePolicy2EvaluatedRolePolicyResponse(rolePolicy, &rolePolicyResp)
		retRolePolicies = append(retRolePolicies, &rolePolicyResp)
	}

	// convert all the policies
	retPolicies := make([]*pb.EvaluatedPolicy, 0, len(evaResult.Policies))
	for _, policy := range evaResult.Policies {
		var policyResp pb.EvaluatedPolicy
		convertAPIPolicy2EvaluatedPolicyResponse(policy, &policyResp)
		retPolicies = append(retPolicies, &policyResp)
	}

	// Construct & return the response
	response := pb.EvaluationDebugResponse{
		Allowed:        evaResult.Allowed,
		Reason:         evaResult.Reason.String(),
		RequestContext: in,
		GrantedRoles:   evaResult.GrantedRoles,
		RolePolicies:   retRolePolicies,
		Policies:       retPolicies,
	}

	// Audit log
	logging.WriteSimpleSucceededAuditLog("[gRPC]Diagnose", reqCtx, &response)

	return &response, nil
}
