// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
)

// CredentialEnv is the environment variable that may carry an instance
// credential in container deployments. A credential is never read from a URL,
// command line, log or policy content.
const CredentialEnv = "SPEEDLE_ADS_CREDENTIAL"

// instanceCredentialPattern is the wire format from the shared contract:
// adsi_<canonical uppercase ULID instanceId>_<43 base62 digits>.
var instanceCredentialPattern = regexp.MustCompile(`^adsi_[0-7][0-9A-HJKMNP-TV-Z]{25}_[0-9A-Za-z]{43}$`)

// ValidateCredential reports whether value is a well-formed instance
// credential. It never echoes the value in the returned error.
func ValidateCredential(value string) error {
	if !instanceCredentialPattern.MatchString(value) {
		return fmt.Errorf("%w: malformed ADS instance credential", ErrProtocol)
	}
	return nil
}

// ReadCredentialFile reads an instance credential from a regular 0600 file.
// The file may contain the credential on one line, or a JSON object with a
// "credential" field.
func ReadCredentialFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("adsync: read credential file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("adsync: credential path must be a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("adsync: credential file must be readable and writable only by its owner")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("adsync: read credential file: %w", err)
	}
	if len(data) > 8*1024 {
		return "", fmt.Errorf("adsync: credential file is too large")
	}
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		var document struct {
			Credential string `json:"credential"`
		}
		if err := decodeStrict([]byte(trimmed), &document); err != nil {
			return "", fmt.Errorf("adsync: parse credential file: %w", err)
		}
		trimmed = strings.TrimSpace(document.Credential)
	}
	if trimmed == "" {
		return "", fmt.Errorf("adsync: credential file is empty")
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return "", fmt.Errorf("adsync: credential file must hold exactly one credential")
	}
	if err := ValidateCredential(trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

// CredentialFromEnvironment returns the credential in SPEEDLE_ADS_CREDENTIAL,
// or an empty string when the variable is unset. A malformed value is an
// error, never an empty credential.
func CredentialFromEnvironment() (string, error) {
	value := strings.TrimSpace(os.Getenv(CredentialEnv))
	if value == "" {
		return "", nil
	}
	if err := ValidateCredential(value); err != nil {
		return "", err
	}
	return value, nil
}
