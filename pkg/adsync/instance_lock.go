// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"
)

func checkInstanceIDLockFile(info os.FileInfo) error {
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return fmt.Errorf("%w: instance id create lock must be a private regular file", ErrUnsafeCache)
	}
	return nil
}

func openInstanceIDCreateLock(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if err := checkInstanceIDLockFile(info); err != nil {
			return nil, fmt.Errorf("adsync: validate create lock %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := openInstanceIDLockFile(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err == nil {
		err = checkInstanceIDLockFile(opened)
	}
	if err == nil {
		var current os.FileInfo
		current, err = os.Lstat(path)
		if err == nil {
			err = checkInstanceIDLockFile(current)
			if err == nil && !os.SameFile(opened, current) {
				err = fmt.Errorf("%w: instance id create lock changed while opening", ErrUnsafeCache)
			}
		}
	}
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("adsync: validate create lock %s: %w", path, err)
	}
	return file, nil
}

var (
	errInstanceIDLockUnsupported = errors.New("exclusive instance id creation locks are unsupported")
	errInstanceIDLockBusy        = errors.New("instance id creation lock is busy")
)

func lockInstanceID(file *os.File) error {
	return waitInstanceIDLock(file, tryLockInstanceID, 5*time.Second)
}

func waitInstanceIDLock(file *os.File, try func(*os.File) error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := try(file)
		if !errors.Is(err, errInstanceIDLockBusy) {
			return err
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("instance id creation lock timed out: %w", os.ErrDeadlineExceeded)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
