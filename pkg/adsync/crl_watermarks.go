// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const crlWatermarksFile = "crl-watermarks.json"
const crlWatermarksFormat = "speedle-adsync-crl-watermarks-v1"

// Preserve incomparable number/time pairs instead of inventing a combined
// watermark which no real CRL covers. Dominated pairs are discarded.
type crlWatermark struct {
	Subject    []byte    `json:"subject"`
	PublicKey  []byte    `json:"publicKey"`
	ThisUpdate time.Time `json:"thisUpdate"`
	Number     *big.Int  `json:"number,omitempty"`
}

func (w crlWatermark) issuer() crlIssuer {
	return crlIssuer{subject: string(w.Subject), publicKey: string(w.PublicKey)}
}

func (w crlWatermark) covers(old crlWatermark) bool {
	return w.issuer() == old.issuer() && !w.ThisUpdate.Before(old.ThisUpdate) &&
		(old.Number == nil || (w.Number != nil && w.Number.Cmp(old.Number) >= 0))
}

func (m *clientTLS) loadCRLFloors() error {
	path := filepath.Join(m.configFiles.InstanceIDDir, crlWatermarksFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("adsync: stat CRL watermarks: %w", err)
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return fmt.Errorf("%w: CRL watermarks must be a regular 0600 file", ErrUnsafeCache)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("adsync: read CRL watermarks: %w", err)
	}
	var envelope cacheEnvelope
	if err := decodeStrict(data, &envelope); err != nil {
		return fmt.Errorf("%w: CRL watermarks: %v", ErrCacheCorrupt, err)
	}
	if envelope.Format != crlWatermarksFormat || hashOf(envelope.Payload) != envelope.Checksum {
		return fmt.Errorf("%w: CRL watermark format/checksum", ErrCacheCorrupt)
	}
	var floors []crlWatermark
	if err := decodeStrict(envelope.Payload, &floors); err != nil {
		return fmt.Errorf("%w: CRL watermark payload: %v", ErrCacheCorrupt, err)
	}
	if len(floors) == 0 {
		return fmt.Errorf("%w: empty CRL watermarks", ErrCacheCorrupt)
	}
	for _, floor := range floors {
		if len(floor.Subject) == 0 || len(floor.PublicKey) == 0 || floor.ThisUpdate.IsZero() || (floor.Number != nil && floor.Number.Sign() < 0) {
			return fmt.Errorf("%w: invalid CRL watermark", ErrCacheCorrupt)
		}
	}
	m.crlFloors, m.floorPayload = floors, append([]byte(nil), envelope.Payload...)
	return nil
}

func (m *clientTLS) persistCRLFloors(next *clientTLSGeneration) error {
	if len(next.crls) == 0 {
		return nil
	}
	candidates := make([]crlWatermark, 0, len(next.crls))
	for _, crl := range next.crls {
		issuer := next.crlIssuers[crl]
		candidates = append(candidates, crlWatermark{Subject: []byte(issuer.subject), PublicKey: []byte(issuer.publicKey), ThisUpdate: crl.ThisUpdate, Number: crl.Number})
	}
	for _, old := range m.crlFloors {
		sameIssuer, covered := false, false
		for _, candidate := range candidates {
			if candidate.issuer() == old.issuer() {
				sameIssuer = true
				covered = covered || candidate.covers(old)
			}
		}
		if sameIssuer && !covered {
			return fmt.Errorf("adsync: refusing an older relay CRL")
		}
	}
	floors := append([]crlWatermark(nil), m.crlFloors...)
	for _, candidate := range candidates {
		covered := false
		for _, old := range floors {
			covered = covered || old.covers(candidate)
		}
		if covered {
			continue
		}
		retained := floors[:0]
		for _, old := range floors {
			if !candidate.covers(old) {
				retained = append(retained, old)
			}
		}
		floors = append(retained, candidate)
	}
	payload, err := json.Marshal(floors)
	if err != nil {
		return fmt.Errorf("adsync: encode CRL watermarks: %w", err)
	}
	if bytes.Equal(payload, m.floorPayload) {
		return nil
	}
	data, err := json.Marshal(cacheEnvelope{Format: crlWatermarksFormat, Checksum: hashOf(payload), Payload: payload})
	if err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(m.configFiles.InstanceIDDir, crlWatermarksFile), data); err != nil {
		return fmt.Errorf("adsync: persist CRL watermarks: %w", err)
	}
	m.crlFloors, m.floorPayload = floors, payload
	return nil
}
