package authzbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cedarCase is one case file for cmd/cedar-rs: the Rust implementation of
// Cedar then answers exactly the requests cedar-go answers here.
type cedarCase struct {
	Name       string     `json:"name"`
	Label      string     `json:"label"`
	Policies   string     `json:"policies"`
	Entities   string     `json:"entities"`
	Requests   []cedarRaw `json:"requests"`
	Want       []bool     `json:"want"`
	CheckFirst int        `json:"check_first,omitempty"`
}

// cedarRaw is CedarReq with the context in Cedar's JSON form, where an
// entity reference is written {"__entity": {...}}.
type cedarRaw struct {
	Principal        CedarRef       `json:"principal"`
	Action           CedarRef       `json:"action"`
	Resource         CedarRef       `json:"resource"`
	Context          map[string]any `json:"context"`
	PrincipalParents []CedarRef     `json:"principal_parents,omitempty"`
}

func rawCedar(c CedarReq) cedarRaw {
	ctx := map[string]any{}
	for k, v := range c.Context {
		if ref, ok := v.(CedarRef); ok {
			v = map[string]any{"__entity": ref}
		}
		ctx[k] = v
	}
	return cedarRaw{c.Principal, c.Action, c.Resource, ctx, c.PrincipalParents}
}

type cedarExporter struct {
	t    *testing.T
	root string
	sets map[string]GenSet
}

func (x *cedarExporter) set(kind string, n int) GenSet {
	key := fmt.Sprintf("%s_%d", kind, n)
	if g, ok := x.sets[key]; ok {
		return g
	}
	dir := filepath.Join(x.root, "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		x.t.Fatal(err)
	}
	g, err := GenPolicies(dir, kind, n, usersPerRole)
	if err != nil {
		x.t.Fatal(err)
	}
	// Only the Cedar files are needed; the 100000 sets are large.
	for _, f := range []string{g.CasbinConf, g.CasbinTunedConf, g.CasbinMatcherConf, g.CasbinCSV, g.CasbinPlainCSV, g.OPARego, g.OPAJSON, g.SpeedleSPDL} {
		if f != "" {
			os.Remove(f)
		}
	}
	x.sets[key] = g
	return g
}

// write stores one case per Cedar variant of a generated set.
func (x *cedarExporter) write(group, name, kind string, n int, reqs []Req, want []bool, checkFirst int) {
	g := x.set(kind, n)
	variants := []struct{ label, policies, entities string }{
		{"cedar-rs", g.CedarPolicies, g.CedarEntities},
		{"cedar-rs-data", g.CedarDataPolicies, g.CedarDataEntities},
	}
	for _, v := range variants {
		if v.policies == "" {
			continue
		}
		c := cedarCase{Name: name, Label: v.label, Policies: v.policies, Entities: v.entities, Want: want, CheckFirst: checkFirst}
		for _, r := range reqs {
			c.Requests = append(c.Requests, rawCedar(CedarDefault(r)))
		}
		x.save(group, c)
	}
}

func (x *cedarExporter) save(group string, c cedarCase) {
	dir := filepath.Join(x.root, group)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		x.t.Fatal(err)
	}
	file := strings.NewReplacer("/", "_", "=", "", " ", "_", ":", "", "(", "", ")", "").Replace(c.Name) + "_" + c.Label + ".json"
	b, err := json.Marshal(c)
	if err != nil {
		x.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), b, 0o644); err != nil {
		x.t.Fatal(err)
	}
}

// TestExportCedarSpecs writes the case files for cmd/cedar-rs into
// $CEDAR_SPEC_DIR, mirroring the Go benchmarks and scenarios one for one:
//
//	scenarios/      functional cases (cedar-rs check)
//	bench-small/    RBAC allow/deny/mixed up to 10000, ABAC and wildcard
//	bench-manyusers/
//	bench-100k/     RBAC allow/deny/mixed at 100000
//	parallel/       BenchmarkParallel's request
func TestExportCedarSpecs(t *testing.T) {
	root := os.Getenv("CEDAR_SPEC_DIR")
	if root == "" {
		t.Skip("set CEDAR_SPEC_DIR")
	}
	x := &cedarExporter{t: t, root: root, sets: map[string]GenSet{}}

	for _, s := range scenarios {
		dir, _ := filepath.Abs(filepath.Join("scenarios", s.Dir))
		c := cedarCase{Name: s.Title, Label: "cedar-rs", Policies: filepath.Join(dir, "cedar.cedar"), Entities: filepath.Join(dir, "cedar.entities.json")}
		m := s.CedarMap
		if m == nil {
			m = CedarDefault
		}
		for _, cs := range s.Cases {
			c.Requests = append(c.Requests, rawCedar(m(cs.Req)))
			c.Want = append(c.Want, cs.Want)
		}
		x.save("scenarios", c)
	}

	single := func(group, bench, kind string, n int, r Req, want bool) {
		x.write(group, fmt.Sprintf("%s/n=%d", bench, n), kind, n, []Req{r}, []bool{want}, 0)
	}
	for _, n := range benchSizes {
		group := "bench-small"
		if n == 100000 {
			group = "bench-100k"
		}
		single(group, "BenchmarkRBACAllow", "plain", n, Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/book%d", n/2)}, true)
		single(group, "BenchmarkRBACDeny", "plain", n, Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/book%d", n/2+1)}, false)
		reqs, want := mixedReqs(n, 4096)
		x.write(group, fmt.Sprintf("BenchmarkRBACMixed/n=%d", n), "plain", n, reqs, want, 32)
	}
	for _, n := range shapeSizes {
		single("bench-small", "BenchmarkABAC", "abac", n, Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/book%d", n/2),
			Attrs: map[string]any{"att1": "val1", "att2": "val2"}}, true)
		single("bench-small", "BenchmarkWildcard", "wildcard", n, Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/type%d/chapter1", n/2)}, true)
	}
	for _, n := range []int{1000, 100000} {
		reqs, want := manyUsersReqs(n, 4096)
		x.write("bench-manyusers", fmt.Sprintf("BenchmarkManyUsers/n=%d", n), "manyusers", n, reqs, want, 32)
	}
	for _, n := range []int{1000, 10000} {
		single("parallel", "BenchmarkParallel", "plain", n, Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/book%d", n/2)}, true)
	}
}
