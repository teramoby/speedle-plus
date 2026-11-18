package condexpr

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/eval/function"
)

// This file proves that the two compile-time memory changes do not change
// decisions. Every expression is compiled three ways:
//
//   - the pre-interning layout, where each leaf keeps its own closure;
//   - the production layout, with leaf closures interned within one compile;
//   - interning plus CompileOptions.DropTokensAfterPlan.
//
// The corpus files are generated and kept current by pkg/eval's
// TestConditionCorpusFilesAreCurrent: one file holds every expression of the
// condition-conformance matrix, the other every policy condition in the
// pkg/eval test fixtures.

const (
	conformanceCorpusPath = "../eval/testdata/condition-conformance-expressions.txt"
	policyCorpusPath      = "../eval/testdata/engine-policy-conditions.txt"
)

// corpusFunctions is the function set the corpus conditions call. The named
// stubs stand in for user-defined and server-backed functions that pkg/eval
// tests install; a deterministic body is enough to compare compile modes.
func corpusFunctions() map[string]ExpressionFunction {
	number := func(args ...interface{}) (interface{}, error) { return 1.0, nil }
	open := func(args ...interface{}) (interface{}, error) { return true, nil }
	return map[string]ExpressionFunction{
		"Sqrt": function.Sqrt, "Max": function.Max, "Min": function.Min,
		"Sum": function.Sum, "Avg": function.Avg, "IsSubSet": function.IsSubSet,
		"f":        func(args ...interface{}) (interface{}, error) { return "via delegator", nil },
		"isOpen":   open,
		"testsum":  number,
		"testsum1": number,
		"boom": func(args ...interface{}) (interface{}, error) {
			return nil, fmt.Errorf("boom function failed")
		},
	}
}

// corpusParameters covers the attributes and value shapes the corpus uses.
// The mismatched and empty sets drive the runtime error and missing-attribute
// paths, which must also be identical across compile modes.
func corpusParameters() []map[string]interface{} {
	full := map[string]interface{}{
		"x": 5.0, "y": 5.0, "other": false, "a": 5.0, "b": 2.0, "c": 2.0, "d": 3.0,
		"e": []interface{}{"s1", "s2"}, "s": "ab", "n": 5.0, "nums": []interface{}{5.0},
		"huge": 1.7976931348623157e308, "empty": []interface{}{}, "flag": true,
		"arr": []interface{}{"a", "b"}, "s1": []interface{}{"BJ", "SZ"},
		"s2": []interface{}{"BJ"}, "hyphen-name": 5.0, "grant": 5.0, "rt_str_attr": "abc",
		"request_user": "admin", "request_groups": []interface{}{"manager"},
		"request_entity": "user:admin", "request_resource": "/node1/x",
		"request_action": "get", "request_time": 1504516800.0, "request_year": 2017.0,
		"request_month": 11.0, "request_day": 13.0, "request_hour": 14.0,
		"request_weekday": "Monday",
	}
	mismatched := map[string]interface{}{
		"x": "5", "y": true, "other": 1.0, "a": "text", "b": true, "c": "2", "d": []interface{}{1.0},
		"e": 7.0, "s": 5.0, "n": "5", "nums": "5", "huge": "huge", "empty": 0.0,
		"flag": 1.0, "arr": "a", "s1": 1.0, "s2": false, "hyphen-name": "5", "grant": "5",
		"rt_str_attr": 5.0, "request_user": 1.0, "request_groups": "manager",
		"request_entity": false, "request_resource": 1.0, "request_action": nil,
		"request_time": "now", "request_year": "2017", "request_month": true,
		"request_day": 1.0, "request_hour": "14", "request_weekday": 1.0,
	}
	withNil := map[string]interface{}{"a": 2.0, "s": "ab", "flag": true, "arr": []interface{}{}, "nilValue": nil}
	return []map[string]interface{}{full, mismatched, withNil, {}}
}

