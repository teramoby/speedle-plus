//go:build !unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import "os"

func inspectRegularFilePerm(string, func(string)) (os.FileMode, uint32, bool) {
	return 0, 0, false
}

func restoreRegularFilePerm(string, os.FileMode, uint32, func(*os.File, os.FileMode) error, func(string)) error {
	return nil
}
