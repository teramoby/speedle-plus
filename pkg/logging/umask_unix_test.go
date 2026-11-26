//go:build unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"compress/gzip"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/pkg/errors"
	"gopkg.in/natefinch/lumberjack.v2"
)

func TestRotationPreservesExistingModeUnderRestrictiveUmask(t *testing.T) {
	previousUmask := syscall.Umask(0077)
	t.Cleanup(func() { syscall.Umask(previousUmask) })

	TestRotationAndConfigRoundTrip(t)

	oldApp, oldAudit := application, audit
	application, audit = newState(), newState()
	t.Cleanup(func() { application, audit = oldApp, oldAudit })

	filename := filepath.Join(t.TempDir(), "partial.log")
	if err := os.WriteFile(filename, []byte("seed\n"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filename, 0640); err != nil {
		t.Fatal(err)
	}
	rotation := &lumberjack.Logger{Filename: filename, MaxSize: 1}
	defer rotation.Close()
	if err := InitLog(&LogConfig{Level: "info", Formatter: "json", RotationConfig: rotation}); err != nil {
		t.Fatal(err)
	}
	Default().Info("before-rotate")
	if err := rotation.Rotate(); err != nil {
		t.Fatal(err)
	}
	// Partial InitLog update after Rotate() must still retain the original 0640 mode.
	if err := InitLog(&LogConfig{Level: "debug"}); err != nil {
		t.Fatal(err)
	}
	Default().Debug("after-partial-init")
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("rotated mode after partial InitLog = %o, want 0640", info.Mode().Perm())
	}
}

