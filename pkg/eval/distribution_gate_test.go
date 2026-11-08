// Licensed under the Universal Permissive License (UPL) Version 1.0.

package eval

import (
	"context"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/adsync"
)

// gateStore implements only the distribution view; the embedded nil interface
// supplies the rest of pms.PolicyStoreManagerADS, which DistributionGate never
// calls.
type gateStore struct {
	pms.PolicyStoreManagerADS
}

func (gateStore) Status(string) (adsync.Status, bool) { return adsync.Status{}, false }
func (gateStore) Instance() adsync.InstanceStatus     { return adsync.InstanceStatus{} }
func (gateStore) Services() []adsync.ServiceStatus    { return nil }
func (gateStore) FallbackAvailable() bool             { return true }
func (gateStore) ForwardFallback(context.Context, adsync.FallbackOperation, []byte) (*adsync.FallbackResponse, error) {
	return nil, adsync.ErrFallbackUnavailable
}

func TestDistributionGateComesFromTheStore(t *testing.T) {
	evaluator := &PolicyEvalImpl{Store: gateStore{}}
	if evaluator.DistributionGate() == nil {
		t.Fatal("a distributing store did not supply a request gate")
	}

	plain := &PolicyEvalImpl{}
	if plain.DistributionGate() != nil {
		t.Fatal("an evaluator without a store supplied a request gate")
	}

	// A store that cannot distribute policy has no gate, and the entry points
	// keep their existing behaviour.
	nonDistributing := &PolicyEvalImpl{Store: nilPolicyStore{}}
	if nonDistributing.DistributionGate() != nil {
		t.Fatal("a non-distributing store supplied a request gate")
	}
}

// nilPolicyStore implements no distribution methods.
type nilPolicyStore struct {
	pms.PolicyStoreManagerADS
}
