//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package svcs

import (
	"fmt"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/svcs/pmsgrpc/pb"
)

// ConvertAttributes converts an attributes map from interface{} values to string values.
func ConvertAttributes(in map[string]interface{}) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// ConvertAPIPrincipals converts API principals to protobuf principals.
func ConvertAPIPrincipals(principals []*adsapi.Principal) []*pb.Principal {
	if principals == nil {
		return nil
	}

	ret := make([]*pb.Principal, 0, len(principals))
	for _, princ := range principals {
		ret = append(ret, &pb.Principal{
			Type: princ.Type,
			Name: princ.Name,
			Idd:  princ.IDD,
		})
	}
	return ret
}

// ConvertAPISubject converts an API subject to a protobuf subject.
func ConvertAPISubject(subject *adsapi.Subject) *pb.Subject {
	if subject == nil {
		return nil
	}
	return &pb.Subject{
		Principals: ConvertAPIPrincipals(subject.Principals),
		Token:      subject.Token,
		TokenType:  subject.TokenType,
	}
}

// ConvertAPIRequestContext converts an API request context to a protobuf context request.
func ConvertAPIRequestContext(req *adsapi.RequestContext) *pb.ContextRequest {
	return &pb.ContextRequest{
		Subject:     ConvertAPISubject(req.Subject),
		ServiceName: req.ServiceName,
		Resource:    req.Resource,
		Action:      req.Action,
		Attributes:  ConvertAttributes(req.Attributes),
	}
}
