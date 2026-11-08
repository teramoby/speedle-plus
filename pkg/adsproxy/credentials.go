// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsproxy

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const (
	// maxCredentialFileBytes bounds the downstream credential file.
	maxCredentialFileBytes = 1 << 20
	// maxCredentialLines bounds how many downstreams one file may declare.
	maxCredentialLines = 1000
	// maxDownstreamIDChars bounds the operational name of a downstream.
	maxDownstreamIDChars = 64
	// tokenHashHexChars is the length of a SHA-256 digest in lowercase hex.
	tokenHashHexChars = 64
)

var downstreamIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Credential is one downstream instance's access grant. It authenticates with
// a bearer token and may read only its service subset.
type Credential struct {
	// DownstreamID is the operator-visible name from the credential file. The
	// fan-out limit counts distinct downstream ids.
	DownstreamID string
	// Token is the plaintext bearer token. It is used only to build an
	// in-memory CredentialSet; the file loader stores the digest instead and
	// never keeps the plaintext.
	Token string
	// Services is the service subset the credential may read. Empty or ["*"]
	// means every service the proxy serves.
	Services []string

	digest  [sha256.Size]byte
	allowed map[string]struct{}
	all     bool
}

// Allows reports whether the credential may read one service.
func (c *Credential) Allows(serviceID string) bool {
	if c == nil {
		return false
	}
	if c.all {
		return true
	}
	_, found := c.allowed[serviceID]
	return found
}

// ServicesList returns the configured subset, or ["*"] for every service.
func (c *Credential) ServicesList() []string {
	if c == nil {
		return nil
	}
	if c.all {
		return []string{"*"}
	}
	services := make([]string, 0, len(c.allowed))
	for serviceID := range c.allowed {
		services = append(services, serviceID)
	}
	sort.Strings(services)
	return services
}

// CredentialSet is the immutable-after-load set of downstream credentials.
// Reload replaces it atomically; a failed reload keeps the previous set.
type CredentialSet struct {
	path string

	mu          sync.RWMutex
	credentials []*Credential
	byID        map[string]*Credential
}

// LoadCredentialFile reads a downstream credential file. The file must be a
// regular 0600 file; each non-empty, non-comment line is:
//
//	downstreamId:tokenHash:serviceIds
//
// tokenHash is the lowercase hexadecimal SHA-256 digest of the bearer token.
// serviceIds is * or a comma-separated list of service ULIDs.
func LoadCredentialFile(path string) (*CredentialSet, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: no path", ErrCredentialFile)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCredentialFile, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular file", ErrCredentialFile, path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: %q must be readable and writable only by its owner", ErrCredentialFile, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCredentialFile, err)
	}
	if len(data) > maxCredentialFileBytes {
		return nil, fmt.Errorf("%w: file is too large", ErrCredentialFile)
	}
	credentials, byID, err := parseCredentialFile(string(data))
	if err != nil {
		return nil, err
	}
	return &CredentialSet{path: path, credentials: credentials, byID: byID}, nil
}

// NewCredentialSet builds an in-memory credential set, for callers that do not
// use a credential file or for tests. Each Token is hashed and not retained.
func NewCredentialSet(credentials []Credential) (*CredentialSet, error) {
	set := &CredentialSet{
		credentials: make([]*Credential, 0, len(credentials)),
		byID:        make(map[string]*Credential, len(credentials)),
	}
	digests := make(map[[sha256.Size]byte]string, len(credentials))
	for index := range credentials {
		credential, err := buildCredential(&credentials[index])
		if err != nil {
			return nil, err
		}
		if _, duplicate := set.byID[credential.DownstreamID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate downstream id %q", ErrCredentialFile, credential.DownstreamID)
		}
		if other, duplicate := digests[credential.digest]; duplicate {
			return nil, fmt.Errorf("%w: downstreams %q and %q share a token",
				ErrCredentialFile, other, credential.DownstreamID)
		}
		digests[credential.digest] = credential.DownstreamID
		set.byID[credential.DownstreamID] = credential
		set.credentials = append(set.credentials, credential)
	}
	return set, nil
}

// Path returns the configured credential file path, if any.
func (s *CredentialSet) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Len returns the number of credentials in the set.
func (s *CredentialSet) Len() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.credentials)
}

// Authenticate returns the credential matching a bearer token. The comparison
// runs over every credential in constant time per digest, so a wrong token does
// not reveal how close it was.
func (s *CredentialSet) Authenticate(token string) (*Credential, bool) {
	if s == nil || token == "" || len(token) > 512 {
		return nil, false
	}
	digest := sha256.Sum256([]byte(token))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, credential := range s.credentials {
		if subtle.ConstantTimeCompare(digest[:], credential.digest[:]) == 1 {
			return credential, true
		}
	}
	return nil, false
}

