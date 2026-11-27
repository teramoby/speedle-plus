//go:build unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"compress/gzip"
	"crypto/rand"
	stderrors "errors"
	"fmt"
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
	maxRotationRetryAttempts   = 8
	maxBackupNameAttempts      = 2000
	compressTempSuffix         = ".tmp"
)

var errConcurrentRotation = stderrors.New("concurrent log rotation")

func sameOwner(uid, expected uint32) bool {
	return uid == expected || int(uid) == os.Geteuid()
}

func compressTempPrefix(base string) string {
	return ".speedle-log-compress-" + base + "-"
}

func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

func (w *rotationWriter) initPlatform() {
	_ = w.captureExistingPerm()
}

func (w *rotationWriter) closePlatform() error {
	return w.logger.Close()
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

func verifyDescriptor(fd int, path string, afterOpen func(string)) (*os.File, os.FileInfo, *syscall.Stat_t, error) {
	f := os.NewFile(uintptr(fd), path)
	if afterOpen != nil {
		afterOpen(path)
	}
	fdInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, nil, err
	}
	if !fdInfo.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, nil, errors.Errorf(errors.LoggingError, "log path %q is not a regular file", path)
	}
	stat, ok := fdInfo.Sys().(*syscall.Stat_t)
	if !ok {
		_ = f.Close()
		return nil, nil, nil, errors.Errorf(errors.LoggingError, "missing unix stat metadata for %q", path)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		_ = f.Close()
		if stderrors.Is(err, os.ErrNotExist) {
			return nil, nil, nil, errConcurrentRotation
		}
		return nil, nil, nil, err
	}
	if !pathInfo.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, nil, errors.Errorf(errors.LoggingError, "log path %q changed during permission check", path)
	}
	if !os.SameFile(fdInfo, pathInfo) {
		_ = f.Close()
		if pathStat, ok := pathInfo.Sys().(*syscall.Stat_t); ok && sameOwner(pathStat.Uid, stat.Uid) {
			return nil, nil, nil, errConcurrentRotation
		}
		return nil, nil, nil, errors.Errorf(errors.LoggingError, "log path %q changed during permission check", path)
	}
	return f, fdInfo, stat, nil
}

func inspectRegularFile(path string) (os.FileInfo, *syscall.Stat_t, bool, error) {
	flags := syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	fd, err := syscall.Open(path, syscall.O_RDONLY|flags, 0)
	if stderrors.Is(err, os.ErrPermission) {
		fd, err = syscall.Open(path, syscall.O_WRONLY|flags, 0)
	}
	if err != nil {
		if stderrors.Is(err, os.ErrNotExist) {
			return nil, nil, false, nil
		}
		return nil, nil, false, errors.Wrapf(err, errors.LoggingError, "inspect log file %q permissions", path)
	}
	f, info, stat, verifyErr := verifyDescriptor(fd, path, nil)
	if verifyErr != nil {
		return nil, nil, false, errors.Wrapf(verifyErr, errors.LoggingError, "inspect log file %q permissions", path)
	}
	_ = f.Close()
	return info, stat, true, nil
}

func (w *rotationWriter) captureExistingPerm() error {
	info, stat, exists, err := inspectRegularFile(w.filename)
	if err != nil {
		return err
	}
	if exists {
		w.active = info
		w.perm = info.Mode().Perm()
		w.uid = stat.Uid
		w.gid = stat.Gid
		w.uidSet = true
		w.preservePerm = true
	}
	w.checked = true
	return nil
}

func openVerifiedAppendOrCreate(path string, afterOpen func(string)) (*os.File, os.FileInfo, *syscall.Stat_t, bool, error) {
	flags := syscall.O_WRONLY | syscall.O_APPEND | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	fd, err := syscall.Open(path, flags, 0)
	created := false
	if stderrors.Is(err, os.ErrNotExist) {
		if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr != nil {
			return nil, nil, nil, false, errors.Wrapf(mkErr, errors.LoggingError, "create directory for log file %q", path)
		}
		fd, err = syscall.Open(path, flags|syscall.O_CREAT|syscall.O_EXCL, 0600)
		if err == nil {
			created = true
		} else if stderrors.Is(err, os.ErrExist) {
			fd, err = syscall.Open(path, flags, 0)
			if stderrors.Is(err, os.ErrNotExist) {
				return nil, nil, nil, false, errConcurrentRotation
			}
		}
	}
	if err != nil {
		return nil, nil, nil, false, errors.Wrapf(err, errors.LoggingError, "open log file %q", path)
	}
	f, info, stat, verifyErr := verifyDescriptor(fd, path, afterOpen)
	if verifyErr != nil {
		return nil, nil, nil, false, errors.Wrapf(verifyErr, errors.LoggingError, "verify log file %q", path)
	}
	return f, info, stat, created, nil
}

