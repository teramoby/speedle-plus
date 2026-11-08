// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// goldenVectors mirrors testdata/golden-vectors.json. Only the parts the
// Engine consumes are declared; the file is shared verbatim with Speedle Cloud
// and must never be rewritten here.
type goldenVectors struct {
	Format  string `json:"format"`
	Purpose string `json:"purpose"`
	TestKey struct {
		KeyID                string `json:"keyId"`
		SeedText             string `json:"seedText"`
		PrivateKeySeedBase64 string `json:"privateKeySeedBase64"`
		PublicKeyBase64      string `json:"publicKeyBase64"`
	} `json:"testKey"`
	StateHashVectors []struct {
		Description       string   `json:"description"`
		Snapshot          Snapshot `json:"snapshot"`
		CanonicalBytesHex string   `json:"canonicalBytesHex"`
		StateHash         string   `json:"stateHash"`
	} `json:"stateHashVectors"`
	ChainVector struct {
		SourceID          string         `json:"sourceId"`
		Epoch             string         `json:"epoch"`
		InitialState      Snapshot       `json:"initialState"`
		InitialStateHash  string         `json:"initialStateHash"`
		InitialCommitHash string         `json:"initialCommitHash"`
		Commits           []goldenCommit `json:"commits"`
		HeadRevision      uint64         `json:"headRevision"`
		HeadSnapshot      Snapshot       `json:"headSnapshot"`
		HeadStateHash     string         `json:"headStateHash"`
		HeadCommitHash    string         `json:"headCommitHash"`
	} `json:"chainVector"`
	SnapshotSignatureVectors []struct {
		SourceID       string    `json:"sourceId"`
		Epoch          string    `json:"epoch"`
		Revision       uint64    `json:"revision"`
		StateHash      string    `json:"stateHash"`
		CommitHash     string    `json:"commitHash"`
		SignedBytesHex string    `json:"signedBytesHex"`
		Signature      Signature `json:"signature"`
	} `json:"snapshotSignatureVectors"`
}

type goldenCommit struct {
	Revision          uint64    `json:"revision"`
	PrevHash          string    `json:"prevHash"`
	Hash              string    `json:"hash"`
	At                int64     `json:"at"`
	Changes           []Change  `json:"changes"`
	CanonicalBytesHex string    `json:"canonicalBytesHex"`
	SignedBytesHex    string    `json:"signedBytesHex"`
	Signature         Signature `json:"signature"`
}

func loadGolden(t *testing.T) *goldenVectors {
	t.Helper()
	data, err := os.ReadFile("testdata/golden-vectors.json")
	if err != nil {
		t.Fatalf("read golden vectors: %v", err)
	}
	var vectors goldenVectors
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("parse golden vectors: %v", err)
	}
	if vectors.Format != "speedle-policy-source-golden-v1" {
		t.Fatalf("unexpected golden vector format %q", vectors.Format)
	}
	return &vectors
}

func goldenPrivateKey(t *testing.T, vectors *goldenVectors) ed25519.PrivateKey {
	t.Helper()
	seed, err := base64.StdEncoding.DecodeString(vectors.TestKey.PrivateKeySeedBase64)
	if err != nil {
		t.Fatalf("decode golden private key: %v", err)
	}
	if len(seed) != ed25519.SeedSize {
		t.Fatalf("golden private seed is %d bytes, want %d", len(seed), ed25519.SeedSize)
	}
	private := ed25519.NewKeyFromSeed(seed)
	expected, err := base64.StdEncoding.DecodeString(vectors.TestKey.PublicKeyBase64)
	if err != nil {
		t.Fatalf("decode golden public key: %v", err)
	}
	if !private.Public().(ed25519.PublicKey).Equal(ed25519.PublicKey(expected)) {
		t.Fatal("golden private seed does not match its public key")
	}
	return private
}

func goldenPublicKeys(t *testing.T, vectors *goldenVectors) *PublicKeySet {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(vectors.TestKey.PublicKeyBase64)
	if err != nil {
		t.Fatalf("decode golden public key: %v", err)
	}
	keys, err := NewPublicKeySet(map[string]ed25519.PublicKey{
		vectors.TestKey.KeyID: ed25519.PublicKey(raw),
	})
	if err != nil {
		t.Fatalf("build public key set: %v", err)
	}
	return keys
}

