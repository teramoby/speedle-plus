// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
			if err != nil || len(entries) != 2 || entries[0].Name() != instanceIDLockFile || entries[1].Name() != InstanceIDFile {
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
	if err != nil || len(entries) != 1 || entries[0].Name() != instanceIDLockFile {
		t.Fatalf("failed publication left files: %v, %v", entries, err)
	}
}

func TestInstanceIDRepairsPartialFile(t *testing.T) {
	requireInstanceIDRepairLock(t)
	for _, partial := range []string{"", testInstanceID[:10], testInstanceID[:35]} {
		t.Run(partial, func(t *testing.T) {
			dir := t.TempDir()
			writeStaleInstanceID(t, dir, partial)
			id, created, err := loadOrCreateInstanceID(dir, func(string, string) error { return syscall.ENOTSUP })
			if err != nil || !created || !ValidInstanceID(id) {
				t.Fatalf("repair = %q, %v, %v", id, created, err)
			}
			again, created, err := LoadOrCreateInstanceID(dir)
			if err != nil || created || again != id {
				t.Fatalf("repair persistence = %q, %v, %v; want %q", again, created, err, id)
			}
		})
	}
}

func TestInstanceIDRejectsUnsafePartialFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions")
	}
	dir := t.TempDir()
	path := writeTestFile(t, dir, InstanceIDFile, []byte(testInstanceID[:10]), 0o644)
	if _, _, err := LoadOrCreateInstanceID(dir); !errors.Is(err, ErrUnsafeCache) {
		t.Fatalf("unsafe partial error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != testInstanceID[:10] {
		t.Fatalf("unsafe partial replaced: %q, %v", data, err)
	}
}

func TestInstanceIDRechecksPartialPublicationSafety(t *testing.T) {
	for _, change := range []string{"size", "mode"} {
		t.Run(change, func(t *testing.T) {
			if change == "mode" && runtime.GOOS == "windows" {
				t.Skip("Unix permissions")
			}
			dir := t.TempDir()
			path := writeTestFile(t, dir, InstanceIDFile, nil, 0o600)
			done := make(chan error, 1)
			time.AfterFunc(20*time.Millisecond, func() {
				if change == "mode" {
					done <- os.Chmod(path, 0o644)
				} else {
					done <- os.WriteFile(path, []byte(strings.Repeat("a", 129)), 0o600)
				}
			})
			_, _, err := LoadOrCreateInstanceID(dir)
			if writeErr := <-done; writeErr != nil {
				t.Fatal(writeErr)
			}
			want := ErrCacheCorrupt
			if change == "mode" {
				want = ErrUnsafeCache
			}
			if !errors.Is(err, want) {
				t.Fatalf("changed %s error = %v", change, err)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || (change == "size" && len(data) != 129) || (change == "mode" && len(data) != 0) {
				t.Fatalf("unsafe file changed: %q, %v", data, readErr)
			}
		})
	}
}

func TestConcurrentInstanceIDRepair(t *testing.T) {
	requireInstanceIDRepairLock(t)
	for _, partial := range []string{"", testInstanceID[:10]} {
		t.Run(partial, func(t *testing.T) {
			dir := t.TempDir()
			writeStaleInstanceID(t, dir, partial)
			type result struct {
				id      string
				created bool
				err     error
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			for range 2 {
				go func() {
					<-start
					id, created, err := loadOrCreateInstanceID(dir, func(string, string) error { return syscall.ENOTSUP })
					results <- result{id, created, err}
				}()
			}
			close(start)
			first, second := <-results, <-results
			if first.err != nil || second.err != nil || !ValidInstanceID(first.id) || first.id != second.id || first.created == second.created {
				t.Fatalf("concurrent repair = %+v, %+v", first, second)
			}
		})
	}
}

func writeStaleInstanceID(t *testing.T, dir, partial string) string {
	t.Helper()
	path := writeTestFile(t, dir, InstanceIDFile, []byte(partial), 0o600)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireInstanceIDRepairLock(t *testing.T) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "lock")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := lockInstanceID(file); errors.Is(err, errInstanceIDLockUnsupported) {
		t.Skip("partial repair requires filesystem locks")
	} else if err != nil {
		t.Fatal(err)
	}
	unlockInstanceID(file)
}

func TestInstanceIDWhitespaceCorruption(t *testing.T) {
	for _, value := range []string{" ", "\n\t", " " + testInstanceID[:10], testInstanceID[:10] + "\n"} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			dir := t.TempDir()
			path := writeStaleInstanceID(t, dir, value)
			id, created, err := LoadOrCreateInstanceID(dir)
			if !errors.Is(err, ErrCacheCorrupt) || errors.Is(err, errPartialInstanceID) || created || id != "" {
				t.Fatalf("whitespace corruption = %q, %v, %v", id, created, err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != value {
				t.Fatalf("corrupt file changed: %q, %v", data, err)
			}
		})
	}
	dir := t.TempDir()
	writeTestFile(t, dir, InstanceIDFile, []byte(" \t"+testInstanceID+"\n"), 0o600)
	if id, created, err := LoadOrCreateInstanceID(dir); err != nil || created || id != testInstanceID {
		t.Fatalf("compatible complete ID = %q, %v, %v", id, created, err)
	}
}

