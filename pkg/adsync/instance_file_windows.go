// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func instanceIDWindowsName(path string) (*uint16, error) {
	// Retain long-path support when bypassing os.OpenFile's default sharing
	// flags. Extended paths must be absolute; UNC paths need their own prefix.
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if len(absolute) >= 248 && !strings.HasPrefix(absolute, `\\?\`) && !strings.HasPrefix(absolute, `\\.\`) {
		if strings.HasPrefix(absolute, `\\`) {
			absolute = `\\?\UNC\` + absolute[2:]
		} else {
			absolute = `\\?\` + absolute
		}
	}
	name, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return name, nil
}

func openInstanceIDPublication(path string) (*os.File, error) {
	name, err := instanceIDWindowsName(path)
	if err != nil {
		return nil, err
	}
	// CREATE_NEW preserves O_EXCL. Delete sharing lets repair/cleanup unlink
	// the path while this handle keeps the original inode alive for checking.
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}

func openInstanceIDLockFile(path string) (*os.File, error) {
	name, err := instanceIDWindowsName(path)
	if err != nil {
		return nil, err
	}
	// Open a reparse point itself, never its target. Common validation rejects
	// reparse/non-regular entries and checks the opened and named identities.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}
