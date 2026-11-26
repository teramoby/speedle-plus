//go:build !unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import "os"

func sameOwner(uint32, uint32) bool {
	return false
}

func inspectRegularFile(string, func(string)) (os.FileInfo, uint32, bool, error) {
	return nil, 0, false, nil
}

func restoreRegularFilePerm(path string, active os.FileInfo, want os.FileMode, expectedUID uint32, chmodFn func(*os.File, os.FileMode) error, afterOpen func(string)) (os.FileInfo, error) {
	return active, nil
}