func TestInstanceIDWithoutFilesystemLock(t *testing.T) {
	unsupported := func(*os.File) error { return errInstanceIDLockUnsupported }
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			dir := t.TempDir()
			if partial {
				writeStaleInstanceID(t, dir, testInstanceID[:10])
			}
			id, created, err := loadOrCreateInstanceIDWithLock(dir, func(string, string) error { return syscall.ENOTSUP }, unsupported)
			if partial {
				if !errors.Is(err, ErrCacheCorrupt) || created || id != "" {
					t.Fatalf("unlocked repair = %q, %v, %v", id, created, err)
				}
				data, err := os.ReadFile(filepath.Join(dir, InstanceIDFile))
				if err != nil || string(data) != testInstanceID[:10] {
					t.Fatalf("unlocked partial changed: %q, %v", data, err)
				}
			} else if err != nil || !created || !ValidInstanceID(id) {
				t.Fatalf("unlocked creation = %q, %v, %v", id, created, err)
			}
		})
	}
	if _, _, err := loadOrCreateInstanceIDWithLock(t.TempDir(), os.Link, func(*os.File) error { return syscall.EIO }); !errors.Is(err, syscall.EIO) {
		t.Fatalf("unexpected lock error swallowed: %v", err)
	}
}

func TestInstanceIDPreservesPausedLegacyWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, InstanceIDFile)
	// Simulate an old fallback writer which does not acquire the new lock.
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	before, err := writer.Stat()
	if err != nil {
		t.Fatal(err)
	}
	id, created, err := LoadOrCreateInstanceID(dir)
	if !errors.Is(err, ErrCacheCorrupt) || created || id != "" {
		t.Fatalf("paused legacy publication = %q, %v, %v", id, created, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, instanceIDLockFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recent partial unnecessarily acquired a creation lock: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != 0 {
		t.Fatalf("paused legacy publication replaced: %v, %v", after, err)
	}
	if _, err := writer.WriteString(testInstanceID + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Sync(); err != nil {
		t.Fatal(err)
	}
	id, created, err = LoadOrCreateInstanceID(dir)
	if err != nil || created || id != testInstanceID {
		t.Fatalf("resumed legacy publication = %q, %v, %v", id, created, err)
	}
}

func TestInstanceIDRechecksBeforeRepair(t *testing.T) {
	for _, change := range []string{"inode", "size", "mtime", "mode", "remove", "unchanged"} {
		t.Run(change, func(t *testing.T) {
			if change == "mode" && runtime.GOOS == "windows" {
				t.Skip("Unix permissions")
			}
			dir := t.TempDir()
			path := writeStaleInstanceID(t, dir, "")
			_, partial, err := loadInstanceIDState(path)
			if !errors.Is(err, errPartialInstanceID) {
				t.Fatalf("partial snapshot = %v", err)
			}
			err = nil
			switch change {
			case "inode":
				// Keep the old inode alive so it cannot be reused.
				err = os.Rename(path, path+".old")
				if err == nil {
					writeStaleInstanceID(t, dir, "")
					err = os.Chtimes(path, partial.ModTime(), partial.ModTime())
				}
			case "size":
				err = os.WriteFile(path, []byte(testInstanceID[:10]), 0o600)
			case "mtime":
				err = os.Chtimes(path, time.Now(), time.Now())
			case "mode":
				err = os.Chmod(path, 0o644)
			case "remove":
				err = os.Remove(path)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = checkPartialInstanceID(path, partial)
			switch change {
			case "unchanged":
				if err != nil {
					t.Fatal(err)
				}
			case "remove":
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("removed publication = %v", err)
				}
			default:
				if !errors.Is(err, ErrCacheCorrupt) {
					t.Fatalf("changed publication = %v", err)
				}
			}
		})
	}
}

func TestInstanceIDLockWait(t *testing.T) {
	for _, outcome := range []string{"success", "unsupported", "timeout"} {
		t.Run(outcome, func(t *testing.T) {
			calls := 0
			err := waitInstanceIDLock(nil, func(*os.File) error {
				calls++
				if outcome == "unsupported" {
					return errInstanceIDLockUnsupported
				}
				if outcome == "success" && calls == 2 {
					return nil
				}
				return errInstanceIDLockBusy
			}, 20*time.Millisecond)
			switch outcome {
			case "success":
				if err != nil || calls != 2 {
					t.Fatalf("lock retry = %d, %v", calls, err)
				}
			case "unsupported":
				if !errors.Is(err, errInstanceIDLockUnsupported) || calls != 1 {
					t.Fatalf("unsupported lock = %d, %v", calls, err)
				}
			case "timeout":
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					t.Fatalf("lock deadline = %v", err)
				}
			}
		})
	}
}

func TestInstanceIDLockContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), instanceIDLockFile)
	first, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := lockInstanceID(first); errors.Is(err, errInstanceIDLockUnsupported) {
		t.Skip("filesystem locks unsupported")
	} else if err != nil {
		t.Fatal(err)
	}
	defer unlockInstanceID(first)
	second, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := waitInstanceIDLock(second, tryLockInstanceID, 20*time.Millisecond); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("contended filesystem lock = %v", err)
	}
	unlockInstanceID(first)
	if err := lockInstanceID(second); err != nil {
		t.Fatalf("released filesystem lock = %v", err)
	}
	unlockInstanceID(second)
}