// Reload re-reads the credential file and replaces the set atomically. On
// error the previous set stays in force.
func (s *CredentialSet) Reload() error {
	if s == nil || s.path == "" {
		return fmt.Errorf("%w: no credential file configured", ErrCredentialFile)
	}
	loaded, err := LoadCredentialFile(s.path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.credentials = loaded.credentials
	s.byID = loaded.byID
	s.mu.Unlock()
	return nil
}

func parseCredentialFile(data string) ([]*Credential, map[string]*Credential, error) {
	var (
		credentials []*Credential
		byID        = make(map[string]*Credential)
		digests     = make(map[[sha256.Size]byte]string)
	)
	lines := strings.Split(data, "\n")
	if len(lines) > maxCredentialLines {
		return nil, nil, fmt.Errorf("%w: too many lines", ErrCredentialFile)
	}
	for lineNumber, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			return nil, nil, fmt.Errorf("%w: line %d is not downstreamId:tokenHash:serviceIds",
				ErrCredentialFile, lineNumber+1)
		}
		downstreamID := strings.TrimSpace(parts[0])
		hashText := strings.TrimSpace(parts[1])
		servicesText := strings.TrimSpace(parts[2])

		if err := validateDownstreamID(downstreamID); err != nil {
			return nil, nil, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		digest, err := parseTokenHash(hashText)
		if err != nil {
			return nil, nil, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		services, all, err := parseServiceSubset(servicesText)
		if err != nil {
			return nil, nil, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if _, duplicate := byID[downstreamID]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate downstream id %q", ErrCredentialFile, downstreamID)
		}
		if other, duplicate := digests[digest]; duplicate {
			return nil, nil, fmt.Errorf("%w: downstreams %q and %q share a token",
				ErrCredentialFile, other, downstreamID)
		}
		credential := &Credential{
			DownstreamID: downstreamID,
			Services:     servicesList(services, all),
			digest:       digest,
			allowed:      services,
			all:          all,
		}
		byID[downstreamID] = credential
		digests[digest] = downstreamID
		credentials = append(credentials, credential)
	}
	if len(credentials) == 0 {
		return nil, nil, fmt.Errorf("%w: no credentials", ErrCredentialFile)
	}
	return credentials, byID, nil
}

func buildCredential(credential *Credential) (*Credential, error) {
	if err := validateDownstreamID(credential.DownstreamID); err != nil {
		return nil, err
	}
	if credential.Token == "" {
		return nil, fmt.Errorf("%w: downstream %q has no token", ErrCredentialFile, credential.DownstreamID)
	}
	if strings.IndexFunc(credential.Token, unicode.IsSpace) >= 0 {
		return nil, fmt.Errorf("%w: downstream %q token contains whitespace", ErrCredentialFile, credential.DownstreamID)
	}
	services, all, err := normalizeServiceSubset(credential.Services)
	if err != nil {
		return nil, fmt.Errorf("downstream %q: %w", credential.DownstreamID, err)
	}
	built := &Credential{
		DownstreamID: credential.DownstreamID,
		Services:     servicesList(services, all),
		digest:       sha256.Sum256([]byte(credential.Token)),
		allowed:      services,
		all:          all,
	}
	return built, nil
}

func validateDownstreamID(id string) error {
	if !downstreamIDPattern.MatchString(id) || len(id) > maxDownstreamIDChars {
		return fmt.Errorf("%w: invalid downstream id %q", ErrCredentialFile, id)
	}
	return nil
}

func parseTokenHash(text string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	if len(text) != tokenHashHexChars {
		return digest, fmt.Errorf("%w: token hash must be %d hexadecimal characters",
			ErrCredentialFile, tokenHashHexChars)
	}
	raw, err := hex.DecodeString(text)
	if err != nil {
		return digest, fmt.Errorf("%w: token hash is not hexadecimal", ErrCredentialFile)
	}
	copy(digest[:], raw)
	return digest, nil
}

func parseServiceSubset(text string) (map[string]struct{}, bool, error) {
	if text == "*" {
		return nil, true, nil
	}
	if text == "" {
		return nil, false, fmt.Errorf("%w: empty service subset", ErrCredentialFile)
	}
	entries := strings.Split(text, ",")
	services := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if !validEntityID(entry) {
			return nil, false, fmt.Errorf("%w: invalid service id %q", ErrCredentialFile, entry)
		}
		if _, duplicate := services[entry]; duplicate {
			return nil, false, fmt.Errorf("%w: duplicate service id %q", ErrCredentialFile, entry)
		}
		services[entry] = struct{}{}
	}
	return services, false, nil
}

func normalizeServiceSubset(entries []string) (map[string]struct{}, bool, error) {
	if len(entries) == 0 {
		return nil, true, nil
	}
	services := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry == "*" {
			if len(entries) != 1 {
				return nil, false, fmt.Errorf("%w: * cannot be combined with explicit services", ErrCredentialFile)
			}
			return nil, true, nil
		}
		if !validEntityID(entry) {
			return nil, false, fmt.Errorf("%w: invalid service id %q", ErrCredentialFile, entry)
		}
		if _, duplicate := services[entry]; duplicate {
			return nil, false, fmt.Errorf("%w: duplicate service id %q", ErrCredentialFile, entry)
		}
		services[entry] = struct{}{}
	}
	return services, false, nil
}

func servicesList(services map[string]struct{}, all bool) []string {
	if all {
		return []string{"*"}
	}
	list := make([]string, 0, len(services))
	for serviceID := range services {
		list = append(list, serviceID)
	}
	sort.Strings(list)
	return list
}
