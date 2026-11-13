package pdl

import (
	"reflect"
	"strings"
	"testing"
)

// SPDL keywords are reserved in every name position, in any letter case and
// quoted or not (#368).
func TestReservedWordsRejectedAsNames(t *testing.T) {
	cases := []struct {
		kind, cmd, word, what, caret string
	}{
		// principal names, every principal type
		{"policy", "grant user role read /x", "role", "a principal name", "           ^"},
		{"policy", "grant group USER read /x", "USER", "a principal name", "            ^"},
		{"policy", "grant entity 'grant' read /x", "grant", "a principal name", "             ^"},
		{"policy", `grant (user a, role "Deny") read /x`, "Deny", "a principal name", "                    ^"},
		{"rolepolicy", "deny role user manager", "user", "a principal name", "          ^"},
		// identity domain
		{"policy", "grant user a from on read /x", "on", "an identity domain", "                  ^"},
		// actions and resources
		{"policy", "grant user william role manager", "role", "an action", "                   ^"},
		{"policy", "grant user a read,From /x", "From", "an action", "                  ^"},
		{"policy", "grant user a read entity", "entity", "a resource", "                  ^"},
		{"policy", "grant user a read /x, write 'in'", "in", "a resource", "                            ^"},
		// role names, with and without the optional prefix
		{"rolepolicy", "grant user a role group", "group", "a role name", "                  ^"},
		{"rolepolicy", "grant user a manager, 'if'", "if", "a role name", "                      ^"},
		{"rolepolicy", "grant user a role role", "role", "a role name", "                  ^"},
		// role policy resources
		{"rolepolicy", "grant user a manager on user", "user", "a resource", "                        ^"},
		// condition attributes and functions, plain and escaped
		{"policy", "grant user a read /x if role == 'admin'", "role", "a condition attribute or function name", "                        ^"},
		{"policy", "grant user a read /x if [user] == 'bob'", "user", "a condition attribute or function name", "                        ^"},
		{"policy", "grant user a read /x if amount > 1 && Grant(x)", "Grant", "a condition attribute or function name", "                                      ^"},
		{"policy", "grant user a read /x if Entity > 1", "Entity", "a condition attribute or function name", "                        ^"},
		{"rolepolicy", "grant user a manager if [from] > 1", "from", "a condition attribute or function name", "                        ^"},
	}
	for _, c := range cases {
		var err error
		if c.kind == "policy" {
			_, _, err = ParsePolicy(c.cmd, "p")
		} else {
			_, _, err = ParseRolePolicy(c.cmd, "p")
		}
		want := `"` + c.word + `" is a reserved SPDL keyword and cannot be used as ` + c.what + "\n" + c.cmd + "\n" + c.caret
		if err == nil || err.Error() != want {
			t.Errorf("%s %q:\n got %v\nwant %s", c.kind, c.cmd, err, want)
		}
	}
}

// With keywords reserved, a statement that ends in "role NAME" has one
// reading: a role policy assigning NAME.
func TestRolePrefixHasOneReading(t *testing.T) {
	if _, _, err := ParsePolicy("grant user william role manager", ""); err == nil {
		t.Fatal("parsed as an authorization policy")
	}
	rp, _, err := ParseRolePolicy("grant user william role manager", "")
	if err != nil || !reflect.DeepEqual(rp.Roles, []string{"manager"}) || !reflect.DeepEqual(rp.Principals, []string{"user:william"}) {
		t.Fatalf("role policy %+v, %v", rp, err)
	}
}

// Keywords stay usable where they are keywords, and inside longer names,
// string literals and the in operator.
func TestKeywordsStillWorkAsKeywords(t *testing.T) {
	for _, cmd := range []string{
		"GRANT USER users FROM domain READ /role/user",
		"grant group roles read,update grants if user_name == 'role' && x in ('user', 'deny')",
		"deny (user alice from corp, group admins), role owner delete /x if department IN ('grant')",
		"grant user a read /x if request_user == 'in' && [user.name] == 'x'",
		"grant user a read /x if amount > 1.5 && name =~ '^role'",
	} {
		if _, _, err := ParsePolicy(cmd, "p"); err != nil {
			t.Errorf("%q: %v", cmd, err)
		}
	}
	for _, cmd := range []string{
		"grant user alice role admin, ROLE ops on /books, expr:/role/.* if inGroup(x)",
		"grant role manager employee",
	} {
		if _, _, err := ParseRolePolicy(cmd, "p"); err != nil {
			t.Errorf("%q: %v", cmd, err)
		}
	}
}

func TestIsReserved(t *testing.T) {
	for _, w := range []string{"grant", "DENY", "User", "group", "entity", "role", "from", "on", "IF", "in"} {
		if !IsReserved(w) {
			t.Errorf("%q not reserved", w)
		}
	}
	for _, w := range []string{"users", "roles", "", "inn", "grant ", "expr:role"} {
		if IsReserved(w) {
			t.Errorf("%q reserved", w)
		}
	}
	if strings.Join(reservedWords, ",") != "grant,deny,user,group,entity,role,from,on,if,in" {
		t.Errorf("reserved words changed: %v", reservedWords)
	}
}
