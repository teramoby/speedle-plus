//go:build unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"context"
	stderrors "errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

	t.Run("rejects pre-existing symlink before open", func(t *testing.T) {
		dir := t.TempDir()
		filename, _, w := seedLog(t, dir, "engine.log")
		target := filepath.Join(dir, "target.txt")
		if err := os.WriteFile(target, []byte("secret\n"), 0600); err != nil {
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
		info, statErr := os.Stat(target)
		if statErr != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("target permissions changed or unreadable: %v %v", info, statErr)
		}
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
		info, statErr := os.Stat(target)
		if statErr != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("target permissions changed or unreadable: %v %v", info, statErr)
		}
	})

	t.Run("rejects regular file replacement after open", func(t *testing.T) {
		dir := t.TempDir()
		filename, rotation, w := seedLog(t, dir, "engine.log")
		other := filepath.Join(dir, "other.log")
		if err := os.WriteFile(other, []byte("other\n"), 0600); err != nil {
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
	})

	t.Run("propagates File.Chmod failure through slog handler", func(t *testing.T) {
		dir := t.TempDir()
		_, rotation, w := seedLog(t, dir, "engine.log")
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
	})
}
