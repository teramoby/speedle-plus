//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package rolegraph

import (
	"sort"

	"github.com/teramoby/speedle-plus/api/pms"
)

// Holder is a principal that may hold a role.
type Holder struct {
	Principal string `json:"principal"`
	// Conditional means every path to the role has a condition or a
	// resource scope, so the principal holds it only for some requests.
	Conditional bool `json:"conditional"`
	// Path is a shortest chain of nodes from the principal to the role.
	Path []string `json:"path"`
}

// Holders describes who may hold a role.
type Holders struct {
	Role    string   `json:"role"`
	Holders []Holder `json:"holders"`
	// Denials are the deny role policies on the role or on the roles it is
	// reached through; they can take the role away.
	Denials    []*Edge     `json:"denials"`
	References []PolicyRef `json:"references"`
}

// reach walks grant edges backwards from target, using only edges that keep
// returns true. It returns, for each node reached, the next node towards
// target on a shortest path.
func (g *Graph) reach(target string, keep func(*Edge) bool) map[string]string {
	into := map[string][]*Edge{}
	for _, e := range g.Edges {
		if e.Effect == pms.Grant && keep(e) {
			into[e.To] = append(into[e.To], e)
		}
	}
	nextHop := map[string]string{target: ""}
	queue := []string{target}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		edges := into[id]
		sort.Slice(edges, func(i, j int) bool { return edges[i].From < edges[j].From })
		for _, e := range edges {
			if _, seen := nextHop[e.From]; !seen {
				nextHop[e.From] = id
				queue = append(queue, e.From)
			}
		}
	}
	return nextHop
}

// Holders lists the principals that may hold role, directly or through
// other roles. It does not evaluate conditions or deny policies; it shows
// which ones are involved.
func (g *Graph) Holders(role string) *Holders {
	target := roleID(role)
	all := g.reach(target, func(*Edge) bool { return true })
	always := g.reach(target, func(e *Edge) bool { return !e.Conditional() })

	result := &Holders{Role: role, Holders: []Holder{}, Denials: []*Edge{}, References: g.References[role]}
	if result.References == nil {
		result.References = []PolicyRef{}
	}
	for id := range all {
		n := g.nodes[id]
		if n == nil || n.Kind == KindRole {
			continue
		}
		path := []string{id}
		for next := all[id]; next != ""; next = all[next] {
			path = append(path, next)
		}
		_, unconditional := always[id]
		result.Holders = append(result.Holders, Holder{Principal: id, Conditional: !unconditional, Path: path})
	}
	sort.Slice(result.Holders, func(i, j int) bool { return result.Holders[i].Principal < result.Holders[j].Principal })

	// Deny edges into the role, or into any role it is reached through.
	for _, e := range g.Edges {
		if e.Effect != pms.Deny {
			continue
		}
		if _, onPath := all[e.To]; onPath && g.nodes[e.To].Kind == KindRole {
			result.Denials = append(result.Denials, e)
		}
	}
	return result
}
