// Licensed under the Universal Permissive License (UPL) Version 1.0.

//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package adsync

import (
	"os"
	"syscall"
)

func cacheDirectoryOwnedByUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
