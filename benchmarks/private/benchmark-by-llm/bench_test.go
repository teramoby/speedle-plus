package authzbench

import (
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

var (
	genDir  string
	cacheMu sync.Mutex
	sets    = map[string]GenSet{}
	loaded  = map[string]map[string]Engine{}
)

func TestMain(m *testing.M) {
	genDir, _ = os.MkdirTemp("", "authzbench-gen-")
	code := m.Run()
	os.RemoveAll(genDir)
	os.Exit(code)
}

const usersPerRole = 10

func genSet(tb testing.TB, kind string, n int) GenSet {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	key := fmt.Sprintf("%s/%d", kind, n)
	if g, ok := sets[key]; ok {
		return g
	}
	g, err := GenPolicies(genDir, kind, n, usersPerRole)
	if err != nil {
		tb.Fatal(err)
	}
	sets[key] = g
	return g
}

// engines loads (once per process) all engines for kind/n.
func engines(tb testing.TB, kind string, n int, synced bool) map[string]Engine {
	g := genSet(tb, kind, n)
	cacheMu.Lock()
	defer cacheMu.Unlock()
	key := fmt.Sprintf("%s/%d/%v", kind, n, synced)
	if es, ok := loaded[key]; ok {
		return es
	}
	es, err := LoadEngines(g, synced)
	if err != nil {
		tb.Fatal(err)
	}
	loaded[key] = es
	return es
}

func names(es map[string]Engine) []string {
	var ns []string
	for n := range es {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}

var benchSizes = []int{100, 1000, 10000, 100000}

func runDecision(b *testing.B, kind string, n int, req Req, want bool, only ...string) {
	es := engines(b, kind, n, false)
	for _, name := range names(es) {
		if len(only) > 0 && !contains(only, name) {
			continue
		}
		check := es[name].Prepare(req)
		b.Run(name, func(b *testing.B) {
			if ok, err := check(); err != nil || ok != want {
				b.Fatalf("%s: got %v, %v; want %v", name, ok, err, want)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if ok, err := check(); err != nil || ok != want {
					b.Fatalf("decision: %v %v", ok, err)
				}
			}
		})
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// BenchmarkRBACAllow: a user asks for the one resource their role grants.
func BenchmarkRBACAllow(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			req := Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/book%d", n/2)}
			runDecision(b, "plain", n, req, true)
		})
	}
}

// BenchmarkRBACDeny: the same user asks for a resource nobody granted them.
func BenchmarkRBACDeny(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			req := Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/book%d", n/2+1)}
			runDecision(b, "plain", n, req, false)
		})
	}
}

// BenchmarkABAC: every rule also carries an attribute condition.
func BenchmarkABAC(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			req := Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/book%d", n/2),
				Attrs: map[string]any{"att1": "val1", "att2": "val2"}}
			runDecision(b, "abac", n, req, true)
		})
	}
}

// BenchmarkWildcard: every rule grants a resource prefix.
func BenchmarkWildcard(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			req := Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/type%d/chapter1", n/2)}
			runDecision(b, "wildcard", n, req, true)
		})
	}
}

// BenchmarkParallel: all cores ask at once (Casbin uses SyncedEnforcer).
func BenchmarkParallel(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			es := engines(b, "plain", n, true)
			req := Req{User: fmt.Sprintf("user%d-7", n/2), Action: "read", Resource: fmt.Sprintf("/books/book%d", n/2)}
			for _, name := range names(es) {
				check := es[name].Prepare(req)
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							if ok, err := check(); !ok || err != nil {
								b.Errorf("%s: %v %v", name, ok, err)
								return
							}
						}
					})
				})
			}
		})
	}
}

