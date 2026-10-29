//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package rolegraph

import (
	"reflect"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
)

func rp(id, effect string, roles, principals []string) *pms.RolePolicy {
	return &pms.RolePolicy{ID: id, Name: id, Effect: effect, Roles: roles, Principals: principals}
}

func list(s ...string) []string { return s }

// library: alice is a manager; managers are readers; staff are readers when
// a condition holds; bob may not be a reader; anyone is a guest.
func library() *pms.Service {
	conditional := rp("staff-read", "grant", list("reader"), list("group:staff"))
	conditional.Condition = "hour < 18"
	scoped := rp("idd-read", "grant", list("reader"), list("idd=corp:user:carol"))
	scoped.Resources = list("/books")
	return &pms.Service{
		Name: "library",
		RolePolicies: []*pms.RolePolicy{
			rp("alice-manager", "grant", list("manager"), list("user:alice")),
			rp("manager-reader", "grant", list("reader"), list("role:manager")),
			conditional,
			scoped,
			rp("bob-no-reader", "deny", list("reader"), list("user:bob")),
			rp("guests", "grant", list("guest"), nil),
			rp("orphan", "grant", list("archivist"), list("user:dave")),
		},
		Policies: []*pms.Policy{
			{ID: "p1", Name: "read", Effect: "grant", Principals: [][]string{{"role:reader"}, {"role:reader", "group:x"}}},
			{ID: "p2", Name: "audit", Effect: "grant", Principals: [][]string{{"role:auditor"}, {"role:everyone_role"}}},
			{ID: "p3", Name: "guests", Effect: "deny", Principals: [][]string{{"role:guest"}}},
		},
	}
}

func nodes(g *Graph, pick func(*Node) bool) []string {
	var out []string
	for _, n := range g.Nodes {
		if pick(n) {
			out = append(out, n.ID)
		}
	}
	return out
}

func TestBuild(t *testing.T) {
	g := Build(library(), nil)
	if g.Service != "library" {
		t.Errorf("service %q", g.Service)
	}
	if got := nodes(g, func(n *Node) bool { return n.Kind == KindRole }); !reflect.DeepEqual(got,
		list("role:archivist", "role:auditor", "role:everyone_role", "role:guest", "role:manager", "role:reader")) {
		t.Errorf("roles %v", got)
	}
	if got := nodes(g, func(n *Node) bool { return n.Kind == KindEveryone }); !reflect.DeepEqual(got, list(Everyone)) {
		t.Errorf("everyone %v", got)
	}
	if got := nodes(g, func(n *Node) bool { return n.Undefined }); !reflect.DeepEqual(got, list("role:auditor")) {
		t.Errorf("undefined %v", got)
	}
	if got := nodes(g, func(n *Node) bool { return n.Unused }); !reflect.DeepEqual(got, list("role:archivist")) {
		t.Errorf("unused %v", got)
	}
	if len(g.Edges) != 7 || len(g.Cycles) != 0 {
		t.Errorf("%d edges, cycles %v", len(g.Edges), g.Cycles)
	}
	if refs := g.References["reader"]; len(refs) != 1 || refs[0].ID != "p1" {
		t.Errorf("reader references %v", refs)
	}
	for _, e := range g.Edges {
		want := e.PolicyID == "staff-read" || e.PolicyID == "idd-read"
		if e.Conditional() != want {
			t.Errorf("%s conditional = %v", e.PolicyID, e.Conditional())
		}
	}
}

func TestHolders(t *testing.T) {
	h := Build(library(), nil).Holders("reader")
	var got []string
	for _, holder := range h.Holders {
		got = append(got, holder.Principal+" "+strings.Join(holder.Path, ">")+map[bool]string{true: " ?", false: ""}[holder.Conditional])
	}
	want := list(
		"group:staff group:staff>role:reader ?",
		"idd=corp:user:carol idd=corp:user:carol>role:reader ?",
		"user:alice user:alice>role:manager>role:reader",
	)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("holders\n got %v\nwant %v", got, want)
	}
	if len(h.Denials) != 1 || h.Denials[0].PolicyID != "bob-no-reader" {
		t.Errorf("denials %+v", h.Denials)
	}
	if len(h.References) != 1 {
		t.Errorf("references %v", h.References)
	}

	guests := Build(library(), nil).Holders("guest")
	if len(guests.Holders) != 1 || guests.Holders[0].Principal != Everyone {
		t.Errorf("guest holders %+v", guests.Holders)
	}
	none := Build(library(), nil).Holders("nobody")
	if len(none.Holders) != 0 || len(none.Denials) != 0 || none.References == nil {
		t.Errorf("unknown role %+v", none)
	}
}

