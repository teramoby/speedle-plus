// Licensed under the Universal Permissive License (UPL) Version 1.0.
package mysql

import "context"

func (s *Store) CheckHealth(ctx context.Context) error {
	var revision int64
	return s.translate(s.db.QueryRowContext(ctx, revisionSQL).Scan(&revision))
}
