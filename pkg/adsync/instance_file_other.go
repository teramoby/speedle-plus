// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build !windows

package adsync

import "os"

func openInstanceIDPublication(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}
