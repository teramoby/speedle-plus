// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOSVersionDoesNotExecuteFromPATH(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	if err := os.WriteFile(filepath.Join(dir, "uname"), []byte("#!/bin/sh\n: > '"+marker+"'\necho untrusted\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	readFile := func(path string) ([]byte, error) {
		if path == "/proc/sys/kernel/osrelease" {
			return []byte("6.1.0-test\n"), nil
		}
		return nil, os.ErrNotExist
	}
	got := osVersionFrom(readFile)
	if runtime.GOOS == "linux" && got != "6.1.0-test" {
		t.Fatalf("kernel release = %q", got)
	}
	if got == "untrusted" {
		t.Fatal("used uname from PATH")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("uname from PATH executed: %v", err)
	}
}

func TestOSVersionPrettyName(t *testing.T) {
	for _, data := range []string{"NAME=Example\nPRETTY_NAME=\"Example Linux 1.0\"\n", "PRETTY_NAME='Example Linux 1.0'", "NAME=Example\r\nPRETTY_NAME=\"Example Linux 1.0\"\r\n"} {
		got := osVersionFrom(func(path string) ([]byte, error) {
			if path != "/etc/os-release" {
				t.Fatalf("unexpected fallback to %q", path)
			}
			return []byte(data), nil
		})
		if got != "Example Linux 1.0" {
			t.Fatalf("pretty name = %q", got)
		}
	}
}
