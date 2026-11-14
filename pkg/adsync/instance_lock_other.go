// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package adsync

import "os"

func openInstanceIDLockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}

func tryLockInstanceID(*os.File) error {
	return errInstanceIDLockUnsupported
}

func unlockInstanceID(*os.File) {}
