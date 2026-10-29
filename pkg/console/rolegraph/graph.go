//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package rolegraph analyses how role policies grant roles: who holds a
// role, through which roles, and which roles are cyclic, unused or never
// granted. ADS only works out roles for one request at a time; this looks
// at all of a service's role policies at once.
package rolegraph

import (
	"sort"
	"strings"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
)

const (
	// Everyone stands for the principals of a role policy that names none:
	// its roles go to every subject.
	Everyone   = "*"
	rolePrefix = "role:"

	KindRole      = "role"
	KindPrincipal = "principal"
	KindEveryone  = "everyone"
)

var builtinRoles = map[string]bool{
	ads.BuiltIn_Role_Anonymous:     true,
	ads.BuiltIn_Role_Authenticated: true,
	ads.BuiltIn_Role_Everyone:      true,
}

// Node is a role or a principal.
type Node struct {
	ID   string `json:"id"`   // role:admin, user:alice or *
	Kind string `json:"kind"` // role, principal or everyone
	Name string `json:"name"` // admin, user:alice or *
	// Roles only.
	Builtin   bool `json:"builtin,omitempty"`
	Undefined bool `json:"undefined,omitempty"` // used, but no role policy grants it
	Unused    bool `json:"unused,omitempty"`    // granted, but no policy or role uses it
	InCycle   bool `json:"inCycle,omitempty"`
}

// Edge gives role To to the holders of From, or denies it to them.
type Edge struct {
	From                string   `json:"from"`
	To                  string   `json:"to"`
	Effect              string   `json:"effect"`
	Service             string   `json:"service"`
	PolicyID            string   `json:"policyId"`
	PolicyName          string   `json:"policyName"`
	Condition           string   `json:"condition,omitempty"`
	Resources           []string `json:"resources,omitempty"`
	ResourceExpressions []string `json:"resourceExpressions,omitempty"`
}

// Conditional reports whether the edge only applies to some requests.
func (e *Edge) Conditional() bool {
	return e.Condition != "" || len(e.Resources) > 0 || len(e.ResourceExpressions) > 0
}

// PolicyRef names an authorization policy.
type PolicyRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Effect string `json:"effect"`
}

// Graph is the role graph of a service.
type Graph struct {
	Service string     `json:"service"`
	Nodes   []*Node    `json:"nodes"`
	Edges   []*Edge    `json:"edges"`
	Cycles  [][]string `json:"cycles"`
	// References maps a role to the authorization policies naming it.
	References map[string][]PolicyRef `json:"references"`

	nodes map[string]*Node
}

func roleID(name string) string { return rolePrefix + name }

// RoleName returns the role a node ID names, if it names one.
func RoleName(id string) (string, bool) {
	if strings.HasPrefix(id, rolePrefix) {
		return id[len(rolePrefix):], true
	}
	return "", false
}

func (g *Graph) node(id string) *Node {
	if n, ok := g.nodes[id]; ok {
		return n
	}
	n := &Node{ID: id, Name: id, Kind: KindPrincipal}
	if name, ok := RoleName(id); ok {
		n.Kind, n.Name, n.Builtin = KindRole, name, builtinRoles[name]
	} else if id == Everyone {
		n.Kind = KindEveryone
	}
	g.nodes[id] = n
	return n
}

func (g *Graph) addRolePolicies(service *pms.Service) {
	if service == nil {
		return
	}
	for _, rp := range service.RolePolicies {
		principals := rp.Principals
		if len(principals) == 0 {
			principals = []string{Everyone}
		}
		for _, role := range rp.Roles {
			to := g.node(roleID(role)).ID
			for _, p := range principals {
				g.Edges = append(g.Edges, &Edge{
					From: g.node(p).ID, To: to, Effect: rp.Effect, Service: service.Name,
					PolicyID: rp.ID, PolicyName: rp.Name, Condition: rp.Condition,
					Resources: rp.Resources, ResourceExpressions: rp.ResourceExpressions,
				})
			}
		}
	}
}

// Build analyses the role policies of service, and of global when it is
// given, since ADS applies global role policies to every service.
func Build(service, global *pms.Service) *Graph {
	g := &Graph{nodes: map[string]*Node{}, References: map[string][]PolicyRef{}, Edges: []*Edge{}, Cycles: [][]string{}}
	if service != nil {
		g.Service = service.Name
	}
	g.addRolePolicies(service)
	if global != nil && (service == nil || service.Name != global.Name) {
		g.addRolePolicies(global)
	}
	if service != nil {
		for _, p := range service.Policies {
			seen := map[string]bool{}
			for _, group := range p.Principals {
				for _, principal := range group {
					if name, ok := RoleName(principal); ok && !seen[name] {
						seen[name] = true
						g.node(principal)
						g.References[name] = append(g.References[name], PolicyRef{ID: p.ID, Name: p.Name, Effect: p.Effect})
					}
				}
			}
		}
	}

	granted, inherited := map[string]bool{}, map[string]bool{}
	for _, e := range g.Edges {
		if e.Effect == pms.Grant {
			granted[e.To] = true
		}
		if g.nodes[e.From].Kind == KindRole {
			inherited[e.From] = true
		}
	}
	for _, n := range g.nodes {
		if n.Kind != KindRole || n.Builtin {
			continue
		}
		n.Undefined = !granted[n.ID]
		n.Unused = granted[n.ID] && !inherited[n.ID] && len(g.References[n.Name]) == 0
	}
	g.findCycles()

	for _, n := range g.nodes {
		g.Nodes = append(g.Nodes, n)
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	return g
}

// findCycles marks the roles that grant each other in a circle, with
// Tarjan's strongly connected components over the grant edges.
func (g *Graph) findCycles() {
	next := map[string][]string{}
	self := map[string]bool{}
	for _, e := range g.Edges {
		if e.Effect == pms.Grant && g.nodes[e.From].Kind == KindRole {
			next[e.From] = append(next[e.From], e.To)
			if e.From == e.To {
				self[e.From] = true
			}
		}
	}
	ids := make([]string, 0, len(next))
	for id := range next {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	index, low, onStack := map[string]int{}, map[string]int{}, map[string]bool{}
	var stack []string
	counter := 0
	var visit func(v string)
	visit = func(v string) {
		index[v], low[v] = counter, counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range next[v] {
			if _, seen := index[w]; !seen {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] != index[v] {
			return
		}
		var component []string
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			component = append(component, w)
			if w == v {
				break
			}
		}
		if len(component) > 1 || self[v] {
			sort.Strings(component)
			for _, id := range component {
				g.nodes[id].InCycle = true
			}
			g.Cycles = append(g.Cycles, component)
		}
	}
	for _, id := range ids {
		if _, seen := index[id]; !seen {
			visit(id)
		}
	}
	sort.Slice(g.Cycles, func(i, j int) bool { return g.Cycles[i][0] < g.Cycles[j][0] })
}