func TestInstanceIDRepairAcrossProcesses(t *testing.T) {
	requireInstanceIDRepairLock(t)
	dir := t.TempDir()
	writeStaleInstanceID(t, dir, testInstanceID[:10])
	var output [2]bytes.Buffer
	var commands [2]*exec.Cmd
	for i := range commands {
		commands[i] = exec.Command(os.Args[0], "-test.run=^TestInstanceIDRepairProcessHelper$")
		commands[i].Env = append(os.Environ(), "SPEEDLE_INSTANCE_ID_TEST_DIR="+dir)
		commands[i].Stdout = &output[i]
		commands[i].Stderr = &output[i]
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("repair process: %v\n%s", err, &output[i])
		}
	}
	first, second := strings.Fields(output[0].String()), strings.Fields(output[1].String())
	if len(first) < 2 || len(second) < 2 || !ValidInstanceID(first[0]) || first[0] != second[0] ||
		!((first[1] == "true" && second[1] == "false") || (first[1] == "false" && second[1] == "true")) {
		t.Fatalf("process repair: %q, %q", &output[0], &output[1])
	}
	if id, created, err := LoadOrCreateInstanceID(dir); err != nil || created || id != first[0] {
		t.Fatalf("process repair persistence = %q, %v, %v", id, created, err)
	}
}

func TestInstanceIDRepairProcessHelper(t *testing.T) {
	dir := os.Getenv("SPEEDLE_INSTANCE_ID_TEST_DIR")
	if dir == "" {
		return
	}
	id, created, err := loadOrCreateInstanceID(dir, func(string, string) error { return syscall.ENOTSUP })
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s %t\n", id, created)
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
