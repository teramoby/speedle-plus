package eval

import "context"

// Health reports backend availability for optional container readiness probes.
// It does not reload the runtime cache or change authorization decisions.
func (p *PolicyEvalImpl) Health(ctx context.Context) error {
	return p.Store.Health(ctx)
}
