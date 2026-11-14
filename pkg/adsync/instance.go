// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

// InstanceIDFile is the file in the cache directory that keeps the instance
// ID across restarts.
const InstanceIDFile = "instance-id"

const instanceIDLockFile = ".instance-id.lock"

var errPartialInstanceID = fmt.Errorf("%w: incomplete instance id publication", ErrCacheCorrupt)

var instanceIDLockWarning sync.Once

// InstanceHeader carries the instance ID on every request.
const InstanceHeader = "X-SC-ADS-Instance"

// NewInstanceID returns a random UUIDv7 in its lowercase canonical form.
func NewInstanceID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("adsync: generate instance id: %w", err)
	}
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], uint64(time.Now().UnixMilli()))
	copy(raw[0:6], stamp[2:8])
	raw[6] = raw[6]&0x0f | 0x70
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}

// ValidInstanceID reports whether value is a lowercase canonical UUIDv7.
func ValidInstanceID(value string) bool {
	return uuidV7Pattern.MatchString(value)
}

// LoadOrCreateInstanceID returns the instance ID stored in dir, creating and
// exclusively persisting a new one (mode 0600) when none exists yet. The
// boolean reports whether the ID was created by this call. Empty files and
// truncated UUIDv7 publications older than an hour are repaired under the
// exclusive creation lock. Recent partial publications, other corrupt files,
// and unsafe files remain errors. If the lock cannot be opened or acquired
// within five seconds, creation still works via exclusive publication, but
// partial files cannot be repaired safely. Degraded locking is warned once
// per process.
//
// Concurrent writers sharing dir must all use this creation-lock protocol.
// Stop older releases using dir before upgrading: file age cannot prove that
// an unlocked writer has exited. Each publication verifies that its opened
// inode is still at the canonical path before reporting success.
func LoadOrCreateInstanceID(dir string) (string, bool, error) {
	return loadOrCreateInstanceID(dir, os.Link)
}

func loadOrCreateInstanceID(dir string, link func(string, string) error) (string, bool, error) {
	return loadOrCreateInstanceIDWithLock(dir, link, lockInstanceID)
}

func loadOrCreateInstanceIDWithLock(dir string, link func(string, string) error, acquire func(*os.File) error) (string, bool, error) {
	if dir == "" {
		return "", false, fmt.Errorf("adsync: instance id directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("adsync: create instance id directory: %w", err)
	}
	path := filepath.Join(dir, InstanceIDFile)
	if id, partial, err := loadInstanceIDState(path); err == nil {
		cleanupInstanceIDTemps(dir)
		return id, false, nil
	} else if errors.Is(err, errPartialInstanceID) && !partial.ModTime().Before(time.Now().Add(-time.Hour)) {
		// A recent partial cannot be repaired, so do not make callers wait
		// for the creation lock and repeat the publication wait under it.
		return "", false, err
	} else if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, errPartialInstanceID) {
		return "", false, err
	}
	// The lock file is stable and never unlinked: waiters must lock the same
	// inode. The OS releases the lock on exit, including a creator's crash.
	lock, err := os.OpenFile(filepath.Join(dir, instanceIDLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	locked := false
	if err == nil {
		defer lock.Close()
		err = acquire(lock)
		locked = err == nil
	}
	repaired := false
	if locked {
		// Log only after publication and unlocking. A slow log sink must
		// not extend the gap between removing a partial and publishing an ID.
		defer func() {
			if repaired {
				log.WithField("directory", dir).Warn("adsync: removed stale partial instance id for repair; stop older releases using this directory before upgrading")
			}
		}()
		defer unlockInstanceID(lock)
	} else {
		instanceIDLockWarning.Do(func() {
			log.WithError(err).WithField("directory", dir).Warn("adsync: instance id creation lock unavailable; continuing with exclusive publication, partial-file repair disabled")
		})
	}
	cleanupInstanceIDTemps(dir)
	for range 3 {
		id, partial, err := loadInstanceIDState(path)
		if err == nil {
			return id, false, nil
		}
		if errors.Is(err, errPartialInstanceID) {
			// Older releases do not take the lock. Leave recent publications
			// alone, even after the bounded read wait, so a paused old writer
			// can finish. Never repair without exclusive filesystem locking.
			if !locked || !partial.ModTime().Before(time.Now().Add(-time.Hour)) {
				return "", false, err
			}
			if err := checkPartialInstanceID(path, partial); err != nil {
				return "", false, err
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "", false, fmt.Errorf("adsync: remove partial instance id: %w", err)
			}
			repaired = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", false, err
		}
		id, err = NewInstanceID()
		if err != nil {
			return "", false, err
		}
		if err := writeInstanceID(path, id, link); errors.Is(err, fs.ErrExist) {
			// Reuse the winner unless its failed exclusive-create publication
			// was removed. In that case retry creation without replacing a file.
			id, err = loadInstanceID(path)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return id, false, err
		} else if err != nil {
			return "", false, fmt.Errorf("adsync: write instance id: %w", err)
		}
		return id, true, nil
	}
	return "", false, fmt.Errorf("adsync: instance id publication repeatedly disappeared: %w", fs.ErrNotExist)
}

func loadInstanceID(path string) (string, error) {
	id, _, err := loadInstanceIDState(path)
	return id, err
}

func loadInstanceIDState(path string) (string, fs.FileInfo, error) {
	deadline := time.Now().Add(time.Second)
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return "", nil, fmt.Errorf("adsync: stat instance id: %w", err)
		}
		if err := checkInstanceIDFile(info); err != nil {
			return "", nil, err
		}
		file, err := os.Open(path)
		if err != nil {
			return "", nil, fmt.Errorf("adsync: read instance id: %w", err)
		}
		// Preserve the opened file's identity, including on Windows where a
		// path-based Stat can defer resolving the file ID until SameFile.
		info, err = file.Stat()
		if err != nil {
			file.Close()
			return "", nil, fmt.Errorf("adsync: stat open instance id: %w", err)
		}
		if err := checkInstanceIDFile(info); err != nil {
			file.Close()
			return "", nil, err
		}
		data, err := io.ReadAll(io.LimitReader(file, 129))
		file.Close()
		if err != nil {
			return "", nil, fmt.Errorf("adsync: read instance id: %w", err)
		}
		if len(data) > 128 {
			return "", nil, fmt.Errorf("%w: instance id file is too large", ErrCacheCorrupt)
		}
		id := strings.TrimSpace(string(data))
		if ValidInstanceID(id) {
			return id, nil, nil
		}
		// Only a canonical UUIDv7 prefix can be a partial publication.
		// Classify raw bytes: whitespace cannot be a prefix of our writes.
		prefix := string(data)
		const template = "00000000-0000-7000-8000-000000000000"
		if len(prefix) >= len(template) || !ValidInstanceID(prefix+template[len(prefix):]) {
			return "", nil, fmt.Errorf("%w: instance id file does not hold a UUIDv7", ErrCacheCorrupt)
		}
		// Keep checking size and mode while a fallback writer finishes.
		if !time.Now().Before(deadline) {
			return "", info, errPartialInstanceID
		}
		time.Sleep(time.Millisecond)
	}
}

