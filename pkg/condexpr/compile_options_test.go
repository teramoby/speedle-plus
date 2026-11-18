package condexpr

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

type differentialObject struct {
	Number int
}

func (o differentialObject) Add(a, b int) int { return a + b }

func differentialFunctions() map[string]ExpressionFunction {
	number := func(value interface{}) (float64, error) {
		number, ok := value.(float64)
		if !ok {
			return 0, fmt.Errorf("%T is not a number", value)
		}
		return number, nil
	}
	return map[string]ExpressionFunction{
		"Sqrt": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("Sqrt expects one argument")
			}
			value, err := number(args[0])
			if err != nil {
				return nil, err
			}
			return math.Sqrt(value), nil
		},
		"Max": func(args ...interface{}) (interface{}, error) {
			maximum := math.Inf(-1)
			for _, arg := range args {
				value, err := number(arg)
				if err != nil {
					return nil, err
				}
				maximum = math.Max(maximum, value)
			}
			return maximum, nil
		},
	}
}

func TestDropTokensAfterPlanDifferential(t *testing.T) {
	expressions := []string{
		// Arithmetic and precedence.
		"a + 2 * 3 - 4 / 2", "(a + b) * 2 == 6", "a % 2 == 0", "2 ** 3 == 8", "2 ** 3 ** 2 == 512",
		"-a == -2", "-(a + 1) == -3", "1 + 2 == 3", "0x10 == 16",
		"1 / (0 * (0 - 1)) < 0",
		// Comparisons over numbers and strings.
		"a > 1", "a >= 2", "a < 3", "a <= 2", "a == 2", "a != 3",
		"s + 'x' == 'abx'", "s > 'a'", "s >= 'ab'", "s < 'b'", "s <= 'ab'",
		// Regular expressions, constants and variables, valid and invalid.
		"s =~ '^a'", "s !~ 'z$'", "s =~ pattern", "s !~ pattern", "s =~ badPattern", "s =~ 1",
		// Logical operators and short-circuit paths.
		"a > 1 && flag", "a > 5 || flag", "!flag", "flag && missing", "flag || missing",
		// Ternary and null coalescing.
		"a > 1 ? 'big' : 'small'", "a > 1 ? (flag ? 1 : 2) : 3", "(nilValue ?? a) == 2",
		"missing ?? a", "nilValue ?? a ?? 0",
		// Membership and separators.
		"a in (1, 2, 3)", "a in arr", "'a' in arr", "'x' in arr",
		// Bitwise operators and prefixes.
		"a & 3", "a | 4", "a ^ 1", "a << 2", "a >> 1", "~a", "-(a * 2)",
		// Functions with valid and invalid arguments.
		"Sqrt(a)", "Max(a, b)", "Max(a, b, 3)", "Sqrt('x')", "Sqrt(missing)", "Max()",
		// Accessors and their error path.
		"obj.Number == 2", "(obj.Add(a, b)) == 3", "obj.Missing == 1",
		// Missing attributes, type errors, literals and nil.
		"missing == 1", "a > missing", "missing", "a && flag", "s - a", "a < s",
		"flag + 1", "!a", "true == flag", "nilValue == nilValue",
	}
	parameterSets := []map[string]interface{}{
		{"a": 2.0, "b": 1.0, "s": "ab", "flag": true, "pattern": "^a", "badPattern": "[", "arr": []interface{}{"a", "b"}, "obj": differentialObject{Number: 2}, "nilValue": nil},
		{"a": -1.0, "b": 0.0, "s": "", "flag": false, "pattern": "^z", "badPattern": "[", "arr": []interface{}{1.0, 2.0}, "obj": differentialObject{Number: 2}},
		{"a": 3.0, "b": 2.0, "s": "b", "flag": true},
		{"a": "text", "b": true, "s": 5.0, "flag": 1.0},
		{},
	}
	for _, expression := range expressions {
		t.Run(expression, func(t *testing.T) {
			kept, err := NewEvaluableExpressionWithFunctionsAndOptions(expression, differentialFunctions(), CompileOptions{})
			if err != nil {
				t.Fatalf("compile with tokens: %v", err)
			}
			dropped, err := NewEvaluableExpressionWithFunctionsAndOptions(expression, differentialFunctions(), CompileOptions{DropTokensAfterPlan: true})
			if err != nil {
				t.Fatalf("compile without tokens: %v", err)
			}
			if len(kept.Tokens()) == 0 || len(kept.Vars()) == 0 && containsVariable(expression) {
				t.Fatalf("default compile kept no token metadata")
			}
			if dropped.Tokens() != nil || dropped.Vars() != nil {
				t.Fatalf("dropped tokens: Tokens=%v Vars=%v", dropped.Tokens(), dropped.Vars())
			}
			if kept.String() != expression || dropped.String() != expression {
				t.Fatalf("String = %q / %q", kept.String(), dropped.String())
			}
			for _, parameters := range parameterSets {
				want, wantErr := kept.Evaluate(parameters)
				got, gotErr := dropped.Evaluate(parameters)
				if !sameResult(got, want) {
					t.Fatalf("parameters %v: got %v, want %v", parameters, got, want)
				}
				if (gotErr == nil) != (wantErr == nil) {
					t.Fatalf("parameters %v: got error %v, want error %v", parameters, gotErr, wantErr)
				}
				if gotErr != nil && gotErr.Error() != wantErr.Error() {
					t.Fatalf("parameters %v: got error %q, want %q", parameters, gotErr, wantErr)
				}
			}
		})
	}
}

// sameResult compares two evaluation results. reflect.DeepEqual reports NaN as
// different from itself, but both compiles must produce the same NaN.
func sameResult(got, want interface{}) bool {
	gotNumber, gotIsNumber := got.(float64)
	wantNumber, wantIsNumber := want.(float64)
	if gotIsNumber && wantIsNumber && math.IsNaN(gotNumber) && math.IsNaN(wantNumber) {
		return true
	}
	return reflect.DeepEqual(got, want)
}

func containsVariable(expression string) bool {
	tokens, err := parseTokens(expression, differentialFunctions())
	if err != nil {
		return false
	}
	for _, token := range tokens {
		if token.Kind == VARIABLE {
			return true
		}
	}
	return false
}

func TestDropTokensAfterPlanKeepsEvaluationAndMetadataDefault(t *testing.T) {
	expression, err := NewEvaluableExpression("x + 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(expression.Tokens()) != 3 {
		t.Fatalf("default Tokens length = %d", len(expression.Tokens()))
	}
	if got := expression.Vars(); !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("default Vars = %v", got)
	}
	if got, err := expression.Evaluate(map[string]interface{}{"x": 1.0}); got != 2.0 || err != nil {
		t.Fatalf("default Evaluate = %v, %v", got, err)
	}

	dropped, err := NewEvaluableExpressionWithFunctionsAndOptions("x + 1", nil, CompileOptions{DropTokensAfterPlan: true})
	if err != nil {
		t.Fatal(err)
	}
	if dropped.Tokens() != nil || dropped.Vars() != nil {
		t.Fatalf("dropped metadata: Tokens=%v Vars=%v", dropped.Tokens(), dropped.Vars())
	}
	if got, err := dropped.Evaluate(map[string]interface{}{"x": 1.0}); got != 2.0 || err != nil {
		t.Fatalf("dropped Evaluate = %v, %v", got, err)
	}
	if got, err := dropped.ToSQLQuery(); got != "" || err != nil {
		t.Fatalf("dropped ToSQLQuery = %q, %v", got, err)
	}
}