func readCorpusFile(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read corpus: %v (regenerate it from pkg/eval)", err)
	}
	var expressions []string
	for _, line := range strings.Split(string(content), "\n") {
		if line != "" {
			expressions = append(expressions, line)
		}
	}
	if len(expressions) == 0 {
		t.Fatalf("%s is empty", path)
	}
	return expressions
}

// compileModes compiles expression without interning, with interning, and with
// interning plus token release. It fails the test when the modes disagree
// about the compile error, the source text or the kept token metadata.
func compileModes(t *testing.T, expression string, functions map[string]ExpressionFunction) *[3]*EvaluableExpression {
	t.Helper()
	noInterning, errNoInterning := newEvaluableExpressionWithFunctionsAndOptions(expression, functions, CompileOptions{}, false)
	interning, errInterning := newEvaluableExpressionWithFunctionsAndOptions(expression, functions, CompileOptions{}, true)
	dropped, errDropped := newEvaluableExpressionWithFunctionsAndOptions(expression, functions, CompileOptions{DropTokensAfterPlan: true}, true)
	errs := [3]error{errNoInterning, errInterning, errDropped}
	for i := 1; i < len(errs); i++ {
		if (errs[i] == nil) != (errs[0] == nil) {
			t.Fatalf("compile errors differ: %v / %v", errs[0], errs[i])
		}
		if errs[i] != nil && errs[i].Error() != errs[0].Error() {
			t.Fatalf("compile error text differs: %q / %q", errs[0], errs[i])
		}
	}
	if errNoInterning != nil {
		return nil
	}
	modes := [3]*EvaluableExpression{noInterning, interning, dropped}
	for i, mode := range modes {
		if mode.String() != expression {
			t.Fatalf("mode %d String = %q, want %q", i, mode.String(), expression)
		}
	}
	if len(interning.Tokens()) == 0 {
		t.Fatalf("interned compile kept no tokens")
	}
	if dropped.Tokens() != nil || dropped.Vars() != nil {
		t.Fatalf("token release kept metadata: Tokens=%v Vars=%v", dropped.Tokens(), dropped.Vars())
	}
	return &modes
}

func assertSameEvaluation(t *testing.T, modes *[3]*EvaluableExpression, parameters map[string]interface{}) {
	t.Helper()
	want, wantErr := modes[0].Evaluate(parameters)
	for i := 1; i < len(modes); i++ {
		got, gotErr := modes[i].Evaluate(parameters)
		if !sameResult(got, want) {
			t.Fatalf("mode %d = %v, mode 0 = %v (parameters %v)", i, got, want, parameters)
		}
		if (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("mode %d error = %v, mode 0 error = %v (parameters %v)", i, gotErr, wantErr, parameters)
		}
		if gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Fatalf("mode %d error text = %q, mode 0 = %q (parameters %v)", i, gotErr, wantErr, parameters)
		}
	}
}

func TestCompileModesDecideIdentically(t *testing.T) {
	expressions := append(readCorpusFile(t, conformanceCorpusPath), readCorpusFile(t, policyCorpusPath)...)
	seen := make(map[string]bool, len(expressions))
	functions := corpusFunctions()
	parameters := corpusParameters()
	for _, expression := range expressions {
		if seen[expression] {
			continue
		}
		seen[expression] = true
		t.Run(expression, func(t *testing.T) {
			modes := compileModes(t, expression, functions)
			if modes == nil {
				return
			}
			for _, set := range parameters {
				assertSameEvaluation(t, modes, set)
			}
		})
	}
	if len(seen) < 100 {
		t.Fatalf("corpus covered only %d expressions", len(seen))
	}
}

