// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// goldenVectors mirrors testdata/golden-vectors.json. Only the parts the
// Engine consumes are declared; the file is shared verbatim with Speedle Cloud
// and must never be rewritten here. The file still carries signature vectors
// from an earlier protocol draft; the Engine no longer verifies signatures, so
// those parts are not declared.
type goldenVectors struct {
	Format           string `json:"format"`
	Purpose          string `json:"purpose"`
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
}

type goldenCommit struct {
	Revision          uint64   `json:"revision"`
	PrevHash          string   `json:"prevHash"`
	Hash              string   `json:"hash"`
	At                int64    `json:"at"`
	Changes           []Change `json:"changes"`
	CanonicalBytesHex string   `json:"canonicalBytesHex"`
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
