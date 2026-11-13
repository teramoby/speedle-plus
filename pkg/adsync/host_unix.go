// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build unix && !linux

package adsync

import "golang.org/x/sys/unix"

func kernelVersion(func(string) ([]byte, error)) string {
	var name unix.Utsname
	if err := unix.Uname(&name); err != nil {
		return ""
	}
	return unix.ByteSliceToString(name.Release[:])
}
