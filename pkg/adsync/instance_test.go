// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestInstanceIDLinkFallback(t *testing.T) {
	for _, errno := range []error{os.ErrPermission, syscall.EPERM, syscall.EACCES, syscall.ENOTSUP, syscall.ENOSYS} {
		t.Run(errno.Error(), func(t *testing.T) {
			dir := t.TempDir()
			link := func(old, new string) error {
				return &os.LinkError{Op: "link", Old: old, New: new, Err: errno}
			}
			id, created, err := loadOrCreateInstanceID(dir, link)
			if err != nil || !created || !ValidInstanceID(id) {
				t.Fatalf("fallback creation = %q, %v, %v", id, created, err)
			}
			path := filepath.Join(dir, InstanceIDFile)
			info, err := os.Stat(path)
			if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
				t.Fatalf("fallback permissions: %v, %v", info, err)
			}
			persisted, created, err := LoadOrCreateInstanceID(dir)
			if err != nil || created || persisted != id {
				t.Fatalf("persisted fallback = %q, %v, %v; want %q", persisted, created, err, id)
			}
			for _, existing := range []string{id + "\n", "corrupt"} {
				if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := writeInstanceID(path, testInstanceID, link); !errors.Is(err, os.ErrExist) {
					t.Fatalf("fallback replaced existing file: %v", err)
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != existing {
					t.Fatalf("existing file changed: %q, %v", data, err)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != InstanceIDFile {
				t.Fatalf("temporary files left behind: %v, %v", entries, err)
			}
		})
	}
}

func TestInstanceIDFallbackConflictReloadsWinner(t *testing.T) {
	dir := t.TempDir()
	link := func(_, path string) error {
		if err := os.WriteFile(path, []byte(testInstanceID+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return syscall.ENOTSUP
	}
	id, created, err := loadOrCreateInstanceID(dir, link)
	if err != nil || created || id != testInstanceID {
		t.Fatalf("fallback conflict = %q, %v, %v", id, created, err)
	}
}

func TestInstanceIDRetriesDisappearingPublication(t *testing.T) {
	for _, failures := range []int{1, 3} {
		t.Run(map[int]string{1: "recover", 3: "bounded"}[failures], func(t *testing.T) {
			dir := t.TempDir()
			attempts := 0
			link := func(_, path string) error {
				attempts++
				if attempts <= failures {
					// A competing fallback creator removed its failed publication
					// after our exclusive link/open had already observed EEXIST.
					if err := os.WriteFile(path, nil, 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					return os.ErrExist
				}
				return syscall.ENOTSUP
			}
			id, created, err := loadOrCreateInstanceID(dir, link)
			if failures == 3 {
				if attempts != 3 || !errors.Is(err, os.ErrNotExist) || created || id != "" {
					t.Fatalf("unbounded/unsafe retry: %d attempts, %q, %v, %v", attempts, id, created, err)
				}
				return
			}
			if err != nil || !created || !ValidInstanceID(id) || attempts != 2 {
				t.Fatalf("retry = %q, %v, %v (%d attempts)", id, created, err, attempts)
			}
			persisted, _, err := LoadOrCreateInstanceID(dir)
			if err != nil || persisted != id {
				t.Fatalf("retried identity not persisted: %q, %v", persisted, err)
			}
		})
	}
}

func TestInstanceIDWaitsForPartialPublication(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, InstanceIDFile, []byte(testInstanceID[:10]), 0o600)
	done := make(chan error, 1)
	time.AfterFunc(20*time.Millisecond, func() {
		done <- os.WriteFile(path, []byte(testInstanceID+"\n"), 0o600)
	})
	id, created, err := LoadOrCreateInstanceID(dir)
	if writeErr := <-done; writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil || created || id != testInstanceID {
		t.Fatalf("partial publication = %q, %v, %v", id, created, err)
	}
}

func TestInstanceIDLinkFailure(t *testing.T) {
	dir := t.TempDir()
	_, _, err := loadOrCreateInstanceID(dir, func(string, string) error { return syscall.EIO })
	if !errors.Is(err, syscall.EIO) {
		t.Fatalf("link I/O error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed publication left files: %v, %v", entries, err)
	}
}

func TestInstanceIDEmptyFileIsNotReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, InstanceIDFile)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateInstanceID(dir); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("empty file error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != 0 {
		t.Fatalf("empty file replaced: %q, %v", data, err)
	}
}

func TestInstanceIDStartupCleansOnlyStaleTemps(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "load"}[persisted], func(t *testing.T) {
			dir := t.TempDir()
			old := time.Now().Add(-2 * time.Hour)
			if persisted {
				writeTestFile(t, dir, InstanceIDFile, []byte(testInstanceID+"\n"), 0o600)
			}
			for _, name := range []string{".adsync-instance-stale", ".adsync-instance-recent", "unrelated"} {
				path := writeTestFile(t, dir, name, []byte("keep"), 0o600)
				if name != ".adsync-instance-recent" {
					if err := os.Chtimes(path, old, old); err != nil {
						t.Fatal(err)
					}
				}
			}
			directory := filepath.Join(dir, ".adsync-instance-directory")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(directory, old, old); err != nil {
				t.Fatal(err)
			}
			id, created, err := LoadOrCreateInstanceID(dir)
			if err != nil || created == persisted || !ValidInstanceID(id) || (persisted && id != testInstanceID) {
				t.Fatalf("startup = %q, %v, %v", id, created, err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".adsync-instance-stale")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stale temporary file survived: %v", err)
			}
			for _, name := range []string{InstanceIDFile, ".adsync-instance-recent", ".adsync-instance-directory", "unrelated"} {
				if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
					t.Fatalf("startup removed %s: %v", name, err)
				}
			}
		})
	}
}
