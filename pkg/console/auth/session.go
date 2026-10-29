//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package auth signs in console users and decides what they may do.
package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"os"
	"strings"
	"time"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/subjectutils"
)

// MinKeySize is the shortest session signing key accepted.
const MinKeySize = 32

var (
	// ErrInvalidSession means the cookie was not issued by this console.
	ErrInvalidSession = stderrors.New("the session is not valid")
	// ErrExpiredSession means the session has ended.
	ErrExpiredSession = stderrors.New("the session has expired")
)

// Session is what the console knows about a signed-in user. It travels in a
// signed cookie, so the server keeps no state; the login token is not kept.
type Session struct {
	Principals []*ads.Principal `json:"p"`
	Expires    int64            `json:"e"`
	CSRF       string           `json:"c"`
}

// PrincipalString formats a principal as policies write it:
// [idd=<IDD>:]<type>:<name>.
func PrincipalString(p *ads.Principal) string {
	return subjectutils.EncodePrincipal(p)
}

// Strings returns the session's principals as policies write them.
func (s *Session) Strings() []string {
	out := make([]string, 0, len(s.Principals))
	for _, p := range s.Principals {
		out = append(out, PrincipalString(p))
	}
	return out
}

// User returns the name PMS records as the author of a change: the first user
// principal, as ADS does, or else the first principal.
func (s *Session) User() string {
	for _, p := range s.Principals {
		if p.Type == ads.PRINCIPAL_TYPE_USER {
			return p.Name
		}
	}
	if len(s.Principals) > 0 {
		return PrincipalString(s.Principals[0])
	}
	return ""
}

// SessionCodec issues sessions and signs them with HMAC-SHA256.
type SessionCodec struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewSessionCodec returns a codec whose sessions last ttl.
func NewSessionCodec(key []byte, ttl time.Duration) (*SessionCodec, error) {
	if len(key) < MinKeySize {
		return nil, errors.Errorf(errors.ConfigError, "the session key must have at least %d bytes", MinKeySize)
	}
	if ttl <= 0 {
		return nil, errors.New(errors.ConfigError, "the session lifetime must be positive")
	}
	return &SessionCodec{key: key, ttl: ttl, now: time.Now}, nil
}

// TTL is how long a session lasts.
func (c *SessionCodec) TTL() time.Duration { return c.ttl }

// New starts a session for principals with a fresh CSRF token.
func (c *SessionCodec) New(principals []*ads.Principal) (*Session, error) {
	csrf, err := randomString(32)
	if err != nil {
		return nil, err
	}
	return &Session{Principals: principals, Expires: c.now().Add(c.ttl).Unix(), CSRF: csrf}, nil
}

func (c *SessionCodec) sign(payload string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Encode returns the cookie value of a session.
func (c *SessionCodec) Encode(s *Session) (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", errors.Wrap(err, errors.SerializationError, "failed to encode the session")
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + c.sign(payload), nil
}

// Decode checks the signature and lifetime of a cookie value.
func (c *SessionCodec) Decode(value string) (*Session, error) {
	payload, signature, ok := strings.Cut(value, ".")
	if !ok || !hmac.Equal([]byte(signature), []byte(c.sign(payload))) {
		return nil, ErrInvalidSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, ErrInvalidSession
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, ErrInvalidSession
	}
	if c.now().Unix() >= s.Expires {
		return nil, ErrExpiredSession
	}
	if len(s.Principals) == 0 || s.CSRF == "" {
		return nil, ErrInvalidSession
	}
	return &s, nil
}

// LoadKey reads a signing key from path, or returns a random key when path is
// empty. generated reports the latter: such a key only suits one replica,
// and sessions end when the console restarts.
func LoadKey(path string) (key []byte, generated bool, err error) {
	if path == "" {
		key = make([]byte, MinKeySize)
		if _, err := rand.Read(key); err != nil {
			return nil, false, errors.Wrap(err, errors.ConfigError, "failed to generate a session key")
		}
		return key, true, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, errors.Wrapf(err, errors.ConfigError, "unable to read the session key from %s", path)
	}
	key = bytes.TrimSpace(raw)
	if len(key) < MinKeySize {
		return nil, false, errors.Errorf(errors.ConfigError, "the session key in %s must have at least %d bytes", path, MinKeySize)
	}
	return key, false, nil
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrap(err, errors.ServerError, "failed to generate a random token")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
