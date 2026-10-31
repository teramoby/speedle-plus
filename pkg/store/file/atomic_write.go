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
	path, err := policyFileDestination(path)
	if err != nil {
		return err
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

// Resolve each final-component link without requiring the target leaf to
// exist. Like the old os.Create writer, a dangling link can recreate its file
// when the destination directory exists. Bound chains/loops explicitly.
func policyFileDestination(path string) (string, error) {
	for hops := 0; hops < 255; hops++ {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			parent, err := filepath.EvalSymlinks(filepath.Dir(path))
			if err != nil {
				return "", errors.Wrap(err, errors.StoreError, "resolve policy file directory")
			}
			return filepath.Join(parent, filepath.Base(path)), nil
		}
		if err != nil {
			return "", errors.Wrap(err, errors.StoreError, "stat policy file")
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return "", errors.Wrap(err, errors.StoreError, "read policy file symlink")
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	return "", errors.New(errors.StoreError, "policy file symlink chain is too long")
}
