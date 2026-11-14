// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
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

func TestInstanceIDVerifiesPublication(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, change := range []string{"unchanged", "remove", "replace", "content"} {
			t.Run(fmt.Sprintf("fallback=%t/%s", fallback, change), func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, InstanceIDFile)
				link := os.Link
				if fallback {
					link = func(string, string) error { return syscall.ENOTSUP }
				}
				winner, err := NewInstanceID()
				if err != nil {
					t.Fatal(err)
				}
				check := func(path string, published os.FileInfo) error {
					// Simulate a resumed writer whose open inode was already
					// unlinked by a repairer, immediately before verification.
					switch change {
					case "remove":
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					case "replace":
						if fallback && runtime.GOOS == "windows" {
							// DeleteFile keeps an open file delete-pending and
							// forbids reusing its name until close. Rename models
							// replacement while the delete-sharing handle is live.
							if err := os.Rename(path, path+".old"); err != nil {
								t.Fatal(err)
							}
						} else if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
						writeTestFile(t, dir, InstanceIDFile, []byte(winner+"\n"), 0o600)
					case "content":
						if err := os.WriteFile(path, []byte(winner+"\n"), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					return checkPublishedInstanceID(path, published, testInstanceID)
				}
				err = writeInstanceIDWithCheck(path, testInstanceID, link, check)
				switch change {
				case "remove":
					// Windows can report access denied while deletion is
					// pending on the original handle, which is also a failure.
					if err == nil || (runtime.GOOS != "windows" && !errors.Is(err, os.ErrNotExist)) {
						t.Fatalf("missing publication reported success: %v", err)
					}
					if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("publication survived handle close: %v", err)
					}
				case "replace", "content":
					if !errors.Is(err, ErrCacheCorrupt) {
						t.Fatalf("replaced publication reported success: %v", err)
					}
					if id, err := loadInstanceID(path); err != nil || id != winner {
						t.Fatalf("publication check deleted the winner: %q, %v", id, err)
					}
				case "unchanged":
					if err != nil {
						t.Fatal(err)
					}
					if id, err := loadInstanceID(path); err != nil || id != testInstanceID {
						t.Fatalf("persisted publication = %q, %v", id, err)
					}
				}
			})
		}
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
	for _, lockErr := range []error{errInstanceIDLockUnsupported, os.ErrDeadlineExceeded, syscall.EIO} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial=%t", lockErr, partial), func(t *testing.T) {
				dir := t.TempDir()
				if partial {
					writeStaleInstanceID(t, dir, testInstanceID[:10])
				}
				id, created, err := loadOrCreateInstanceIDWithLock(dir, func(string, string) error { return syscall.ENOTSUP }, func(*os.File) error { return lockErr })
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
	}

}

func TestInstanceIDLockPathDirectory(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			dir := t.TempDir()
			lockPath := filepath.Join(dir, instanceIDLockFile)
			if err := os.Mkdir(lockPath, 0o700); err != nil {
				t.Fatal(err)
			}
			var before os.FileInfo
			if partial {
				path := writeStaleInstanceID(t, dir, testInstanceID[:10])
				var err error
				before, err = os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			id, created, err := LoadOrCreateInstanceID(dir)
			if partial {
				if !errors.Is(err, ErrCacheCorrupt) || created || id != "" {
					t.Fatalf("unlocked repair = %q, %v, %v", id, created, err)
				}
				after, err := os.Stat(filepath.Join(dir, InstanceIDFile))
				data, readErr := os.ReadFile(filepath.Join(dir, InstanceIDFile))
				if err != nil || !os.SameFile(before, after) || readErr != nil || string(data) != testInstanceID[:10] {
					t.Fatalf("unlocked partial replaced: %v, %q, %v, %v", after, data, err, readErr)
				}
			} else {
				if err != nil || !created || !ValidInstanceID(id) {
					t.Fatalf("unlocked creation = %q, %v, %v", id, created, err)
				}
				if again, created, err := LoadOrCreateInstanceID(dir); err != nil || created || again != id {
					t.Fatalf("unlocked persistence = %q, %v, %v", again, created, err)
				}
			}
			if info, err := os.Stat(lockPath); err != nil || !info.IsDir() {
				t.Fatalf("lock directory replaced: %v, %v", info, err)
			}
		})
	}
}

func TestInstanceIDUnsafeLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode and symlink creation privileges")
	}
	for _, kind := range []string{"symlink", "dangling symlink", "mode"} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial=%t", kind, partial), func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, instanceIDLockFile)
				target := filepath.Join(t.TempDir(), "target")
				if kind == "mode" {
					writeTestFile(t, dir, instanceIDLockFile, nil, 0o666)
				} else {
					if kind == "symlink" {
						if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.Symlink(target, path); err != nil {
						t.Fatal(err)
					}
				}
				if partial {
					writeStaleInstanceID(t, dir, testInstanceID[:10])
				}
				id, created, err := loadOrCreateInstanceIDWithLock(dir, os.Link, func(*os.File) error {
					t.Fatal("unsafe lock was passed to acquisition")
					return nil
				})
				if partial {
					if !errors.Is(err, ErrCacheCorrupt) || created || id != "" {
						t.Fatalf("unsafe-lock repair = %q, %v, %v", id, created, err)
					}
					data, err := os.ReadFile(filepath.Join(dir, InstanceIDFile))
					if err != nil || string(data) != testInstanceID[:10] {
						t.Fatalf("unsafe lock enabled repair: %q, %v", data, err)
					}
				} else if err != nil || !created || !ValidInstanceID(id) {
					t.Fatalf("unsafe-lock creation = %q, %v, %v", id, created, err)
				}
				if kind == "symlink" {
					if data, err := os.ReadFile(target); err != nil || string(data) != "unchanged" {
						t.Fatalf("lock target changed: %q, %v", data, err)
					}
				} else if kind == "dangling symlink" {
					if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("dangling lock target was created: %v", err)
					}
				}
			})
		}
	}
}

func TestInstanceIDDoesNotRepairUnannouncedPartial(t *testing.T) {
	requireInstanceIDRepairLock(t)
	dir := t.TempDir()
	id, created, err := loadOrCreateInstanceIDWithLock(dir, os.Link, func(file *os.File) error {
		writeStaleInstanceID(t, dir, "")
		return lockInstanceID(file)
	})
	if !errors.Is(err, ErrCacheCorrupt) || created || id != "" {
		t.Fatalf("unannounced repair = %q, %v, %v", id, created, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, InstanceIDFile)); err != nil || len(data) != 0 {
		t.Fatalf("unannounced partial changed: %q, %v", data, err)
	}
	if id, created, err := LoadOrCreateInstanceID(dir); err != nil || !created || !ValidInstanceID(id) {
		t.Fatalf("announced repair = %q, %v, %v", id, created, err)
	}
}

func TestInstanceIDLockWarningOnce(t *testing.T) {
	if os.Getenv("SPEEDLE_INSTANCE_ID_WARNING_TEST") == "" {
		command := exec.Command(os.Args[0], "-test.run=^TestInstanceIDLockWarningOnce$")
		command.Env = append(os.Environ(), "SPEEDLE_INSTANCE_ID_WARNING_TEST=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("warning process: %v\n%s", err, output)
		}
		return
	}
	// Isolate the process-wide Once and logger from the other package tests.
	var output bytes.Buffer
	previous := log.StandardLogger().Out
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	for _, lockErr := range []error{errInstanceIDLockUnsupported, os.ErrDeadlineExceeded, syscall.EIO} {
		id, created, err := loadOrCreateInstanceIDWithLock(t.TempDir(), os.Link, func(*os.File) error { return lockErr })
		if err != nil || !created || !ValidInstanceID(id) {
			t.Fatalf("unlocked creation = %q, %v, %v", id, created, err)
		}
	}
	if strings.Count(output.String(), "instance id creation lock unavailable") != 1 ||
		!strings.Contains(output.String(), errInstanceIDLockUnsupported.Error()) ||
		!strings.Contains(output.String(), "partial-file repair disabled") {
		t.Fatalf("expected one actionable warning: %s", &output)
	}
}

