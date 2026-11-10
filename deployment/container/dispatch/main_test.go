// Licensed under the Universal Permissive License (UPL) Version 1.0.

package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestNoCommandPrintsTheImageCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	code := run(nil, &stdout, &stderr, func(string, []string, []string) error {
		called = true
		return nil
	})
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if called {
		t.Fatal("no command must not exec a service")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %q", stderr.String())
	}
	if stdout.String() != "speedle-pms\nspeedle-ads\nspctl\n" {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestKnownCommandReplacesTheProcess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var gotPath string
	var gotArgv []string
	var gotEnv []string
	code := run([]string{"speedle-ads", "--endpoint", "0.0.0.0:6734"}, &stdout, &stderr,
		func(path string, argv []string, env []string) error {
			gotPath = path
			gotArgv = append([]string(nil), argv...)
			gotEnv = env
			return nil
		})
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if gotPath != "/speedle-ads" {
		t.Fatalf("path %s", gotPath)
	}
	if strings.Join(gotArgv, "\x00") != "speedle-ads\x00--endpoint\x000.0.0.0:6734" {
		t.Fatalf("argv %#v", gotArgv)
	}
	if len(gotEnv) == 0 {
		t.Fatal("exec environment is empty")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

func TestExecFailureIsReported(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"spctl"}, io.Discard, &stderr, func(string, []string, []string) error {
		return errors.New("boom")
	})
	if code != 127 {
		t.Fatalf("exit %d, want 127", code)
	}
	if !strings.Contains(stderr.String(), "exec /spctl: boom") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestUnknownCommandPrintsTheImageCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"speedle"}, &stdout, &stderr, func(string, []string, []string) error {
		t.Fatal("unknown command must not exec")
		return nil
	})
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown command: speedle\n") ||
		!strings.Contains(stderr.String(), "speedle-pms\nspeedle-ads\nspctl\n") {
		t.Fatalf("stderr %q", stderr.String())
	}
}
