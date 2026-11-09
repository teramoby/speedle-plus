// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
)

// PublicKeyFile is the out-of-band trust anchor for source signatures:
//
//	{"keys":[{"keyId":"cloud-1","publicKey":"base64-encoded-ed25519-public-key"}]}
//
// Public keys are never downloaded from the policy source.
type PublicKeyFile struct {
	Keys []PublicKeyEntry `json:"keys"`
}

// PublicKeyEntry maps one signature keyId to a raw Ed25519 public key.
type PublicKeyEntry struct {
	KeyID     string `json:"keyId"`
	PublicKey string `json:"publicKey"`
}

// PublicKeySet verifies signatures by keyId. Multiple keys may coexist so a
// rotation has an overlap window.
type PublicKeySet struct {
	keys map[string]ed25519.PublicKey
}

// NewPublicKeySet copies the supplied keys. It rejects empty or duplicate key
// identifiers and keys that are not exactly 32 bytes.
func NewPublicKeySet(keys map[string]ed25519.PublicKey) (*PublicKeySet, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: no policy source public keys configured", ErrProtocol)
	}
	copied := make(map[string]ed25519.PublicKey, len(keys))
	for keyID, key := range keys {
		if keyID == "" || len(keyID) > 64 {
			return nil, fmt.Errorf("%w: invalid public key id", ErrProtocol)
		}
		if len(key) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: public key %q must be %d bytes", ErrProtocol, keyID, ed25519.PublicKeySize)
		}
		copied[keyID] = append(ed25519.PublicKey(nil), key...)
	}
	return &PublicKeySet{keys: copied}, nil
}

// ParsePublicKeys parses and validates a public key document.
func ParsePublicKeys(data []byte) (*PublicKeySet, error) {
	var file PublicKeyFile
	if err := decodeStrict(data, &file); err != nil {
		return nil, err
	}
	if len(file.Keys) == 0 {
		return nil, fmt.Errorf("%w: no policy source public keys configured", ErrProtocol)
	}
	keys := make(map[string]ed25519.PublicKey, len(file.Keys))
	for _, entry := range file.Keys {
		if entry.KeyID == "" || len(entry.KeyID) > 64 {
			return nil, fmt.Errorf("%w: invalid public key id", ErrProtocol)
		}
		if _, duplicate := keys[entry.KeyID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate public key id %q", ErrProtocol, entry.KeyID)
		}
		raw, err := base64.StdEncoding.Strict().DecodeString(entry.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("%w: public key %q is not valid base64", ErrProtocol, entry.KeyID)
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: public key %q must be %d bytes", ErrProtocol, entry.KeyID, ed25519.PublicKeySize)
		}
		keys[entry.KeyID] = ed25519.PublicKey(raw)
	}
	return NewPublicKeySet(keys)
}

// LoadPublicKeysFile reads and parses a public key document.
func LoadPublicKeysFile(path string) (*PublicKeySet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("adsync: read public keys: %w", err)
	}
	return ParsePublicKeys(data)
}

// KeyIDs returns the configured key identifiers in sorted order.
func (s *PublicKeySet) KeyIDs() []string {
	if s == nil {
		return nil
	}
	ids := make([]string, 0, len(s.keys))
	for keyID := range s.keys {
		ids = append(ids, keyID)
	}
	sort.Strings(ids)
	return ids
}

// Verify checks a signature over message. An unknown keyId and a bad signature
// are both integrity failures; they are reported distinctly in the wrapped
// message but share the sentinel.
func (s *PublicKeySet) Verify(signature Signature, message []byte) error {
	if err := signature.validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrIntegrity, err)
	}
	if s == nil {
		return fmt.Errorf("%w: no public keys configured", ErrIntegrity)
	}
	key, found := s.keys[signature.KeyID]
	if !found {
		return fmt.Errorf("%w: unknown signing key %q", ErrIntegrity, signature.KeyID)
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(signature.Value)
	if err != nil {
		return fmt.Errorf("%w: invalid signature encoding", ErrIntegrity)
	}
	if !ed25519.Verify(key, message, raw) {
		return fmt.Errorf("%w: signature from key %q did not verify", ErrIntegrity, signature.KeyID)
	}
	return nil
}

// verifySnapshot checks the signature over a snapshot identity tuple.
func (s *PublicKeySet) verifySnapshot(response *SnapshotResponse) error {
	return s.Verify(response.Signature, response.signedSnapshotBytes())
}

// verifyCommit checks the signature over one commit.
func (s *PublicKeySet) verifyCommit(sourceID, epoch string, commit *Commit) error {
	return s.Verify(commit.Signature, commit.signedBytes(sourceID, epoch))
}
