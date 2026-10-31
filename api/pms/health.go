// Licensed under the Universal Permissive License (UPL) Version 1.0.
package pms

import "context"

// StoreHealthChecker checks whether the authoritative store can still be read.
// Built-in stores implement this optional interface so cached decisions do not
// silently outlive a failed watch or an unavailable store. Callers bound ctx.
type StoreHealthChecker interface {
	CheckHealth(ctx context.Context) error
}
