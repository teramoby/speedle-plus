package protocompat_test

import (
	ads "github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	pms "github.com/teramoby/speedle-plus/pkg/svcs/pmsgrpc/pb"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"testing"
)

func TestBothServicesShareRegistry(t *testing.T) {
	if ads.Evaluator_ServiceDesc.ServiceName != "pb.Evaluator" || pms.PolicyManager_ServiceDesc.ServiceName != "pb.PolicyManager" {
		t.Fatal("legacy RPC service name changed")
	}
	for _, name := range []string{"pb.Evaluator", "pb.PolicyManager", "speedle.ads.ContextRequest", "speedle.pms.ContextRequest"} {
		if _, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(name)); err != nil {
			t.Fatal(err)
		}
	}
}
