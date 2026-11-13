// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import "strings"

func kernelVersion(readFile func(string) ([]byte, error)) string {
	data, err := readFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
