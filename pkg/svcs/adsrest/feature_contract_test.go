package adsrest

import (
	"encoding/json"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/testutil/featuretest"
	"testing"
)

func TestFeatureBuiltinRESTContract(t *testing.T) {
	_, manager := newHandlerRouterWithStore(t)
	if err := manager.WritePolicyStore(featuretest.BuiltinStore()); err != nil {
		t.Fatal(err)
	}
	e, err := eval.NewWithStore(&cfg.Config{}, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer e.(*eval.PolicyEvalImpl).Close()
	router, err := NewRouter(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range featuretest.Builtins {
		t.Run(c.Name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{"serviceName": "feature-functions", "resource": c.Name, "action": "read"})
			rec := post(t, router, "is-allowed", string(body), 200)
			var got IsAllowedResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
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
