// Licensed under the Universal Permissive License (UPL) Version 1.0.

package eval

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	condexpr "github.com/teramoby/speedle-plus/pkg/condexpr"
)

// This file benchmarks the engine's condition path: compiling a condition,
// evaluating a compiled condition, and an end-to-end IsAllowed request over a
// policy set whose policies all carry conditions.

func benchmarkConditions() []struct {
	name      string
	condition string
} {
	return []struct {
		name      string
		condition string
	}{
		{"small", benchmarkCondition(2)},
		{"medium", benchmarkCondition(20)},
		{"large", benchmarkCondition(111)},
	}
}

func benchmarkCondition(comparisons int) string {
	parts := make([]string, 0, comparisons)
	for i := 0; i < comparisons; i++ {
		switch i % 3 {
		case 0:
			parts = append(parts, fmt.Sprintf("a >= %d", i%2))
		case 1:
			parts = append(parts, fmt.Sprintf("b < %d", i+2))
		default:
			parts = append(parts, "s == 'x'")
		}
	}
	return strings.Join(parts, " && ")
}

func benchmarkAttributes() map[string]interface{} {
	return map[string]interface{}{"a": 1.0, "b": 0.0, "s": "x"}
}

func BenchmarkConditionCompile(b *testing.B) {
	for _, fixture := range benchmarkConditions() {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := condexpr.NewEvaluableExpressionWithFunctions(fixture.condition, builtinFunctions); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkConditionEvaluate(b *testing.B) {
	attributes := benchmarkAttributes()
	for _, fixture := range benchmarkConditions() {
		condition, err := condexpr.NewEvaluableExpressionWithFunctions(fixture.condition, builtinFunctions)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				allowed, err := evaluateCondition(condition, attributes)
				if err != nil || !allowed {
					b.Fatalf("evaluateCondition = %v, %v", allowed, err)
				}
			}
		})
	}
}

// benchmarkPolicyEvaluator loads a service whose policies all grant the same
// request and all carry a condition, so IsAllowed evaluates every condition of
// the set on each call.
func benchmarkPolicyEvaluator(b *testing.B, policies int) InternalEvaluator {
	b.Helper()
	condition, err := json.Marshal(benchmarkCondition(5))
	if err != nil {
		b.Fatal(err)
	}
	var store strings.Builder
	store.WriteString(`{"services":[{"name":"bench","policies":[`)
	for i := 0; i < policies; i++ {
		if i > 0 {
			store.WriteString(",")
		}
		fmt.Fprintf(&store, `{"id":"p%d","effect":"grant","permissions":[{"resource":"/bench","actions":["get"]}],"condition":%s}`, i, condition)
	}
	store.WriteString(`]}]}`)
	var policyStore pms.PolicyStore
	if err := json.Unmarshal([]byte(store.String()), &policyStore); err != nil {
		b.Fatal(err)
	}
	if err := testPS.WritePolicyStore(&policyStore); err != nil {
		b.Fatal(err)
	}
	evaluator, err := NewWithStore(&cfg.Config{}, testPS)
	if err != nil {
		b.Fatal(err)
	}
	return evaluator
}

func BenchmarkIsAllowedManyConditions(b *testing.B) {
	const policies = 100
	evaluator := benchmarkPolicyEvaluator(b, policies)
	request := adsapi.RequestContext{
		Subject: &adsapi.Subject{Principals: []*adsapi.Principal{
			{Type: adsapi.PRINCIPAL_TYPE_USER, Name: "bench"},
		}},
		ServiceName: "bench",
		Resource:    "/bench",
		Action:      "get",
		Attributes:  benchmarkAttributes(),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		allowed, _, err := evaluator.IsAllowed(request)
		if err != nil || !allowed {
			b.Fatalf("IsAllowed = %v, %v", allowed, err)
		}
	}
}
