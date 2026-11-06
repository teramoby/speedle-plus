// Licensed under the Universal Permissive License (UPL) Version 1.0.
// Package config defines webhook configuration without importing its HTTP client.
package config

type AsserterConfig struct {
	Endpoint    string `json:"endpoint"`
	CACert      string `json:"caCert"`
	ClientCert  string `json:"clientCert"`
	ClientKey   string `json:"clientKey"`
	HTTPTimeout int    `json:"httpTimeout"`
}