func heap() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// TestLoadCost measures load time and retained heap per engine.
// Run with LOADCOST=1 (it takes a while at n=100000).
func TestLoadCost(t *testing.T) {
	if os.Getenv("LOADCOST") == "" {
		t.Skip("set LOADCOST=1")
	}
	fmt.Println("\n| n roles (users) | engine | load time | retained heap |\n|---|---|---|---|")
	for _, n := range []int{1000, 10000, 100000} {
		g := genSet(t, "plain", n)
		loaders := []struct {
			name string
			load func() (Engine, error)
		}{
			{"casbin", func() (Engine, error) { return NewCasbin(g.CasbinConf, g.CasbinCSV, CasbinArgs) }},
			{"opa", func() (Engine, error) { return NewOPA(g.OPARego, g.OPAJSON) }},
			{"speedle", func() (Engine, error) { return NewSpeedle(g.SpeedleSPDL, false) }},
		}
		for _, l := range loaders {
			before := heap()
			start := time.Now()
			e, err := l.load()
			dt := time.Since(start)
			if err != nil {
				t.Fatalf("%s: %v", l.name, err)
			}
			after := heap()
			runtime.KeepAlive(e)
			fmt.Printf("| %d (%d) | %s | %v | %.1f MB |\n", n, n*usersPerRole, l.name, dt.Round(time.Millisecond), float64(after-before)/(1<<20))
			e = nil
		}
	}
}

// mixedReqs returns k requests over random roles and users: 80% allowed,
// 20% denied (another role's book), like cmd/loadgen.
func mixedReqs(n, k int) ([]Req, []bool) {
	rng := rand.New(rand.NewPCG(42, 7))
	reqs, want := make([]Req, k), make([]bool, k)
	for j := range reqs {
		i := rng.IntN(n) + 1
		res := fmt.Sprintf("/books/book%d", i)
		want[j] = rng.IntN(10) < 8
		if !want[j] {
			res = fmt.Sprintf("/books/book%d", i%n+1)
		}
		reqs[j] = Req{User: fmt.Sprintf("user%d-%d", i, rng.IntN(usersPerRole)+1), Action: "read", Resource: res}
	}
	return reqs, want
}

// BenchmarkRBACMixed: 4096 different requests cycled, so no engine benefits
// from asking the same question repeatedly.
func BenchmarkRBACMixed(b *testing.B) { benchmarkMixed(b, "plain", benchSizes) }

func BenchmarkManyUsers(b *testing.B) { benchmarkMixed(b, "manyusers", []int{1000, 100000}) }

func benchmarkMixed(b *testing.B, kind string, sizes []int) {
	for _, n := range sizes {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			es := engines(b, kind, n, false)
			reqs, want := mixedReqs(n, 4096)
			if kind == "manyusers" {
				reqs, want = manyUsersReqs(n, 4096)
			}
			for _, name := range names(es) {
				checks := make([]func() (bool, error), len(reqs))
				for j, r := range reqs {
					checks[j] = es[name].Prepare(r)
				}
				b.Run(name, func(b *testing.B) {
					for j, c := range checks {
						if ok, err := c(); err != nil || ok != want[j] {
							b.Fatalf("%s: %+v: got %v, %v; want %v", name, reqs[j], ok, err, want[j])
						}
						if j == 31 && name != "casbin-cached" {
							break
						}
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						j := i & 4095
						if ok, err := checks[j](); err != nil || ok != want[j] {
							b.Fatalf("%s decision: %v %v want %v", name, ok, err, want[j])
						}
					}
				})
			}
		})
	}
}

func manyUsersReqs(n, k int) ([]Req, []bool) {
	rng := rand.New(rand.NewPCG(42, 7))
	reqs, want := make([]Req, k), make([]bool, k)
	for j := range reqs {
		u := rng.IntN(n) + 1
		r := (u-1)%10 + 1
		book := rng.IntN(10) + 1
		want[j] = rng.IntN(10) < 8
		if !want[j] {
			r = r%10 + 1
		}
		reqs[j] = Req{User: fmt.Sprintf("user%d", u), Action: "read", Resource: fmt.Sprintf("/books/book%d-%d", r, book)}
	}
	return reqs, want
}
