package condexpr

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

const retainedExpressions = 200

// retainedBenchmarkCondition builds the 1 KiB, 111-comparison condition used
// for the memory measurements: the same attribute and two literals repeat in
// every comparison, as conditions generated from large policy sets do.
func retainedBenchmarkCondition() string {
	comparisons := make([]string, 0, 111)
	for i := 0; i < 110; i++ {
		comparisons = append(comparisons, fmt.Sprintf("a >= %d", i%2))
	}
	comparisons = append(comparisons, "s == 'x'")
	return strings.Join(comparisons, " && ")
}

// BenchmarkRetainedBytesPerExpression reports the live heap retained by one
// compiled expression after a garbage collection. The reported value includes
// the 8-byte pointer that holds each expression in the benchmark slice.
func BenchmarkRetainedBytesPerExpression(b *testing.B) {
	condition := retainedBenchmarkCondition()
	functions := map[string]ExpressionFunction{}
	for _, drop := range []bool{false, true} {
		b.Run(fmt.Sprintf("dropTokensAfterPlan=%t", drop), func(b *testing.B) {
			options := CompileOptions{DropTokensAfterPlan: drop}
			warm, err := NewEvaluableExpressionWithFunctionsAndOptions(condition, functions, options)
			if err != nil {
				b.Fatal(err)
			}
			runtime.GC()
			var baseline runtime.MemStats
			runtime.ReadMemStats(&baseline)
			for i := 0; i < b.N; i++ {
				expressions := make([]*EvaluableExpression, retainedExpressions)
				for j := range expressions {
					expressions[j], err = NewEvaluableExpressionWithFunctionsAndOptions(condition, functions, options)
					if err != nil {
						b.Fatal(err)
					}
				}
				runtime.GC()
				var after runtime.MemStats
				runtime.ReadMemStats(&after)
				b.ReportMetric(float64(after.HeapAlloc-baseline.HeapAlloc)/retainedExpressions, "retained-B/expr")
				runtime.KeepAlive(expressions)
				runtime.KeepAlive(warm)
			}
		})
	}
}
