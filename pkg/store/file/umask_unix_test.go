//go:build unix

// Licensed under the Universal Permissive License (UPL) Version 1.0.
package file

import (
	"syscall"
	"testing"
)

func TestAtomicWritePreservesModeUnderRestrictiveUmask(t *testing.T) {
	previousUmask := syscall.Umask(0077)
	t.Cleanup(func() { syscall.Umask(previousUmask) })

	TestAtomicWritePreservesModeAndSymlink(t)
}
