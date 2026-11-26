//go:build unix

//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

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
