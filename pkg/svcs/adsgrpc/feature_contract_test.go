package adsgrpc

import (
	"context"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"github.com/teramoby/speedle-plus/testutil/featuretest"
	"testing"
)

func TestFeatureBuiltinGRPCContract(t *testing.T) {
	service := newService(t)
	e := service.evaluator.(*eval.PolicyEvalImpl)
	manager := e.Store.(interface{ WritePolicyStore(*pms.PolicyStore) error })
	if err := manager.WritePolicyStore(featuretest.BuiltinStore()); err != nil {
		t.Fatal(err)
	}
	if err := e.Refresh(); err != nil {
		t.Fatal(err)
	}
	for _, c := range featuretest.Builtins {
		t.Run(c.Name, func(t *testing.T) {
			got, err := service.IsAllowed(context.Background(), &pb.ContextRequest{ServiceName: "feature-functions", Resource: c.Name, Action: "read"})
			if err != nil {
				t.Fatal(err)
			}
			reason := int32(ads.NO_APPLICABLE_POLICIES)
			if c.Want {
				reason = int32(ads.GRANT_POLICY_FOUND)
			}
			if got.Allowed != c.Want || got.Reason != reason {
				t.Fatalf("%s: %+v", c.Expression, got)
			}
		})
	}
}
