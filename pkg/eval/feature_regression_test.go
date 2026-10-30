package eval

import (
	"encoding/json"
	"fmt"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/testutil/featuretest"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func featureEvaluator(t *testing.T, ps *pms.PolicyStore) *PolicyEvalImpl {
	t.Helper()
	b, err := json.Marshal(ps)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := newFixtureEvaluator(t, string(b))
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	return e
}
func featurePolicy(id, expression string) *pms.Policy {
	return &pms.Policy{ID: id, Name: id, Effect: pms.Grant, Condition: expression, Permissions: []*pms.Permission{{Resource: "book", Actions: []string{"read"}}}}
}
func featureContext() ads.RequestContext {
	return ads.RequestContext{ServiceName: "app", Resource: "book", Action: "read", Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}}}}
}
func TestFeatureBuiltinAuthorization(t *testing.T) {
	e := featureEvaluator(t, featuretest.BuiltinStore())
	for _, c := range featuretest.Builtins {
		t.Run(c.Name, func(t *testing.T) {
			for repeat := 0; repeat < 2; repeat++ {
				got, reason, err := e.IsAllowed(ads.RequestContext{ServiceName: "feature-functions", Resource: c.Name, Action: "read"})
				wantReason := ads.NO_APPLICABLE_POLICIES
				if c.Want {
					wantReason = ads.GRANT_POLICY_FOUND
				}
				if err != nil || got != c.Want || reason != wantReason {
					t.Fatalf("%s: %v %v %v", c.Expression, got, reason, err)
				}
			}
		})
	}
	// Embedded callers can supply unsupported nested/map values: reject without panic.
	for _, x := range []interface{}{[]interface{}{[]interface{}{"a"}}, []interface{}{map[string]int{"a": 1}}} {
		p := featurePolicy("subset", "IsSubSet(x,x)")
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{p}}}})
		ctx := featureContext()
		ctx.Attributes = map[string]interface{}{"x": x}
		if ok, _, err := e.IsAllowed(ctx); ok || err != nil {
			t.Fatalf("invalid set %v %v", ok, err)
		}
	}
}
func TestFeatureTypedFunctionCache(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req ext.CustomerFunctionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		_, numeric := req.Params[0].(float64)
		json.NewEncoder(w).Encode(map[string]interface{}{"result": numeric})
	}))
	defer server.Close()
	for _, cached := range []bool{false, true} {
		e := featureEvaluator(t, &pms.PolicyStore{Functions: []*pms.Function{{Name: "typed", FuncURL: server.URL, ResultCachable: cached}}, Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{featurePolicy("p", "typed(x)")}}}})
		before := calls.Load()
		for repeat := 0; repeat < 2; repeat++ {
			for _, x := range []interface{}{1.0, "1"} {
				ctx := featureContext()
				ctx.Attributes = map[string]interface{}{"x": x}
				want := x == 1.0
				got, _, err := e.IsAllowed(ctx)
				if got != want || err != nil {
					t.Fatalf("cache=%v x=%T result=%v err=%v", cached, x, got, err)
				}
			}
		}
		want := int64(4)
		if cached {
			want = 2
		}
		if calls.Load()-before != want {
			t.Fatalf("calls %d want %d", calls.Load()-before, want)
		}
	}
	inputs := [][]interface{}{{1.0}, {"1"}, {true}, {"true"}, {nil}, {"<nil>"}, {"a b"}, {"a", "b"}, {[]interface{}{"a", "b"}}, {"[a b]"}}
	seen := map[string]bool{}
	for _, args := range inputs {
		key, err := getKey("f", args)
		if err != nil || seen[key] {
			t.Fatalf("key collision %v: %q %v", args, key, err)
		}
		seen[key] = true
	}
	if _, err := getKey("f", []interface{}{make(chan int)}); err == nil {
		t.Fatal("unencodable key")
	}
}
func TestFeatureRegexDifferential(t *testing.T) {
	patterns := []string{"book/.*", "^book/.*", ".*book", ".*book$", "^book/.*$", ".*书籍$", "book/(one|two)$", ".*", "^.*$"}
	resources := []string{"book/one", "prefixbook/one", "booktail", "xbooktail", "xbook", "x书籍", "\nbook/one", "", "unrelated"}
	for _, role := range []bool{false, true} {
		for _, effect := range []string{pms.Grant, pms.Deny} {
			s := &pms.Service{Name: "app"}
			for i, pattern := range patterns {
				id := fmt.Sprint(i)
				if role {
					s.RolePolicies = append(s.RolePolicies, &pms.RolePolicy{ID: "r" + id, Effect: effect, Principals: []string{"user:alice"}, Roles: []string{"r" + id}, ResourceExpressions: []string{pattern}})
					p := featurePolicy("p"+id, "")
					p.Principals = [][]string{{"role:r" + id}}
					p.Permissions[0].Resource = ""
					s.Policies = append(s.Policies, p)
					if effect == pms.Deny {
						s.RolePolicies = append(s.RolePolicies, &pms.RolePolicy{ID: "base" + id, Effect: pms.Grant, Principals: []string{"user:alice"}, Roles: []string{"r" + id}})
					}
				} else {
					p := featurePolicy(id, "")
					p.Effect = effect
					p.Principals = [][]string{{"user:alice"}}
					p.Permissions[0].Resource = ""
					p.Permissions[0].ResourceExpression = pattern
					p.Permissions[0].Actions = []string{id}
					s.Policies = append(s.Policies, p)
				}
			}
			if !role && effect == pms.Deny {
				s.Policies = append(s.Policies, &pms.Policy{ID: "base", Effect: pms.Grant})
			}
			e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{s}})
			for i, pattern := range patterns {
				for _, resource := range resources {
					matched := regexp.MustCompile(pattern).MatchString(resource)
					want := matched
					if effect == pms.Deny {
						want = !matched
					}
					ctx := featureContext()
					ctx.Resource = resource
					ctx.Action = fmt.Sprint(i)
					if role { // Restrict the authorization policy to the role under comparison.
						e.RuntimePolicyStore.reloadPolicyStore(&pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{{ID: "check", Effect: pms.Grant, Principals: [][]string{{"role:r" + fmt.Sprint(i)}}}}, RolePolicies: s.RolePolicies}}})
					}
					got, _, err := e.IsAllowed(ctx)
					if got != want || err != nil {
						t.Fatalf("role=%v %s %q %q got=%v want=%v err=%v", role, effect, pattern, resource, got, want, err)
					}
				}
			}
		}
	}
}
func TestFeaturePrincipalsTruthTable(t *testing.T) {
	p := featurePolicy("grant", "amount < 100 && risk < 50 && Avg(Min(3,4),Max(2,8),Sqrt(4)) == 13 / 3")
	p.Principals = [][]string{{"idd=corp:user:alice", "group:devs"}, {"role:reader"}}
	deny := featurePolicy("deny", "blocked")
	deny.Effect = pms.Deny
	s := &pms.Service{Name: "app", Policies: []*pms.Policy{p, deny}, RolePolicies: []*pms.RolePolicy{{ID: "r1", Effect: pms.Grant, Principals: []string{"group:ops"}, Roles: []string{"admin"}}, {ID: "r2", Effect: pms.Grant, Principals: []string{"role:admin"}, Roles: []string{"reader"}}}}
	for order := 0; order < 2; order++ {
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{s}})
		for mask := 0; mask < 512; mask++ {
			bit := func(i int) bool { return mask&(1<<i) != 0 }
			ctx := featureContext()
			ctx.Subject = &ads.Subject{}
			ctx.Resource = "other"
			ctx.Action = "write"
			ctx.Attributes = map[string]interface{}{"amount": 1000.0, "risk": 1000.0, "blocked": bit(6)}
			if bit(0) {
				idd := "other"
				if bit(3) {
					idd = "corp"
				}
				ctx.Subject.Principals = append(ctx.Subject.Principals, &ads.Principal{Type: "user", Name: "alice", IDD: idd})
			}
			if bit(1) {
				ctx.Subject.Principals = append(ctx.Subject.Principals, &ads.Principal{Type: "group", Name: "devs"})
			}
			if bit(2) {
				ctx.Subject.Principals = append(ctx.Subject.Principals, &ads.Principal{Type: "group", Name: "ops"})
			}
			if bit(4) {
				ctx.Attributes["amount"] = 1.0
			}
			if bit(5) {
				ctx.Attributes["risk"] = 1.0
			}
			if bit(7) {
				ctx.Action = "read"
			}
			if bit(8) {
				ctx.Resource = "book"
			}
			want := (bit(0) && bit(1) && bit(3) || bit(2)) && bit(4) && bit(5) && !bit(6) && bit(7) && bit(8)
			for repeat := 0; repeat < 2; repeat++ {
				got, _, err := e.IsAllowed(ctx)
				if got != want || err != nil {
					t.Fatalf("order %d mask %09b: %v want %v: %v", order, mask, got, want, err)
				}
			}
			diagnostic, err := e.Diagnose(ctx)
			if err != nil || diagnostic.Allowed != want {
				t.Fatalf("diagnose mask %d: %+v %v", mask, diagnostic, err)
			}
		}
		slices.Reverse(s.Policies)
		slices.Reverse(s.RolePolicies)
	}
}
func TestFeatureRoleGraphReference(t *testing.T) {
	// An independent fixed-point oracle covers sparse/dense, cyclic and diamond
	// grant graphs; explicit user denial of a leaf must remove only that role.
	for _, seed := range []int64{18, 120, 214} {
		rng := rand.New(rand.NewSource(seed))
		s := &pms.Service{Name: "app"}
		const nodes = 32
		edges := make([][]int, nodes)
		for i := 0; i < nodes; i++ {
			for j := 0; j < nodes; j++ {
				if rng.Intn(12) == 0 {
					edges[i] = append(edges[i], j)
					s.RolePolicies = append(s.RolePolicies, &pms.RolePolicy{ID: fmt.Sprintf("edge-%d-%d", i, j), Effect: pms.Grant, Principals: []string{fmt.Sprintf("role:r%d", i)}, Roles: []string{fmt.Sprintf("r%d", j)}})
				}
			}
		}
		s.RolePolicies = append(s.RolePolicies, &pms.RolePolicy{ID: "root", Effect: pms.Grant, Principals: []string{"user:alice"}, Roles: []string{"r0", "leaf"}}, &pms.RolePolicy{ID: "deny-leaf", Effect: pms.Deny, Principals: []string{"user:alice"}, Roles: []string{"leaf"}})
		expected := map[string]bool{"r0": true}
		for changed := true; changed; {
			changed = false
			for i, targets := range edges {
				if expected[fmt.Sprintf("r%d", i)] {
					for _, j := range targets {
						role := fmt.Sprintf("r%d", j)
						if !expected[role] {
							expected[role] = true
							changed = true
						}
					}
				}
			}
		}
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{s}})
		got, err := e.GetAllGrantedRoles(featureContext())
		if err != nil {
			t.Fatal(err)
		}
		actual := map[string]bool{}
		for _, r := range got {
			actual[r] = true
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("seed %d: %v want %v", seed, actual, expected)
		}
	}
}
func TestFeatureDeepRolesAndConditions(t *testing.T) {
	for _, depth := range []int{100, 500} {
		s := &pms.Service{Name: "app", Policies: []*pms.Policy{featurePolicy("p", "")}}
		s.Policies[0].Principals = [][]string{{fmt.Sprintf("role:r%d", depth-1)}}
		for i := 0; i < depth; i++ {
			principal := "user:alice"
			if i > 0 {
				principal = fmt.Sprintf("role:r%d", i-1)
			}
			s.RolePolicies = append(s.RolePolicies, &pms.RolePolicy{ID: fmt.Sprint(i), Effect: pms.Grant, Principals: []string{principal}, Roles: []string{fmt.Sprintf("r%d", i)}})
		}
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{s}})
		if ok, _, err := e.IsAllowed(featureContext()); !ok || err != nil {
			t.Fatalf("depth %d: %v %v", depth, ok, err)
		}
	}
	for _, size := range []int{100, 1000} {
		expression := strings.Repeat("(x > 0 || y < 0) && ", size) + "true"
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{featurePolicy("p", expression)}}}})
		for _, x := range []float64{1, -1} {
			ctx := featureContext()
			ctx.Attributes = map[string]interface{}{"x": x, "y": 1.0}
			got, _, err := e.IsAllowed(ctx)
			if got != (x > 0) || err != nil {
				t.Fatalf("terms %d x %v: %v %v", size, x, got, err)
			}
		}
	}
}
func TestFeaturePermissionDecisionConsistency(t *testing.T) {
	for _, permissions := range [][]*pms.Permission{nil, {}, {{Resource: "book"}}, {{ResourceExpression: "book"}}, {{Resource: "book", Actions: []string{"read"}}}} {
		grant := featurePolicy("g", "")
		deny := &pms.Policy{ID: "d", Effect: pms.Deny, Permissions: permissions}
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{grant, deny}}}})
		ctx := featureContext()
		ok, reason, err := e.IsAllowed(ctx)
		if ok || reason != ads.DENY_POLICY_FOUND || err != nil {
			t.Fatalf("deny failed %v %v %v", ok, reason, err)
		}
		ctx.Resource = ""
		ctx.Action = ""
		perms, err := e.GetAllGrantedPermissions(ctx)
		if err != nil || len(perms) != 0 {
			t.Fatalf("denied permission listed: %+v %v", perms, err)
		}
	}
}
func TestFeatureConcurrentSnapshotRefresh(t *testing.T) {
	snapshot := func(generation int) *pms.PolicyStore {
		role := fmt.Sprintf("role%d", generation)
		p := featurePolicy("p", "")
		p.Principals = [][]string{{"role:" + role}}
		return &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{p}, RolePolicies: []*pms.RolePolicy{{ID: "rp", Effect: pms.Grant, Principals: []string{"user:alice"}, Roles: []string{role}}}}}}
	}
	b, _ := json.Marshal(snapshot(0))
	e, store := newFixtureEvaluator(t, string(b))
	defer e.Close()
	var wg sync.WaitGroup
	var failed atomic.Bool
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				if ok, _, err := e.IsAllowed(featureContext()); !ok || err != nil {
					failed.Store(true)
				}
			}
		}()
	}
	for generation := 1; generation <= 12; generation++ {
		if err := store.WritePolicyStore(snapshot(generation)); err != nil {
			t.Fatal(err)
		}
		if err := e.Refresh(); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if failed.Load() {
		t.Fatal("authorization saw a mixed snapshot")
	}
}
