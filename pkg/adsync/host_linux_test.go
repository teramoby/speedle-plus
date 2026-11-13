// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"os"
	"testing"
)

func TestLinuxKernelVersionFallback(t *testing.T) {
	for _, releaseFile := range []string{"", "NAME=Example\n"} {
		for _, available := range []bool{true, false} {
			got := osVersionFrom(func(path string) ([]byte, error) {
				switch path {
				case "/etc/os-release":
					return []byte(releaseFile), nil
				case "/proc/sys/kernel/osrelease":
					if available {
						return []byte("6.1.0-test\n"), nil
					}
					return nil, os.ErrPermission
				default:
					t.Fatalf("unexpected file %q", path)
					return nil, os.ErrNotExist
				}
			})
			want := ""
			if available {
				want = "6.1.0-test"
			}
			if got != want {
				t.Fatalf("kernel release = %q, want %q", got, want)
			}
		}
	}
}
