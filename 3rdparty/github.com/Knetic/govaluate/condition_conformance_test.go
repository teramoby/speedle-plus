// Licensed under the Universal Permissive License (UPL) Version 1.0.
package govaluate

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// These are supplemental fork-level tests for parser errors and embedded Go
// values which cannot be represented in an is-allowed JSON request. The SPDL
// decision matrix and both authorization paths live in pkg/eval.
func TestConditionConformanceFork(t *testing.T) {
	params := map[string]interface{}{"x": 5.0, "s": "abc", "pattern": "^a", "badPattern": "[", "nilValue": nil, "yes": true, "no": false, "obj": conformanceObject{Number: 5}, "ptr": &conformanceObject{Number: 5}, "bad": 3}
	for _, c := range []struct {
		expression string
		want       interface{}
		bad        bool
	}{
		{"0xabcdef == 11259375", true, false}, {"0 == 0", true, false}, {"0.5 < 1", true, false},
		{"s =~ pattern", true, false}, {"s !~ pattern", false, false}, {"s !~ '^z'", true, false},
		{"s =~ badPattern", nil, true}, {"s !~ badPattern", nil, true}, {"s =~ 1", nil, true},
		{"(nilValue ?? x) == 5", true, false},
		{"(x ?? missing) == 5", true, false},
		{"(obj.Add(1)) == 5", nil, true},
		{"(obj.Echo(1,2)) == 5", nil, true}, {"(no ? 1 : x) == 5", true, false},
		{"(yes ? x : missing) == 5", true, false}, {"(x ? 1 : 2) == 1", nil, true},
		{"no && missing", false, false}, {"yes || missing", true, false}, {"yes && x", nil, true},
		{"x && yes", nil, true}, {"no || x", nil, true}, {"x || no", nil, true},
		{"s - x", nil, true}, {"x - s", nil, true}, {"!x", nil, true}, {"-s", nil, true}, {"x < s", nil, true},
		{"(1+2) * 3 == 9", true, false}, {"(1 + 'a') == '1a'", true, false},
		{"(true + false) == 1", nil, true}, {"('s' - 1) == 0", nil, true}, {"(1 - 's') == 0", nil, true},
		{"('s' < 1)", nil, true}, {"(1 + 2 + 3 + 4 + x) == 15", true, false},
		{"(nilValue ?? nilValue ?? x) == 5", true, false},
		{"obj.Number == 5", true, false}, {"ptr.Number == 5", true, false},
		// known difference, behaviour pinned: an unparenthesized accessor call
		// consumes the following comparison as its argument stage.
		{"obj.Echo(5) == 5", nil, true},
		{"(obj.Echo(5)) == 5", true, false}, {"(ptr.Pointer()) == 5", true, false},
		{"(obj.Add(2,3)) == 5", true, false}, {"obj.Echo('bad') == 5", nil, true},
		{"obj.Echo() == 5", nil, true}, {"obj.Echo(1,2) == 5", nil, true},
		{"obj.Empty()", nil, true}, {"(obj.Pair()) == 5", true, false}, {"obj.Fail()", nil, true},
		{"obj.Many()", nil, true}, {"obj.Missing", nil, true}, {"ptr.Missing", nil, true},
		{"bad.Number", nil, true}, {"absent.Number", nil, true},
	} {
		t.Run(c.expression, func(t *testing.T) {
			expression, err := NewEvaluableExpression(c.expression)
			if err != nil {
				t.Fatal(err)
			}
			got, err := expression.Evaluate(params)
			if c.bad {
				if err == nil {
					t.Fatalf("got %v, want error", got)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v %v, want %v", got, err, c.want)
			}
		})
	}
	for _, value := range []interface{}{uint8(5), uint16(5), uint32(5), uint64(5), int8(5), int16(5), int32(5), int64(5), int(5), float32(5)} {
		exp, err := NewEvaluableExpression("x == 5")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := exp.Evaluate(map[string]interface{}{"x": value}); got != true || err != nil {
			t.Fatalf("%T: %v %v", value, got, err)
		}
	}
}

type conformanceObject struct{ Number int }

func (o conformanceObject) Echo(n int) int        { return n }
func (o conformanceObject) Add(a, b int) int      { return a + b }
func (o *conformanceObject) Pointer() int         { return o.Number }
func (o conformanceObject) Empty()                {}
func (o conformanceObject) Pair() (int, error)    { return 5, nil }
func (o conformanceObject) Fail() (int, error)    { return 0, errors.New("deliberate") }
func (o conformanceObject) Many() (int, int, int) { return 1, 2, 3 }

func TestConditionConformanceParserErrors(t *testing.T) {
	for _, s := range []string{"0xg", "0xffffffffffffffffffff", "1.2.3", "[unclosed", "'unclosed", "a..b", "a.", "unknown(1)", "1 === 2", "1 +", "(1", "1)", "(1))", "1 2", "'a' =~ '['", "true ?", "1 <<", "@"} {
		t.Run(s, func(t *testing.T) {
			if _, err := NewEvaluableExpression(s); err == nil {
				t.Fatalf("accepted %q", s)
			}
		})
	}
	// Public token construction exercises validation used by the text parser, too.
	for _, tokens := range [][]ExpressionToken{
		{{Kind: CLAUSE, Value: '('}}, {{Kind: NUMERIC, Value: nil}}, {{Kind: NUMERIC, Value: 1.0}, {Kind: NUMERIC, Value: 2.0}},
		{{Kind: STRING, Value: "a"}, {Kind: COMPARATOR, Value: "=~"}, {Kind: STRING, Value: "["}},
		{{Kind: VARIABLE, Value: "unknown"}, {Kind: CLAUSE, Value: '('}, {Kind: CLAUSE_CLOSE, Value: ')'}},
		make([]ExpressionToken, MaxExpressionTokens+1),
	} {
		if _, err := NewEvaluableExpressionFromTokens(tokens); err == nil {
			t.Fatalf("accepted %v", tokens[:min(5, len(tokens))])
		}
	}
	exp, err := NewEvaluableExpressionFromTokens([]ExpressionToken{{Kind: NUMERIC, Value: 1.0}, {Kind: MODIFIER, Value: "+"}, {Kind: NUMERIC, Value: 2.0}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := exp.Evaluate(nil); got != 3.0 || err != nil {
		t.Fatalf("%v %v", got, err)
	}
	// Syntax permits no empty expression; a zero-value Eval has no stage.
	if got, err := (EvaluableExpression{}).Evaluate(nil); got != nil || err != nil {
		t.Fatalf("zero expression %v %v", got, err)
	}
	exp, err = NewEvaluableExpression("x + y")
	if err != nil {
		t.Fatal(err)
	}
	if exp.String() != "x + y" || !reflect.DeepEqual(exp.Vars(), []string{"x", "y"}) || len(exp.Tokens()) != 3 {
		t.Fatal("expression metadata")
	}
	if _, err := getLexerStateForToken(TokenKind(-1)); err == nil {
		t.Fatal("unknown state")
	}
	if withPosition(nil, 1) != nil {
		t.Fatal("nil error changed")
	}
	prior := &ExpressionError{Message: "pinned", Position: 2}
	if withPosition(prior, 4) != prior || prior.Error() != "pinned" {
		t.Fatal("position changed")
	}
	if got := (&ComplexityError{Limit: "test", Maximum: 1}).Error(); got != "expression exceeds test limit 1" {
		t.Fatal(got)
	}
	// Plain regex attributes and precompiled literal regexes take different paths.
	if _, err := regexStage("a", regexp.MustCompile("a"), nil); err != nil {
		t.Fatal(err)
	}
}

func TestConditionConformanceForkHelpers(t *testing.T) {
	// Operator diagnostics should print meaningful symbols for all error paths.
	for symbol := VALUE; symbol <= SEPARATE; symbol++ {
		_ = symbol.String()
		_ = findOperatorPrecedenceForSymbol(symbol)
	}
	for kind := UNKNOWN; kind <= TERNARY; kind++ {
		if kind.String() == "" {
			t.Fatalf("blank kind %d", kind)
		}
	}
	if !PLUS.IsModifierType([]OperatorSymbol{MINUS, PLUS}) || PLUS.IsModifierType([]OperatorSymbol{MINUS}) {
		t.Fatal("modifier membership")
	}
	if TokenKind(-1).String() != "UNKNOWN" || OperatorSymbol(-1).String() != "" {
		t.Fatal("unknown diagnostics")
	}
	_ = findOperatorPrecedenceForSymbol(OperatorSymbol(-1))
	// Constants and single-element arrays have different syntax; typed slices
	// supplied by embedded callers must become []interface{} for membership.
	for _, array := range []interface{}{[]string{"a"}, []interface{}{"a"}} {
		exp, err := NewEvaluableExpression("'a' in xs")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := exp.Evaluate(map[string]interface{}{"xs": array}); got != true || err != nil {
			t.Fatalf("%v %v", got, err)
		}
	}
	for _, c := range []struct {
		value interface{}
		want  bool
	}{{nil, false}, {[1]int{1}, false}, {[]int{1}, false}, {1, true}} {
		if got := isSingle(c.value); got != c.want {
			t.Errorf("isSingle(%T)=%v", c.value, got)
		}
	}
	// The JSON path rejects these scalar/array shapes before evaluation.
	for _, args := range [][]interface{}{nil, {1.0}, {1.0, 2.0}} {
		fn := makeFunctionStage(func(values ...interface{}) (interface{}, error) { return float64(len(values)), nil })
		var right interface{}
		if len(args) == 1 {
			right = args[0]
		} else if len(args) > 1 {
			right = args
		}
		if got, err := fn(nil, right, nil); got != float64(len(args)) || err != nil {
			t.Fatal(got, err)
		}
	}
	// A real method that panics with a string is converted to an accessor error.
	exp, err := NewEvaluableExpression("obj.Panic()")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exp.Evaluate(map[string]interface{}{"obj": conformanceObject{}}); err == nil || !strings.Contains(err.Error(), "deliberate") {
		t.Fatal(err)
	}
	if got, err := typeConvertParam(reflect.ValueOf("bad"), reflect.TypeOf(1)); err == nil || got.Kind() != reflect.String {
		t.Fatal(got, err)
	}
	if got, err := typeConvertParam(reflect.ValueOf(1.0), reflect.TypeOf(1)); err != nil || got.Int() != 1 {
		t.Fatal(got, err)
	}
}
func (o conformanceObject) Panic() int { panic("deliberate") }
