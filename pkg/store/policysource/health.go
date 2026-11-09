// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"context"

	"github.com/teramoby/speedle-plus/pkg/adsync"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// CheckHealth reports whether the instance may still evaluate policies
// locally. Only a stopped instance fails the check: its local copies may be
// arbitrarily stale, so authorization must fail closed. A stale instance and a
// syncing service keep the check healthy because the request gate, not the
// evaluator, decides how to route them.
func (s *Store) CheckHealth(ctx context.Context) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	instance := s.machine.Instance()
	if instance.Liveness != adsync.LivenessStopped {
		return nil
	}
	reason := instance.Reason
	if reason == "" {
		reason = adsync.ReasonHeartbeatLost
	}
	return errors.Errorf(errors.StoreError, "policy source is stopped: %s", reason)
}
