// Licensed under the Universal Permissive License (UPL) Version 1.0.

package cfg

import (
	"encoding/json"
	"fmt"
)

// RedactedSecret is the diagnostic placeholder for a configuration credential.
const RedactedSecret = "[REDACTED]"

// Secret retains configuration-only credentials while redacting diagnostic
// formatting and JSON dumps. Convert explicitly to string only at the point
// where the credential is consumed.
type Secret string

func (Secret) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(RedactedSecret)) }
func (Secret) MarshalJSON() ([]byte, error)   { return json.Marshal(RedactedSecret) }

// UnmarshalJSON keeps APIKey out of ordinary map formatting after loading a
// policy-source configuration (including a type selected later by flags).
// Explicitly configured other stores retain their existing semantics.
func (c *StoreConfig) UnmarshalJSON(data []byte) error {
	type plain StoreConfig
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = StoreConfig(decoded)
	if c.StoreType == "policy-source" || c.StoreType == "" {
		if key, ok := c.StoreProps["APIKey"].(string); ok {
			c.StoreProps["APIKey"] = Secret(key)
		}
	}
	return nil
}

func (c StoreConfig) MarshalJSON() ([]byte, error) {
	type plain StoreConfig
	copy := plain(c)
	if c.StoreType == "policy-source" || c.StoreType == "" {
		copy.StoreProps = make(map[string]interface{}, len(c.StoreProps))
		for key, value := range c.StoreProps {
			if key == "APIKey" {
				value = Secret("")
			}
			copy.StoreProps[key] = value
		}
	}
	return json.Marshal(copy)
}

func (c StoreConfig) Format(state fmt.State, _ rune) {
	data, err := json.Marshal(c)
	if err != nil {
		data = []byte("[invalid store configuration]")
	}
	_, _ = state.Write(data)
}
