//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package errors

import (
	goerrors "errors"
	"fmt"
	"os"
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

func TestStandardErrorChain(t *testing.T) {
	root := &os.PathError{Op: "open", Path: "policy.json", Err: os.ErrNotExist}
	wrapped := Wrapf(root, StoreError, "read %s", "snapshot")
	outer := fmt.Errorf("request failed: %w", wrapped)
	if outer.Error() != "request failed: SPDL-1001 read snapshot: open policy.json: file does not exist" {
		t.Fatalf("error text changed: %q", outer)
	}
	if !goerrors.Is(outer, root) || !goerrors.Is(outer, os.ErrNotExist) {
		t.Fatal("errors.Is lost the cause")
	}
	var pathErr *os.PathError
	if !goerrors.As(outer, &pathErr) || pathErr != root {
		t.Fatal("errors.As lost the typed cause")
	}
	var coded *speedleError
	if !goerrors.As(outer, &coded) || coded.Code() != StoreError {
		t.Fatal("errors.As lost the Speedle code")
	}
	if goerrors.Unwrap(wrapped) != root {
		t.Fatal("Unwrap lost the direct cause")
	}
	if Cause(outer) != os.ErrNotExist {
		t.Fatalf("Cause did not traverse standard wrappers: %v", Cause(outer))
	}
	if Code(outer) != UnknownError {
		t.Fatal("Code's direct error semantics changed")
	}
}

type legacyCause struct{ cause error }

func (e legacyCause) Error() string { return "legacy" }
func (e legacyCause) Cause() error  { return e.cause }

func TestCauseCompatibility(t *testing.T) {
	root := goerrors.New("disk full")
	for _, err := range []error{root, Wrap(Wrap(root, StoreError, "inner"), ConfigError, "outer"), legacyCause{root}, fmt.Errorf("outer: %w", legacyCause{root})} {
		if Cause(err) != root {
			t.Fatalf("Cause(%v) = %v", err, Cause(err))
		}
	}
	for _, err := range []error{nil, New(StoreError, "no cause"), legacyCause{}} {
		if Cause(err) != nil {
			t.Fatalf("nil cause behavior changed: %v", Cause(err))
		}
	}
	if goerrors.Unwrap(New(StoreError, "no cause")) != nil {
		t.Fatal("new error has a cause")
	}
	if goerrors.Is(Wrap(root, StoreError, "write"), goerrors.New("disk full")) {
		t.Fatal("unrelated errors matched")
	}
	var pathErr *os.PathError
	if goerrors.As(Wrap(root, StoreError, "write"), &pathErr) {
		t.Fatal("unrelated type matched")
	}
}
