package authzbench

import (
	"bufio"

	"fmt"
	"github.com/casbin/casbin/v3"
	"os"
	"path/filepath"
	"strings"
)

// Generated policy sets mirror Speedle+'s own benchmark: n roles, roleI may
// read /books/bookI, and users userI-1..userI-U hold roleI.
//
// Kinds:
//
//	plain     exact resource match
//	abac      each rule also requires att1 == "val1" && att2 == "val2"
//	wildcard  roleI may read anything under /books/typeI/
type GenSet struct {
	Kind              string
	N, Users          int
	CasbinConf        string
	CasbinMatcherConf string // abac only: condition in the matcher, not per rule
	CasbinTunedConf   string // plain/wildcard: g() moved to the end of the matcher
	CasbinCSV         string
	CasbinPlainCSV    string // abac only: rules without the per-rule condition
	OPARego, OPAJSON  string
	SpeedleSPDL       string
}

const casbinRBAC = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && %s && r.act == p.act
`

const casbinABACEval = `[request_definition]
r = sub, obj, act, attrs

[policy_definition]
p = sub, obj, act, cond

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act && eval(p.cond)
`

const casbinABACMatcher = `[request_definition]
r = sub, obj, act, attrs

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act && r.attrs.Att1 == "val1" && r.attrs.Att2 == "val2"
`

const opaRBAC = `package authz

import rego.v1

default allow := false

