// Licensed under the Universal Permissive License (UPL) Version 1.0.
package adsgrpc

import (
	"context"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"github.com/teramoby/speedle-plus/pkg/svcs/gate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (impl *GRPCService) dispatch(ctx context.Context, op gate.Operation, request *ads.RequestContext) (*gate.Response, bool, error) {
	response, handled, err := gate.Dispatch(ctx, impl.gate, op, request)
	if !handled {
		return nil, false, nil
	}
	fields := map[string]interface{}{"serviceName": request.ServiceName, "decisionSource": "upstream"}
	if err != nil {
		logging.WriteSimpleFailedAuditLog("[gRPC]"+string(op), fields, gate.Reason(err))
		return nil, true, status.Error(codes.Unavailable, gate.Reason(err))
	}
	if response.ErrorMessage != "" {
		logging.WriteSimpleFailedAuditLog("[gRPC]"+string(op), fields, response.ErrorMessage)
		return nil, true, status.Error(codes.Internal, response.ErrorMessage)
	}
	_ = grpc.SetHeader(ctx, metadata.Pairs("x-speedle-decision-source", "upstream"))
	logging.WriteSimpleSucceededAuditLog("[gRPC]"+string(op), fields, map[string]interface{}{"allowed": response.Allowed, "reason": response.Reason, "roles": response.Roles, "permissions": response.Permissions})
	return response, true, nil
}

func decisionResponse(response *gate.Response) *pb.IsAllowedResponse {
	return &pb.IsAllowedResponse{Allowed: response.Allowed, Reason: int32(response.Reason)}
}
func rolesResponse(response *gate.Response) *pb.AllRoleResponse {
	roles := response.Roles
	if roles == nil {
		roles = []string{}
	}
	return &pb.AllRoleResponse{Roles: roles}
}
func permissionsResponse(response *gate.Response) *pb.AllPermissionResponse {
	result := &pb.AllPermissionResponse{Permissions: make([]*pb.AllPermissionResponse_Permission, 0, len(response.Permissions))}
	for _, permission := range response.Permissions {
		result.Permissions = append(result.Permissions, &pb.AllPermissionResponse_Permission{Resource: permission.Resource, Actions: permission.Actions})
	}
	return result
}
