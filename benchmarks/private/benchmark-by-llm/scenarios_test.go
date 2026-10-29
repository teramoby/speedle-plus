package authzbench

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type Case struct {
	Req  Req
	Want bool
}

type Scenario struct {
	Dir   string
	Title string
	// CasbinArgs maps a Req to the scenario's Casbin request tuple.
	CasbinArgs func(Req) []any
	// SpeedleMap rewrites a Req for Speedle+ when its model needs a
	// different resource name. Nil means no rewrite.
	SpeedleMap func(Req) Req
	Cases      []Case
}

func amount(v float64) map[string]any { return map[string]any{"amount": v} }

var scenarios = []Scenario{
	{
		Dir: "s1_rbac", Title: "Basic RBAC",
		Cases: []Case{
			{Req{User: "alice", Action: "write", Resource: "/data1"}, true},
			{Req{User: "bob", Action: "write", Resource: "/data1"}, false},
			{Req{User: "bob", Action: "read", Resource: "/data1"}, true},
			{Req{User: "carol", Action: "read", Resource: "/data1"}, false},
		},
	},
	{
		Dir: "s2_hierarchy", Title: "Role hierarchy",
		Cases: []Case{
			{Req{User: "dave", Action: "read", Resource: "/docs"}, true},
			{Req{User: "dave", Action: "write", Resource: "/docs"}, true},
			{Req{User: "vic", Action: "read", Resource: "/docs"}, true},
			{Req{User: "vic", Action: "write", Resource: "/docs"}, false},
		},
	},
	{
		Dir: "s3_groups", Title: "Groups",
		Cases: []Case{
			{Req{User: "erin", Groups: []string{"eng"}, Action: "deploy", Resource: "/svc"}, true},
			{Req{User: "frank", Action: "deploy", Resource: "/svc"}, false},
			{Req{User: "frank", Groups: []string{"ops"}, Action: "deploy", Resource: "/svc"}, false},
		},
	},
	{
		Dir: "s4_tenants", Title: "Multi-tenant roles",
		CasbinArgs: func(r Req) []any { return []any{r.User, r.Tenant, r.Resource, r.Action} },
		SpeedleMap: func(r Req) Req { r.Resource = "/" + r.Tenant + r.Resource; return r },
		Cases: []Case{
			{Req{User: "gina", Tenant: "tenant1", Action: "write", Resource: "/data"}, true},
			{Req{User: "gina", Tenant: "tenant2", Action: "write", Resource: "/data"}, false},
			{Req{User: "gina", Tenant: "tenant2", Action: "read", Resource: "/data"}, true},
			{Req{User: "gina", Tenant: "tenant3", Action: "read", Resource: "/data"}, false},
		},
	},
	{
		Dir: "s5_abac_limit", Title: "ABAC: approval limit",
		CasbinArgs: CasbinAttrArgs,
		Cases: []Case{
			{Req{User: "hank", Action: "approve", Resource: "/expense", Attrs: amount(500)}, true},
			{Req{User: "hank", Action: "approve", Resource: "/expense", Attrs: amount(5000)}, false},
			{Req{User: "ivy", Action: "approve", Resource: "/expense", Attrs: amount(10)}, false},
		},
	},
	{
		Dir: "s6_owner", Title: "ABAC: owner can edit",
		CasbinArgs: func(r Req) []any {
			owner, _ := r.Attrs["owner"].(string)
			return []any{r.User, CasbinObj{Name: r.Resource, Owner: owner}, r.Action}
		},
		Cases: []Case{
			{Req{User: "ivan", Action: "edit", Resource: "/docs/doc1", Attrs: map[string]any{"owner": "ivan"}}, true},
			{Req{User: "judy", Action: "edit", Resource: "/docs/doc1", Attrs: map[string]any{"owner": "ivan"}}, false},
			{Req{User: "ivan", Action: "delete", Resource: "/docs/doc1", Attrs: map[string]any{"owner": "ivan"}}, false},
		},
	},
	{
		Dir: "s7_deny", Title: "Deny overrides grant",
		Cases: []Case{
			{Req{User: "kim", Action: "read", Resource: "/hr/salaries"}, false},
			{Req{User: "kim", Action: "read", Resource: "/hr/handbook"}, true},
			{Req{User: "lee", Action: "read", Resource: "/hr/salaries"}, true},
		},
	},
	{
		Dir: "s8_wildcard", Title: "Wildcard resources",
		Cases: []Case{
			{Req{User: "leo", Action: "read", Resource: "/reports/2026/q3"}, true},
			{Req{User: "leo", Action: "read", Resource: "/secret/x"}, false},
			{Req{User: "leo", Action: "write", Resource: "/reports/a"}, false},
		},
	},
	{
		Dir: "s9_relationship", Title: "Relationship: doc in (nested) folder",
		// Speedle+ has no relationship store; the folder tree is encoded
		// in resource paths instead.
		SpeedleMap: func(r Req) Req {
			r.Resource = map[string]string{"doc:D1": "/F1/D1", "doc:D2": "/F2/D2", "doc:D3": "/F1/F1a/D3"}[r.Resource]
			return r
		},
		Cases: []Case{
			{Req{User: "mia", Action: "read", Resource: "doc:D1"}, true},
			{Req{User: "mia", Action: "read", Resource: "doc:D2"}, false},
			{Req{User: "mia", Action: "read", Resource: "doc:D3"}, true},
		},
	},
}