// TestCompileModesEdgeCases pins the interning edge cases explicitly: equal
// text with different types, NaN produced by constant folding, nil, the same
// function name called with different arguments, and regexp literals.
func TestCompileModesEdgeCases(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		parameters []map[string]interface{}
	}{
		{
			name:       "numeric string and float literals",
			expression: "a == 1 && b == '1' && c == 1.0 && '1' != 1",
			parameters: []map[string]interface{}{
				{"a": 1.0, "b": "1", "c": 1.0},
				{"a": "1", "b": 1.0, "c": "1"},
			},
		},
		{
			name:       "nan constant folding",
			expression: "(0/0) != (0/0) && !((0/0) == (0/0))",
			parameters: []map[string]interface{}{{}},
		},
		{
			name:       "nil coalesce and compare",
			expression: "(nilValue ?? 7) == 7 && nilValue == nilValue",
			parameters: []map[string]interface{}{
				{"nilValue": nil},
				{},
				{"nilValue": 3.0},
			},
		},
		{
			name:       "same function different arguments",
			expression: "Max(a, b) == 2 && Max(a, 1) == 2 && Max(a, b, 3) == 3 && Sqrt(a) == 2",
			parameters: []map[string]interface{}{
				{"a": 4.0, "b": 2.0},
				{"a": 2.0, "b": 2.0},
				{"a": 1.0, "b": 2.0},
			},
		},
		{
			name:       "regexp literals",
			expression: "s =~ '^a.*' && t =~ '^a.*' && t !~ 'z$' && s =~ '^a.*'",
			parameters: []map[string]interface{}{
				{"s": "abc", "t": "abc"},
				{"s": "abc", "t": "zabc"},
				{"s": 1.0, "t": "abc"},
			},
		},
	}
	functions := corpusFunctions()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			modes := compileModes(t, c.expression, functions)
			if modes == nil {
				t.Fatal("expression did not compile")
			}
			for _, parameters := range c.parameters {
				assertSameEvaluation(t, modes, parameters)
			}
		})
	}
}

// TestSharedExpressionEvaluatesConcurrently evaluates one compiled expression
// from several goroutines with different inputs. Run with -race, this proves
// the interned closures are read-only and a single compile can be shared.
func TestSharedExpressionEvaluatesConcurrently(t *testing.T) {
	const expression = "a >= 0 && a < 10 && a != 5 && s == 'x' && s != 'y' && flag && (a > 2 ? s == 'x' : flag)"
	shared, err := NewEvaluableExpression(expression)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 8
	const iterations = 64
	type request struct {
		attributes map[string]interface{}
		want       interface{}
		wantErr    bool
	}
	requests := make([]request, workers*iterations)
	for worker := 0; worker < workers; worker++ {
		for i := 0; i < iterations; i++ {
			attributes := map[string]interface{}{
				"a":    float64((worker + i) % 12),
				"s":    []string{"x", "y", "z"}[i%3],
				"flag": i%2 == 0,
			}
			// Compile a separate expression for the expected value so the
			// comparison does not depend on the shared compile a second time.
			reference, err := NewEvaluableExpression(expression)
			if err != nil {
				t.Fatal(err)
			}
			want, wantErr := reference.Evaluate(attributes)
			requests[worker*iterations+i] = request{attributes, want, wantErr != nil}
		}
	}
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				r := requests[worker*iterations+i]
				got, err := shared.Evaluate(r.attributes)
				if (err != nil) != r.wantErr || !sameResult(got, r.want) {
					t.Errorf("worker %d iteration %d: got %v, %v; want %v", worker, i, got, err, r.want)
				}
			}
		}(worker)
	}
	wg.Wait()
}

// TestConcurrentCompilesShareNoState compiles the same expression from several
// goroutines. Each compile builds its own interner, so no closure or cached
// state crosses compiles; -race checks that this holds.
func TestConcurrentCompilesShareNoState(t *testing.T) {
	const expression = "a >= 0 && a < 10 && s == 'x' && flag && a >= 0"
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 64; i++ {
				compiled, err := NewEvaluableExpression(expression)
				if err != nil {
					t.Errorf("worker %d: %v", worker, err)
					return
				}
				if got, err := compiled.Evaluate(map[string]interface{}{
					"a": float64(worker), "s": "x", "flag": true,
				}); err != nil || got != true {
					t.Errorf("worker %d: got %v, %v", worker, got, err)
				}
			}
		}(worker)
	}
	wg.Wait()
}
