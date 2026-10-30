package govaluate

import (
	"errors"
	"strings"
	"testing"
)

func TestExpressionComplexityBoundaries(t *testing.T) {
	for _, depth := range []int{MaxExpressionNesting, MaxExpressionNesting + 1} {
		expression := strings.Repeat("(", depth) + "true" + strings.Repeat(")", depth)
		_, err := NewEvaluableExpression(expression)
		if depth == MaxExpressionNesting && err != nil {
			t.Fatalf("nesting boundary: %v", err)
		}
		if depth > MaxExpressionNesting {
			var limit *ComplexityError
			if !errors.As(err, &limit) {
				t.Fatalf("missing complexity error: %v", err)
			}
		}
	}
	// Both constructors enforce the limit; tokenized input cannot bypass it.
	tokens := make([]ExpressionToken, MaxExpressionTokens+1)
	if _, err := NewEvaluableExpressionFromTokens(tokens); err == nil {
		t.Fatal("oversized token stream accepted")
	}
	if _, err := NewEvaluableExpression(strings.Repeat("true && ", MaxExpressionTokens/2) + "true"); err == nil {
		t.Fatal("oversized expression accepted")
	}
	text := strings.Repeat("(", MaxExpressionNesting+1)
	expression, err := NewEvaluableExpression("'" + text + "' == '" + text + "'")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := expression.Evaluate(nil); value != true || err != nil {
		t.Fatalf("quoted parentheses: %v %v", value, err)
	}
}
