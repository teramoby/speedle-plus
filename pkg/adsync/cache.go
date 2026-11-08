// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	cacheFormat   = "speedle-adsync-cache-v1"
	maxCacheBytes = 8 << 20
)

// Cache stores one verified service copy per file. Files are written with mode
// 0600 through a same-directory temporary file and rename, and carry a
// SHA-256 checksum of their payload. The cache is optional and never
// authoritative: Sync still confirms the revision against the source.
type Cache struct {
	dir  string
	keys *PublicKeySet
}

// NewCache creates cacheDir with mode 0700 when needed.
func NewCache(dir string, keys *PublicKeySet) (*Cache, error) {
	if dir == "" {
		return nil, fmt.Errorf("adsync: cache directory is empty")
	}
	if keys == nil {
		return nil, fmt.Errorf("adsync: cache requires public keys")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("adsync: create cache directory: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("adsync: stat cache directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("adsync: cache path is not a directory")
	}
	return &Cache{dir: dir, keys: keys}, nil
}

type cacheEnvelope struct {
	Format   string          `json:"format"`
	Checksum Hash            `json:"checksum"`
	Payload  json.RawMessage `json:"payload"`
}

type cachePayload struct {
	ServiceID  string    `json:"serviceId"`
	SourceID   string    `json:"sourceId"`
	Epoch      string    `json:"epoch"`
	Revision   uint64    `json:"revision"`
	StateHash  Hash      `json:"stateHash"`
	CommitHash Hash      `json:"commitHash"`
	Snapshot   Snapshot  `json:"snapshot"`
	Signature  Signature `json:"signature"`
}

// Load returns the cached state for one service, or nil when no file exists.
// A file that is corrupt, has unsafe permissions or no longer matches its own
// checksum is an error and must not be used.
func (c *Cache) Load(serviceID string) (*ServiceState, error) {
	if !validEntityID(serviceID) {
		return nil, fmt.Errorf("%w: invalid service id", ErrCacheCorrupt)
	}
	path := c.path(serviceID)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("adsync: stat cache file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: cache file is not a regular file", ErrUnsafeCache)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: cache file must be 0600", ErrUnsafeCache)
	}
	if info.Size() > maxCacheBytes {
		return nil, fmt.Errorf("%w: cache file is too large", ErrCacheCorrupt)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("adsync: read cache file: %w", err)
	}

	var envelope cacheEnvelope
	if err := decodeStrict(data, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheCorrupt, err)
	}
	if envelope.Format != cacheFormat {
		return nil, fmt.Errorf("%w: unknown cache format %q", ErrCacheCorrupt, envelope.Format)
	}
	payloadBytes := []byte(envelope.Payload)
	if len(payloadBytes) == 0 {
		return nil, fmt.Errorf("%w: cache payload is empty", ErrCacheCorrupt)
	}
	if actual := hashOf(payloadBytes); actual != envelope.Checksum {
		return nil, fmt.Errorf("%w: cache checksum does not match", ErrCacheCorrupt)
	}

	var payload cachePayload
	if err := decodeStrict(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheCorrupt, err)
	}
	if payload.ServiceID != serviceID {
		return nil, fmt.Errorf("%w: cache file holds service %q", ErrCacheCorrupt, payload.ServiceID)
	}
	if !validEntityID(payload.SourceID) || !validEntityID(payload.Epoch) {
		return nil, fmt.Errorf("%w: invalid source identity", ErrCacheCorrupt)
	}
	if !validRevision(payload.Revision) {
		return nil, fmt.Errorf("%w: revision is out of range", ErrCacheCorrupt)
	}
	if payload.StateHash.IsZero() || payload.CommitHash.IsZero() {
		return nil, fmt.Errorf("%w: cache hashes are missing", ErrCacheCorrupt)
	}
	if err := payload.Snapshot.validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheCorrupt, err)
	}
	if computed := StateHash(payload.Snapshot); computed != payload.StateHash {
		return nil, fmt.Errorf("%w: state hash does not match the cached content", ErrCacheCorrupt)
	}

	if payload.Signature.KeyID != "" || payload.Signature.Alg != "" || payload.Signature.Value != "" {
		snapshot := &SnapshotResponse{
			ServiceID:  payload.ServiceID,
			SourceID:   payload.SourceID,
			Epoch:      payload.Epoch,
			Revision:   payload.Revision,
			StateHash:  payload.StateHash,
			CommitHash: payload.CommitHash,
			Signature:  payload.Signature,
			Snapshot:   payload.Snapshot,
		}
		if err := c.keys.verifySnapshot(snapshot); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCacheCorrupt, err)
		}
	}

	return &ServiceState{
		ServiceID:  payload.ServiceID,
		SourceID:   payload.SourceID,
		Epoch:      payload.Epoch,
		Revision:   payload.Revision,
		StateHash:  payload.StateHash,
		CommitHash: payload.CommitHash,
		Snapshot:   cloneSnapshot(payload.Snapshot),
		signature:  payload.Signature,
	}, nil
}

