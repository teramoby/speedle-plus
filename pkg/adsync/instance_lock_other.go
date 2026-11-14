// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package adsync

import "os"

func tryLockInstanceID(*os.File) error {
	return errInstanceIDLockUnsupported
}

func unlockInstanceID(*os.File) {}
