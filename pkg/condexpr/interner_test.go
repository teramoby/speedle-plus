package condexpr

import (
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
	if len(interner.literals) != 3 {
		t.Fatalf("literal entries = %d, want 3", len(interner.literals))
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
	if len(interner.parameters) != 2 || len(interner.literals) != 4 {
		t.Fatalf("interned %d parameters and %d literals, want 2 and 4",
			len(interner.parameters), len(interner.literals))
	}
}
