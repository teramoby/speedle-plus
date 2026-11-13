// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// InstanceIDFile is the file in the cache directory that keeps a direct-mode
// instance ID across restarts.
const InstanceIDFile = "instance-id"

// InstanceHeader carries the instance ID on every request of a direct-mode
// instance.
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
// atomically persisting a new one (mode 0600) when none exists yet. The
// boolean reports whether the ID was created by this call. A file that exists
// but does not hold a valid ID is an error: replacing it silently would
// register a second instance.
func LoadOrCreateInstanceID(dir string) (string, bool, error) {
	if dir == "" {
		return "", false, fmt.Errorf("adsync: instance id directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("adsync: create instance id directory: %w", err)
	}
	path := filepath.Join(dir, InstanceIDFile)
	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("%w: instance id file is not a regular file", ErrUnsafeCache)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return "", false, fmt.Errorf("%w: instance id file must be 0600", ErrUnsafeCache)
		}
		if info.Size() > 128 {
			return "", false, fmt.Errorf("%w: instance id file is too large", ErrCacheCorrupt)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false, fmt.Errorf("adsync: read instance id: %w", err)
		}
		id := strings.TrimSpace(string(data))
		if !ValidInstanceID(id) {
			return "", false, fmt.Errorf("%w: instance id file does not hold a UUIDv7", ErrCacheCorrupt)
		}
		return id, false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", false, fmt.Errorf("adsync: stat instance id: %w", err)
	}
	id, err := NewInstanceID()
	if err != nil {
		return "", false, err
	}
	if err := writeFileAtomic(path, []byte(id+"\n")); err != nil {
		return "", false, fmt.Errorf("adsync: write instance id: %w", err)
	}
	return id, true, nil
}
