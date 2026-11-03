// Licensed under the Universal Permissive License (UPL) Version 1.0.
package config

import (
	"encoding/json"
	"testing"
)

// Keep cfg.Config's transport-free type wire-compatible with the original
// assertion.AsserterConfig, including the non-omitempty zero-value fields.
func TestAsserterConfigJSON(t *testing.T) {
	for _, tc := range []struct {
		config AsserterConfig
		want   string
	}{
		{AsserterConfig{}, `{"endpoint":"","caCert":"","clientCert":"","clientKey":"","httpTimeout":0}`},
		{AsserterConfig{Endpoint: "https://assertion.example", CACert: "ca.pem", ClientCert: "client.pem", ClientKey: "client.key", HTTPTimeout: 10}, `{"endpoint":"https://assertion.example","caCert":"ca.pem","clientCert":"client.pem","clientKey":"client.key","httpTimeout":10}`},
	} {
		data, err := json.Marshal(tc.config)
		if err != nil || string(data) != tc.want {
			t.Fatalf("encoded=%s err=%v want=%s", data, err, tc.want)
		}
		var decoded AsserterConfig
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded != tc.config {
			t.Fatalf("decoded=%+v want=%+v", decoded, tc.config)
		}
	}
}
