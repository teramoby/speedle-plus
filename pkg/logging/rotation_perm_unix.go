//go:build unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"compress/gzip"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

const (
	lumberjackBackupTimeFormat = "2006-01-02T15-04-05.000"
	lumberjackCompressSuffix   = ".gz"
	lumberjackDefaultMaxMB     = 100
	lumberjackMegabyte         = 1024 * 1024
)

func sameOwner(uid, expected uint32) bool {
	return uid == expected || int(uid) == os.Geteuid()
}

func (w *rotationWriter) initPlatform() {
	_ = w.captureExistingPerm()
}

func (w *rotationWriter) maxBytes() int64 {
	if w.maxSizeBytes > 0 {
		return w.maxSizeBytes
	}
	if w.logger.MaxSize <= 0 {
		return int64(lumberjackDefaultMaxMB) * lumberjackMegabyte
	}
	return int64(w.logger.MaxSize) * lumberjackMegabyte
}

func verifyDescriptor(fd int, path string, afterOpen func(string)) (*os.File, os.FileInfo, uint32, error) {
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

func inspectRegularFile(path string) (os.FileInfo, uint32, bool, error) {
	flags := syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	fd, err := syscall.Open(path, syscall.O_RDONLY|flags, 0)
	if stderrors.Is(err, os.ErrPermission) {
		fd, err = syscall.Open(path, syscall.O_WRONLY|flags, 0)
	}
	if err != nil {
		if stderrors.Is(err, os.ErrNotExist) {
			return nil, 0, false, nil
		}
		return nil, 0, false, errors.Wrapf(err, errors.LoggingError, "inspect log file %q permissions", path)
	}
	f, info, uid, verifyErr := verifyDescriptor(fd, path, nil)
	if verifyErr != nil {
		return nil, 0, false, errors.Wrapf(verifyErr, errors.LoggingError, "inspect log file %q permissions", path)
	}
	_ = f.Close()
	return info, uid, true, nil
}

func (w *rotationWriter) captureExistingPerm() error {
	info, uid, exists, err := inspectRegularFile(w.filename)
	if err != nil {
		return err
	}
	if exists {
		w.active = info
		w.perm = info.Mode().Perm()
		w.uid = uid
		w.uidSet = true
		w.preservePerm = true
	}
	w.checked = true
	return nil
}

func openVerifiedAppendOrCreate(path string, afterOpen func(string)) (*os.File, os.FileInfo, uint32, bool, error) {
	flags := syscall.O_WRONLY | syscall.O_APPEND | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	fd, err := syscall.Open(path, flags, 0)
	created := false
	if stderrors.Is(err, os.ErrNotExist) {
		if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr != nil {
			return nil, nil, 0, false, errors.Wrapf(mkErr, errors.LoggingError, "create directory for log file %q", path)
		}
		fd, err = syscall.Open(path, flags|syscall.O_CREAT|syscall.O_EXCL, 0600)
		if err == nil {
			created = true
		} else if stderrors.Is(err, os.ErrExist) {
			fd, err = syscall.Open(path, flags, 0)
		}
	}
	if err != nil {
		return nil, nil, 0, false, errors.Wrapf(err, errors.LoggingError, "open log file %q", path)
	}
	f, info, uid, verifyErr := verifyDescriptor(fd, path, afterOpen)
	if verifyErr != nil {
		return nil, nil, 0, false, errors.Wrapf(verifyErr, errors.LoggingError, "verify log file %q", path)
	}
	return f, info, uid, created, nil
}

func createVerifiedNewLogFile(path string, afterOpen func(string)) (*os.File, os.FileInfo, error) {
	if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr != nil {
		return nil, nil, errors.Wrapf(mkErr, errors.LoggingError, "create directory for rotated log file %q", path)
	}
	flags := syscall.O_WRONLY | syscall.O_APPEND | syscall.O_CREAT | syscall.O_EXCL | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	fd, err := syscall.Open(path, flags, 0600)
	if err != nil {
		return nil, nil, errors.Wrapf(err, errors.LoggingError, "create rotated log file %q", path)
	}
	f, info, _, verifyErr := verifyDescriptor(fd, path, afterOpen)
	if verifyErr != nil {
		return nil, nil, errors.Wrapf(verifyErr, errors.LoggingError, "verify rotated log file %q", path)
	}
	return f, info, nil
}

func applyDescriptorPerm(f *os.File, path string, want os.FileMode, chmodFn func(*os.File, os.FileMode) error) (os.FileInfo, error) {
	if chmodFn == nil {
		chmodFn = (*os.File).Chmod
	}
	if err := chmodFn(f, want); err != nil {
		return nil, errors.Wrapf(err, errors.LoggingError, "restore log file %q permissions", path)
	}
	updated, err := f.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, errors.LoggingError, "stat log file %q after permission restore", path)
	}
	return updated, nil
}

