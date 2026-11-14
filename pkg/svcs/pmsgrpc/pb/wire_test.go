package pb

import (
	"github.com/teramoby/speedle-plus/testutil/protocompat"
	"google.golang.org/protobuf/proto"
	"testing"
)

// Fixtures were captured from the pre-migration github.com/golang/protobuf bindings.
func TestLegacyWireFormat(t *testing.T) {
	protocompat.CheckLegacy(t, "testdata/legacy-wire.json", []proto.Message{
		&DiscoverRequestsRequest{},
		&Principal{},
		&Subject{},
		&ContextRequest{},
		&DiscoverRequestsResponse{},
		&ResetRequestsRequest{},
		&ResetRequestsResponse{},
		&DiscoverPoliciesRequest{},
		&DiscoverPoliciesResponse{},
		&Function{},
		&FunctionQueryRequest{},
		&FunctionQueryResponse{},
		&AndPrincipals{},
		&Empty{},
		&ServiceRequest{},
		&PolicyRequest{},
		&ServiceQueryResponse{},
		&ServiceQueryRequest{},
		&PolicyQueryRequest{},
		&PolicyQueryResponse{},
		&Policy{},
		&Policy_Permission{},
		&RolePolicyRequest{},
		&RolePolicyQueryRequest{},
		&RolePolicyQueryResponse{},
		&RolePolicy{},
		&Service{},
		&PolicyAndRolePolicyCounts{},
		&PolicyCountsMap{},
	})
}