func TestInstanceIDRepairWarning(t *testing.T) {
	if os.Getenv("SPEEDLE_INSTANCE_ID_REPAIR_WARNING_TEST") == "" {
		command := exec.Command(os.Args[0], "-test.run=^TestInstanceIDRepairWarning$")
		command.Env = append(os.Environ(), "SPEEDLE_INSTANCE_ID_REPAIR_WARNING_TEST=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("repair warning process: %v\n%s", err, output)
		}
		return
	}
	requireInstanceIDRepairLock(t)
	var output bytes.Buffer
	previousOutput, previousFormatter := log.StandardLogger().Out, log.StandardLogger().Formatter
	log.SetOutput(&output)
	log.SetFormatter(&log.JSONFormatter{})
	defer log.SetOutput(previousOutput)
	defer log.SetFormatter(previousFormatter)
	for _, partial := range []string{"", testInstanceID[:10]} {
		output.Reset()
		dir := t.TempDir()
		writeStaleInstanceID(t, dir, partial)
		id, created, err := LoadOrCreateInstanceID(dir)
		if err != nil || !created || !ValidInstanceID(id) {
			t.Fatalf("repair = %q, %v, %v", id, created, err)
		}
		if again, created, err := LoadOrCreateInstanceID(dir); err != nil || created || again != id {
			t.Fatalf("repair persistence = %q, %v, %v", again, created, err)
		}
		// Every repaired directory must identify itself and explain the
		// upgrade restriction; ordinary reuse must not emit another warning.
		var warning struct {
			Directory string `json:"directory"`
			Level     string `json:"level"`
			Message   string `json:"msg"`
		}
		if err := json.Unmarshal(output.Bytes(), &warning); err != nil || warning.Directory != dir ||
			warning.Level != "warning" || !strings.Contains(warning.Message, "stop older releases") {
			t.Fatalf("expected one operator-visible repair warning for %s: %s (%v)", dir, &output, err)
		}
	}

	t.Run("blocked log sink", func(t *testing.T) {
		dir := t.TempDir()
		path := writeStaleInstanceID(t, dir, "")
		sink := &instanceIDBlockingLogWriter{entered: make(chan struct{}, 1), release: make(chan struct{})}
		log.SetOutput(sink)
		done := make(chan error, 1)
		func() {
			defer close(sink.release)
			go func() {
				id, created, err := LoadOrCreateInstanceID(dir)
				if err == nil && (!created || !ValidInstanceID(id)) {
					err = fmt.Errorf("repair = %q, %v", id, created)
				}
				done <- err
			}()
			select {
			case <-sink.entered:
			case <-time.After(10 * time.Second):
				t.Fatal("repair did not reach the log sink")
			}
			if data, err := os.ReadFile(path); err != nil || len(data) != 0 {
				t.Fatalf("partial removed or changed before warning completed: %q, %v", data, err)
			}
			lock, err := os.OpenFile(filepath.Join(dir, instanceIDLockFile), os.O_CREATE|os.O_RDWR, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if err := waitInstanceIDLock(lock, tryLockInstanceID, 20*time.Millisecond); err != nil {
				t.Fatalf("repair lock held during warning: %v", err)
			}
			unlockInstanceID(lock)
		}()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if id, err := loadInstanceID(path); err != nil || !ValidInstanceID(id) {
			t.Fatalf("repair not published after warning: %q, %v", id, err)
		}
	})
}

type instanceIDBlockingLogWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (writer *instanceIDBlockingLogWriter) Write(data []byte) (int, error) {
	select {
	case writer.entered <- struct{}{}:
	default:
	}
	<-writer.release
	return len(data), nil
}

func TestInstanceIDFrozenLockHolder(t *testing.T) {
	requireInstanceIDRepairLock(t)
	for _, state := range []string{"create", "winner", "partial"} {
		t.Run(state, func(t *testing.T) {
			dir := t.TempDir()
			var before os.FileInfo
			if state == "partial" {
				path := writeStaleInstanceID(t, dir, testInstanceID[:10])
				var err error
				before, err = os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			holder := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstanceIDFrozenLockProcessHelper$")
			holder.Env = append(os.Environ(), "SPEEDLE_INSTANCE_ID_LOCK_TEST_DIR="+dir)
			input, err := holder.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := holder.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			holder.Stderr = &stderr
			if err := holder.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				input.Close()
				if err := holder.Wait(); err != nil {
					t.Errorf("lock holder: %v\n%s", err, &stderr)
				}
			}()
			scanner := bufio.NewScanner(output)
			if !scanner.Scan() || scanner.Text() != "locked" {
				t.Fatalf("holder did not acquire lock: %q, %v", scanner.Text(), scanner.Err())
			}
			// The child holds the native lock without making progress until
			// stdin closes, modelling a frozen process beyond the real timeout.
			// Publish a winner after the fast-path miss to test reloading it
			// after the timeout as well as first creation and refusal to repair.
			acquire := func(file *os.File) error {
				if state == "winner" {
					if err := writeInstanceID(filepath.Join(dir, InstanceIDFile), testInstanceID, os.Link); err != nil {
						t.Fatal(err)
					}
				}
				start := time.Now()
				err := lockInstanceID(file)
				if !errors.Is(err, os.ErrDeadlineExceeded) || time.Since(start) < 5*time.Second {
					t.Fatalf("frozen holder lock attempt = %v after %v", err, time.Since(start))
				}
				return err
			}
			id, created, err := loadOrCreateInstanceIDWithLock(dir, func(string, string) error { return syscall.ENOTSUP }, acquire)
			switch state {
			case "create":
				if err != nil || !created || !ValidInstanceID(id) {
					t.Fatalf("frozen holder creation = %q, %v, %v", id, created, err)
				}
			case "winner":
				if err != nil || created || id != testInstanceID {
					t.Fatalf("frozen holder winner = %q, %v, %v", id, created, err)
				}
			case "partial":
				if !errors.Is(err, ErrCacheCorrupt) || created || id != "" {
					t.Fatalf("frozen holder repair = %q, %v, %v", id, created, err)
				}
				after, err := os.Stat(filepath.Join(dir, InstanceIDFile))
				data, readErr := os.ReadFile(filepath.Join(dir, InstanceIDFile))
				if err != nil || !os.SameFile(before, after) || readErr != nil || string(data) != testInstanceID[:10] {
					t.Fatalf("frozen holder partial replaced: %v, %q, %v, %v", after, data, err, readErr)
				}
			}
			if state != "partial" {
				if again, created, err := LoadOrCreateInstanceID(dir); err != nil || created || again != id {
					t.Fatalf("frozen holder persistence = %q, %v, %v", again, created, err)
				}
			}
		})
	}
}

func TestInstanceIDFrozenLockProcessHelper(t *testing.T) {
	dir := os.Getenv("SPEEDLE_INSTANCE_ID_LOCK_TEST_DIR")
	if dir == "" {
		return
	}
	file, err := os.OpenFile(filepath.Join(dir, instanceIDLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := lockInstanceID(file); err != nil {
		t.Fatal(err)
	}
	defer unlockInstanceID(file)
	fmt.Println("locked")
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
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
	var diagnostics [2]bytes.Buffer
	var commands [2]*exec.Cmd
	for i := range commands {
		commands[i] = exec.Command(os.Args[0], "-test.run=^TestInstanceIDRepairProcessHelper$")
		commands[i].Env = append(os.Environ(), "SPEEDLE_INSTANCE_ID_TEST_DIR="+dir)
		commands[i].Stdout = &output[i]
		commands[i].Stderr = &diagnostics[i]
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("repair process: %v\n%s\n%s", err, &output[i], &diagnostics[i])
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
