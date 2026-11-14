// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const crlWatermarksFile = "crl-watermarks.json"
const crlWatermarksFormat = "speedle-adsync-crl-watermarks-v2"

// Preserve incomparable number/time pairs instead of inventing a combined
// watermark which no real CRL covers. Dominated pairs are discarded.
type crlWatermark struct {
	Subject    []byte    `json:"subject"`
	PublicKey  []byte    `json:"publicKey"`
	ThisUpdate time.Time `json:"thisUpdate"`
	Number     *big.Int  `json:"number,omitempty"`
	RawHash    Hash      `json:"rawHash"`
}

func (w crlWatermark) issuer() crlIssuer {
	return crlIssuer{subject: string(w.Subject), publicKey: string(w.PublicKey)}
}

func (w crlWatermark) covers(old crlWatermark) bool {
	if w.issuer() != old.issuer() || w.ThisUpdate.Before(old.ThisUpdate) ||
		(old.Number != nil && (w.Number == nil || w.Number.Cmp(old.Number) < 0)) {
		return false
	}
	newer := w.ThisUpdate.After(old.ThisUpdate) || (w.Number != nil && old.Number != nil && w.Number.Cmp(old.Number) > 0)
	return newer || w.RawHash == old.RawHash
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
		if len(floor.Subject) == 0 || len(floor.PublicKey) == 0 || floor.ThisUpdate.IsZero() || floor.RawHash.IsZero() || (floor.Number != nil && floor.Number.Sign() < 0) {
			return fmt.Errorf("%w: invalid CRL watermark", ErrCacheCorrupt)
		}
	}
	m.crlFloors, m.floorPayload = floors, append([]byte(nil), envelope.Payload...)
	return nil
}

// trustedCRLIssuers returns every certificate identity in the generation's
// explicit CAFile, matching the identity addCRLDER records for a CRL issuer.
func trustedCRLIssuers(caPEM []byte) map[crlIssuer]struct{} {
	issuers := make(map[crlIssuer]struct{})
	for data := caPEM; len(data) > 0; {
		block, rest := pem.Decode(data)
		data = rest
		if block == nil {
			break
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		issuers[crlIssuer{subject: string(certificate.RawSubject), publicKey: string(certificate.RawSubjectPublicKeyInfo)}] = struct{}{}
	}
	return issuers
}

// retainsFloor reports whether a verified chain issuer still has a retained
// CRL watermark. Path building can accept an issuer that is not itself listed
// in CAFile when it chains to a listed CA, so CAFile membership alone cannot
// decide whether the issuer is still trusted.
func (m *clientTLS) retainsFloor(issuer *x509.Certificate) bool {
	identity := crlIssuer{subject: string(issuer.RawSubject), publicKey: string(issuer.RawSubjectPublicKeyInfo)}
	for _, floor := range m.crlFloors {
		if floor.issuer() == identity {
			return true
		}
	}
	return false
}

// mayRetainFloorForName reports whether a retained floor could belong to an
// issuer whose certificate is absent from the verified chain. Floors are keyed
// by subject and key; the peer's issuer name rules out every other subject but
// cannot confirm the key, so a matching name is treated as possibly applicable.
func (m *clientTLS) mayRetainFloorForName(rawIssuer []byte) bool {
	for _, floor := range m.crlFloors {
		if bytes.Equal(floor.Subject, rawIssuer) {
			return true
		}
	}
	return false
}

func (m *clientTLS) persistCRLFloors(next *clientTLSGeneration, publish func() error) error {
	candidates := make([]crlWatermark, 0, len(next.crls))
	for _, crl := range next.crls {
		issuer := next.crlIssuers[crl]
		candidates = append(candidates, crlWatermark{Subject: []byte(issuer.subject), PublicKey: []byte(issuer.publicKey), ThisUpdate: crl.ThisUpdate, Number: crl.Number, RawHash: hashOf(crl.Raw)})
	}
	// A generation must keep covering every retained floor whose issuer is
	// still in CAFile. Otherwise removing a CRL source would silently accept
	// certificates that issuer can sign, and a generation with no CRLs at all
	// would skip revocation entirely. A floor outside the trust domain is
	// retained for a possible CA rollback but no longer constrains the
	// generation: it cannot sign a certificate that verifies against this
	// CAFile.
	trusted := trustedCRLIssuers(next.caPEM)
	for _, old := range m.crlFloors {
		if _, ok := trusted[old.issuer()]; !ok {
			continue
		}
		sameIssuer, covered := false, false
		for _, candidate := range candidates {
			if candidate.issuer() == old.issuer() {
				sameIssuer = true
				covered = covered || candidate.covers(old)
			}
		}
		if !sameIssuer {
			return fmt.Errorf("adsync: no configured relay CRL covers the retained floor for trusted issuer %x", old.Subject)
		}
		if !covered {
			return fmt.Errorf("adsync: refusing an older relay CRL or conflicting retained contents")
		}
	}
	if len(m.crlFloors) == 0 && len(candidates) == 0 {
		return publish()
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
		return publish()
	}
	data, err := json.Marshal(cacheEnvelope{Format: crlWatermarksFormat, Checksum: hashOf(payload), Payload: payload})
	if err != nil {
		return err
	}
	if err := writeFileAtomicChecked(filepath.Join(m.configFiles.InstanceIDDir, crlWatermarksFile), data, publish); err != nil {
		return fmt.Errorf("adsync: persist CRL watermarks: %w", err)
	}
	m.crlFloors, m.floorPayload = floors, payload
	return nil
}
