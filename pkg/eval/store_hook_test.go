// Licensed under the Universal Permissive License (UPL) Version 1.0.

package eval

import (
	"github.com/teramoby/speedle-plus/api/pms"
	"testing"
)

func TestBackingStoreHook(t *testing.T) {
	type customStore struct{ pms.PolicyStoreManagerADS }
	backing := &customStore{}
	evaluator := &PolicyEvalImpl{Store: backing}
	if evaluator.DistributionStore() != backing {
		t.Fatal("custom store hook did not return the backing store")
	}
	if (&PolicyEvalImpl{}).DistributionStore() != nil {
		t.Fatal("empty evaluator returned a backing store")
	}
}
