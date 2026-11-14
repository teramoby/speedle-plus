// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceIDPublicationWindowsCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), InstanceIDFile)
	file, err := openInstanceIDPublication(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(testInstanceID[:10]); err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPublishedInstanceID(path, info, ""); err != nil {
		t.Fatal(err)
	}
	// Failed partial writes must be removable while their identity remains
	// pinned by the original handle; os.OpenFile does not allow this on Windows.
	if err := os.Remove(path); err != nil {
		t.Fatalf("cleanup with publication handle open: %v", err)
	}
	// DeleteFile completes the pending deletion when the final handle closes.
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial publication survived cleanup: %v", err)
	}
}

func TestInstanceIDPublicationWindowsLongPath(t *testing.T) {
	dir := t.TempDir()
	for len(dir) < 280 {
		dir = filepath.Join(dir, strings.Repeat("nested", 8))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, InstanceIDFile)
	file, err := openInstanceIDPublication(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if duplicate, err := openInstanceIDPublication(path); !errors.Is(err, os.ErrExist) {
		if duplicate != nil {
			duplicate.Close()
		}
		t.Fatalf("long-path exclusive creation = %v", err)
	}
}
