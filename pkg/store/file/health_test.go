//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	storeDir, err := os.MkdirTemp("", "file-store-health-test")
	if err != nil {
		t.Fatal("failed to create temp dir:", err)
	}
	defer os.RemoveAll(storeDir)

	// Health with a valid file.
	t.Run("healthy file", func(t *testing.T) {
		tmpFile := storeDir + "/ps.json"
		if err := os.WriteFile(tmpFile, []byte("{}"), 0644); err != nil {
			t.Fatal("failed to create temp file:", err)
		}
		s := &Store{FileLocation: tmpFile}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Health(ctx); err != nil {
			t.Errorf("Health should succeed for a valid file, got: %v", err)
		}
	})

	// Health with a non-existent file.
	t.Run("missing file", func(t *testing.T) {
		s := &Store{FileLocation: storeDir + "/nonexistent.json"}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Health(ctx); err == nil {
			t.Error("Health should fail for a missing file")
		}
	})

	// Health with empty file location.
	t.Run("empty file location", func(t *testing.T) {
		s := &Store{FileLocation: ""}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Health(ctx); err == nil {
			t.Error("Health should fail when file location is empty")
		}
	})
}

func TestClose(t *testing.T) {
	// Close a fresh store with no watcher.
	s := &Store{FileLocation: "/tmp/dummy.json"}
	if err := s.Close(); err != nil {
		t.Errorf("Close should succeed on a fresh store, got: %v", err)
	}
	if s.discoverStore != nil {
		t.Error("discoverStore should be nil after Close")
	}

	// Close with a nil discoverStore.
	s2 := &Store{FileLocation: "/tmp/dummy2.json"}
	if err := s2.Close(); err != nil {
		t.Errorf("Close should succeed, got: %v", err)
	}

	// Close can be called multiple times safely.
	if err := s2.Close(); err != nil {
		t.Errorf("Second Close should also succeed, got: %v", err)
	}
}
