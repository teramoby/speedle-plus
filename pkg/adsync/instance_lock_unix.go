// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package adsync

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockInstanceID(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

func unlockInstanceID(file *os.File) {
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
