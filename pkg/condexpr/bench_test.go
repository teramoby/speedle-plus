package condexpr

import (
	"fmt"
	"strings"
	"testing"
)

// benchmarkConditions returns the small, medium and large condition fixtures.
// They repeat the same attributes and literals, as conditions generated from
// policy sets do, so the compile benchmarks also exercise leaf interning. The
// large fixture is the 1 KiB, 111-comparison condition of the retained-bytes
// benchmark.
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

// benchmarkParameters satisfies every fixture comparison, so the evaluation
// benchmarks measure the matching path without logging errors.
func benchmarkParameters() map[string]interface{} {
	return map[string]interface{}{"a": 1.0, "b": 0.0, "s": "x"}
}

func BenchmarkCompileCondition(b *testing.B) {
	functions := map[string]ExpressionFunction{}
	for _, fixture := range benchmarkConditions() {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := NewEvaluableExpressionWithFunctions(fixture.condition, functions); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkEvaluateCondition(b *testing.B) {
	parameters := benchmarkParameters()
	for _, fixture := range benchmarkConditions() {
		expression, err := NewEvaluableExpressionWithFunctions(fixture.condition, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, err := expression.Evaluate(parameters)
				if err != nil || result != true {
					b.Fatalf("Evaluate = %v, %v", result, err)
				}
			}
		})
	}
}
