//go:build unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"os"
	"syscall"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

func openVerifiedRegularFile(path string, afterOpen func(string)) (*os.File, os.FileInfo, uint32, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
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

func inspectRegularFilePerm(path string, afterOpen func(string)) (os.FileMode, uint32, bool) {
	f, info, uid, err := openVerifiedRegularFile(path, afterOpen)
	if err != nil {
		return 0, 0, false
	}
	_ = f.Close()
	return info.Mode().Perm(), uid, true
}

func restoreRegularFilePerm(path string, want os.FileMode, expectedUID uint32, chmodFn func(*os.File, os.FileMode) error, afterOpen func(string)) error {
	f, info, uid, err := openVerifiedRegularFile(path, afterOpen)
	if err != nil {
		return errors.Wrapf(err, errors.LoggingError, "open log file %q to restore permissions", path)
	}
	defer f.Close()
	if uid != expectedUID && int(uid) != os.Geteuid() {
		return errors.Errorf(errors.LoggingError, "log file %q owner uid %d does not match expected uid %d", path, uid, expectedUID)
	}
	if info.Mode().Perm() == want {
		return nil
	}
	if chmodFn == nil {
		chmodFn = (*os.File).Chmod
	}
	if err := chmodFn(f, want); err != nil {
		return errors.Wrapf(err, errors.LoggingError, "restore log file %q permissions", path)
	}
	return nil
}
