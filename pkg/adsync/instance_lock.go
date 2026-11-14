// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"errors"
	"fmt"
	"os"
	"time"
)

var (
	errInstanceIDLockUnsupported = errors.New("exclusive instance id creation locks are unsupported")
	errInstanceIDLockBusy        = errors.New("instance id creation lock is busy")
)

func lockInstanceID(file *os.File) error {
	return waitInstanceIDLock(file, tryLockInstanceID, 5*time.Second)
}

func waitInstanceIDLock(file *os.File, try func(*os.File) error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := try(file)
		if !errors.Is(err, errInstanceIDLockBusy) {
			return err
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("instance id creation lock timed out: %w", os.ErrDeadlineExceeded)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
