package authzbench

import (
	"fmt"
	"testing"
)

// Verify the new shape against all variants, including first/last users and
// all ten permissions for each role, with an adjacent-role denial for each.
func TestGeneratedManyUsers(t *testing.T) {
	g, err := GenPolicies(t.TempDir(), "manyusers", 101, 10)
	if err != nil {
		t.Fatal(err)
	}
	es, err := LoadEngines(g, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names(es) {
		t.Run(name, func(t *testing.T) {
			for u := 1; u <= 101; u++ {
				for k := 1; k <= 10; k++ {
					r := (u-1)%10 + 1
					for _, allow := range []bool{true, false} {
						rr := r
						if !allow {
							rr = r%10 + 1
						}
						req := Req{User: fmt.Sprintf("user%d", u), Action: "read", Resource: fmt.Sprintf("/books/book%d-%d", rr, k)}
						ok, e := es[name].Allowed(req)
						if e != nil || ok != allow {
							t.Fatalf("%+v got %v %v want %v", req, ok, e, allow)
						}
					}
				}
			}
		})
	}
}

// Every engine and variant must give the same decision on each generated
// shape: each role's own grant, its neighbour's (denied), and for ABAC the
// same requests without the attributes (denied).
func TestGeneratedShapesAgree(t *testing.T) {
	for _, kind := range []string{"plain", "abac", "wildcard"} {
		for _, n := range []int{10, 100} {
			g, err := GenPolicies(t.TempDir(), kind, n, 3)
			if err != nil {
				t.Fatal(err)
			}
			es, err := LoadEngines(g, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range names(es) {
				for i := 1; i <= n; i++ {
					for _, own := range []bool{true, false} {
						book := i
						if !own {
							book = i%n + 1
						}
						res := fmt.Sprintf("/books/book%d", book)
						if kind == "wildcard" {
							res = fmt.Sprintf("/books/type%d/chapter1", book)
						}
						req := Req{User: fmt.Sprintf("user%d-%d", i, i%3+1), Action: "read", Resource: res}
						want := own
						if kind == "abac" {
							req.Attrs = map[string]any{"att1": "val1", "att2": "val2"}
							if i%2 == 0 {
								req.Attrs = map[string]any{"att1": "val1", "att2": "other"}
								want = false
							}
						}
						if ok, err := es[name].Allowed(req); err != nil || ok != want {
							t.Fatalf("%s/%d/%s: %+v: got %v, %v; want %v", kind, n, name, req, ok, err, want)
						}
					}
				}
			}
		}
	}
}
