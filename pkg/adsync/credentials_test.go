// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateCredential(t *testing.T) {
	if err := ValidateCredential(testCredential); err != nil {
		t.Errorf("ValidateCredential(valid) = %v", err)
	}
	for _, invalid := range []string{
		"",
		"adsi_short",
		"adsi_" + testInstanceID + "_" + strings.Repeat("a", 42),
		"adsi_" + testInstanceID + "_" + strings.Repeat("a", 44),
		"adsi_" + testInstanceID + "_" + strings.Repeat("!", 43),
		"spdl_" + testInstanceID + "_" + strings.Repeat("a", 43),
	} {
		if err := ValidateCredential(invalid); err == nil {
			t.Errorf("ValidateCredential(%q) succeeded", invalid)
		}
	}
}

func TestReadCredentialFile(t *testing.T) {
	directory := t.TempDir()
	cases := map[string]string{
		"plain": testCredential + "\n",
		"json":  `{"credential":"` + testCredential + `"}`,
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(directory, name)
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatalf("write credential file: %v", err)
			}
			credential, err := ReadCredentialFile(path)
			if err != nil {
				t.Fatalf("ReadCredentialFile: %v", err)
			}
			if credential != testCredential {
				t.Errorf("credential = %q", credential)
			}
		})
	}
}

func TestReadCredentialFileRejectsUnsafeInput(t *testing.T) {
	directory := t.TempDir()
	plain := filepath.Join(directory, "plain")
	if err := os.WriteFile(plain, []byte(testCredential+"\n"), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(plain, 0o644); err != nil {
			t.Fatalf("chmod credential file: %v", err)
		}
		if _, err := ReadCredentialFile(plain); err == nil {
			t.Error("ReadCredentialFile accepted a group-readable file")
		}
		if err := os.Chmod(plain, 0o600); err != nil {
			t.Fatalf("chmod credential file: %v", err)
		}
	}

	multiple := filepath.Join(directory, "multiple")
	if err := os.WriteFile(multiple, []byte(testCredential+"\n"+testCredential+"\n"), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	if _, err := ReadCredentialFile(multiple); err == nil {
		t.Error("ReadCredentialFile accepted multiple lines")
	}

	malformed := filepath.Join(directory, "malformed")
	if err := os.WriteFile(malformed, []byte("adsi_bad\n"), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	if _, err := ReadCredentialFile(malformed); err == nil {
		t.Error("ReadCredentialFile accepted a malformed credential")
	}

	if _, err := ReadCredentialFile(filepath.Join(directory, "missing")); err == nil {
		t.Error("ReadCredentialFile accepted a missing file")
	}
}

func TestCredentialFromEnvironment(t *testing.T) {
	t.Setenv(CredentialEnv, "")
	credential, err := CredentialFromEnvironment()
	if err != nil || credential != "" {
		t.Fatalf("unset environment = %q, %v", credential, err)
	}

	t.Setenv(CredentialEnv, testCredential)
	credential, err = CredentialFromEnvironment()
	if err != nil || credential != testCredential {
		t.Fatalf("set environment = %q, %v", credential, err)
	}

	t.Setenv(CredentialEnv, "not-a-credential")
	if _, err := CredentialFromEnvironment(); err == nil {
		t.Error("CredentialFromEnvironment accepted a malformed value")
	}
}
