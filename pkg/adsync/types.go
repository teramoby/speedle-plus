// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"unicode/utf8"
)

const (
	// Protocol bounds from the shared contract. The client enforces them so a
	// malformed or hostile response cannot allocate unbounded memory.
	maxServicesPerSource  = 10
	maxPoliciesPerList    = 1000
	maxChangesPerCommit   = 200
	maxCommitsPerPage     = 100
	maxPolicyIDCharacters = 128
	maxPolicyNameChars    = 128
	maxSPDLBytes          = 4 * 1024
	maxSafeInteger        = 1<<53 - 1
)

var (
	serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
	ulidPattern        = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
	uuidV7Pattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

// Policy is one authz policy or role policy in an Engine snapshot.
type Policy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	SPDL string `json:"spdl"`
}

// Snapshot is the EngineSnapshotSchema payload carried by a signed snapshot.
type Snapshot struct {
	Name         string   `json:"name"`
	Policies     []Policy `json:"policies"`
	RolePolicies []Policy `json:"rolePolicies"`
}

// Signature is the Cloud-produced Ed25519 signature. Proxies forward it
// unchanged.
type Signature struct {
	KeyID string `json:"keyId"`
	Alg   string `json:"alg"`
	Value string `json:"value"`
}

// Service is one service as reported by the source, either by a discovery
// response or as one entry of a confirming ack. Revision is the source's head
// for the service.
type Service struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Revision      uint64 `json:"revision"`
	Epoch         string `json:"epoch"`
	HeadStateHash Hash   `json:"headStateHash"`
}

// ServicesResponse is GET /policy-source/v1/services.
type ServicesResponse struct {
	ServicesVersion int64     `json:"servicesVersion"`
	Services        []Service `json:"services"`
}

// SnapshotResponse is GET /policy-source/v1/services/{serviceId}/snapshot.
type SnapshotResponse struct {
	ServiceID  string    `json:"serviceId"`
	SourceID   string    `json:"sourceId"`
	Epoch      string    `json:"epoch"`
	Revision   uint64    `json:"revision"`
	StateHash  Hash      `json:"stateHash"`
	CommitHash Hash      `json:"commitHash"`
	Signature  Signature `json:"signature"`
	Snapshot   Snapshot  `json:"snapshot"`
}

// Change is one upsert or delete in a commit. Name and SPDL are pointers so
// validation can distinguish "absent" from "present but empty"; the contract
// requires a name on every upsert and an SPDL on authz and role upserts.
type Change struct {
	Op   string  `json:"op"`
	Kind string  `json:"kind"`
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
	SPDL *string `json:"spdl,omitempty"`
}

// Commit is one signed revision.
type Commit struct {
	Revision  uint64    `json:"revision"`
	PrevHash  Hash      `json:"prevHash"`
	Hash      Hash      `json:"hash"`
	At        int64     `json:"at"`
	Signature Signature `json:"signature"`
	Changes   []Change  `json:"changes"`
}

// ChangesResponse is GET /policy-source/v1/services/{serviceId}/changes.
type ChangesResponse struct {
	ServiceID     string   `json:"serviceId"`
	SourceID      string   `json:"sourceId"`
	Epoch         string   `json:"epoch"`
	From          uint64   `json:"from"`
	To            uint64   `json:"to"`
	Head          uint64   `json:"head"`
	HeadStateHash Hash     `json:"headStateHash"`
	Commits       []Commit `json:"commits"`
	NextSince     uint64   `json:"nextSince"`
}

func validServiceName(name string) bool {
	return serviceNamePattern.MatchString(name)
}

func validEntityID(id string) bool {
	return ulidPattern.MatchString(id) || uuidV7Pattern.MatchString(id)
}

func validRevision(revision uint64) bool {
	return revision <= maxSafeInteger
}

func (s Service) validate() error {
	if !validEntityID(s.ID) {
		return fmt.Errorf("%w: invalid service id %q", ErrProtocol, s.ID)
	}
	if !validServiceName(s.Name) {
		return fmt.Errorf("%w: invalid service name %q", ErrProtocol, s.Name)
	}
	if !validRevision(s.Revision) {
		return fmt.Errorf("%w: service %q revision %d is out of range", ErrProtocol, s.ID, s.Revision)
	}
	if !validEntityID(s.Epoch) {
		return fmt.Errorf("%w: invalid epoch %q", ErrProtocol, s.Epoch)
	}
	if s.HeadStateHash.IsZero() {
		return fmt.Errorf("%w: service %q has no headStateHash", ErrProtocol, s.ID)
	}
	return nil
}

func (r *ServicesResponse) validate() error {
	if r.ServicesVersion <= 0 || r.ServicesVersion > maxSafeInteger {
		return fmt.Errorf("%w: invalid servicesVersion %d", ErrProtocol, r.ServicesVersion)
	}
	if len(r.Services) > maxServicesPerSource {
		return fmt.Errorf("%w: %d services exceed the protocol bound", ErrProtocol, len(r.Services))
	}
	seen := make(map[string]struct{}, len(r.Services))
	for index := range r.Services {
		if err := r.Services[index].validate(); err != nil {
			return err
		}
		if _, duplicate := seen[r.Services[index].ID]; duplicate {
			return fmt.Errorf("%w: duplicate service id %q", ErrProtocol, r.Services[index].ID)
		}
		seen[r.Services[index].ID] = struct{}{}
	}
	return nil
}

