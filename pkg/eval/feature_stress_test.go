//go:build feature_stress

package eval

import (
	"fmt"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"os"
	"strconv"
	"testing"
)

func TestFeatureLargePolicyRoleSets(t *testing.T) {
	maximum := 10000
	if raw := os.Getenv("SPEEDLE_STRESS_POLICIES"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 10000 || n > 100000 {
			t.Fatalf("invalid stress size %q", raw)
		}
		maximum = n
	}
	for _, size := range []int{1000, maximum} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			s := &pms.Service{Name: "app"}
			for i := 0; i < size; i++ {
				id := fmt.Sprint(i)
				p := featurePolicy("p"+id, "amount < 100 && risk < 50")
				p.Principals = [][]string{{"role:r" + id}}
				p.Permissions[0].Resource = "book" + id
				s.Policies = append(s.Policies, p)
				s.RolePolicies = append(s.RolePolicies, &pms.RolePolicy{ID: "r" + id, Effect: pms.Grant, Principals: []string{"user:u" + id}, Roles: []string{"r" + id}})
			}
			e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{s}})
			for _, i := range []int{0, size / 2, size - 1} {
				for _, permitted := range []bool{true, false} {
					ctx := featureContext()
					ctx.Resource = "book" + fmt.Sprint(i)
					ctx.Subject = &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "u" + fmt.Sprint(i)}}}
					amount := 1.0
					if !permitted {
						amount = 1000
					}
					ctx.Attributes = map[string]interface{}{"amount": amount, "risk": 1.0}
					for repeat := 0; repeat < 2; repeat++ {
						got, _, err := e.IsAllowed(ctx)
						if got != permitted || err != nil {
							t.Fatalf("size=%d user=%d permitted=%v got=%v err=%v", size, i, permitted, got, err)
						}
					}
				}
			}
		})
	}
}
