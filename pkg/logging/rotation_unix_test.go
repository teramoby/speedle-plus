//go:build unix

package logging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Change umask only in a dedicated test subprocess, never in the parent test
// process. Rotated backups retain their mode; the newly opened file is masked.
func TestRotationPermissionsWithUmask(t *testing.T) {
	value := os.Getenv("SPEEDLE_TEST_ROTATION_UMASK")
	if value == "" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		for _, mask := range []string{"0022", "0077"} {
			t.Run(mask, func(t *testing.T) {
				command := exec.Command(executable, "-test.run=^TestRotationPermissionsWithUmask$", "-test.v")
				command.Env = append(os.Environ(), "SPEEDLE_TEST_ROTATION_UMASK="+mask)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("umask %s: %v\n%s", mask, err, output)
				}
			})
		}
		return
	}
	mask, err := strconv.ParseInt(value, 8, 32)
	if err != nil {
		t.Fatal(err)
	}
	previous := syscall.Umask(int(mask))
	defer syscall.Umask(previous)
	filename := filepath.Join(t.TempDir(), "existing.log")
	if err := os.WriteFile(filename, []byte("existing log\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filename, 0640); err != nil {
		t.Fatal(err)
	}
	rotation := &lumberjack.Logger{Filename: filename}
	t.Cleanup(func() { _ = rotation.Close() })
	logger := log.New()
	if err := InitLogInstance(logger, &LogConfig{RotationConfig: rotation}); err != nil {
		t.Fatal(err)
	}
	logger.Info("before rotation")
	if err := rotation.Rotate(); err != nil {
		t.Fatal(err)
	}
	logger.Info("after rotation")
	if err := rotation.Close(); err != nil {
		t.Fatal(err)
	}
	assertLogPermissions(t, filename, 0640 & ^os.FileMode(mask))
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(filename), "existing-*.log"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups %v: %v", backups, err)
	}
	assertLogPermissions(t, backups[0], 0640)
}