func (s Signature) validate() error {
	if s.KeyID == "" || len(s.KeyID) > 64 {
		return fmt.Errorf("%w: invalid signature keyId", ErrProtocol)
	}
	if s.Alg != "Ed25519" {
		return fmt.Errorf("%w: unsupported signature algorithm %q", ErrProtocol, s.Alg)
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(s.Value)
	if err != nil || len(raw) != 64 {
		return fmt.Errorf("%w: invalid signature value", ErrProtocol)
	}
	if base64.StdEncoding.EncodeToString(raw) != s.Value {
		return fmt.Errorf("%w: non-canonical signature value", ErrProtocol)
	}
	return nil
}

func (s Snapshot) validate() error {
	if !validServiceName(s.Name) {
		return fmt.Errorf("%w: invalid snapshot service name %q", ErrProtocol, s.Name)
	}
	if err := validatePolicies("policy", s.Policies); err != nil {
		return err
	}
	return validatePolicies("role policy", s.RolePolicies)
}

func validatePolicies(label string, policies []Policy) error {
	if len(policies) > maxPoliciesPerList {
		return fmt.Errorf("%w: %d %ss exceed the protocol bound", ErrProtocol, len(policies), label)
	}
	seen := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		characters := utf8.RuneCountInString(policy.ID)
		if characters < 1 || characters > maxPolicyIDCharacters {
			return fmt.Errorf("%w: %s id has %d characters", ErrProtocol, label, characters)
		}
		if utf8.RuneCountInString(policy.Name) > maxPolicyNameChars {
			return fmt.Errorf("%w: %s %q name exceeds the character limit", ErrProtocol, label, policy.ID)
		}
		if policy.SPDL == "" || len(policy.SPDL) > maxSPDLBytes {
			return fmt.Errorf("%w: %s %q SPDL is empty or too large", ErrProtocol, label, policy.ID)
		}
		if _, duplicate := seen[policy.ID]; duplicate {
			return fmt.Errorf("%w: duplicate %s id %q", ErrProtocol, label, policy.ID)
		}
		seen[policy.ID] = struct{}{}
	}
	return nil
}

func (r *SnapshotResponse) validate() error {
	if !validEntityID(r.ServiceID) {
		return fmt.Errorf("%w: invalid serviceId %q", ErrProtocol, r.ServiceID)
	}
	if !validEntityID(r.SourceID) {
		return fmt.Errorf("%w: invalid sourceId %q", ErrProtocol, r.SourceID)
	}
	if !validEntityID(r.Epoch) {
		return fmt.Errorf("%w: invalid epoch %q", ErrProtocol, r.Epoch)
	}
	if !validRevision(r.Revision) {
		return fmt.Errorf("%w: revision %d is out of range", ErrProtocol, r.Revision)
	}
	if r.StateHash.IsZero() || r.CommitHash.IsZero() {
		return fmt.Errorf("%w: snapshot hashes are missing", ErrProtocol)
	}
	if err := r.Signature.validate(); err != nil {
		return err
	}
	return r.Snapshot.validate()
}

func (c *Change) validate() error {
	switch c.Op {
	case changeOpUpsertName:
	case changeOpDeleteName:
		if c.Name != nil || c.SPDL != nil {
			return fmt.Errorf("%w: delete carries only op, kind and id", ErrProtocol)
		}
		return c.validateIDAndKind()
	default:
		return fmt.Errorf("%w: unknown change op %q", ErrProtocol, c.Op)
	}

	if err := c.validateIDAndKind(); err != nil {
		return err
	}
	if c.Name == nil {
		return fmt.Errorf("%w: upsert requires a name", ErrProtocol)
	}
	if utf8.RuneCountInString(*c.Name) > maxPolicyNameChars {
		return fmt.Errorf("%w: change name exceeds the character limit", ErrProtocol)
	}
	if c.Kind == changeKindServiceName {
		if !validServiceName(*c.Name) {
			return fmt.Errorf("%w: invalid service name %q", ErrProtocol, *c.Name)
		}
		if c.SPDL != nil {
			return fmt.Errorf("%w: service changes carry no SPDL", ErrProtocol)
		}
		return nil
	}
	if c.SPDL == nil || *c.SPDL == "" || len(*c.SPDL) > maxSPDLBytes {
		return fmt.Errorf("%w: authz and role upserts require SPDL", ErrProtocol)
	}
	return nil
}

func (c *Change) validateIDAndKind() error {
	characters := utf8.RuneCountInString(c.ID)
	if characters < 1 || characters > maxPolicyIDCharacters {
		return fmt.Errorf("%w: change id has %d characters", ErrProtocol, characters)
	}
	switch c.Kind {
	case changeKindAuthzName, changeKindRoleName, changeKindServiceName:
		return nil
	default:
		return fmt.Errorf("%w: unknown change kind %q", ErrProtocol, c.Kind)
	}
}

