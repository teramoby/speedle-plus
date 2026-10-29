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
