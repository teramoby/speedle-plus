// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package adsync

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openInstanceIDLockFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
}

func tryLockInstanceID(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	switch {
	case errors.Is(err, unix.EWOULDBLOCK), errors.Is(err, unix.EINTR):
		return errInstanceIDLockBusy
	case errors.Is(err, unix.ENOSYS), errors.Is(err, unix.ENOTSUP), errors.Is(err, unix.ENOLCK):
		return errInstanceIDLockUnsupported
	default:
		return err
	}
}

func unlockInstanceID(file *os.File) {
	_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