// validatePage checks the envelope, contiguity and hash chain of one changes
// page against the caller's current anchor. Signatures are verified by the
// caller because they need the configured public keys.
func (r *ChangesResponse) validatePage(serviceID, sourceID, epoch string, since uint64, anchor Hash, limit int) error {
	if r.ServiceID != serviceID {
		return fmt.Errorf("%w: changes page is for service %q, want %q", ErrProtocol, r.ServiceID, serviceID)
	}
	if r.SourceID != sourceID {
		return fmt.Errorf("%w: changes page source %q, want %q", ErrProtocol, r.SourceID, sourceID)
	}
	if r.Epoch != epoch {
		return fmt.Errorf("%w: changes page epoch %q, want %q", ErrProtocol, r.Epoch, epoch)
	}
	if !validRevision(r.From) || !validRevision(r.To) || !validRevision(r.Head) || !validRevision(r.NextSince) {
		return fmt.Errorf("%w: revision out of range in changes page", ErrProtocol)
	}
	if since >= maxSafeInteger || r.From != since+1 {
		return fmt.Errorf("%w: changes page starts at %d, want %d", ErrProtocol, r.From, since+1)
	}
	if r.To > r.Head {
		return fmt.Errorf("%w: changes page ends after the head", ErrProtocol)
	}
	if r.NextSince != r.To {
		return fmt.Errorf("%w: nextSince %d does not equal the last returned revision %d", ErrProtocol, r.NextSince, r.To)
	}
	if r.HeadStateHash.IsZero() {
		return fmt.Errorf("%w: changes page has no headStateHash", ErrProtocol)
	}
	if len(r.Commits) > maxCommitsPerPage || (limit > 0 && len(r.Commits) > limit) {
		return fmt.Errorf("%w: changes page has too many commits", ErrProtocol)
	}

	if len(r.Commits) == 0 {
		if r.To != r.From-1 {
			return fmt.Errorf("%w: empty changes page ends at %d, want %d", ErrProtocol, r.To, r.From-1)
		}
		if r.Head != r.To {
			return fmt.Errorf("%w: empty changes page hides commits up to %d", ErrProtocol, r.Head)
		}
		return nil
	}

	if r.Commits[0].Revision != r.From {
		return fmt.Errorf("%w: first commit is at %d, want %d", ErrProtocol, r.Commits[0].Revision, r.From)
	}
	if r.Commits[len(r.Commits)-1].Revision != r.To {
		return fmt.Errorf("%w: last commit is at %d, want %d", ErrProtocol, r.Commits[len(r.Commits)-1].Revision, r.To)
	}
	previousHash := anchor
	for index := range r.Commits {
		commit := &r.Commits[index]
		if commit.Revision != r.From+uint64(index) {
			return fmt.Errorf("%w: revisions are not contiguous at %d", ErrProtocol, commit.Revision)
		}
		if commit.PrevHash != previousHash {
			return fmt.Errorf("%w: commit %d prevHash does not chain", ErrProtocol, commit.Revision)
		}
		if commit.Hash.IsZero() {
			return fmt.Errorf("%w: commit %d has no hash", ErrProtocol, commit.Revision)
		}
		if commit.At < 0 || commit.At > maxSafeInteger {
			return fmt.Errorf("%w: commit %d has an invalid time", ErrProtocol, commit.Revision)
		}
		if len(commit.Changes) > maxChangesPerCommit {
			return fmt.Errorf("%w: commit %d has too many changes", ErrProtocol, commit.Revision)
		}
		for changeIndex := range commit.Changes {
			if err := commit.Changes[changeIndex].validate(); err != nil {
				return fmt.Errorf("commit %d: %w", commit.Revision, err)
			}
		}
		computed, err := CommitHash(commit.PrevHash, commit.Changes)
		if err != nil {
			return err
		}
		if computed != commit.Hash {
			return fmt.Errorf("%w: commit %d hash does not match its changes", ErrIntegrity, commit.Revision)
		}
		previousHash = commit.Hash
	}
	return nil
}

// nameOrEmpty returns the change name with the contract's absent-to-empty
// encoding.
func (c Change) nameOrEmpty() string {
	if c.Name == nil {
		return ""
	}
	return *c.Name
}

func (c Change) spdlOrEmpty() string {
	if c.SPDL == nil {
		return ""
	}
	return *c.SPDL
}

// snapshotIdentity describes the exact state a snapshot signed.
func (r *SnapshotResponse) signedSnapshotBytes() []byte {
	return SnapshotSignedBytes(r.SourceID, r.Epoch, r.Revision, r.StateHash, r.CommitHash)
}

func (c *Commit) signedBytes(sourceID, epoch string) []byte {
	return CommitSignedBytes(sourceID, epoch, c.Revision, c.Hash)
}

func isServiceChange(change Change) bool {
	return change.Kind == changeKindServiceName
}
