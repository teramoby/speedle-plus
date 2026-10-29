//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func check(t *testing.T, args, files []string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := runCheck(args, files, &out)
	return out.String(), code
}

func writeCheckFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckPolicyLine(t *testing.T) {
	cases := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"policy", "grant user alice approve /orders if amount <= 1000"}, 0, "policy OK\n"},
		// The policy may also be given unquoted.
		{[]string{"rolepolicy", "grant", "user", "alice", "manager", "on", "/orders"}, 0, "rolepolicy OK\n"},
		{[]string{"policy", "grant user alice approve /orders if amount <="}, 1,
			"Invalid condition: Unexpected end of expression\ngrant user alice approve /orders if amount <=\n                                             ^\n"},
		{[]string{"rolepolicy", "grant user alice manager on /orders if (x"}, 1,
			"Invalid condition: Unbalanced parenthesis\ngrant user alice manager on /orders if (x\n                                       ^\n"},
		{[]string{"policy"}, 1, "policy: give the policy to check after it\n"},
		{[]string{"service", "x"}, 1, "unknown kind \"service\": use policy or rolepolicy\n"},
	}
	for _, c := range cases {
		if out, code := check(t, c.args, nil); code != c.code || out != c.want {
			t.Errorf("check %q: exit %d, output %q; want exit %d, %q", c.args, code, out, c.code, c.want)
		}
	}
}

func TestCheckFiles(t *testing.T) {
	good := writeCheckFile(t, "good.spdl", "[service.a]\n[policy]\nGRANT USER alice read /x if n < 3\n[rolepolicy]\nGRANT USER bob r on /x\n")
	bad := writeCheckFile(t, "bad.spdl", "[service.a]\n[policy]\nGRANT USER alice read /x if n <\nGRAN USER bob read /x\n")
	goodJSON := writeCheckFile(t, "store.json", `{"services":[{"name":"a","policies":[{"effect":"grant","permissions":[{"resource":"/x","actions":["read"]}]}]}]}`)
	badJSON := writeCheckFile(t, "bad.json", `{"services":[{"name":"a","policies":[{"effect":"Deny","permissions":[{"resource":"/x"}]}]}]}`)

	out, code := check(t, nil, []string{good, goodJSON})
	if code != 0 || out != good+": OK, 1 service(s), 1 policies, 1 role policies\n"+goodJSON+": OK, 1 service(s), 1 policies, 0 role policies\n" {
		t.Errorf("good files: exit %d\n%s", code, out)
	}

	// Every file is checked, and every error in each is listed.
	out, code = check(t, nil, []string{bad, good, badJSON})
	for _, want := range []string{
		bad + ":3: Invalid condition: Unexpected end of expression",
		bad + ":4: Not found valid effect",
		bad + ": 2 error(s)",
		good + ": OK",
		badJSON + `: service "a" policy 1: effect "Deny" is neither "grant" nor "deny"`,
		badJSON + ": 1 error(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}

	if out, code := check(t, nil, []string{filepath.Join(t.TempDir(), "missing.spdl")}); code != 1 || !strings.Contains(out, "no such file") {
		t.Errorf("missing file: exit %d\n%s", code, out)
	}
	// A policy and files can be checked together.
	if out, code := check(t, []string{"policy", "grant user a read /x"}, []string{good}); code != 0 || !strings.HasPrefix(out, "policy OK\n") {
		t.Errorf("policy and file: exit %d\n%s", code, out)
	}
}
