package authzbench

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/topdown"
	"github.com/teramoby/speedle-plus/pkg/eval"
)

func s1Dir() string { return filepath.Join("scenarios", "s1_rbac") }

// TestListPermissions asks each engine "what may alice do?" (S1).
func TestListPermissions(t *testing.T) {
	d := s1Dir()
	c, err := NewCasbin(filepath.Join(d, "casbin.conf"), filepath.Join(d, "casbin.csv"), CasbinArgs)
	if err != nil {
		t.Fatal(err)
	}
	perms, err := c.E.(interface {
		GetImplicitPermissionsForUser(string, ...string) ([][]string, error)
	}).GetImplicitPermissionsForUser("alice")
	fmt.Printf("casbin  GetImplicitPermissionsForUser(alice) = %v, err=%v\n", perms, err)

	mod, _ := os.ReadFile(filepath.Join(d, "opa.rego"))
	o, err := NewOPA(filepath.Join(d, "opa.rego"), filepath.Join(d, "opa.json"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := rego.New(rego.Query("data.authz.permissions"), rego.Module("opa.rego", string(mod)), rego.Store(o.Store)).
		PrepareForEval(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rs, err := q.Eval(context.Background(), rego.EvalInput(map[string]any{"user": "alice"}))
	fmt.Printf("opa     data.authz.permissions (a rule written for it) = %v, err=%v\n", rs[0].Expressions[0].Value, err)

	s, err := NewSpeedle(filepath.Join(d, "speedle.spdl"), false)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := s.Ev.GetAllGrantedPermissions(s.Ctx(Req{User: "alice"}))
	var out []string
	for _, p := range sp {
		out = append(out, fmt.Sprintf("%s:%v", p.Resource, p.Actions))
	}
	fmt.Printf("speedle GetAllGrantedPermissions(alice) = %v, err=%v\n", out, err)
}

// TestExplain asks each engine why kim may not read /hr/salaries (S7).
func TestExplain(t *testing.T) {
	d := filepath.Join("scenarios", "s7_deny")
	req := Req{User: "kim", Action: "read", Resource: "/hr/salaries"}

	c, err := NewCasbin(filepath.Join(d, "casbin.conf"), filepath.Join(d, "casbin.csv"), CasbinArgs)
	if err != nil {
		t.Fatal(err)
	}
	ok, explain, err := c.E.(interface {
		EnforceEx(...interface{}) (bool, []string, error)
	}).EnforceEx(CasbinArgs(req)...)
	fmt.Printf("casbin  EnforceEx -> allowed=%v explain=%v err=%v\n", ok, explain, err)

	o, err := NewOPA(filepath.Join(d, "opa.rego"), filepath.Join(d, "opa.json"))
	if err != nil {
		t.Fatal(err)
	}
	buf := topdown.NewBufferTracer()
	rs, err := o.Q.Eval(context.Background(), rego.EvalInput(OPAInput(req)), rego.EvalQueryTracer(buf))
	var tr bytes.Buffer
	topdown.PrettyTraceWithLocation(&tr, *buf)
	lines := strings.Split(strings.TrimSpace(tr.String()), "\n")
	fmt.Printf("opa     allowed=%v err=%v; trace has %d events / %d lines; first lines:\n", rs.Allowed(), err, len(*buf), len(lines))
	for _, l := range lines[:min(8, len(lines))] {
		fmt.Println("        " + l)
	}

	s, err := NewSpeedle(filepath.Join(d, "speedle.spdl"), false)
	if err != nil {
		t.Fatal(err)
	}
	allowed, reason, err := s.Ev.IsAllowed(s.Ctx(req))
	fmt.Printf("speedle IsAllowed -> allowed=%v reason=%v err=%v\n", allowed, reason, err)
	res, err := s.Ev.Diagnose(s.Ctx(req))
	if err == nil {
		fmt.Printf("speedle Diagnose -> grantedRoles=%v", res.GrantedRoles)
		for _, p := range res.Policies {
			fmt.Printf(" policy{%+v}", *p)
		}
		fmt.Println()
	} else {
		fmt.Printf("speedle Diagnose err=%v\n", err)
	}
}

// TestRuntimeUpdate grants viewer write on /data1 at runtime and measures how
// long until bob is allowed (S1).
func TestRuntimeUpdate(t *testing.T) {
	d := s1Dir()
	bobWrite := Req{User: "bob", Action: "write", Resource: "/data1"}
	ctx := context.Background()

	c, _ := NewCasbin(filepath.Join(d, "casbin.conf"), filepath.Join(d, "casbin.csv"), CasbinArgs)
	start := time.Now()
	_, err := c.E.(interface {
		AddPolicy(...interface{}) (bool, error)
	}).AddPolicy("viewer", "/data1", "write")
	ok, _ := c.Allowed(bobWrite)
	fmt.Printf("casbin  AddPolicy in-process: allowed=%v after %v (err=%v)\n", ok, time.Since(start), err)

	o, _ := NewOPA(filepath.Join(d, "opa.rego"), filepath.Join(d, "opa.json"))
	start = time.Now()
	err = storage.WriteOne(ctx, o.Store, storage.AddOp, storage.MustParsePath("/role_perms/viewer/-"),
		map[string]any{"action": "write", "resource": "/data1"})
	ok, _ = o.Allowed(bobWrite)
	fmt.Printf("opa     storage write in-process: allowed=%v after %v (err=%v)\n", ok, time.Since(start), err)

	// Not t.TempDir(): Speedle+'s watcher calls log.Fatalf when the watched
	// file is removed, which would kill the test binary at cleanup.
	tdir, _ := os.MkdirTemp("", "speedle-watch-")
	tmp := filepath.Join(tdir, "s1.spdl")
	orig, _ := os.ReadFile(filepath.Join(d, "speedle.spdl"))
	os.WriteFile(tmp, orig, 0o644)
	ev, err := eval.NewFromFile(tmp, true)
	if err != nil {
		t.Fatal(err)
	}
	s := &SpeedleEngine{Ev: ev}
	updated := strings.Replace(string(orig), "[rolepolicy]", "GRANT ROLE viewer write /data1\n[rolepolicy]", 1)
	start = time.Now()
	os.WriteFile(tmp, []byte(updated), 0o644)
	for time.Since(start) < 15*time.Second {
		if ok, _ = s.Allowed(bobWrite); ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	fmt.Printf("speedle embedded file watch: allowed=%v after %v\n", ok, time.Since(start).Round(time.Millisecond))
}

// TestErrorMessages feeds each engine a broken policy and prints the error.
func TestErrorMessages(t *testing.T) {
	dir := t.TempDir()
	w := func(name, body string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), 0o644)
		return p
	}
	conf, _ := os.ReadFile(filepath.Join(s1Dir(), "casbin.conf"))
	csv := filepath.Join(s1Dir(), "casbin.csv")

	badMatcher := w("bad1.conf", strings.Replace(string(conf), "r.act == p.act", "(r.act == p.act", 1))
	c, err := NewCasbin(badMatcher, csv, CasbinArgs)
	if err == nil {
		_, err = c.Allowed(Req{User: "alice", Action: "read", Resource: "/data1"})
	}
	fmt.Printf("casbin  unbalanced paren in matcher: %v\n", err)

	typo := w("bad2.conf", strings.Replace(string(conf), "r.obj == p.obj", "r.obj == p.ob", 1))
	c, err = NewCasbin(typo, csv, CasbinArgs)
	if err == nil {
		_, err = c.Allowed(Req{User: "alice", Action: "read", Resource: "/data1"})
	}
	fmt.Printf("casbin  typo p.ob in matcher: %v\n", err)

	rg, _ := os.ReadFile(filepath.Join(s1Dir(), "opa.rego"))
	_, err = NewOPA(w("bad1.rego", strings.Replace(string(rg), "perm.resource == input.resource\n}", "perm.resource == input.resource\n", 1)), "")
	fmt.Printf("opa     missing closing brace: %v\n", err)
	_, err = NewOPA(w("bad2.rego", strings.Replace(string(rg), "some perm in data.role_perms[role]\n\tperm.action", "some perm in data.role_perms[role]\n\tpermm.action", 1)), "")
	fmt.Printf("opa     typo permm.action: %v\n", err)

	_, err = NewSpeedle(w("bad1.spdl", "[service.app]\n[policy]\nGRANT ROLE admin read\n"), false)
	fmt.Printf("speedle missing resource: %v\n", err)
	_, err = NewSpeedle(w("bad2.spdl", "[service.app]\n[policy]\nGRAN ROLE admin read /data1\n"), false)
	fmt.Printf("speedle typo GRAN: %v\n", err)
	_, err = NewSpeedle(w("bad3.spdl", "[service.app]\n[policy]\nGRANT ROLE admin read /data1 if amount <= \n"), false)
	fmt.Printf("speedle incomplete condition: %v\n", err)
}
