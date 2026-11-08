// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePublicKeys(t *testing.T) {
	vectors := loadGolden(t)
	keys, err := ParsePublicKeys([]byte(`{"keys":[{"keyId":"` + vectors.TestKey.KeyID + `","publicKey":"` + vectors.TestKey.PublicKeyBase64 + `"}]}`))
	if err != nil {
		t.Fatalf("ParsePublicKeys: %v", err)
	}
	if ids := keys.KeyIDs(); len(ids) != 1 || ids[0] != vectors.TestKey.KeyID {
		t.Errorf("KeyIDs = %v", ids)
	}

	message := []byte("message")
	signature := Signature{
		KeyID: vectors.TestKey.KeyID,
		Alg:   "Ed25519",
		Value: base64.StdEncoding.EncodeToString(ed25519.Sign(goldenPrivateKey(t, vectors), message)),
	}
	if err := keys.Verify(signature, message); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := keys.Verify(signature, []byte("other")); !errors.Is(err, ErrIntegrity) {
		t.Errorf("Verify other message = %v", err)
	}

	unknown := signature
	unknown.KeyID = "other"
	if err := keys.Verify(unknown, message); !errors.Is(err, ErrIntegrity) {
		t.Errorf("Verify unknown key = %v", err)
	}
}

func TestParsePublicKeysRejectsInvalidDocuments(t *testing.T) {
	vectors := loadGolden(t)
	publicKey := vectors.TestKey.PublicKeyBase64
	shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
	cases := map[string]string{
		"empty":         `{"keys":[]}`,
		"unknown field": `{"keys":[{"keyId":"a","publicKey":"` + publicKey + `","extra":1}]}`,
		"bad base64":    `{"keys":[{"keyId":"a","publicKey":"!!!"}]}`,
		"wrong size":    `{"keys":[{"keyId":"a","publicKey":"` + shortKey + `"}]}`,
		"duplicate":     `{"keys":[{"keyId":"a","publicKey":"` + publicKey + `"},{"keyId":"a","publicKey":"` + publicKey + `"}]}`,
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePublicKeys([]byte(document)); !errors.Is(err, ErrProtocol) {
				t.Fatalf("ParsePublicKeys error = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestLoadPublicKeysFile(t *testing.T) {
	vectors := loadGolden(t)
	document := []byte(`{"keys":[{"keyId":"` + vectors.TestKey.KeyID + `","publicKey":"` + vectors.TestKey.PublicKeyBase64 + `"}]}`)
	path := filepath.Join(t.TempDir(), "keys.json")
	if err := os.WriteFile(path, document, 0o644); err != nil {
		t.Fatalf("write keys: %v", err)
	}
	keys, err := LoadPublicKeysFile(path)
	if err != nil {
		t.Fatalf("LoadPublicKeysFile: %v", err)
	}
	if len(keys.KeyIDs()) != 1 {
		t.Errorf("KeyIDs = %v", keys.KeyIDs())
	}
	if _, err := LoadPublicKeysFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("LoadPublicKeysFile accepted a missing file")
	}
}
