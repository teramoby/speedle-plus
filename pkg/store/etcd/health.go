// Licensed under the Universal Permissive License (UPL) Version 1.0.
package etcd

import (
	"context"

	"github.com/teramoby/speedle-plus/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func (s *Store) CheckHealth(ctx context.Context) error {
	_, err := s.client.Get(ctx, s.KeyPrefix, clientv3.WithPrefix(), clientv3.WithLimit(1))
	if err != nil {
		return errors.New(errors.StoreError, "etcd policy store unavailable")
	}
	return nil
}
