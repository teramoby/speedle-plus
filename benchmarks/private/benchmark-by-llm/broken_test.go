package authzbench

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBrokenPolicyBehavior: when is a broken policy reported, and what does
// the engine decide afterwards?
func TestBrokenPolicyBehavior(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cond.spdl")
	os.WriteFile(p, []byte("[service.app]\n[policy]\nGRANT ROLE manager approve /expense if amount <= \nGRANT ROLE manager read /expense\n[rolepolicy]\nGRANT USER hank manager\n"), 0o644)
	s, err := NewSpeedle(p, false)
	fmt.Printf("speedle load err=%v\n", err)
	if err == nil {
		for _, a := range []float64{1, 1e9} {
			ok, reason, e := s.Ev.IsAllowed(s.Ctx(Req{User: "hank", Action: "approve", Resource: "/expense", Attrs: amount(a)}))
			fmt.Printf("speedle approve amount=%v -> allowed=%v reason=%v err=%v\n", a, ok, reason, e)
		}
		ok, _, e := s.Ev.IsAllowed(s.Ctx(Req{User: "hank", Action: "read", Resource: "/expense"}))
		fmt.Printf("speedle read (valid sibling policy) -> allowed=%v err=%v\n", ok, e)
	}

	conf, _ := os.ReadFile(filepath.Join(s1Dir(), "casbin.conf"))
	bad := filepath.Join(dir, "typo.conf")
	os.WriteFile(bad, []byte(strings.Replace(string(conf), "r.obj == p.obj", "r.obj == p.ob", 1)), 0o644)
	c, err := NewCasbin(bad, filepath.Join(s1Dir(), "casbin.csv"), CasbinArgs)
	fmt.Printf("casbin typo model: load err=%v\n", err)
	if err == nil {
		ok, e := c.Allowed(Req{User: "alice", Action: "read", Resource: "/data1"})
		fmt.Printf("casbin typo model: first Enforce -> allowed=%v err=%v\n", ok, e)
	}
}
