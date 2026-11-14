package pb

import (
	"github.com/teramoby/speedle-plus/testutil/protocompat"
	"google.golang.org/protobuf/proto"
	"testing"
)

// Fixtures were captured from the pre-migration github.com/golang/protobuf bindings.
func TestLegacyWireFormat(t *testing.T) {
	protocompat.CheckLegacy(t, "testdata/legacy-wire.json", []proto.Message{
		&Principal{},
		&Subject{},
		&ContextRequest{},
		&IsAllowedResponse{},
		&AndPrincipals{},
		&RolePolicy{},
		&Policy{},
		&Policy_Permission{},
		&EvaluatedCondition{},
		&EvaluatedRolePolicy{},
		&EvaluatedPolicy{},
		&EvaluatedPolicy_Permission{},
		&EvaluationDebugResponse{},
		&AllRoleResponse{},
		&AllPermissionResponse{},
		&AllPermissionResponse_Permission{},
	})
}
