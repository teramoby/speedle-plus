//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package errors

import (
	goerrors "errors"
	"testing"
)

func TestNewAndErrorf(t *testing.T) {
	err := New(StoreError, "store down")
	if err.Error() != "SPDL-1001 store down" || Code(err) != StoreError {
		t.Fatalf("New: %q %s", err, Code(err))
	}
	err = Errorf(EntityNotFound, "service %q", "app")
	if err.Error() != `SPDL-1002 service "app"` || Code(err) != EntityNotFound {
		t.Fatalf("Errorf: %q", err)
	}
}

func TestWrap(t *testing.T) {
	if Wrap(nil, StoreError, "x") != nil || Wrapf(nil, StoreError, "%d", 1) != nil {
		t.Fatal("wrapping nil must give nil")
	}
	root := goerrors.New("disk full")
	err := Wrapf(root, StoreError, "write %s", "a.json")
	if err.Error() != "SPDL-1001 write a.json: disk full" {
		t.Fatalf("Wrapf: %q", err)
	}
	if Cause(err) != root || Code(err) != StoreError {
		t.Fatalf("cause %v, code %s", Cause(err), Code(err))
	}
	if Code(root) != UnknownError {
		t.Fatalf("a plain error has code %s", Code(root))
	}
}