func createVerifiedNewLogFile(path string, afterOpen func(string)) (*os.File, os.FileInfo, *syscall.Stat_t, error) {
	if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr != nil {
		return nil, nil, nil, errors.Wrapf(mkErr, errors.LoggingError, "create directory for rotated log file %q", path)
	}
	flags := syscall.O_WRONLY | syscall.O_APPEND | syscall.O_CREAT | syscall.O_EXCL | syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	fd, err := syscall.Open(path, flags, 0600)
	if err != nil {
		if stderrors.Is(err, os.ErrExist) {
			return nil, nil, nil, errConcurrentRotation
		}
		return nil, nil, nil, errors.Wrapf(err, errors.LoggingError, "create rotated log file %q", path)
	}
	f, info, stat, verifyErr := verifyDescriptor(fd, path, afterOpen)
	if verifyErr != nil {
		return nil, nil, nil, errors.Wrapf(verifyErr, errors.LoggingError, "verify rotated log file %q", path)
	}
	return f, info, stat, nil
}

func applyDescriptorOwner(f *os.File, path string, wantUID, wantGID uint32, cur *syscall.Stat_t, chownFn func(*os.File, int, int) error) (*syscall.Stat_t, os.FileInfo, error) {
	if cur.Uid == wantUID && cur.Gid == wantGID {
		return cur, nil, nil
	}
	if chownFn == nil {
		chownFn = (*os.File).Chown
	}
	// Apply ownership before permissions because chown on Unix can clear mode
	// bits (such as setgid) after chmod sets them.
	if err := chownFn(f, int(wantUID), int(wantGID)); err != nil {
		return nil, nil, errors.Wrapf(err, errors.LoggingError, "restore log file %q ownership", path)
	}
	updated, err := f.Stat()
	if err != nil {
		return nil, nil, errors.Wrapf(err, errors.LoggingError, "stat log file %q after ownership restore", path)
	}
	stat, ok := updated.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, nil, errors.Errorf(errors.LoggingError, "missing unix stat metadata for %q", path)
	}
	return stat, updated, nil
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

func (w *rotationWriter) nextBackupName() (string, error) {
	dir := filepath.Dir(w.filename)
	base := filepath.Base(w.filename)
	ext := filepath.Ext(base)
	prefix := base[:len(base)-len(ext)] + "-"
	for range maxBackupNameAttempts {
		now := time.Now()
		if !w.logger.LocalTime {
			now = now.UTC()
		}
		candidate := filepath.Join(dir, prefix+now.Format(lumberjackBackupTimeFormat)+ext)
		_, err := os.Lstat(candidate)
		if err != nil && !stderrors.Is(err, os.ErrNotExist) {
			return "", errors.Wrapf(err, errors.LoggingError, "probe backup log path %q", candidate)
		}
		exists := err == nil
		if w.logger.Compress {
			gzPath := candidate + lumberjackCompressSuffix
			_, gzErr := os.Lstat(gzPath)
			if gzErr != nil && !stderrors.Is(gzErr, os.ErrNotExist) {
				return "", errors.Wrapf(gzErr, errors.LoggingError, "probe compressed backup log path %q", gzPath)
			}
			exists = exists || gzErr == nil
		}
		if !exists {
			return candidate, nil
		}
		time.Sleep(time.Millisecond)
	}
	return "", errors.Errorf(errors.LoggingError, "exhausted backup filenames for %q", w.filename)
}

