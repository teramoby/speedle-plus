package govaluate

/*
	Represents a single parsed token.
*/
type ExpressionToken struct {
	Kind  TokenKind
	Value interface{}
	// Position is where the token starts in the expression, in characters.
	// Added in this repository (not in upstream govaluate); set by the parser.
	Position int
}

// ExpressionError is a syntax error with the position, in characters from the
// start of the expression, where it was found. Position is -1 when unknown.
// Added in this repository (not in upstream govaluate).
type ExpressionError struct {
	Message  string
	Position int
}

func (e *ExpressionError) Error() string {
	return e.Message
}

// withPosition returns err as an *ExpressionError at position, unless it
// already is one.
func withPosition(err error, position int) error {
	if _, ok := err.(*ExpressionError); ok || err == nil {
		return err
	}
	return &ExpressionError{Message: err.Error(), Position: position}
}
