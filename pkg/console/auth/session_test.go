//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package auth

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teramoby/speedle-plus/api/ads"
)

var testKey = []byte(strings.Repeat("k", MinKeySize))

func newCodec(t *testing.T) *SessionCodec {
	t.Helper()
	c, err := NewSessionCodec(testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var alice = []*ads.Principal{{Type: "user", Name: "alice"}, {Type: "group", Name: "ops", IDD: "corp"}}

func TestSessionRoundTrip(t *testing.T) {
	c := newCodec(t)
	s, err := c.New(alice)
	if err != nil {
		t.Fatal(err)
	}
	value, err := c.Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Decode(value)
	if err != nil {
		t.Fatal(err)
	}
	if got.CSRF != s.CSRF || got.User() != "alice" || strings.Join(got.Strings(), ",") != "user:alice,idd=corp:group:ops" {
		t.Errorf("decoded %+v", got)
	}
	if c.TTL() != time.Hour {
		t.Errorf("TTL %v", c.TTL())
	}
}

func TestSessionRejectsForgeries(t *testing.T) {
	c := newCodec(t)
	s, _ := c.New(alice)
	value, _ := c.Encode(s)
	payload, signature, _ := strings.Cut(value, ".")
	other, _ := NewSessionCodec([]byte(strings.Repeat("x", MinKeySize)), time.Hour)
	signed := func(raw string) string {
		p := base64.RawURLEncoding.EncodeToString([]byte(raw))
		return p + "." + c.sign(p)
	}
	foreign, _ := other.Encode(s)
	for name, v := range map[string]string{
		"no signature":     payload,
		"bad signature":    payload + "." + signature[1:],
		"other key":        foreign,
		"not base64":       "!!!." + c.sign("!!!"),
		"not JSON":         signed("{"),
		"no principals":    signed(`{"e":9999999999,"c":"x"}`),
		"no CSRF token":    signed(`{"p":[{"type":"user","name":"a"}],"e":9999999999}`),
		"already expired":  signed(`{"p":[{"type":"user","name":"a"}],"e":1,"c":"x"}`),
		"empty cookie":     "",
		"swapped payloads": signed(`{"p":[{"type":"user","name":"root"}],"e":1,"c":"x"}`)[:10] + "." + signature,
	} {
		if _, err := c.Decode(v); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := c.Decode(value); err != ErrExpiredSession {
		t.Errorf("expired session: %v", err)
	}
}

func TestNewSessionCodecChecksSettings(t *testing.T) {
	if _, err := NewSessionCodec([]byte("short"), time.Hour); err == nil {
		t.Error("a short key was accepted")
	}
	if _, err := NewSessionCodec(testKey, 0); err == nil {
		t.Error("a zero lifetime was accepted")
	}
}

func TestSessionUser(t *testing.T) {
	for _, c := range []struct {
		principals []*ads.Principal
		want       string
	}{
		{alice, "alice"},
		{[]*ads.Principal{{Type: "group", Name: "ops"}}, "group:ops"},
		{nil, ""},
	} {
		if got := (&Session{Principals: c.principals}).User(); got != c.want {
			t.Errorf("User() = %q, want %q", got, c.want)
		}
	}
}

func TestLoadKey(t *testing.T) {
	key, generated, err := LoadKey("")
	if err != nil || !generated || len(key) != MinKeySize {
		t.Errorf("generated key %d bytes, %v, %v", len(key), generated, err)
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	os.WriteFile(good, append(testKey, '\n'), 0600)
	key, generated, err = LoadKey(good)
	if err != nil || generated || string(key) != string(testKey) {
		t.Errorf("loaded %q, %v, %v", key, generated, err)
	}
	short := filepath.Join(dir, "short")
	os.WriteFile(short, []byte("short"), 0600)
	if _, _, err := LoadKey(short); err == nil {
		t.Error("a short key file was accepted")
	}
	if _, _, err := LoadKey(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing key file was accepted")
	}
}
