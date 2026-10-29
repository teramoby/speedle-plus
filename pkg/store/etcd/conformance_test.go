//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package etcd

import (
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

// newPrefixedStore returns a store on the TestMain etcd under a key prefix
// of its own, so it starts empty.
func newPrefixedStore(t *testing.T) pms.PolicyStoreManager {
	t.Helper()
	s, err := store.NewStore(StoreType, map[string]interface{}{
		EtcdEndpointKey:  storeConfig.StoreProps[EtcdEndpointKey],
		EtcdKeyPrefixKey: "/conformance/" + strings.ReplaceAll(t.Name(), "/", "_") + "/",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.(*Store).destroy() })
	return s
}

func TestConformance(t *testing.T) {
	storetest.RunConformance(t, newPrefixedStore)
}