func (w *rotationWriter) nextBackupName() string {
	dir := filepath.Dir(w.filename)
	base := filepath.Base(w.filename)
	ext := filepath.Ext(base)
	prefix := base[:len(base)-len(ext)] + "-"
	for {
		now := time.Now()
		if !w.logger.LocalTime {
			now = now.UTC()
		}
		candidate := filepath.Join(dir, prefix+now.Format(lumberjackBackupTimeFormat)+ext)
		_, err := os.Lstat(candidate)
		_, gzErr := os.Lstat(candidate + lumberjackCompressSuffix)
		if stderrors.Is(err, os.ErrNotExist) && stderrors.Is(gzErr, os.ErrNotExist) {
			return candidate
		}
		time.Sleep(time.Millisecond)
	}
}

func (w *rotationWriter) rotateVerifiedFile(prevInfo os.FileInfo) (*os.File, os.FileInfo, error) {
	curInfo, curUID, exists, err := inspectRegularFile(w.filename)
	if err != nil {
		return nil, nil, err
	}
	if exists {
		if !os.SameFile(prevInfo, curInfo) {
			return nil, nil, errors.Errorf(errors.LoggingError, "log path %q changed before rotation", w.filename)
		}
		if w.uidSet && !sameOwner(curUID, w.uid) {
			return nil, nil, errors.Errorf(errors.LoggingError, "log file %q owner uid %d does not match expected uid %d", w.filename, curUID, w.uid)
		}
		if err := os.Rename(w.filename, w.nextBackupName()); err != nil {
			return nil, nil, errors.Wrapf(err, errors.LoggingError, "rename rotated log file %q", w.filename)
		}
	}
	f, newInfo, err := createVerifiedNewLogFile(w.filename, nil)
	if err != nil {
		return nil, nil, err
	}
	if w.preservePerm && newInfo.Mode().Perm() != w.perm {
		updated, chmodErr := applyDescriptorPerm(f, w.filename, w.perm, w.chmod)
		if chmodErr != nil {
			_ = f.Close()
			return nil, nil, chmodErr
		}
		newInfo = updated
	}
	w.active = newInfo
	w.maintainBackups()
	return f, newInfo, nil
}

type backupEntry struct {
	timestamp time.Time
	name      string
}

func (w *rotationWriter) maintainBackups() {
	if w.logger.MaxBackups <= 0 && w.logger.MaxAge <= 0 && !w.logger.Compress {
		return
	}
	dir := filepath.Dir(w.filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	base := filepath.Base(w.filename)
	ext := filepath.Ext(base)
	prefix := base[:len(base)-len(ext)] + "-"

	var backups []backupEntry
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if ts, ok := parseBackupTime(name, prefix, ext); ok {
			backups = append(backups, backupEntry{timestamp: ts, name: name})
		} else if ts, ok := parseBackupTime(name, prefix, ext+lumberjackCompressSuffix); ok {
			backups = append(backups, backupEntry{timestamp: ts, name: name})
		}
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].timestamp.After(backups[j].timestamp)
	})

	var remove, compress []backupEntry
	if w.logger.MaxBackups > 0 && w.logger.MaxBackups < len(backups) {
		preserved := make(map[string]bool)
		var remaining []backupEntry
		for _, b := range backups {
			key := strings.TrimSuffix(b.name, lumberjackCompressSuffix)
			preserved[key] = true
			if len(preserved) > w.logger.MaxBackups {
				remove = append(remove, b)
			} else {
				remaining = append(remaining, b)
			}
		}
		backups = remaining
	}
	if w.logger.MaxAge > 0 {
		cutoff := time.Now().Add(-time.Duration(int64(24*time.Hour) * int64(w.logger.MaxAge)))
		var remaining []backupEntry
		for _, b := range backups {
			if b.timestamp.Before(cutoff) {
				remove = append(remove, b)
			} else {
				remaining = append(remaining, b)
			}
		}
		backups = remaining
	}
	if w.logger.Compress {
		for _, b := range backups {
			if !strings.HasSuffix(b.name, lumberjackCompressSuffix) {
				compress = append(compress, b)
			}
		}
	}
	for _, b := range remove {
		_ = os.Remove(filepath.Join(dir, b.name))
	}
	for _, b := range compress {
		src := filepath.Join(dir, b.name)
		_ = compressBackupRegularFile(src, src+lumberjackCompressSuffix)
	}
}

