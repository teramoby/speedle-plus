//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import (
	"path/filepath"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/testutil/storetest"
)

func newTempStore(t *testing.T) pms.PolicyStoreManager {
	t.Helper()
	s, err := store.NewStore(StoreType, map[string]interface{}{FileLocationKey: filepath.Join(t.TempDir(), "policies.json")})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestConformance(t *testing.T) {
	storetest.RunConformance(t, newTempStore)
}
