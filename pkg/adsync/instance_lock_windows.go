// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryLockInstanceID(file *os.File) error {
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	switch {
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return errInstanceIDLockBusy
	case errors.Is(err, windows.ERROR_NOT_SUPPORTED), errors.Is(err, windows.ERROR_INVALID_FUNCTION):
		return errInstanceIDLockUnsupported
	default:
		return err
	}
}

func unlockInstanceID(file *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}
