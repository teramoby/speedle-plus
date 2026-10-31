// Licensed under the Universal Permissive License (UPL) Version 1.0.
package file

import (
	"context"
	"os"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

func (s *Store) CheckHealth(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.Open(s.FileLocation)
	if err != nil {
		return errors.Wrap(err, errors.StoreError, "policy file unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New(errors.StoreError, "policy file is not a readable regular file")
	}
	return nil
}