func TestRotationPermissionRestoreRejectsPathReplacementAndPropagatesChmodError(t *testing.T) {
	previousUmask := syscall.Umask(0077)
	t.Cleanup(func() { syscall.Umask(previousUmask) })

	seedLog := func(t *testing.T, dir, name string) (string, *lumberjack.Logger, *rotationWriter) {
		t.Helper()
		filename := filepath.Join(dir, name)
		if err := os.WriteFile(filename, []byte("seed\n"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename, 0640); err != nil {
			t.Fatal(err)
		}
		rotation := &lumberjack.Logger{Filename: filename, MaxSize: 1}
		t.Cleanup(func() { _ = rotation.Close() })
		w := newRotationWriter(rotation, nil)
		if _, err := w.Write([]byte("initial\n")); err != nil {
			t.Fatal(err)
		}
		return filename, rotation, w
	}

	assertTargetUntouched := func(t *testing.T, target string) {
		t.Helper()
		info, statErr := os.Stat(target)
		if statErr != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("target permissions changed or unreadable: %v %v", info, statErr)
		}
		data, readErr := os.ReadFile(target)
		if readErr != nil || string(data) != "secret\n" {
			t.Fatalf("target file modified unexpectedly: %q %v", data, readErr)
		}
	}

	t.Run("rejects pre-existing symlink before open", func(t *testing.T) {
		dir := t.TempDir()
		filename, rotation, w := seedLog(t, dir, "engine.log")
		target := filepath.Join(dir, "target.txt")
		if err := os.WriteFile(target, []byte("secret\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := rotation.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filename); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filename); err != nil {
			t.Fatal(err)
		}
		chmodCalls := 0
		w.chmod = func(*os.File, os.FileMode) error {
			chmodCalls++
			return nil
		}
		_, err := w.Write([]byte("after-symlink\n"))
		if errors.Code(err) != errors.LoggingError {
			t.Fatalf("expected LoggingError, got %v", err)
		}
		if chmodCalls != 0 {
			t.Fatalf("chmod called %d times on symlink, want 0", chmodCalls)
		}
		assertTargetUntouched(t, target)
	})

	t.Run("rejects symlink replacement after Close on newly created log", func(t *testing.T) {
		dir := t.TempDir()
		filename := filepath.Join(dir, "new.log")
		target := filepath.Join(dir, "target.txt")
		if err := os.WriteFile(target, []byte("secret\n"), 0600); err != nil {
			t.Fatal(err)
		}
		rotation := &lumberjack.Logger{Filename: filename, MaxSize: 1}
		defer rotation.Close()
		w := newRotationWriter(rotation, nil)
		if _, err := w.Write([]byte("first\n")); err != nil {
			t.Fatal(err)
		}
		if err := rotation.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filename); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filename); err != nil {
			t.Fatal(err)
		}
		_, err := w.Write([]byte("second\n"))
		if errors.Code(err) != errors.LoggingError {
			t.Fatalf("expected LoggingError after symlink swap, got %v", err)
		}
		assertTargetUntouched(t, target)
	})

	t.Run("rejects symlink replacement after open", func(t *testing.T) {
		dir := t.TempDir()
		_, rotation, w := seedLog(t, dir, "engine.log")
		target := filepath.Join(dir, "target.txt")
		if err := os.WriteFile(target, []byte("secret\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := rotation.Rotate(); err != nil {
			t.Fatal(err)
		}
		chmodCalls := 0
		w.chmod = func(*os.File, os.FileMode) error {
			chmodCalls++
			return nil
		}
		w.afterOpen = func(path string) {
			_ = os.Remove(path)
			_ = os.Symlink(target, path)
		}
		_, err := w.Write([]byte("after-rotate\n"))
		if errors.Code(err) != errors.LoggingError || !strings.Contains(err.Error(), "changed during permission check") {
			t.Fatalf("expected path-change LoggingError, got %v", err)
		}
		if chmodCalls != 0 {
			t.Fatalf("chmod called %d times after symlink swap, want 0", chmodCalls)
		}
		assertTargetUntouched(t, target)
	})

	t.Run("rejects regular file replacement after open", func(t *testing.T) {
		dir := t.TempDir()
		filename, rotation, w := seedLog(t, dir, "engine.log")
		if err := rotation.Rotate(); err != nil {
			t.Fatal(err)
		}
		chmodCalls := 0
		w.chmod = func(*os.File, os.FileMode) error {
			chmodCalls++
			return nil
		}
		seq := 0
		w.afterOpen = func(path string) {
			seq++
			other := fmt.Sprintf("%s.%d", filepath.Join(dir, "other.log"), seq)
			_ = os.WriteFile(other, []byte("other\n"), 0600)
			_ = os.Rename(other, path)
		}
		_, err := w.Write([]byte("after-rotate\n"))
		if errors.Code(err) != errors.LoggingError || !strings.Contains(err.Error(), "changed during permission check") {
			t.Fatalf("expected path-change LoggingError, got %v", err)
		}
		if chmodCalls != 0 {
			t.Fatalf("chmod called %d times after regular-file swap, want 0", chmodCalls)
		}
		info, statErr := os.Stat(filename)
		if statErr != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("replacement regular file permissions changed: %v %v", info, statErr)
		}
		data, readErr := os.ReadFile(filename)
		if readErr != nil || string(data) != "other\n" {
			t.Fatalf("replacement regular file modified unexpectedly: %q %v", data, readErr)
		}
	})

	t.Run("propagates File.Chmod failure through slog handler", func(t *testing.T) {
		dir := t.TempDir()
		filename, rotation, w := seedLog(t, dir, "engine.log")
		handler, err := newHandler(w, LogConfig{Formatter: "json"})
		if err != nil {
			t.Fatal(err)
		}
		if err := rotation.Rotate(); err != nil {
			t.Fatal(err)
		}
		sentinelErr := stderrors.New("chmod denied")
		w.chmod = func(*os.File, os.FileMode) error {
			return sentinelErr
		}
		err = handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0))
		if errors.Code(err) != errors.LoggingError || !stderrors.Is(err, sentinelErr) {
			t.Fatalf("expected wrapped LoggingError with sentinel, got %v", err)
		}
		data, readErr := os.ReadFile(filename)
		if readErr != nil || len(data) != 0 {
			t.Fatalf("wrote log record despite Chmod failure: %q %v", data, readErr)
		}
	})

	t.Run("honors operator mode tightening on active file without rotation and across later rotation", func(t *testing.T) {
		dir := t.TempDir()
		filename1 := filepath.Join(dir, "active-before-write.log")
		if err := os.WriteFile(filename1, []byte("seed\n"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename1, 0640); err != nil {
			t.Fatal(err)
		}
		rot1 := &lumberjack.Logger{Filename: filename1, MaxSize: 1}
		defer rot1.Close()
		logger1, err := NewLogger(&LogConfig{Level: "info", Formatter: "json", RotationConfig: rot1})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename1, 0600); err != nil {
			t.Fatal(err)
		}
		logger1.Info("first")
		info1, err := os.Stat(filename1)
		if err != nil || info1.Mode().Perm() != 0600 {
			t.Fatalf("active file mode reverted before first write: %v %v", info1, err)
		}

		filename2 := filepath.Join(dir, "active-after-write.log")
		if err := os.WriteFile(filename2, []byte("seed\n"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename2, 0640); err != nil {
			t.Fatal(err)
		}
		rot2 := &lumberjack.Logger{Filename: filename2, MaxSize: 1}
		defer rot2.Close()
		logger2, err := NewLogger(&LogConfig{Level: "info", Formatter: "json", RotationConfig: rot2})
		if err != nil {
			t.Fatal(err)
		}
		logger2.Info("first")
		if err := os.Chmod(filename2, 0600); err != nil {
			t.Fatal(err)
		}
		logger2.Info("second")
		info2, err := os.Stat(filename2)
		if err != nil || info2.Mode().Perm() != 0600 {
			t.Fatalf("active file mode reverted after second write: %v %v", info2, err)
		}
		if err := rot2.Rotate(); err != nil {
			t.Fatal(err)
		}
		logger2.Info("third")
		info2AfterRotate, err := os.Stat(filename2)
		if err != nil || info2AfterRotate.Mode().Perm() != 0600 {
			t.Fatalf("rotated file mode reverted to startup mode: %v %v", info2AfterRotate, err)
		}
	})

	t.Run("preserves write-only 0200 log across rotation", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses 0200 read restriction")
		}
		dir := t.TempDir()
		filename := filepath.Join(dir, "writeonly.log")
		if err := os.WriteFile(filename, []byte("seed\n"), 0200); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename, 0200); err != nil {
			t.Fatal(err)
		}
		rotation := &lumberjack.Logger{Filename: filename, MaxSize: 1}
		defer rotation.Close()
		logger, err := NewLogger(&LogConfig{Level: "info", Formatter: "json", RotationConfig: rotation})
		if err != nil {
			t.Fatal(err)
		}
		logger.Info("first")
		if err := rotation.Rotate(); err != nil {
			t.Fatal(err)
		}
		logger.Info("second")
		info, err := os.Stat(filename)
		if err != nil || info.Mode().Perm() != 0200 {
			t.Fatalf("write-only log mode = %o, want 0200 (err=%v)", info.Mode().Perm(), err)
		}
	})

	t.Run("surfaces unexpected initial inspection failure on Write", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target.txt")
		if err := os.WriteFile(target, []byte("secret\n"), 0600); err != nil {
			t.Fatal(err)
		}
		broken := filepath.Join(dir, "broken.log")
		if err := os.Symlink(target, broken); err != nil {
			t.Fatal(err)
		}
		rotation := &lumberjack.Logger{Filename: broken, MaxSize: 1}
		defer rotation.Close()
		w := newRotationWriter(rotation, nil)
		_, err := w.Write([]byte("line\n"))
		if errors.Code(err) != errors.LoggingError {
			t.Fatalf("expected LoggingError on pre-existing symlink, got %v", err)
		}
		assertTargetUntouched(t, target)
	})

	t.Run("automatic size rotation preserves 0640 mode, compresses and prunes backups under umask 0077", func(t *testing.T) {
		dir := t.TempDir()
		filename := filepath.Join(dir, "auto.log")
		if err := os.WriteFile(filename, []byte("012345678901234567890123456789\n"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename, 0640); err != nil {
			t.Fatal(err)
		}
		rotation := &lumberjack.Logger{Filename: filename, MaxSize: 1, MaxBackups: 1, Compress: true}
		defer rotation.Close()
		w := newRotationWriter(rotation, nil)
		w.maxSizeBytes = 64
		for _, line := range []string{
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n",
			"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n",
			"cccccccccccccccccccccccccccccccccccccccccccc\n",
		} {
			if _, err := w.Write([]byte(line)); err != nil {
				t.Fatal(err)
			}
		}
		info, err := os.Stat(filename)
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("auto-rotated active mode = %o, want 0640 (err=%v)", info.Mode().Perm(), err)
		}
		backups, err := filepath.Glob(filepath.Join(dir, "auto-*.log.gz"))
		if err != nil || len(backups) != 1 {
			t.Fatalf("compressed backups = %v (err=%v), want 1 retained backup", backups, err)
		}
		bInfo, err := os.Stat(backups[0])
		if err != nil || bInfo.Mode().Perm() != 0640 {
			t.Fatalf("compressed backup mode = %o, want 0640 (err=%v)", bInfo.Mode().Perm(), err)
		}
		rawFile, err := os.Open(backups[0])
		if err != nil {
			t.Fatal(err)
		}
		defer rawFile.Close()
		gz, err := gzip.NewReader(rawFile)
		if err != nil {
			t.Fatal(err)
		}
		defer gz.Close()
		decompressed, err := io.ReadAll(gz)
		if err != nil || !strings.Contains(string(decompressed), "bbbbbbbb") {
			t.Fatalf("unexpected compressed backup content: %q %v", decompressed, err)
		}
	})

	t.Run("synchronized Rotate preserves 0640 immediately and concurrent Write+Rotate loses no records", func(t *testing.T) {
		oldApp, oldAudit := application, audit
		application, audit = newState(), newState()
		t.Cleanup(func() { application, audit = oldApp, oldAudit })

		dir := t.TempDir()
		filename := filepath.Join(dir, "sync.log")
		if err := os.WriteFile(filename, []byte("seed\n"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename, 0640); err != nil {
			t.Fatal(err)
		}
		rotation := &lumberjack.Logger{Filename: filename, MaxSize: 1}
		defer rotation.Close()
		if err := InitLog(&LogConfig{Level: "info", Formatter: "json", RotationConfig: rotation}); err != nil {
			t.Fatal(err)
		}
		if err := Rotate(); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filename)
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("mode immediately after synchronized Rotate = %o, want 0640 (err=%v)", info.Mode().Perm(), err)
		}

		const writers = 4
		const perWriter = 50
		var wg sync.WaitGroup
		for g := range writers {
			wg.Go(func() {
				for i := range perWriter {
					Default().Info("sync", "id", fmt.Sprintf("rec-%d-%d", g, i))
				}
			})
		}
		wg.Go(func() {
			for range 20 {
				if err := Rotate(); err != nil {
					t.Errorf("concurrent Rotate failed: %v", err)
				}
			}
		})
		wg.Wait()

		files, err := filepath.Glob(filepath.Join(dir, "sync*.log"))
		if err != nil {
			t.Fatal(err)
		}
		var combined strings.Builder
		for _, f := range files {
			raw, readErr := os.ReadFile(f)
			if readErr != nil {
				t.Fatal(readErr)
			}
			combined.Write(raw)
		}
		all := combined.String()
		for g := range writers {
			for i := range perWriter {
				needle := fmt.Sprintf("rec-%d-%d", g, i)
				if !strings.Contains(all, needle) {
					t.Fatalf("missing log record %q across active+backup logs", needle)
				}
			}
		}
		info, err = os.Stat(filename)
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("final active log mode = %o, want 0640 (err=%v)", info.Mode().Perm(), err)
		}
	})

	t.Run("concurrent Write and external RotationConfig.Rotate return no write errors", func(t *testing.T) {
		dir := t.TempDir()
		filename := filepath.Join(dir, "ext.log")
		if err := os.WriteFile(filename, []byte("seed\n"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename, 0640); err != nil {
			t.Fatal(err)
		}
		rotation := &lumberjack.Logger{Filename: filename, MaxSize: 1}
		defer rotation.Close()
		w := newRotationWriter(rotation, nil)
		w.maxSizeBytes = 128

		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				for i := range 25 {
					if _, err := fmt.Fprintf(w, "external-race-line-%d\n", i); err != nil {
						t.Errorf("concurrent Write returned error: %v", err)
					}
				}
			})
		}
		wg.Go(func() {
			for range 10 {
				if err := rotation.Rotate(); err != nil {
					t.Errorf("external Rotate returned error: %v", err)
				}
				time.Sleep(time.Millisecond)
			}
		})
		wg.Wait()

		info, err := os.Stat(filename)
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("final external-rotated log mode = %o, want 0640 (err=%v)", info.Mode().Perm(), err)
		}
	})

	t.Run("startup first write runs backup maintenance without waiting for rotation", func(t *testing.T) {
		dir := t.TempDir()
		filename := filepath.Join(dir, "startup.log")
		oldBackup := filepath.Join(dir, "startup-2020-01-01T00-00-00.000.log")
		recentTS := time.Now().UTC().Add(-time.Minute).Format(lumberjackBackupTimeFormat)
		recentBackup := filepath.Join(dir, "startup-"+recentTS+".log")
		for _, p := range []string{filename, oldBackup, recentBackup} {
			if err := os.WriteFile(p, []byte("existing\n"), 0640); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p, 0640); err != nil {
				t.Fatal(err)
			}
		}
		rotation := &lumberjack.Logger{Filename: filename, MaxSize: 10, MaxBackups: 1, MaxAge: 7, Compress: true}
		defer rotation.Close()
		w := newRotationWriter(rotation, nil)
		if _, err := w.Write([]byte("small-entry\n")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(oldBackup); !os.IsNotExist(err) {
			t.Fatalf("expired backup was not pruned on startup write: %v", err)
		}
		if _, err := os.Stat(recentBackup + ".gz"); err != nil {
			t.Fatalf("retained backup was not compressed on startup write: %v", err)
		}
	})

	t.Run("long basename backup probe handles disabled vs enabled compression without hanging", func(t *testing.T) {
		dir := t.TempDir()
		longBase := strings.Repeat("a", 225) + ".log"
		filename := filepath.Join(dir, longBase)
		if err := os.WriteFile(filename, []byte("seed\n"), 0640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filename, 0640); err != nil {
			t.Fatal(err)
		}
		rotNoComp := &lumberjack.Logger{Filename: filename, MaxSize: 1, Compress: false}
		defer rotNoComp.Close()
		wNoComp := newRotationWriter(rotNoComp, nil)
		if err := wNoComp.Rotate(); err != nil {
			t.Fatalf("rotation with long basename and Compress=false failed: %v", err)
		}

		rotComp := &lumberjack.Logger{Filename: filename, MaxSize: 1, Compress: true}
		defer rotComp.Close()
		wComp := newRotationWriter(rotComp, nil)
		err := wComp.Rotate()
		if errors.Code(err) != errors.LoggingError {
			t.Fatalf("expected immediate LoggingError when compressed backup exceeds path limit, got %v", err)
		}
	})

	t.Run("concurrent partial InitLog and first Write are race-free", func(t *testing.T) {
		oldApp, oldAudit := application, audit
		application, audit = newState(), newState()
		t.Cleanup(func() { application, audit = oldApp, oldAudit })

		filename := filepath.Join(t.TempDir(), "concurrent.log")
		rotation := &lumberjack.Logger{Filename: filename, MaxSize: 1}
		defer rotation.Close()
		if err := InitLog(&LogConfig{Level: "info", Formatter: "json", RotationConfig: rotation}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				for range 25 {
					Default().Info("concurrent")
				}
			})
			wg.Go(func() {
				for i := range 25 {
					level := "debug"
					if i%2 == 1 {
						level = "info"
					}
					if err := InitLog(&LogConfig{Level: level}); err != nil {
						t.Errorf("partial InitLog failed: %v", err)
					}
				}
			})
		}
		wg.Wait()
	})
}