allow if {
	some role in data.user_roles[input.user]
	some perm in data.role_perms[role]
	perm.action == input.action
	%s
}
`

func writeFile(path string, fill func(w *bufio.Writer)) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	fill(w)
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func GenPolicies(dir, kind string, n, users int) (GenSet, error) {
	if n <= 0 || users <= 0 {
		return GenSet{}, fmt.Errorf("n and users must be positive")
	}
	if kind == "manyusers" {
		return genManyUsers(dir, n)
	}
	if kind != "plain" && kind != "abac" && kind != "wildcard" {
		return GenSet{}, fmt.Errorf("unknown policy kind %q", kind)
	}
	g := GenSet{Kind: kind, N: n, Users: users}
	base := filepath.Join(dir, fmt.Sprintf("%s_%d", kind, n))
	g.CasbinConf, g.CasbinCSV = base+"_casbin.conf", base+"_casbin.csv"
	g.OPARego, g.OPAJSON, g.SpeedleSPDL = base+"_opa.rego", base+"_opa.json", base+"_speedle.spdl"

	res := func(i int) string { return fmt.Sprintf("/books/book%d", i) }
	if kind == "wildcard" {
		res = func(i int) string { return fmt.Sprintf("/books/type%d/*", i) }
	}

	// ---- Casbin
	conf := fmt.Sprintf(casbinRBAC, "r.obj == p.obj")
	switch kind {
	case "wildcard":
		conf = fmt.Sprintf(casbinRBAC, "keyMatch(r.obj, p.obj)")
	case "abac":
		conf = casbinABACEval
	}
	if err := os.WriteFile(g.CasbinConf, []byte(conf), 0o644); err != nil {
		return g, err
	}
	if kind != "abac" {
		objMatch := "r.obj == p.obj"
		if kind == "wildcard" {
			objMatch = "keyMatch(r.obj, p.obj)"
		}
		tuned := strings.Replace(fmt.Sprintf(casbinRBAC, objMatch),
			"m = g(r.sub, p.sub) && "+objMatch+" && r.act == p.act",
			"m = "+objMatch+" && r.act == p.act && g(r.sub, p.sub)", 1)
		g.CasbinTunedConf = base + "_casbin_tuned.conf"
		if err := os.WriteFile(g.CasbinTunedConf, []byte(tuned), 0o644); err != nil {
			return g, err
		}
	}
	writeCSV := func(path string, cond string) error {
		return writeFile(path, func(w *bufio.Writer) {
			for i := 1; i <= n; i++ {
				fmt.Fprintf(w, "p, role%d, %s, read%s\n", i, res(i), cond)
				for j := 1; j <= users; j++ {
					fmt.Fprintf(w, "g, user%d-%d, role%d\n", i, j, i)
				}
			}
		})
	}
	cond := ""
	if kind == "abac" {
		cond = ", r.attrs.Att1 == 'val1' && r.attrs.Att2 == 'val2'"
	}
	if err := writeCSV(g.CasbinCSV, cond); err != nil {
		return g, err
	}
	if kind == "abac" {
		g.CasbinMatcherConf, g.CasbinPlainCSV = base+"_casbin_matcher.conf", base+"_casbin_plain.csv"
		if err := os.WriteFile(g.CasbinMatcherConf, []byte(casbinABACMatcher), 0o644); err != nil {
			return g, err
		}
		if err := writeCSV(g.CasbinPlainCSV, ""); err != nil {
			return g, err
		}
	}

	// ---- OPA
	match := "perm.resource == input.resource"
	switch kind {
	case "wildcard":
		match = `glob.match(perm.resource, ["/"], input.resource)`
	case "abac":
		match = "perm.resource == input.resource\n\tevery k, v in perm.require { input.attrs[k] == v }"
	}
	if err := os.WriteFile(g.OPARego, []byte(fmt.Sprintf(opaRBAC, match)), 0o644); err != nil {
		return g, err
	}
	err := writeFile(g.OPAJSON, func(w *bufio.Writer) {
		w.WriteString(`{"user_roles":{`)
		for i := 1; i <= n; i++ {
			for j := 1; j <= users; j++ {
				if i > 1 || j > 1 {
					w.WriteByte(',')
				}
				fmt.Fprintf(w, `"user%d-%d":["role%d"]`, i, j, i)
			}
		}
		w.WriteString(`},"role_perms":{`)
		for i := 1; i <= n; i++ {
			if i > 1 {
				w.WriteByte(',')
			}
			extra := ""
			if kind == "abac" {
				extra = `,"require":{"att1":"val1","att2":"val2"}`
			}
			fmt.Fprintf(w, `"role%d":[{"action":"read","resource":"%s"%s}]`, i, res(i), extra)
		}
		w.WriteString("}}")
	})
	if err != nil {
		return g, err
	}

	// ---- Speedle+
	err = writeFile(g.SpeedleSPDL, func(w *bufio.Writer) {
		w.WriteString("[service.app]\n[policy]\n")
		for i := 1; i <= n; i++ {
			switch kind {
			case "wildcard":
				fmt.Fprintf(w, "GRANT ROLE role%d read expr:/books/type%d/.*\n", i, i)
			case "abac":
				fmt.Fprintf(w, "GRANT ROLE role%d read /books/book%d if att1 == \"val1\" && att2 == \"val2\"\n", i, i)
			default:
				fmt.Fprintf(w, "GRANT ROLE role%d read /books/book%d\n", i, i)
			}
		}
		w.WriteString("[rolepolicy]\n")
		for i := 1; i <= n; i++ {
			us := make([]string, users)
			for j := range us {
				us[j] = fmt.Sprintf("USER user%d-%d", i, j+1)
			}
			fmt.Fprintf(w, "GRANT %s role%d\n", strings.Join(us, ", "), i)
		}
	})
	return g, err
}

// LoadEngines builds every engine variant for a generated set. If
// AUTHZ_ENGINES is set (comma-separated names), only those are built, so a
// large set can be measured one engine per process.
func LoadEngines(g GenSet, synced bool) (map[string]Engine, error) {
	only := map[string]bool{}
	for _, n := range strings.Split(os.Getenv("AUTHZ_ENGINES"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			only[n] = true
		}
	}
	want := func(name string) bool { return len(only) == 0 || only[name] }
	newC := NewCasbin
	if synced {
		newC = NewCasbinSynced
	}
	args := CasbinArgs
	if g.Kind == "abac" {
		args = CasbinAttrArgs
	}
	out := map[string]Engine{}
	if want("casbin") {
		c, err := newC(g.CasbinConf, g.CasbinCSV, args)
		if err != nil {
			return nil, fmt.Errorf("casbin: %w", err)
		}
		out["casbin"] = c
	}
	if g.Kind == "abac" && want("casbin-matcher") {
		cm, err := newC(g.CasbinMatcherConf, g.CasbinPlainCSV, args)
		if err != nil {
			return nil, fmt.Errorf("casbin-matcher: %w", err)
		}
		out["casbin-matcher"] = cm
	}
	if g.CasbinTunedConf != "" && want("casbin-tuned") {
		ct, err := newC(g.CasbinTunedConf, g.CasbinCSV, args)
		if err != nil {
			return nil, fmt.Errorf("casbin-tuned: %w", err)
		}
		out["casbin-tuned"] = ct
	}
	if (g.Kind == "plain" || g.Kind == "manyusers") && !synced && want("casbin-cached") {
		ce, err := casbin.NewCachedEnforcer(g.CasbinTunedConf, g.CasbinCSV)
		if err != nil {
			return nil, fmt.Errorf("casbin-cached: %w", err)
		}
		out["casbin-cached"] = &CasbinEngine{E: ce, ToArgs: args}
	}
	if want("opa") {
		o, err := NewOPA(g.OPARego, g.OPAJSON)
		if err != nil {
			return nil, fmt.Errorf("opa: %w", err)
		}
		out["opa"] = o
	}
	if want("speedle") {
		s, err := NewSpeedle(g.SpeedleSPDL, false)
		if err != nil {
			return nil, fmt.Errorf("speedle: %w", err)
		}
		out["speedle"] = s
	}
	return out, nil
}

// manyusers keeps permission count fixed at 100 while growing membership.
func genManyUsers(dir string, n int) (GenSet, error) {
	tmp, err := os.MkdirTemp(dir, "manyusers-build-")
	if err != nil {
		return GenSet{}, err
	}
	defer os.RemoveAll(tmp)
	g, err := GenPolicies(tmp, "plain", 10, 1)
	if err != nil {
		return g, err
	}
	g.Kind, g.N, g.Users = "manyusers", n, n
	err = writeFile(g.CasbinCSV, func(w *bufio.Writer) {
		for r := 1; r <= 10; r++ {
			for k := 1; k <= 10; k++ {
				fmt.Fprintf(w, "p, role%d, /books/book%d-%d, read\n", r, r, k)
			}
		}
		for u := 1; u <= n; u++ {
			fmt.Fprintf(w, "g, user%d, role%d\n", u, (u-1)%10+1)
		}
	})
	if err != nil {
		return g, err
	}
	err = writeFile(g.OPAJSON, func(w *bufio.Writer) {
		w.WriteString(`{"user_roles":{`)
		for u := 1; u <= n; u++ {
			if u > 1 {
				w.WriteByte(',')
			}
			fmt.Fprintf(w, `"user%d":["role%d"]`, u, (u-1)%10+1)
		}
		w.WriteString(`},"role_perms":{`)
		for r := 1; r <= 10; r++ {
			if r > 1 {
				w.WriteByte(',')
			}
			fmt.Fprintf(w, `"role%d":[`, r)
			for k := 1; k <= 10; k++ {
				if k > 1 {
					w.WriteByte(',')
				}
				fmt.Fprintf(w, `{"action":"read","resource":"/books/book%d-%d"}`, r, k)
			}
			w.WriteByte(']')
		}
		w.WriteString("}}")
	})
	if err != nil {
		return g, err
	}
	err = writeFile(g.SpeedleSPDL, func(w *bufio.Writer) {
		w.WriteString("[service.app]\n[policy]\n")
		for r := 1; r <= 10; r++ {
			for k := 1; k <= 10; k++ {
				fmt.Fprintf(w, "GRANT ROLE role%d read /books/book%d-%d\n", r, r, k)
			}
		}
		w.WriteString("[rolepolicy]\n")
		for u := 1; u <= n; u++ {
			fmt.Fprintf(w, "GRANT USER user%d role%d\n", u, (u-1)%10+1)
		}
	})
	// Files must be distinct from plain/10, which can coexist in one process.
	for _, pp := range []*string{&g.CasbinConf, &g.CasbinTunedConf, &g.CasbinCSV, &g.OPARego, &g.OPAJSON, &g.SpeedleSPDL} {
		dst := filepath.Join(dir, strings.Replace(filepath.Base(*pp), "plain_10_", fmt.Sprintf("manyusers_%d_", n), 1))
		b, e := os.ReadFile(*pp)
		if e != nil {
			return g, e
		}
		if e = os.WriteFile(dst, b, 0644); e != nil {
			return g, e
		}
		*pp = dst
	}
	return g, err
}
