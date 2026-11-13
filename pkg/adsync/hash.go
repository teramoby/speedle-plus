// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	// Wire names of the change operations and kinds.
	changeOpUpsertName    = "upsert"
	changeOpDeleteName    = "delete"
	changeKindAuthzName   = "authz"
	changeKindRoleName    = "role"
	changeKindServiceName = "service"

	// Change op tags.
	changeOpUpsert = 1
	changeOpDelete = 2
	// Change kind tags.
	changeKindAuthz   = 1
	changeKindRole    = 2
	changeKindService = 3

	hashPrefix = "sha256:"
	hashSize   = sha256.Size
)

// Hash is a raw SHA-256 digest. Its wire form is "sha256:" followed by 64
// lowercase hexadecimal digits.
type Hash [hashSize]byte

// ParseHash parses the wire form of a policy-source hash.
func ParseHash(value string) (Hash, error) {
	var hash Hash
	if !strings.HasPrefix(value, hashPrefix) ||
		len(value) != len(hashPrefix)+2*hashSize {
		return hash, fmt.Errorf("%w: invalid hash %q", ErrProtocol, value)
	}
	encoded := value[len(hashPrefix):]
	for index := 0; index < len(encoded); index++ {
		character := encoded[index]
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return hash, fmt.Errorf("%w: invalid hash %q", ErrProtocol, value)
		}
	}
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return hash, fmt.Errorf("%w: invalid hash %q", ErrProtocol, value)
	}
	copy(hash[:], raw)
	return hash, nil
}

// String returns the wire form.
func (h Hash) String() string {
	return hashPrefix + hex.EncodeToString(h[:])
}

// IsZero reports whether the value was never assigned. A SHA-256 output is
// never all zeroes in practice, so a zero value marks a missing wire field.
func (h Hash) IsZero() bool {
	return h == Hash{}
}

// MarshalText implements encoding.TextMarshaler for JSON and cache files.
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for JSON and cache files.
func (h *Hash) UnmarshalText(text []byte) error {
	parsed, err := ParseHash(string(text))
	if err != nil {
		return err
	}
	*h = parsed
	return nil
}

func hashOf(data []byte) Hash {
	return hashOfSum(sha256.Sum256(data))
}

func hashOfSum(sum [hashSize]byte) Hash {
	return Hash(sum)
}

// canonicalWriter emits the big-endian encoding shared with Speedle Cloud.
// Strings are UTF-8 with a u32 byte length; lengths are never derived from
// JSON text, whose escaping differs between languages.
type canonicalWriter struct {
	buffer bytes.Buffer
}

func (w *canonicalWriter) uint8(value byte) {
	w.buffer.WriteByte(value)
}

func (w *canonicalWriter) uint32(value uint32) {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], value)
	w.buffer.Write(encoded[:])
}

func (w *canonicalWriter) uint64(value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	w.buffer.Write(encoded[:])
}

func (w *canonicalWriter) bytes(value []byte) {
	w.buffer.Write(value)
}

func (w *canonicalWriter) string(value string) {
	w.uint32(uint32(len(value)))
	w.buffer.WriteString(value)
}

// CanonicalStateBytes returns the hash input for a snapshot. Policies are
// sorted by id in UTF-8 byte order; Go's string comparison is byte-wise, which
// matches the Cloud implementation's explicit UTF-8 comparison.
func CanonicalStateBytes(snapshot Snapshot) []byte {
	var writer canonicalWriter
	writer.string(snapshot.Name)
	writer.uint32(uint32(len(snapshot.Policies)))
	writeCanonicalPolicies(&writer, snapshot.Policies)
	writer.uint32(uint32(len(snapshot.RolePolicies)))
	writeCanonicalPolicies(&writer, snapshot.RolePolicies)
	return writer.buffer.Bytes()
}

func writeCanonicalPolicies(writer *canonicalWriter, policies []Policy) {
	sorted := make([]Policy, len(policies))
	copy(sorted, policies)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].ID < sorted[right].ID
	})
	for _, policy := range sorted {
		writer.string(policy.ID)
		writer.string(policy.Name)
		writer.string(policy.SPDL)
	}
}

// StateHash returns the wire-form canonical state hash of a snapshot.
func StateHash(snapshot Snapshot) Hash {
	return hashOf(CanonicalStateBytes(snapshot))
}

// CanonicalCommitBytes returns the hash input for a commit: the previous
// 32-byte anchor, the change count, then every change in sequence order. An
// absent name or SPDL on an upsert encodes as an empty string, which is the
// service-rename case.
func CanonicalCommitBytes(prev Hash, changes []Change) ([]byte, error) {
	var writer canonicalWriter
	writer.bytes(prev[:])
	writer.uint32(uint32(len(changes)))
	for index, change := range changes {
		op, err := changeOpCode(change.Op)
		if err != nil {
			return nil, fmt.Errorf("change %d: %w", index, err)
		}
		kind, err := changeKindCode(change.Kind)
		if err != nil {
			return nil, fmt.Errorf("change %d: %w", index, err)
		}
		writer.uint8(op)
		writer.uint8(kind)
		writer.string(change.ID)
		if change.Op == changeOpUpsertName {
			writer.string(change.nameOrEmpty())
			writer.string(change.spdlOrEmpty())
		}
	}
	return writer.buffer.Bytes(), nil
}

// CommitHash returns the wire-form hash of a commit chained from prev.
func CommitHash(prev Hash, changes []Change) (Hash, error) {
	encoded, err := CanonicalCommitBytes(prev, changes)
	if err != nil {
		return Hash{}, err
	}
	return hashOf(encoded), nil
}

func changeOpCode(op string) (byte, error) {
	switch op {
	case changeOpUpsertName:
		return changeOpUpsert, nil
	case changeOpDeleteName:
		return changeOpDelete, nil
	default:
		return 0, fmt.Errorf("%w: unknown change op %q", ErrProtocol, op)
	}
}

func changeKindCode(kind string) (byte, error) {
	switch kind {
	case changeKindAuthzName:
		return changeKindAuthz, nil
	case changeKindRoleName:
		return changeKindRole, nil
	case changeKindServiceName:
		return changeKindService, nil
	default:
		return 0, fmt.Errorf("%w: unknown change kind %q", ErrProtocol, kind)
	}
}
