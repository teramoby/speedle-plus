//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

func TestReadFilePolicyStore(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	os.WriteFile(good, []byte(`{"services":[{"name":"app","policies":[{"id":"p1"}]}],"functions":[{"name":"f"}]}`), 0600)
	store, err := ReadFilePolicyStore(good)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Services) != 1 || store.Services[0].Policies[0].ID != "p1" || store.Functions[0].Name != "f" {
		t.Fatalf("store = %+v", store)
	}

	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"services":`), 0600)
	if _, err := ReadFilePolicyStore(bad); errors.Code(err) != errors.SerializationError {
		t.Fatalf("malformed file: %v", err)
	}
	if _, err := ReadFilePolicyStore(filepath.Join(dir, "missing.json")); errors.Code(err) != errors.StoreError {
		t.Fatalf("missing file: %v", err)
	}
}