func TestHolderWithAnUnconditionalDetour(t *testing.T) {
	// The direct grant has a condition, the longer path does not.
	direct := rp("direct", "grant", list("r"), list("user:a"))
	direct.Condition = "x > 1"
	s := &pms.Service{Name: "s", RolePolicies: []*pms.RolePolicy{
		direct,
		rp("a-mid", "grant", list("mid"), list("user:a")),
		rp("mid-r", "grant", list("r"), list("role:mid")),
	}}
	h := Build(s, nil).Holders("r")
	if len(h.Holders) != 1 || h.Holders[0].Conditional || len(h.Holders[0].Path) != 2 {
		t.Errorf("holders %+v", h.Holders)
	}
}

func TestDeniedAncestorIsReported(t *testing.T) {
	s := &pms.Service{Name: "s", RolePolicies: []*pms.RolePolicy{
		rp("a-mid", "grant", list("mid"), list("user:a")),
		rp("mid-r", "grant", list("r"), list("role:mid")),
		rp("no-mid", "deny", list("mid"), list("user:b")),
		rp("unrelated", "deny", list("other"), list("user:c")),
	}}
	h := Build(s, nil).Holders("r")
	if len(h.Denials) != 1 || h.Denials[0].PolicyID != "no-mid" {
		t.Errorf("denials %+v", h.Denials)
	}
	other := Build(s, nil)
	for _, n := range other.Nodes {
		if n.ID == "role:other" && !n.Undefined {
			t.Error("a role that is only denied is undefined")
		}
	}
}

func TestCycles(t *testing.T) {
	s := &pms.Service{Name: "s", RolePolicies: []*pms.RolePolicy{
		rp("a-b", "grant", list("b"), list("role:a")),
		rp("b-c", "grant", list("c"), list("role:b")),
		rp("c-a", "grant", list("a"), list("role:c")),
		rp("self", "grant", list("d"), list("role:d")),
		rp("deny-loop", "deny", list("e"), list("role:e")),
		rp("tail", "grant", list("f"), list("role:c")),
	}}
	g := Build(s, nil)
	if !reflect.DeepEqual(g.Cycles, [][]string{list("role:a", "role:b", "role:c"), list("role:d")}) {
		t.Errorf("cycles %v", g.Cycles)
	}
	if got := nodes(g, func(n *Node) bool { return n.InCycle }); !reflect.DeepEqual(got, list("role:a", "role:b", "role:c", "role:d")) {
		t.Errorf("in cycles %v", got)
	}
	// Holders terminate on cycles.
	if h := g.Holders("f"); len(h.Holders) != 0 {
		t.Errorf("holders of f %+v", h.Holders)
	}
}

func TestGlobalRolePolicies(t *testing.T) {
	global := &pms.Service{Name: pms.GlobalService, RolePolicies: []*pms.RolePolicy{
		rp("global-admin", "grant", list("auditor"), list("group:audit")),
	}}
	g := Build(library(), global)
	if got := nodes(g, func(n *Node) bool { return n.Undefined }); len(got) != 0 {
		t.Errorf("undefined with global %v", got)
	}
	if h := g.Holders("auditor"); len(h.Holders) != 1 || h.Holders[0].Principal != "group:audit" {
		t.Errorf("auditor holders %+v", h.Holders)
	}
	for _, e := range g.Edges {
		if e.PolicyID == "global-admin" && e.Service != pms.GlobalService {
			t.Errorf("edge service %q", e.Service)
		}
	}
	// The global service is not merged into itself, and nil services work.
	if g := Build(global, global); len(g.Edges) != 1 {
		t.Errorf("global with itself: %d edges", len(g.Edges))
	}
	if g := Build(nil, global); len(g.Edges) != 1 || g.Service != "" {
		t.Errorf("only global: %+v", g)
	}
	if g := Build(nil, nil); len(g.Nodes) != 0 {
		t.Errorf("empty graph %+v", g)
	}
}
