// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// decodeStrict decodes exactly one JSON value and rejects unknown fields and
// trailing data. The policy-source contract uses strict objects, so a
// misspelled or unexpected field is a protocol violation rather than a value
// silently ignored.
func decodeStrict(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrProtocol, err)
	}
	var extra json.RawMessage
	switch err := decoder.Decode(&extra); err {
	case io.EOF:
		return nil
	case nil:
		return fmt.Errorf("%w: unexpected data after the JSON value", ErrProtocol)
	default:
		return fmt.Errorf("%w: %v", ErrProtocol, err)
	}
}

// decodeLenient decodes a JSON value without rejecting unknown fields. It is
// only used for error envelopes, where extra detail must not hide the status.
func decodeLenient(data []byte, dst any) error {
	return json.Unmarshal(data, dst)
}
