// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsgrpc

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgate"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// route returns the routing decision for one service. A service without a
// distribution gate keeps the existing local behaviour.
func (impl *GRPCService) route(serviceName string) adsgate.Route {
	if impl.gate == nil {
		return adsgate.Route{Kind: adsgate.RouteLocal}
	}
	return impl.gate.Route(serviceName)
}

// unavailableError reports a fail-closed state with the reason code from the
// distribution protocol. The caller must treat any non-OK status as deny.
func unavailableError(reason adsync.Reason) error {
	return status.Error(codes.Unavailable, string(reason))
}

// fallbackContextRequest is the REST body Cloud accepts for a forwarded gRPC
// request. Cloud serves only the REST authorization endpoint.
type fallbackContextRequest struct {
	Subject     *fallbackSubject    `json:"subject,omitempty"`
	ServiceName string              `json:"serviceName"`
	Resource    string              `json:"resource"`
	Action      string              `json:"action"`
	Attributes  []fallbackAttribute `json:"attributes,omitempty"`
}

type fallbackSubject struct {
	Principals []*fallbackPrincipal `json:"principals,omitempty"`
	TokenType  string               `json:"tokenType,omitempty"`
	Token      string               `json:"token,omitempty"`
}

type fallbackPrincipal struct {
	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`
	IDD  string `json:"idd,omitempty"`
}

type fallbackAttribute struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

func fallbackRequestBody(in *pb.ContextRequest) ([]byte, error) {
	request := fallbackContextRequest{
		ServiceName: in.GetServiceName(),
		Resource:    in.GetResource(),
		Action:      in.GetAction(),
	}
	if subject := in.GetSubject(); subject != nil {
		converted := &fallbackSubject{
			TokenType: subject.GetTokenType(),
			Token:     subject.GetToken(),
		}
		for _, principal := range subject.GetPrincipals() {
			converted.Principals = append(converted.Principals, &fallbackPrincipal{
				Type: principal.GetType(),
				Name: principal.GetName(),
				IDD:  principal.GetIdd(),
			})
		}
		request.Subject = converted
	}
	names := make([]string, 0, len(in.GetAttributes()))
	for name := range in.GetAttributes() {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		request.Attributes = append(request.Attributes, fallbackAttribute{
			Name:  name,
			Value: in.GetAttributes()[name],
		})
	}
	return json.Marshal(request)
}

// forward sends one authorization request to Cloud and marks the response as a
// fallback decision. Cloud answers with the same JSON the REST endpoint uses.
func (impl *GRPCService) forward(ctx context.Context, operation adsync.FallbackOperation, in *pb.ContextRequest) (*adsync.FallbackResponse, error) {
	body, err := fallbackRequestBody(in)
	if err != nil {
		return nil, status.Error(codes.Internal, "unable to build the fallback request")
	}
	response, err := impl.gate.Forward(ctx, operation, body)
	if err != nil {
		return nil, unavailableError(adsgate.UnavailableReason(err))
	}
	// SetHeader is best effort: a direct in-process call has no server
	// transport stream to carry the metadata.
	source := svcs.DecisionSourceCloudFallback
	if response.DecisionSource == adsync.DecisionSourceUpstream {
		source = svcs.DecisionSourceUpstream
	}
	_ = grpc.SetHeader(ctx, metadata.Pairs(strings.ToLower(svcs.DecisionSourceHeader), source))
	return response, nil
}

// cloudDecision is the JSON body of Cloud's is-allowed response.
type cloudDecision struct {
	Allowed      bool   `json:"allowed"`
	Reason       int32  `json:"reason"`
	ErrorMessage string `json:"errorMessage"`
}

func (impl *GRPCService) fallbackIsAllowed(ctx context.Context, in *pb.ContextRequest) (*pb.IsAllowedResponse, error) {
	auditAction := "[gRPC]" + adsgate.FallbackAuditOperation(adsync.FallbackIsAllowed)
	auditFields := adsgate.FallbackAuditFields(adsync.FallbackIsAllowed, in.GetServiceName())
	response, err := impl.forward(ctx, adsync.FallbackIsAllowed, in)
	if err != nil {
		logging.WriteSimpleFailedAuditLog(auditAction, auditFields, err.Error())
		return nil, err
	}
	var decision cloudDecision
	if err := adsync.ReadFallbackResponse(response.Body, &decision); err != nil {
		logging.WriteSimpleFailedAuditLog(auditAction, auditFields, "invalid fallback response: "+err.Error())
		return nil, status.Error(codes.Internal, "invalid fallback response")
	}
	if decision.ErrorMessage != "" {
		// The local evaluator reports an evaluation error as an error, never as
		// a normal decision; a forwarded call must do the same.
		logging.WriteSimpleFailedAuditLog(auditAction, auditFields, decision.ErrorMessage)
		return nil, status.Error(codes.Internal, decision.ErrorMessage)
	}
	result := &pb.IsAllowedResponse{
		Allowed: decision.Allowed,
		Reason:  decision.Reason,
	}
	logging.WriteSimpleSucceededAuditLog(auditAction, auditFields,
		map[string]interface{}{"allowed": decision.Allowed, "reason": decision.Reason})
	return result, nil
}

func (impl *GRPCService) fallbackAllGrantedRoles(ctx context.Context, in *pb.ContextRequest) (*pb.AllRoleResponse, error) {
	auditAction := "[gRPC]" + adsgate.FallbackAuditOperation(adsync.FallbackAllGrantedRoles)
	auditFields := adsgate.FallbackAuditFields(adsync.FallbackAllGrantedRoles, in.GetServiceName())
	response, err := impl.forward(ctx, adsync.FallbackAllGrantedRoles, in)
	if err != nil {
		logging.WriteSimpleFailedAuditLog(auditAction, auditFields, err.Error())
		return nil, err
	}
	var roles []string
	if err := adsync.ReadFallbackResponse(response.Body, &roles); err != nil {
		logging.WriteSimpleFailedAuditLog(auditAction, auditFields, "invalid fallback response: "+err.Error())
		return nil, status.Error(codes.Internal, "invalid fallback response")
	}
	if roles == nil {
		roles = []string{}
	}
	result := &pb.AllRoleResponse{Roles: roles}
	logging.WriteSimpleSucceededAuditLog(auditAction, auditFields,
		map[string]interface{}{"roles": roles})
	return result, nil
}

func (impl *GRPCService) fallbackAllGrantedPermissions(ctx context.Context, in *pb.ContextRequest) (*pb.AllPermissionResponse, error) {
	auditAction := "[gRPC]" + adsgate.FallbackAuditOperation(adsync.FallbackAllGrantedPermissions)
	auditFields := adsgate.FallbackAuditFields(adsync.FallbackAllGrantedPermissions, in.GetServiceName())
	response, err := impl.forward(ctx, adsync.FallbackAllGrantedPermissions, in)
	if err != nil {
		logging.WriteSimpleFailedAuditLog(auditAction, auditFields, err.Error())
		return nil, err
	}
	var permissions []struct {
		Resource string   `json:"resource"`
		Actions  []string `json:"actions"`
	}
	if err := adsync.ReadFallbackResponse(response.Body, &permissions); err != nil {
		logging.WriteSimpleFailedAuditLog(auditAction, auditFields, "invalid fallback response: "+err.Error())
		return nil, status.Error(codes.Internal, "invalid fallback response")
	}
	ret := pb.AllPermissionResponse{
		Permissions: make([]*pb.AllPermissionResponse_Permission, 0, len(permissions)),
	}
	for _, permission := range permissions {
		ret.Permissions = append(ret.Permissions, &pb.AllPermissionResponse_Permission{
			Resource: permission.Resource,
			Actions:  permission.Actions,
		})
	}
	logging.WriteSimpleSucceededAuditLog(auditAction, auditFields,
		map[string]interface{}{"permissions": ret.Permissions})
	return &ret, nil
}
