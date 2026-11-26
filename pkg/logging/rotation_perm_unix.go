//go:build unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	stderrors "errors"
	"os"
	"syscall"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

func sameOwner(uid, expected uint32) bool {
	return uid == expected || int(uid) == os.Geteuid()
}

func openVerifiedRegularFile(path string, afterOpen func(string)) (*os.File, os.FileInfo, uint32, error) {
	flags := syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	fd, err := syscall.Open(path, syscall.O_RDONLY|flags, 0)
	if stderrors.Is(err, os.ErrPermission) {
		fd, err = syscall.Open(path, syscall.O_WRONLY|flags, 0)
	}
	if err != nil {
		return nil, nil, 0, err
	}
	f := os.NewFile(uintptr(fd), path)
	if afterOpen != nil {
		afterOpen(path)
	}
	fdInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, 0, err
	}
	if !fdInfo.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, 0, errors.Errorf(errors.LoggingError, "log path %q is not a regular file", path)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		_ = f.Close()
		return nil, nil, 0, err
	}
	if !pathInfo.Mode().IsRegular() || !os.SameFile(fdInfo, pathInfo) {
		_ = f.Close()
		return nil, nil, 0, errors.Errorf(errors.LoggingError, "log path %q changed during permission check", path)
	}
	stat, ok := fdInfo.Sys().(*syscall.Stat_t)
	if !ok {
		_ = f.Close()
		return nil, nil, 0, errors.Errorf(errors.LoggingError, "missing unix stat metadata for %q", path)
	}
	return f, fdInfo, stat.Uid, nil
}

func inspectRegularFile(path string, afterOpen func(string)) (os.FileInfo, uint32, bool, error) {
	f, info, uid, err := openVerifiedRegularFile(path, afterOpen)
	if err != nil {
		if stderrors.Is(err, os.ErrNotExist) {
			return nil, 0, false, nil
		}
		return nil, 0, false, errors.Wrapf(err, errors.LoggingError, "inspect log file %q permissions", path)
	}
	_ = f.Close()
	return info, uid, true, nil
}

func restoreRegularFilePerm(path string, active os.FileInfo, want os.FileMode, expectedUID uint32, chmodFn func(*os.File, os.FileMode) error, afterOpen func(string)) (os.FileInfo, error) {
	f, info, uid, err := openVerifiedRegularFile(path, afterOpen)
	if err != nil {
		if stderrors.Is(err, os.ErrNotExist) {
			return active, nil
		}
		return active, errors.Wrapf(err, errors.LoggingError, "open log file %q to restore permissions", path)
	}
	defer f.Close()
	if !sameOwner(uid, expectedUID) {
		return active, errors.Errorf(errors.LoggingError, "log file %q owner uid %d does not match expected uid %d", path, uid, expectedUID)
	}
	if active != nil && os.SameFile(active, info) {
		return info, nil
	}
	if info.Mode().Perm() != want {
		if chmodFn == nil {
			chmodFn = (*os.File).Chmod
		}
		if err := chmodFn(f, want); err != nil {
			return active, errors.Wrapf(err, errors.LoggingError, "restore log file %q permissions", path)
		}
		if updated, statErr := f.Stat(); statErr == nil {
			info = updated
		}
	}
	return info, nil
}
