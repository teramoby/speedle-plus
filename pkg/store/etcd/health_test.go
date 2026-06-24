//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package etcd

import (
	"context"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/store"
)

func TestHealth(t *testing.T) {
	config, err := cfg.ReadStoreConfig("./etcdStoreConfig.json")
	if err != nil {
		t.Fatal("fail to read config file:", err)
	}

	s, err := store.NewStore(config.StoreType, config.StoreProps)
	if err != nil {
		t.Fatal("fail to create etcd store:", err)
	}
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.(*Store).Health(ctx); err != nil {
		t.Errorf("Health check should succeed against embedded etcd, got: %v", err)
	}
}
