//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pdl

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
)

func TestParsePolicyForms(t *testing.T) {
	cases := []struct {
		cmd  string
		want pms.Policy
	}{
		{"grant user alice from corp get,put res1, list expr:/docs/.* if a > 1  ", pms.Policy{
			Name: "p", Effect: "grant", Principals: [][]string{{"idd=corp:user:alice"}},
			Permissions: []*pms.Permission{{Resource: "res1", Actions: []string{"get", "put"}}, {ResourceExpression: "/docs/.*", Actions: []string{"list"}}},
			Condition:   "a > 1"}},
		{"DENY ( group devs , entity bot ), role admin delete 'my res'", pms.Policy{
			Name: "p", Effect: "deny", Principals: [][]string{{"group:devs", "entity:bot"}, {"role:admin"}},
			Permissions: []*pms.Permission{{Resource: "my res", Actions: []string{"delete"}}}}},
	}
	for _, c := range cases {
		got, reader, err := ParsePolicy(c.cmd, "p")
		if err != nil {
			t.Errorf("%q: %v", c.cmd, err)
			continue
		}
		if !reflect.DeepEqual(*got, c.want) {
			gotJSON, _ := json.Marshal(got)
			t.Errorf("%q:\n got %s", c.cmd, gotJSON)
		}
		var decoded pms.Policy
		if err := json.NewDecoder(reader).Decode(&decoded); err != nil || decoded.Name != "p" {
			t.Errorf("%q: JSON %+v, %v", c.cmd, decoded, err)
		}
	}
}

func TestParsePolicyErrors(t *testing.T) {
	for _, cmd := range []string{
		"",
		"permit user a get r",
		"grant ",
		"grant user",
		"grant user a",
		"grant user a get",
		"grant user a get r,",
		"grant user a get r, list",
		"grant user a get, r",
		"grant thing a get r",
		"grant user a from",
		"grant (",
		"grant ( ",
		"grant ()",
		"grant (user a",
		"grant (user a user b) get r",
		"grant (user a,",
		"grant (user a, nobody) get r",
		"grant user a, ",
		"grant user a, nobody get r",
		"grant user a get r if ",
	} {
		if p, _, err := ParsePolicy(cmd, "p"); err == nil {
			t.Errorf("%q parsed as %+v", cmd, p)
		}
	}
}

func TestParseRolePolicyForms(t *testing.T) {
	cases := []struct {
		cmd  string
		want pms.RolePolicy
	}{
		{"grant user alice, group devs role admin, role ops on res1, expr:/x/.* if b == 'c'", pms.RolePolicy{
			Name: "rp", Effect: "grant", Principals: []string{"user:alice", "group:devs"}, Roles: []string{"admin", "ops"},
			Resources: []string{"res1"}, ResourceExpressions: []string{"/x/.*"}, Condition: "b == 'c'"}},
		{"deny entity bot reader", pms.RolePolicy{
			Name: "rp", Effect: "deny", Principals: []string{"entity:bot"}, Roles: []string{"reader"}, ResourceExpressions: []string{}}},
	}
	for _, c := range cases {
		got, _, err := ParseRolePolicy(c.cmd, "rp")
		if err != nil {
			t.Errorf("%q: %v", c.cmd, err)
			continue
		}
		if !reflect.DeepEqual(*got, c.want) {
			gotJSON, _ := json.Marshal(got)
			t.Errorf("%q:\n got %s", c.cmd, gotJSON)
		}
	}
}

func TestParseRolePolicyErrors(t *testing.T) {
	for _, cmd := range []string{
		"",
		"grant",
		"grant user",
		"grant user a",
		"grant user a,",
		"grant user a, nobody r",
		"grant user a role",
		"grant user a r,",
		"grant user a r on res,",
	} {
		if p, _, err := ParseRolePolicy(cmd, "rp"); err == nil {
			t.Errorf("%q parsed as %+v", cmd, p)
		}
	}
}

func TestGetService(t *testing.T) {
	cases := []struct {
		cmd     string
		want    string
		wantErr bool
	}{
		{" in app rest", "app", false},
		{"in ", "", true},
		{"other", "", false},
	}
	for _, c := range cases {
		got, _, err := getService(c.cmd, 0)
		if got != c.want || (err != nil) != c.wantErr {
			t.Errorf("getService(%q) = %q, %v", c.cmd, got, err)
		}
	}
}
