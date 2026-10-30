// Package featuretest supplies identical authorization cases to the embedded,
// REST and gRPC suites, independent of the selected policy-store backend.
package featuretest

import "github.com/teramoby/speedle-plus/api/pms"

type ConditionCase struct {
	Name, Expression string
	Want             bool
}

var Builtins = []ConditionCase{
	{"sqrt-true", "Sqrt(16) == 4", true}, {"sqrt-false", "Sqrt(16) > 4", false},
	{"sqrt-empty", "Sqrt() > 0", false}, {"sqrt-arity", "Sqrt(1,2) > 0", false}, {"sqrt-type", "Sqrt('x') > 0", false}, {"sqrt-domain", "Sqrt(-1) != Sqrt(-1)", false},
	{"max-true", "Max(-3,2,1) == 2", true}, {"max-false", "Max(1,2) < 2", false}, {"max-empty", "Max() > 0", false}, {"max-type", "Max(1,'x') > 0", false},
	{"min-true", "Min(-3,2,1) == -3", true}, {"min-false", "Min(1,2) > 1", false}, {"min-empty", "Min() > 0", false}, {"min-type", "Min('x',1) > 0", false},
	{"sum-true", "Sum(-3,2,1) == 0", true}, {"sum-false", "Sum(1,2) > 3", false}, {"sum-empty", "Sum() == 0", true}, {"sum-type", "Sum(1,'x') > 0", false},
	{"avg-true", "Avg(-3,2,1) == 0", true}, {"avg-false", "Avg(1,2) > 2", false}, {"avg-empty", "Avg() == 0", true}, {"avg-type", "Avg('x',1) > 0", false},
	{"subset-true", "IsSubSet(('a','b'),('a','b','c'))", true}, {"subset-false", "IsSubSet(('a','z'),('a','b','c'))", false}, {"subset-type", "IsSubSet(('a',true),('a','b'))", false}, {"subset-arity", "IsSubSet()", false},
	{"nested-true", "Avg(Min(3,4),Max(2,8),Sqrt(4)) == 13 / 3", true},
	{"short-circuit-or", "true || Sqrt('x') > 0", true}, {"short-circuit-and", "false && Sqrt('x') > 0", false},
}

func BuiltinStore() *pms.PolicyStore {
	s := &pms.Service{Name: "feature-functions"}
	for _, c := range Builtins {
		s.Policies = append(s.Policies, &pms.Policy{ID: c.Name, Name: c.Name, Effect: pms.Grant, Condition: c.Expression, Permissions: []*pms.Permission{{Resource: c.Name, Actions: []string{"read"}}}})
	}
	return &pms.PolicyStore{Services: []*pms.Service{s}}
}
