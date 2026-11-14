// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package adsync

import (
	"fmt"
	"os"
)

func lockInstanceID(*os.File) error {
	return fmt.Errorf("exclusive instance id creation locks are unsupported on this platform")
}

func unlockInstanceID(*os.File) {}
