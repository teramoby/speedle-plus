package condexpr

import (
	"math"
	"reflect"
	"testing"
)

func TestStageInternerReusesLeafOperators(t *testing.T) {
	interner := newStageInterner()
	parameters := MapParameters(map[string]interface{}{"a": 7.0})

	for i := 0; i < 3; i++ {
		operator := interner.parameter("a")
		if got, err := operator(nil, nil, parameters); got != 7.0 || err != nil {
			t.Fatalf("parameter operator = %v, %v", got, err)
		}
	}
	if len(interner.parameters) != 1 {
		t.Fatalf("parameter entries = %d, want 1", len(interner.parameters))
	}

	for _, literal := range []interface{}{1.0, 1.0, "x", "x", true, true} {
		operator := interner.literal(literal)
		if got, err := operator(nil, nil, nil); !reflect.DeepEqual(got, literal) || err != nil {
			t.Fatalf("literal %v returned %v, %v", literal, got, err)
		}
	}
	if len(interner.numbers) != 1 || len(interner.literals) != 2 {
		t.Fatalf("number and literal entries = %d and %d, want 1 and 2",
			len(interner.numbers), len(interner.literals))
	}
}

func TestStageInternerKeepsSignedZero(t *testing.T) {
	interner := newStageInterner()
	negativeZero := math.Copysign(0, -1)
	positive := interner.literal(0.0)
	negative := interner.literal(negativeZero)
	positiveValue, err := positive(nil, nil, nil)
	if err != nil || math.Signbit(positiveValue.(float64)) {
		t.Fatalf("positive zero returned %v, %v", positiveValue, err)
	}
	negativeValue, err := negative(nil, nil, nil)
	if err != nil || !math.Signbit(negativeValue.(float64)) {
		t.Fatalf("negative zero returned %v, %v", negativeValue, err)
	}
	if len(interner.numbers) != 2 {
		t.Fatalf("number entries = %d, want 2", len(interner.numbers))
	}
	if repeated := interner.literal(0.0); repeated == nil || len(interner.numbers) != 2 {
		t.Fatal("repeated positive zero was not reused")
	}
}

func TestSignedZeroElision(t *testing.T) {
	// Interning must not replace the negative zero produced by elision with
	// the earlier positive-zero closure.
	for _, c := range []struct {
		expression string
		want       interface{}
	}{
		{"1 / (0 * (0 - 1)) < 0", true},
		{"1 / (0 - 0) > 0", true},
		{"1 / (0 - 0) > 0 && 1 / (0 * (0 - 1)) < 0", true},
		{"0 == 0 && 1 / -0 < 0", true},
	} {
		expression, err := NewEvaluableExpression(c.expression)
		if err != nil {
			t.Fatalf("%s: %v", c.expression, err)
		}
		if got, err := expression.Evaluate(nil); err != nil || got != c.want {
			t.Fatalf("%s = %v, %v, want %v", c.expression, got, err, c.want)
		}
	}
}

func TestStageInternerDoesNotKeyUncomparableLiterals(t *testing.T) {
	interner := newStageInterner()
	value := []interface{}{"x"}
	operator := interner.literal(value)
	if got, err := operator(nil, nil, nil); !reflect.DeepEqual(got, value) || err != nil {
		t.Fatalf("uncomparable literal returned %v, %v", got, err)
	}
	if got, err := interner.literal(nil)(nil, nil, nil); got != nil || err != nil {
		t.Fatalf("nil literal returned %v, %v", got, err)
	}
	if len(interner.literals) != 0 {
		t.Fatalf("uncomparable literals were keyed: %d entries", len(interner.literals))
	}
}

func TestPlanStagesInternsRepeatedLeaves(t *testing.T) {
	tokens, err := parseTokens("a >= 0 && a < 1 && a != 2 && s =~ 'x'", nil)
	if err != nil {
		t.Fatal(err)
	}
	interner := newStageInterner()
	if _, err := planStages(tokens, interner); err != nil {
		t.Fatal(err)
	}
	if len(interner.parameters) != 2 || len(interner.numbers) != 3 || len(interner.literals) != 1 {
		t.Fatalf("interned %d parameters, %d numbers and %d literals, want 2, 3 and 1",
			len(interner.parameters), len(interner.numbers), len(interner.literals))
	}
}