func (w *rotationWriter) rotateVerifiedFile(prevInfo os.FileInfo) (*os.File, os.FileInfo, error) {
	curInfo, curStat, exists, err := inspectRegularFile(w.filename)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, errConcurrentRotation
	}
	if w.uidSet && !sameOwner(curStat.Uid, w.uid) {
		return nil, nil, errors.Errorf(errors.LoggingError, "log file %q owner uid %d does not match expected uid %d", w.filename, curStat.Uid, w.uid)
	}
	if !os.SameFile(prevInfo, curInfo) {
		return nil, nil, errConcurrentRotation
	}
	if w.uidSet {
		w.gid = curStat.Gid
	}
	if w.preservePerm {
		w.perm = curInfo.Mode().Perm()
	}
	backup, err := w.nextBackupName()
	if err != nil {
		return nil, nil, err
	}
	if err := os.Rename(w.filename, backup); err != nil {
		if stderrors.Is(err, os.ErrNotExist) {
			return nil, nil, errConcurrentRotation
		}
		return nil, nil, errors.Wrapf(err, errors.LoggingError, "rename rotated log file %q", w.filename)
	}
	f, newInfo, newStat, err := createVerifiedNewLogFile(w.filename, nil)
	if err != nil {
		return nil, nil, err
	}
	if w.uidSet {
		_, updatedInfo, chownErr := applyDescriptorOwner(f, w.filename, w.uid, w.gid, newStat, w.chown)
		if chownErr != nil {
			_ = f.Close()
			return nil, nil, chownErr
		}
		if updatedInfo != nil {
			newInfo = updatedInfo
		}
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
	w.maintained = true
	w.maintainBackups()
	return f, newInfo, nil
}

func (w *rotationWriter) rotatePlatform() error {
	if !w.checked {
		if err := w.captureExistingPerm(); err != nil {
			return err
		}
	}
	_ = w.logger.Close()
	curInfo, curStat, exists, err := inspectRegularFile(w.filename)
	if err != nil {
		return err
	}
	if exists {
		if w.uidSet && !sameOwner(curStat.Uid, w.uid) {
			return errors.Errorf(errors.LoggingError, "log file %q owner uid %d does not match expected uid %d", w.filename, curStat.Uid, w.uid)
		}
		if !w.uidSet {
			w.uid = curStat.Uid
			w.gid = curStat.Gid
			w.uidSet = true
		} else if w.active != nil && os.SameFile(w.active, curInfo) {
			w.active = curInfo
			w.gid = curStat.Gid
			if w.preservePerm {
				w.perm = curInfo.Mode().Perm()
			}
		}
		backup, err := w.nextBackupName()
		if err != nil {
			return err
		}
		if err := os.Rename(w.filename, backup); err != nil && !stderrors.Is(err, os.ErrNotExist) {
			return errors.Wrapf(err, errors.LoggingError, "rename rotated log file %q", w.filename)
		}
	}
	f, newInfo, newStat, err := createVerifiedNewLogFile(w.filename, w.afterOpen)
	if err != nil {
		return err
	}
	if !w.uidSet {
		w.uid = newStat.Uid
		w.gid = newStat.Gid
		w.uidSet = true
	} else {
		_, updatedInfo, chownErr := applyDescriptorOwner(f, w.filename, w.uid, w.gid, newStat, w.chown)
		if chownErr != nil {
			_ = f.Close()
			return chownErr
		}
		if updatedInfo != nil {
			newInfo = updatedInfo
		}
	}
	if w.preservePerm && newInfo.Mode().Perm() != w.perm {
		updated, chmodErr := applyDescriptorPerm(f, w.filename, w.perm, w.chmod)
		if chmodErr != nil {
			_ = f.Close()
			return chmodErr
		}
		newInfo = updated
	}
	w.active = newInfo
	w.maintained = true
	if closeErr := f.Close(); closeErr != nil {
		return errors.Wrapf(closeErr, errors.LoggingError, "close rotated log file %q", w.filename)
	}
	w.maintainBackups()
	return nil
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
	tmpPrefix := compressTempPrefix(base)

	var backups []backupEntry
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() {
			continue
		}
		if strings.HasPrefix(name, tmpPrefix) && strings.HasSuffix(name, compressTempSuffix) {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
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
		_ = compressBackupRegularFile(base, src, src+lumberjackCompressSuffix, w.chown)
	}
}

func parseBackupTime(filename, prefix, ext string) (time.Time, bool) {
	if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, ext) || len(filename) < len(prefix)+len(ext) {
		return time.Time{}, false
	}
	ts, err := time.Parse(lumberjackBackupTimeFormat, filename[len(prefix):len(filename)-len(ext)])
	return ts, err == nil
}

