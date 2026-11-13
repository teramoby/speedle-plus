// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOSVersionDoesNotExecuteFromPATH(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	if err := os.WriteFile(filepath.Join(dir, "uname"), []byte("#!/bin/sh\n: > '"+marker+"'\necho untrusted\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if got := osVersion(); got == "untrusted" {
		t.Fatal("production osVersion used uname from PATH")
	}
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

func TestHostFileReadIsBounded(t *testing.T) {
	dir := t.TempDir()
	for _, size := range []int{32, maxHostFileBytes, maxHostFileBytes + 1} {
		path := writeTestFile(t, dir, "os-release", []byte(strings.Repeat("x", size)), 0o600)
		data, err := readHostFile(path)
		if size > maxHostFileBytes {
			if !errors.Is(err, io.ErrShortBuffer) || data != nil {
				t.Fatalf("oversized host file = %d bytes, %v", len(data), err)
			}
		} else if err != nil || len(data) != size {
			t.Fatalf("host file = %d bytes, %v; want %d", len(data), err, size)
		}
	}
	if _, err := readHostFile(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing host file error = %v", err)
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
