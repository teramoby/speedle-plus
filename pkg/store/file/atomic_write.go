// Licensed under the Universal Permissive License (UPL) Version 1.0.
package file

import (
	"os"
	"path/filepath"

	"github.com/teramoby/speedle-plus/pkg/errors"
)

// Independent PMS/ADS processes do not share rwLock. Serialize and sync a
// complete temporary file before publishing it with one same-directory rename,
// so readers see the old or new snapshot, never truncation/partial JSON.
func replacePolicyFile(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return errors.Wrap(err, errors.StoreError, "resolve policy file symlink")
		}
		path = resolved
	}
	var existing os.FileInfo
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New(errors.StoreError, "policy target must be a regular file")
		}
		existing = info
		// Preserve the old writer's access checks as well as its permission
		// bits: directory write permission alone must not replace a read-only
		// policy file. Opening without truncation does not change its data.
		probe, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return errors.Wrap(err, errors.StoreError, "open policy file for writing")
		}
		if err := probe.Close(); err != nil {
			return errors.Wrap(err, errors.StoreError, "close policy file")
		}
	} else if !os.IsNotExist(err) {
		return errors.Wrap(err, errors.StoreError, "stat policy file")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".speedle-policy-*")
	if err != nil {
		return errors.Wrap(err, errors.StoreError, "create temporary policy file")
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if existing != nil {
		if err := temporary.Chmod(existing.Mode().Perm()); err != nil {
			return errors.Wrap(err, errors.StoreError, "preserve policy file permissions")
		}
	}
	if _, err := temporary.Write(data); err != nil {
		return errors.Wrap(err, errors.StoreError, "write temporary policy file")
	}
	if err := temporary.Sync(); err != nil {
		return errors.Wrap(err, errors.StoreError, "sync temporary policy file")
	}
	if err := temporary.Close(); err != nil {
		return errors.Wrap(err, errors.StoreError, "close temporary policy file")
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return errors.Wrap(err, errors.StoreError, "replace policy file")
	}
	return nil
}
