// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package adsync

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInstanceIDFIFOLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, instanceIDLockFile)
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	id, created, err := loadOrCreateInstanceIDWithLock(dir, os.Link, func(*os.File) error {
		t.Fatal("FIFO lock was passed to acquisition")
		return nil
	})
	if err != nil || !created || !ValidInstanceID(id) {
		t.Fatalf("FIFO lock creation = %q, %v, %v", id, created, err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("FIFO lock replaced: %v, %v", info, err)
	}
}

func TestInstanceIDLockOpenNoFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), instanceIDLockFile)
	target := filepath.Join(t.TempDir(), "missing-target")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	file, err := openInstanceIDLockFile(path)
	if err == nil {
		file.Close()
		t.Fatal("lock opener followed a symlink")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("lock opener created symlink target: %v", err)
	}
}
