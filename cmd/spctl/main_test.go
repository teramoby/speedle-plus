//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"os"
	"testing"
)

func TestPrintVersionInfo(t *testing.T) {
	oldArgs, oldExit := os.Args, exit
	defer func() { os.Args, exit = oldArgs, oldExit }()
	code := -1
	exit = func(c int) { code = c }

	os.Args = []string{"spctl", "get", "service"}
	printVersionInfo()
	if code != -1 {
		t.Fatalf("other commands must not exit, got %d", code)
	}
	os.Args = []string{"spctl", "version"}
	printVersionInfo()
	if code != 0 {
		t.Fatalf("spctl version exited %d, want 0", code)
	}
}
