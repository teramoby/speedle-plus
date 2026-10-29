package testutil

import (
	"reflect"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
)

func TestPermissionResponseComparison(t *testing.T) {
	want := []pms.Permission{{Resource: "one", Actions: []string{"read", "write"}}, {ResourceExpression: "two.*", Actions: []string{"read"}}}
	before := []pms.Permission{{Resource: "one", Actions: []string{"read", "write"}}, {ResourceExpression: "two.*", Actions: []string{"read"}}}
	for _, tc := range []struct {
		name  string
		got   []pms.Permission
		equal bool
	}{
		{"order", []pms.Permission{{ResourceExpression: "two.*", Actions: []string{"read"}}, {Resource: "one", Actions: []string{"write", "read"}}}, true},
		{"missing permission", want[:1], false},
		{"extra action", []pms.Permission{{Resource: "one", Actions: []string{"read", "write", "delete"}}, want[1]}, false},
		{"duplicate", []pms.Permission{want[0], want[1], want[1]}, false},
		{"expression is not literal", []pms.Permission{want[0], {Resource: "two.*", Actions: []string{"read"}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if equalResponseBodies(&want, &tc.got) != tc.equal {
				t.Fatalf("comparison = %v, want %v", !tc.equal, tc.equal)
			}
		})
	}
	if !reflect.DeepEqual(want, before) {
		t.Fatal("comparison mutated expected permissions")
	}
	if equalResponseBodies([]string{"a", "b"}, []string{"b", "a"}) {
		t.Fatal("ordering of unrelated response types must still be checked")
	}
}