func scenarioEngines(s Scenario) (map[string]Engine, map[string]error) {
	dir := filepath.Join("scenarios", s.Dir)
	engines := map[string]Engine{}
	errs := map[string]error{}
	args := s.CasbinArgs
	if args == nil {
		args = CasbinArgs
	}
	if e, err := NewCasbin(filepath.Join(dir, "casbin.conf"), filepath.Join(dir, "casbin.csv"), args); err != nil {
		errs["casbin"] = err
	} else {
		engines["casbin"] = e
	}
	if e, err := NewOPA(filepath.Join(dir, "opa.rego"), filepath.Join(dir, "opa.json")); err != nil {
		errs["opa"] = err
	} else {
		engines["opa"] = e
	}
	if e, err := NewSpeedle(filepath.Join(dir, "speedle.spdl"), false); err != nil {
		errs["speedle"] = err
	} else {
		e.Map = s.SpeedleMap
		engines["speedle"] = e
	}
	return engines, errs
}

var engineNames = []string{"casbin", "opa", "speedle"}

// TestScenarios runs every scenario on every engine and prints a pass matrix.
func TestScenarios(t *testing.T) {
	var rows []string
	for _, s := range scenarios {
		engines, errs := scenarioEngines(s)
		row := fmt.Sprintf("| %s |", s.Title)
		for _, name := range engineNames {
			if err, bad := errs[name]; bad {
				t.Errorf("%s/%s: load error: %v", s.Dir, name, err)
				row += " load error |"
				continue
			}
			pass := 0
			for _, c := range s.Cases {
				got, err := engines[name].Allowed(c.Req)
				switch {
				case err != nil:
					t.Errorf("%s/%s: %+v: error: %v", s.Dir, name, c.Req, err)
				case got != c.Want:
					t.Errorf("%s/%s: %+v: got %v, want %v", s.Dir, name, c.Req, got, c.Want)
				default:
					pass++
				}
			}
			row += fmt.Sprintf(" %d/%d |", pass, len(s.Cases))
		}
		rows = append(rows, row)
	}
	fmt.Println("\n| Scenario | Casbin | OPA | Speedle+ |\n|---|---|---|---|")
	fmt.Println(strings.Join(rows, "\n"))
}

// policySize counts non-blank lines and non-whitespace characters.
func policySize(paths ...string) (lines, chars int) {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			lines++
			chars += len(strings.Join(strings.Fields(l), ""))
		}
	}
	return
}

// TestPolicySize prints how much policy text each engine needed per scenario.
func TestPolicySize(t *testing.T) {
	fmt.Println("\n| Scenario | Casbin lines (chars) | OPA lines (chars) | Speedle+ lines (chars) |\n|---|---|---|---|")
	tot := map[string][2]int{}
	for _, s := range scenarios {
		d := filepath.Join("scenarios", s.Dir)
		files := map[string][]string{
			"casbin":  {filepath.Join(d, "casbin.conf"), filepath.Join(d, "casbin.csv")},
			"opa":     {filepath.Join(d, "opa.rego"), filepath.Join(d, "opa.json")},
			"speedle": {filepath.Join(d, "speedle.spdl")},
		}
		row := "| " + s.Title + " |"
		for _, n := range engineNames {
			l, c := policySize(files[n]...)
			v := tot[n]
			tot[n] = [2]int{v[0] + l, v[1] + c}
			row += fmt.Sprintf(" %d (%d) |", l, c)
		}
		fmt.Println(row)
	}
	row := "| **Total** |"
	for _, n := range engineNames {
		row += fmt.Sprintf(" **%d (%d)** |", tot[n][0], tot[n][1])
	}
	fmt.Println(row)
}

func sortedKeys(m map[string]bool) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
