// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import "fmt"

// maxAPIKeyBytes bounds an API key. Keys are opaque to the Engine; only their
// shape is checked so a broken configuration fails at startup.
const maxAPIKeyBytes = 1024

// ValidateAPIKey reports whether value can be sent as a bearer API key: 1 to
// 1024 printable ASCII characters without spaces. It never echoes the value
// in the returned error.
func ValidateAPIKey(value string) error {
	if value == "" || len(value) > maxAPIKeyBytes {
		return fmt.Errorf("%w: malformed API key", ErrProtocol)
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return fmt.Errorf("%w: malformed API key", ErrProtocol)
		}
	}
	return nil
}
