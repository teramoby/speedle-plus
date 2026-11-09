// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package adsync

import "os"

// Platforms without Unix ownership metadata rely on the file permission checks.
func cacheDirectoryOwnedByUser(os.FileInfo) bool {
	return true
}
