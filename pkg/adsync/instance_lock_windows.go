// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockInstanceID(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}

func unlockInstanceID(file *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}