func checkInstanceIDFile(info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: instance id file is not a regular file", ErrUnsafeCache)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: instance id file must be 0600", ErrUnsafeCache)
	}
	if info.Size() > 128 {
		return fmt.Errorf("%w: instance id file is too large", ErrCacheCorrupt)
	}
	return nil
}

// Re-stat immediately before removal under the creation lock. This detects
// changes since the last read, but cannot exclude a nonparticipating writer
// changing the file after this check (see LoadOrCreateInstanceID).
func checkPartialInstanceID(path string, partial fs.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("adsync: re-stat partial instance id: %w", err)
	}
	if !os.SameFile(partial, current) || current.Size() != partial.Size() ||
		current.Mode() != partial.Mode() || !current.ModTime().Equal(partial.ModTime()) {
		return fmt.Errorf("%w: partial instance id changed before repair", ErrCacheCorrupt)
	}
	return nil
}

// cleanupInstanceIDTemps removes only old regular publication files. Recent
// files may belong to another caller that is still publishing its identity.
func cleanupInstanceIDTemps(dir string) {
	entries, _ := os.ReadDir(dir)
	cutoff := time.Now().Add(-time.Hour)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".adsync-instance-") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// writeInstanceID normally publishes a fully written, private file with Link.
// Filesystems without hard links use exclusive creation instead; neither path
// replaces an existing identity.
func writeInstanceID(path, id string, link func(string, string) error) error {
	return writeInstanceIDWithCheck(path, id, link, func(path string, published fs.FileInfo) error {
		return checkPublishedInstanceID(path, published, id)
	})
}

func writeInstanceIDWithCheck(path, id string, link func(string, string) error, check func(string, fs.FileInfo) error) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".adsync-instance-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.WriteString(id + "\n"); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	published, err := temporary.Stat()
	if err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	var fallback *os.File
	if err := link(temporary.Name(), path); err != nil {
		if !errors.Is(err, fs.ErrPermission) &&
			!errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.ENOSYS) {
			return err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		fallback = file
		defer file.Close()
		if _, err = file.WriteString(id + "\n"); err == nil {
			err = file.Sync()
		}
		// Capture the opened inode before closing, including on Windows.
		var statErr error
		published, statErr = file.Stat()
		if err == nil {
			err = statErr
		}
		if err != nil {
			// A paused unlocked writer may have lost its publication to a
			// repairer. Do not clean up a different writer's winning inode.
			if statErr == nil && checkPublishedInstanceID(path, published, "") == nil {
				_ = os.Remove(path)
			}
			return err
		}
	}
	if err := check(path, published); err != nil {
		return err
	}
	// Keep the fallback inode alive until verification finishes. Closing its
	// last handle earlier would let a replacement reuse its inode number.
	if fallback != nil {
		if err := fallback.Close(); err != nil {
			return err
		}
	}
	if handle, err := os.Open(directory); err == nil {
		_ = handle.Sync()
		_ = handle.Close()
	}
	return nil
}

// A fallback writer can resume after a repairer removed its stale open inode.
// Verify the published path before reporting success on either publication path.
// An empty wantID checks only ownership for cleanup of a failed partial write.
func checkPublishedInstanceID(path string, published fs.FileInfo, wantID string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("adsync: verify published instance id %s: %w", path, err)
	}
	if err := checkInstanceIDFile(info); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("adsync: open published instance id %s: %w", path, err)
	}
	defer file.Close()
	current, err := file.Stat()
	if err != nil {
		return fmt.Errorf("adsync: stat published instance id %s: %w", path, err)
	}
	if !os.SameFile(published, current) {
		return fmt.Errorf("%w: published instance id %s was replaced", ErrCacheCorrupt, path)
	}
	if err := checkInstanceIDFile(current); err != nil {
		return err
	}
	if wantID != "" {
		data, err := io.ReadAll(io.LimitReader(file, 129))
		if err != nil {
			return fmt.Errorf("adsync: read published instance id %s: %w", path, err)
		}
		if len(data) > 128 || strings.TrimSpace(string(data)) != wantID {
			return fmt.Errorf("%w: published instance id %s changed", ErrCacheCorrupt, path)
		}
	}
	return nil
}
