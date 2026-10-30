package eval

import (
	"reflect"
	"sort"
	"testing"
)

func TestDifPolicySets(t *testing.T) {
	testCases := []struct {
		name            string
		old, new        []int
		missed, removed []int
	}{
		{"same", []int{1, 2, 3}, []int{1, 2, 3}, nil, nil},
		{"added in the middle", []int{1, 3}, []int{1, 2, 3}, []int{2}, nil},
		{"added at the start", []int{2, 3}, []int{1, 2, 3}, []int{1}, nil},
		{"added at the end", []int{1, 2}, []int{1, 2, 3}, []int{3}, nil},
		{"removed", []int{1, 2, 3}, []int{1, 3}, nil, []int{2}},
		{"mixed", []int{1, 3, 5}, []int{2, 3, 4, 6}, []int{2, 4, 6}, []int{1, 5}},
		{"from empty", nil, []int{1, 2}, []int{1, 2}, nil},
		{"to empty", []int{1, 2}, nil, nil, []int{1, 2}},
	}
	for _, tc := range testCases {
		missed, removed := difPolicySets(tc.old, tc.new)
		if !reflect.DeepEqual(missed, tc.missed) || !reflect.DeepEqual(removed, tc.removed) {
			t.Errorf("%s: got missed %v removed %v, want missed %v removed %v", tc.name, missed, removed, tc.missed, tc.removed)
		}
	}
}

func TestDifFuncSets(t *testing.T) {
	missed, removed := difFuncSets([]string{"a", "b", "c"}, []string{"b", "c", "d"})
	sort.Strings(missed)
	sort.Strings(removed)
	if !reflect.DeepEqual(missed, []string{"d"}) || !reflect.DeepEqual(removed, []string{"a"}) {
		t.Errorf("got missed %v removed %v, want missed [d] removed [a]", missed, removed)
	}
}

func TestReachableRolesWithCycle(t *testing.T) {
	// A cycle without an independent root cannot grant itself.
	roles := map[string]*Role{
		"role1": {Name: "role1", ParentRoles: map[string]bool{"role2": true}, ChildRoles: map[string]bool{"role2": true}},
		"role2": {Name: "role2", ParentRoles: map[string]bool{"role1": true}, ChildRoles: map[string]bool{"role1": true}},
	}
	if got := reachableRoles(roles, nil); len(got) != 0 {
		t.Fatalf("unrooted cycle granted %v", got)
	}
	roles["role1"].ParentPrincipals = map[string]bool{"user:alice": true}
	if got := reachableRoles(roles, nil); !reflect.DeepEqual(got, map[string]bool{"role1": true, "role2": true}) {
		t.Fatalf("explicit user root: %v", got)
	}
	if got := reachableRoles(roles, map[string]bool{"role1": true}); len(got) != 0 {
		t.Fatalf("blocked root left cycle granted %v", got)
	}
}