// Save atomically replaces the cache file for the state's service.
func (c *Cache) Save(state *ServiceState) error {
	if state == nil {
		return fmt.Errorf("adsync: cannot cache a nil state")
	}
	if !validEntityID(state.ServiceID) || !validEntityID(state.SourceID) || !validEntityID(state.Epoch) {
		return fmt.Errorf("%w: cannot cache an invalid service identity", ErrProtocol)
	}
	if !validRevision(state.Revision) || state.StateHash.IsZero() || state.CommitHash.IsZero() {
		return fmt.Errorf("%w: cannot cache an invalid resume position", ErrProtocol)
	}
	if err := state.Snapshot.validate(); err != nil {
		return err
	}
	payload := cachePayload{
		ServiceID:  state.ServiceID,
		SourceID:   state.SourceID,
		Epoch:      state.Epoch,
		Revision:   state.Revision,
		StateHash:  state.StateHash,
		CommitHash: state.CommitHash,
		Snapshot:   state.Snapshot,
		Signature:  state.signature,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("adsync: encode cache payload: %w", err)
	}
	envelopeBytes, err := json.Marshal(cacheEnvelope{
		Format:   cacheFormat,
		Checksum: hashOf(payloadBytes),
		Payload:  payloadBytes,
	})
	if err != nil {
		return fmt.Errorf("adsync: encode cache file: %w", err)
	}
	if err := writeFileAtomic(c.path(state.ServiceID), envelopeBytes); err != nil {
		return fmt.Errorf("adsync: write cache file: %w", err)
	}
	return nil
}

// Remove deletes the cache file for one service. A missing file is not an
// error.
func (c *Cache) Remove(serviceID string) error {
	if !validEntityID(serviceID) {
		return fmt.Errorf("adsync: invalid service id")
	}
	if err := os.Remove(c.path(serviceID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("adsync: remove cache file: %w", err)
	}
	return nil
}

func (c *Cache) path(serviceID string) string {
	return filepath.Join(c.dir, cacheFileName(serviceID))
}

// cacheFileName keeps the ULID or UUIDv7 readable. Anything else is hashed so
// a service id can never escape the cache directory.
func cacheFileName(serviceID string) string {
	if len(serviceID) <= 64 && strings.IndexFunc(serviceID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '-')
	}) < 0 {
		return "service-" + serviceID + ".json"
	}
	digest := hashOf([]byte(serviceID))
	return "service-" + strings.TrimPrefix(digest.String(), hashPrefix) + ".json"
}

func writeFileAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".adsync-cache-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	// Persist the rename itself when the platform allows it. A missing flush
	// only weakens the crash guarantee, never the atomicity of the replace.
	if handle, err := os.Open(directory); err == nil {
		_ = handle.Sync()
		_ = handle.Close()
	}
	return nil
}