func compressBackupRegularFile(base, src, dst string, chownFn func(*os.File, int, int) error) error {
	flags := syscall.O_NOFOLLOW | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	srcFD, err := syscall.Open(src, syscall.O_RDONLY|flags, 0)
	if err != nil {
		return err
	}
	srcFile, srcInfo, srcStat, verifyErr := verifyDescriptor(srcFD, src, nil)
	if verifyErr != nil {
		return verifyErr
	}
	defer srcFile.Close()

	if dstInfo, lerr := os.Lstat(dst); lerr == nil {
		if !dstInfo.Mode().IsRegular() {
			return errors.Errorf(errors.LoggingError, "compressed backup target %q is not a regular file", dst)
		}
	} else if !stderrors.Is(lerr, os.ErrNotExist) {
		return lerr
	}

	suffix, err := randomHex(8)
	if err != nil {
		return err
	}
	tmpPath := filepath.Join(filepath.Dir(dst), compressTempPrefix(base)+suffix+compressTempSuffix)
	tmpFD, err := syscall.Open(tmpPath, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|flags, 0600)
	if err != nil {
		return err
	}
	tmpFile, _, tmpStat, tmpVerifyErr := verifyDescriptor(tmpFD, tmpPath, nil)
	if tmpVerifyErr != nil {
		_ = os.Remove(tmpPath)
		return tmpVerifyErr
	}
	published := false
	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
		}
		if !published {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, _, err := applyDescriptorOwner(tmpFile, tmpPath, srcStat.Uid, srcStat.Gid, tmpStat, chownFn); err != nil {
		return err
	}
	if err := tmpFile.Chmod(srcInfo.Mode().Perm()); err != nil {
		return err
	}
	gz := gzip.NewWriter(tmpFile)
	if _, err := io.Copy(gz, srcFile); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		tmpFile = nil
		return err
	}
	tmpFile = nil

	if dstInfo, lerr := os.Lstat(dst); lerr == nil {
		if !dstInfo.Mode().IsRegular() {
			return errors.Errorf(errors.LoggingError, "compressed backup target %q is not a regular file", dst)
		}
	} else if !stderrors.Is(lerr, os.ErrNotExist) {
		return lerr
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return err
	}
	published = true

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
	for attempt := range maxRotationRetryAttempts {
		f, info, stat, created, err := openVerifiedAppendOrCreate(w.filename, w.afterOpen)
		if err != nil {
			if stderrors.Is(err, errConcurrentRotation) && attempt < maxRotationRetryAttempts-1 {
				continue
			}
			if stderrors.Is(err, errConcurrentRotation) {
				return 0, errors.Wrapf(err, errors.LoggingError, "log path %q changed during permission check", w.filename)
			}
			return 0, err
		}
		if w.uidSet && !sameOwner(stat.Uid, w.uid) {
			_ = f.Close()
			return 0, errors.Errorf(errors.LoggingError, "log file %q owner uid %d does not match expected uid %d", w.filename, stat.Uid, w.uid)
		}
		if !w.uidSet {
			w.uid = stat.Uid
			w.gid = stat.Gid
			w.uidSet = true
		} else if !created && w.active != nil && os.SameFile(w.active, info) {
			w.gid = stat.Gid
			if w.preservePerm {
				w.perm = info.Mode().Perm()
			}
		} else {
			updatedStat, updatedInfo, chownErr := applyDescriptorOwner(f, w.filename, w.uid, w.gid, stat, w.chown)
			if chownErr != nil {
				_ = f.Close()
				return 0, chownErr
			}
			if updatedInfo != nil {
				info = updatedInfo
				stat = updatedStat
			}
			if w.preservePerm && info.Mode().Perm() != w.perm {
				updated, chmodErr := applyDescriptorPerm(f, w.filename, w.perm, w.chmod)
				if chmodErr != nil {
					_ = f.Close()
					return 0, chmodErr
				}
				info = updated
			}
		}
		w.active = info
		if !w.maintained {
			w.maintained = true
			w.maintainBackups()
		}

		if info.Size()+int64(len(p)) > maxBytes {
			_ = f.Close()
			_ = w.logger.Close()
			rotated, rotatedInfo, rotErr := w.rotateVerifiedFile(info)
			if rotErr != nil {
				if stderrors.Is(rotErr, errConcurrentRotation) && attempt < maxRotationRetryAttempts-1 {
					continue
				}
				if stderrors.Is(rotErr, errConcurrentRotation) {
					return 0, errors.Wrapf(rotErr, errors.LoggingError, "log path %q changed during rotation", w.filename)
				}
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
	return 0, errors.Errorf(errors.LoggingError, "log path %q changed during permission check", w.filename)
}