func mustHash(t *testing.T, value string) Hash {
	t.Helper()
	hash, err := ParseHash(value)
	if err != nil {
		t.Fatalf("parse hash %q: %v", value, err)
	}
	return hash
}

func TestGoldenStateHashVectors(t *testing.T) {
	vectors := loadGolden(t)
	for _, vector := range vectors.StateHashVectors {
		vector := vector
		t.Run(vector.Description, func(t *testing.T) {
			encoded := CanonicalStateBytes(vector.Snapshot)
			if got := hex.EncodeToString(encoded); got != vector.CanonicalBytesHex {
				t.Errorf("canonical bytes mismatch\n got %s\nwant %s", got, vector.CanonicalBytesHex)
			}
			if got, want := StateHash(vector.Snapshot), mustHash(t, vector.StateHash); got != want {
				t.Errorf("state hash = %s, want %s", got, want)
			}
		})
	}
}

func TestGoldenChainVector(t *testing.T) {
	vectors := loadGolden(t)
	keys := goldenPublicKeys(t, vectors)
	chain := vectors.ChainVector

	if got, want := StateHash(chain.InitialState), mustHash(t, chain.InitialStateHash); got != want {
		t.Fatalf("initial state hash = %s, want %s", got, want)
	}
	anchor := mustHash(t, chain.InitialCommitHash)
	if anchor != mustHash(t, chain.InitialStateHash) {
		t.Fatalf("initial commit anchor should equal the initial state hash")
	}

	for _, commit := range chain.Commits {
		commit := commit
		t.Run(fmt.Sprintf("revision_%d", commit.Revision), func(t *testing.T) {
			encoded, err := CanonicalCommitBytes(anchor, commit.Changes)
			if err != nil {
				t.Fatalf("canonical commit bytes: %v", err)
			}
			if got := hex.EncodeToString(encoded); got != commit.CanonicalBytesHex {
				t.Errorf("canonical bytes mismatch\n got %s\nwant %s", got, commit.CanonicalBytesHex)
			}
			hash, err := CommitHash(anchor, commit.Changes)
			if err != nil {
				t.Fatalf("commit hash: %v", err)
			}
			if got, want := hash.String(), commit.Hash; got != want {
				t.Errorf("commit hash = %s, want %s", got, want)
			}
			signed := CommitSignedBytes(chain.SourceID, chain.Epoch, commit.Revision, hash)
			if got := hex.EncodeToString(signed); got != commit.SignedBytesHex {
				t.Errorf("signed bytes mismatch\n got %s\nwant %s", got, commit.SignedBytesHex)
			}
			if err := keys.Verify(commit.Signature, signed); err != nil {
				t.Errorf("verify golden commit signature: %v", err)
			}
			anchor = hash
		})
	}

	if got, want := StateHash(chain.HeadSnapshot), mustHash(t, chain.HeadStateHash); got != want {
		t.Errorf("head state hash = %s, want %s", got, want)
	}
	if got, want := anchor.String(), chain.HeadCommitHash; got != want {
		t.Errorf("head commit hash = %s, want %s", got, want)
	}
}

func TestGoldenSnapshotSignatureVectors(t *testing.T) {
	vectors := loadGolden(t)
	keys := goldenPublicKeys(t, vectors)
	for _, vector := range vectors.SnapshotSignatureVectors {
		vector := vector
		t.Run(vector.StateHash, func(t *testing.T) {
			signed := SnapshotSignedBytes(
				vector.SourceID,
				vector.Epoch,
				vector.Revision,
				mustHash(t, vector.StateHash),
				mustHash(t, vector.CommitHash),
			)
			if got := hex.EncodeToString(signed); got != vector.SignedBytesHex {
				t.Errorf("signed bytes mismatch\n got %s\nwant %s", got, vector.SignedBytesHex)
			}
			if err := keys.Verify(vector.Signature, signed); err != nil {
				t.Errorf("verify golden snapshot signature: %v", err)
			}
		})
	}
}