func parseBackupTime(filename, prefix, ext string) (time.Time, bool) {
	if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, ext) || len(filename) < len(prefix)+len(ext) {
		return time.Time{}, false
	}
	ts, err := time.Parse(lumberjackBackupTimeFormat, filename[len(prefix):len(filename)-len(ext)])
	return ts, err == nil
}

func compressBackupRegularFile(src, dst string) error {
	flags := syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	srcFD, err := syscall.Open(src, syscall.O_RDONLY|flags, 0)
	if err != nil {
		return err
	}
	srcFile, srcInfo, _, verifyErr := verifyDescriptor(srcFD, src, nil)
	if verifyErr != nil {
		return verifyErr
	}
	defer srcFile.Close()

	dstFD, err := syscall.Open(dst, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|flags, 0600)
	if err != nil {
		return err
	}
	dstFile, _, _, dstVerifyErr := verifyDescriptor(dstFD, dst, nil)
	if dstVerifyErr != nil {
		return dstVerifyErr
	}
	success := false
	defer func() {
		_ = dstFile.Close()
		if !success {
			_ = os.Remove(dst)
		}
	}()

	if err := dstFile.Chmod(srcInfo.Mode().Perm()); err != nil {
		return err
	}
	gz := gzip.NewWriter(dstFile)
	if _, err := io.Copy(gz, srcFile); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := dstFile.Sync(); err != nil {
		return err
	}
	success = true
	if linfo, err := os.Lstat(src); err == nil && linfo.Mode().IsRegular() && os.SameFile(srcInfo, linfo) {
		return os.Remove(src)
	}
	return nil
}

func (w *rotationWriter) writePlatform(p []byte) (int, error) {
	maxBytes := w.maxBytes()
	if int64(len(p)) > maxBytes {
		return 0, errors.Errorf(errors.LoggingError, "write length %d exceeds maximum file size %d", len(p), maxBytes)
	}
	if !w.checked {
		if err := w.captureExistingPerm(); err != nil {
			return 0, err
		}
	}
	f, info, uid, created, err := openVerifiedAppendOrCreate(w.filename, w.afterOpen)
	if err != nil {
		return 0, err
	}
	if w.uidSet && !sameOwner(uid, w.uid) {
		_ = f.Close()
		return 0, errors.Errorf(errors.LoggingError, "log file %q owner uid %d does not match expected uid %d", w.filename, uid, w.uid)
	}
	if !w.uidSet {
		w.uid = uid
		w.uidSet = true
	}
	if w.preservePerm {
		if !created && w.active != nil && os.SameFile(w.active, info) {
			w.active = info
			w.perm = info.Mode().Perm()
		} else {
			if info.Mode().Perm() != w.perm {
				updated, chmodErr := applyDescriptorPerm(f, w.filename, w.perm, w.chmod)
				if chmodErr != nil {
					_ = f.Close()
					return 0, chmodErr
				}
				info = updated
			}
			w.active = info
		}
	} else {
		w.active = info
	}

	if info.Size()+int64(len(p)) > maxBytes {
		_ = f.Close()
		_ = w.logger.Close()
		rotated, rotatedInfo, rotErr := w.rotateVerifiedFile(info)
		if rotErr != nil {
			return 0, rotErr
		}
		f = rotated
		info = rotatedInfo
	}

	n, writeErr := f.Write(p)
	if writeErr == nil {
		if updated, statErr := f.Stat(); statErr == nil {
			w.active = updated
		} else {
			w.active = info
		}
	}
	closeErr := f.Close()
	if writeErr != nil {
		return n, writeErr
	}
	return n, closeErr
}
