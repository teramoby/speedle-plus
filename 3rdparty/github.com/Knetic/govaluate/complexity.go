package govaluate

import "fmt"

// Bound the recursive planner and evaluator before either can exhaust the Go
// stack. Quoted text and escaped variable names are single tokens, so brackets
// and parentheses within those values do not contribute to nesting.
const (
	MaxExpressionTokens  = 32768
	MaxExpressionNesting = 128
)

// ComplexityError reports an expression rejected before recursive planning.
type ComplexityError struct {
	Limit   string
	Maximum int
}

func (e *ComplexityError) Error() string {
	return fmt.Sprintf("expression exceeds %s limit %d", e.Limit, e.Maximum)
}

func checkComplexity(tokens []ExpressionToken) error {
	if len(tokens) > MaxExpressionTokens {
		return &ComplexityError{Limit: "token count", Maximum: MaxExpressionTokens}
	}
	depth := 0
	for _, token := range tokens {
		switch token.Kind {
		case CLAUSE:
			depth++
			if depth > MaxExpressionNesting {
				return &ComplexityError{Limit: "nesting depth", Maximum: MaxExpressionNesting}
			}
		case CLAUSE_CLOSE:
			depth--
		}
	}
	return nil
}
