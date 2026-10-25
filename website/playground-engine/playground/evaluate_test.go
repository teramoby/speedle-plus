package playground

import (
	"encoding/json"
	"testing"
)

func run(t *testing.T, policies string, request map[string]interface{}) Result {
	t.Helper()
	if request == nil {
		request = map[string]interface{}{"subject": map[string]interface{}{"principals": []interface{}{map[string]interface{}{"type": "user", "name": "alice"}}}, "serviceName": "playground", "resource": "book", "action": "read"}
	}
	input, _ := json.Marshal(map[string]interface{}{"policies": policies, "request": request})
	return EvaluateJSON(input)
}
func TestDecisions(t *testing.T) {
	cases := []struct {
		name, policy string
		want         bool
	}{
		{"grant", "grant user alice read book", true},
		{"no match", "grant user bob read book", false},
		{"deny wins", "grant user alice read book\ndeny user alice read book", false},
		{"deny first", "deny user alice read book\ngrant user alice read book", false},
		{"role scoped", "grant user alice reader on book\ngrant role reader read book", true},
		{"role unscoped", "grant user alice reader\ngrant role reader read book", true},
		{"role explicit", "grant user alice role reader\ngrant role reader read book", true},
		{"role other resource", "grant user alice reader on document\ngrant role reader read book", false},
		{"role chain", "grant user alice reader\ngrant role reader editor\ngrant role editor read book", true},
		{"deny role", "grant user alice reader\ndeny user alice reader\ngrant role reader read book", false},
		{"regex", "grant user alice read expr:^bo.*$", true},
		{"literal star", "grant user alice read *", false},
		{"multi action", "grant user alice write,read book", true},
		{"multi principal", "grant user bob, user alice read book", true},
		{"and mismatch", "grant (user alice, group admin) read book", false},
		{"condition true", "grant user alice read book if request_user == 'alice'", true},
		{"condition false", "grant user alice read book if 2 > 3", false},
		{"condition functions", "grant user alice read book if Sqrt(16)==4 && Max(1,5,3)==5", true},
		{"comments", "# example\n\ngrant user alice read book\n// another comment", true},
		{"case effect", "GRANT user alice read book", true},
		{"everyone", "grant role everyone_role read book", true},
		{"authenticated", "grant role authenticated_role read book", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := run(t, c.policy, nil)
			if !r.OK || r.Allowed == nil || *r.Allowed != c.want {
				t.Fatalf("result: %+v error %+v", r, r.Error)
			}
		})
	}
}
func TestErrorsDistinct(t *testing.T) {
	cases := []string{"", "grant user", "grant user alice read book trailing", "grant user alice read book if", "grant user alice read book if missing > 3", "grant user alice read book if 3", "grant user alice read book if clearance >>> 3", "grant user alice read expr:[", "grant user alice read book if RemoteFunction(3)", "grant user alice read book\ndeny user alice read book if missing > 3"}
	for _, policy := range cases {
		t.Run(policy, func(t *testing.T) {
			r := run(t, policy, nil)
			if r.OK || r.Allowed != nil || r.Error == nil {
				t.Fatalf("invalid input gave ordinary result: %+v", r)
			}
		})
	}
}
func TestTypedAttributes(t *testing.T) {
	req := map[string]interface{}{"subject": map[string]interface{}{"principals": []interface{}{map[string]interface{}{"type": "user", "name": "alice"}, map[string]interface{}{"type": "group", "name": "admin"}}}, "resource": "book", "action": "read", "attributes": []interface{}{map[string]interface{}{"name": "clearance", "type": "numeric", "value": 4}, map[string]interface{}{"name": "department", "type": "string", "value": "eng"}, map[string]interface{}{"name": "active", "type": "bool", "value": true}, map[string]interface{}{"name": "teams", "type": "string", "value": []string{"eng", "ops"}}, map[string]interface{}{"name": "expiry", "type": "datetime", "value": "2030-01-01T00:00:00Z"}}}
	r := run(t, "grant (user alice, group admin) read book if clearance >= 3 && department == 'eng' && active && 'eng' in teams && expiry > '2020-01-01T00:00:00Z'", req)
	if !r.OK || r.Allowed == nil || !*r.Allowed {
		t.Fatalf("result %+v error %+v", r, r.Error)
	}
}
func TestIdentityDomainAndAnonymous(t *testing.T) {
	req := map[string]interface{}{"subject": map[string]interface{}{"principals": []interface{}{map[string]interface{}{"type": "user", "name": "alice", "idd": "github"}}}, "resource": "book", "action": "read"}
	r := run(t, "grant user alice from github read book", req)
	if !r.OK || !*r.Allowed {
		t.Fatalf("IDD %+v %+v", r, r.Error)
	}
	r = run(t, "grant role anonymous_role read book", map[string]interface{}{"resource": "book", "action": "read"})
	if !r.OK || !*r.Allowed {
		t.Fatalf("anonymous %+v %+v", r, r.Error)
	}
}
func TestUnknownRequestAndTokenRejected(t *testing.T) {
	for _, req := range []map[string]interface{}{{"action": "read", "resource": "book", "username": "alice"}, {"action": "read", "resource": "book", "subject": map[string]interface{}{"token": "secret"}}, {"action": "read", "resource": "book", "attributes": []interface{}{map[string]interface{}{"name": "request_user", "type": "string", "value": "admin"}}}} {
		r := run(t, "grant user alice read book", req)
		if r.OK || r.Error == nil {
			t.Fatalf("expected request error: %+v", r)
		}
	}
}
