// Licensed under the Universal Permissive License (UPL) Version 1.0.
package eval

import (
	"regexp"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
)

func expressionPolicy(id string, expressions ...string) *pms.Policy {
	policy := &pms.Policy{ID: id, Effect: "grant", Principals: [][]string{{"user:alice"}}}
	for _, expression := range expressions {
		policy.Permissions = append(policy.Permissions, &pms.Permission{ResourceExpression: expression, Actions: []string{"read"}})
	}
	return policy
}

func TestPolicyExpressionsFollowPolicies(t *testing.T) {
	cache := NewPolicyCacheData()
	cache.AddPolicyToCache(expressionPolicy("p1", "/books/x/.*", "/books/y/.*"), nil)
	cache.AddPolicyToCache(expressionPolicy("p2", "/books/x/.*"), nil)
	if got := cache.Expressions.size(); got != 2 {
		t.Fatalf("expressions after adding: %d, want 2", got)
	}

	cache.DeletePolicyFromCache("p1")
	if got := cache.Expressions.size(); got != 1 {
		t.Fatalf("expressions after deleting p1: %d, want 1 (p2 still uses /books/x/.*)", got)
	}
	if _, ok := cache.Expressions.expressions["/books/x/.*"]; !ok {
		t.Fatal("the expression p2 uses was dropped")
	}

	cache.DeletePolicyFromCache("p2")
	cache.DeletePolicyFromCache("p2") // deleting again changes nothing
	if got := cache.Expressions.size(); got != 0 {
		t.Fatalf("expressions after deleting every policy: %d, want 0", got)
	}
}

func TestPolicyAddedAgainReleasesOldExpressions(t *testing.T) {
	cache := NewPolicyCacheData()
	cache.AddPolicyToCache(expressionPolicy("p1", "/old/.*"), nil)
	cache.AddPolicyToCache(expressionPolicy("p1", "/new/.*"), nil)
	if _, ok := cache.Expressions.expressions["/old/.*"]; ok {
		t.Fatal("the replaced policy's expression is still held")
	}
	cache.DeletePolicyFromCache("p1")
	if got := cache.Expressions.size(); got != 0 {
		t.Fatalf("expressions after deleting: %d, want 0", got)
	}
}

func TestRolePolicyExpressionsFollowRolePolicies(t *testing.T) {
	cache := NewRolePolicyCacheData()
	role := func(id string, expressions ...string) *pms.RolePolicy {
		return &pms.RolePolicy{ID: id, Effect: "grant", Roles: []string{"reader"},
			Principals: []string{"user:alice"}, ResourceExpressions: expressions}
	}
	cache.AddRolePolicyToCache(role("r1", "/books/x/.*", "/books/x/.*"), nil)
	cache.AddRolePolicyToCache(role("r2", "/books/x/.*", "/books/z/.*"), nil)
	if got := cache.Expressions.size(); got != 2 {
		t.Fatalf("expressions after adding: %d, want 2", got)
	}
	cache.AddRolePolicyToCache(role("r2", "/books/z/.*"), nil)
	cache.DeleteRolePolicyFromCache("r1")
	if _, ok := cache.Expressions.expressions["/books/x/.*"]; ok {
		t.Fatal("/books/x/.* is held after every role policy using it is gone")
	}
	cache.DeleteRolePolicyFromCache("r2")
	if got := cache.Expressions.size(); got != 0 {
		t.Fatalf("expressions after deleting every role policy: %d, want 0", got)
	}
}

// The cached match must agree with regexp.MatchString, including on an
// expression that does not compile.
func TestExpressionCacheMatchesLikeRegexp(t *testing.T) {
	cases := []struct{ expression, s string }{
		{"/books/x/.*", "/books/x/1"},
		{"/books/x/.*", "/shelf/books/x/1"}, // unanchored: matches mid-string
		{"^/books/x/.*", "/shelf/books/x/1"},
		{"/books/[0-9]+$", "/books/12"},
		{"/books/[0-9]+$", "/books/12a"},
		{"/books/(", "/books/("},
	}
	var cache ExpressionCache
	for _, c := range cases {
		cache.acquire(c.expression)
	}
	for _, c := range cases {
		want, wantErr := regexp.MatchString(c.expression, c.s)
		for name, m := range map[string]*ExpressionCache{"cached": &cache, "miss": {}, "nil": nil} {
			got, err := m.matchString(c.expression, c.s)
			if got != want || (err == nil) != (wantErr == nil) {
				t.Errorf("%s: matchString(%q, %q) = %v, %v; want %v, %v", name, c.expression, c.s, got, err, want, wantErr)
			}
		}
	}
}

func TestInvalidExpressionKeepsItsMeaning(t *testing.T) {
	cache := NewPolicyCacheData()
	policy := expressionPolicy("p1", "/books/(")
	cache.AddPolicyToCache(policy, nil)
	ctx := &internalRequestContext{Resource: "/books/(", Action: "read"}
	if matchResourceAction(&cache.Expressions, policy, ctx) {
		t.Error("a policy whose expression does not compile matched a request")
	}
	// A deny permission whose expression does not compile denies, as before.
	granted := []pms.Permission{{Resource: "/books/1", Actions: []string{"read"}}}
	denied := []pms.Permission{{ResourceExpression: "/books/(", Actions: []string{"read"}}}
	if got := calculatePermissions(&cache.Expressions, granted, denied); len(got) != 0 {
		t.Errorf("permissions = %v, want none", got)
	}
}

// Evaluation must use the expressions compiled when the policy was added
// rather than compile them on every request (issue #179).
func TestCachedExpressionIsNotCompiledPerMatch(t *testing.T) {
	const expression = "/books/x/.*"
	var cache ExpressionCache
	cache.acquire(expression)
	cached := testing.AllocsPerRun(100, func() { cache.matchString(expression, "/books/x/1") })
	compiled := testing.AllocsPerRun(100, func() { regexp.MatchString(expression, "/books/x/1") })
	if cached >= compiled {
		t.Fatalf("a cached match allocates %.0f times, as many as compiling (%.0f)", cached, compiled)
	}
}
