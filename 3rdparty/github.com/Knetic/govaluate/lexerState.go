package govaluate

import (
	"errors"
	"fmt"
)

type lexerState struct {
	isEOF          bool
	isNullable     bool
	kind           TokenKind
	validNextKinds []TokenKind
}

// lexer states.
// Constant for all purposes except compiler.
var validLexerStates = []lexerState{

	lexerState{
		kind:       UNKNOWN,
		isEOF:      false,
		isNullable: true,
		validNextKinds: []TokenKind{

			PREFIX,
			NUMERIC,
			BOOLEAN,
			VARIABLE,
			PATTERN,
			FUNCTION,
			ACCESSOR,
			STRING,
			TIME,
			CLAUSE,
		},
	},

	lexerState{

		kind:       CLAUSE,
		isEOF:      false,
		isNullable: true,
		validNextKinds: []TokenKind{

			PREFIX,
			NUMERIC,
			BOOLEAN,
			VARIABLE,
			PATTERN,
			FUNCTION,
			ACCESSOR,
			STRING,
			TIME,
			CLAUSE,
			CLAUSE_CLOSE,
		},
	},

	lexerState{

		kind:       CLAUSE_CLOSE,
		isEOF:      true,
		isNullable: true,
		validNextKinds: []TokenKind{

			COMPARATOR,
			MODIFIER,
			NUMERIC,
			BOOLEAN,
			VARIABLE,
			STRING,
			PATTERN,
			TIME,
			CLAUSE,
			CLAUSE_CLOSE,
			LOGICALOP,
			TERNARY,
			SEPARATOR,
		},
	},

	lexerState{

		kind:       NUMERIC,
		isEOF:      true,
		isNullable: false,
		validNextKinds: []TokenKind{

			MODIFIER,
			COMPARATOR,
			LOGICALOP,
			CLAUSE_CLOSE,
			TERNARY,
			SEPARATOR,
		},
	},
	lexerState{

		kind:       BOOLEAN,
		isEOF:      true,
		isNullable: false,
		validNextKinds: []TokenKind{

			MODIFIER,
			COMPARATOR,
			LOGICALOP,
			CLAUSE_CLOSE,
			TERNARY,
			SEPARATOR,
		},
	},
	lexerState{

		kind:       STRING,
		isEOF:      true,
		isNullable: false,
		validNextKinds: []TokenKind{

			MODIFIER,
			COMPARATOR,
			LOGICALOP,
			CLAUSE_CLOSE,
			TERNARY,
			SEPARATOR,
		},
	},
	lexerState{

		kind:       TIME,
		isEOF:      true,
		isNullable: false,
		validNextKinds: []TokenKind{

			MODIFIER,
			COMPARATOR,
			LOGICALOP,
			CLAUSE_CLOSE,
			SEPARATOR,
		},
	},
	lexerState{

		kind:       PATTERN,
		isEOF:      true,
		isNullable: false,
		validNextKinds: []TokenKind{

			MODIFIER,
			COMPARATOR,
			LOGICALOP,
			CLAUSE_CLOSE,
			SEPARATOR,
		},
	},
	lexerState{

		kind:       VARIABLE,
		isEOF:      true,
		isNullable: false,
		validNextKinds: []TokenKind{

			MODIFIER,
			COMPARATOR,
			LOGICALOP,
			CLAUSE_CLOSE,
			TERNARY,
			SEPARATOR,
		},
	},
	lexerState{

		kind:       MODIFIER,
		isEOF:      false,
		isNullable: false,
		validNextKinds: []TokenKind{

			PREFIX,
			NUMERIC,
			VARIABLE,
			FUNCTION,
			ACCESSOR,
			STRING,
			BOOLEAN,
			CLAUSE,
			CLAUSE_CLOSE,
		},
	},
	lexerState{

		kind:       COMPARATOR,
		isEOF:      false,
		isNullable: false,
		validNextKinds: []TokenKind{

			PREFIX,
			NUMERIC,
			BOOLEAN,
			VARIABLE,
			FUNCTION,
			ACCESSOR,
			STRING,
			TIME,
			CLAUSE,
			CLAUSE_CLOSE,
			PATTERN,
		},
	},
	lexerState{

		kind:       LOGICALOP,
		isEOF:      false,
		isNullable: false,
		validNextKinds: []TokenKind{

			PREFIX,
			NUMERIC,
			BOOLEAN,
			VARIABLE,
			FUNCTION,
			ACCESSOR,
			STRING,
			TIME,
			CLAUSE,
			CLAUSE_CLOSE,
		},
	},
	lexerState{

		kind:       PREFIX,
		isEOF:      false,
		isNullable: false,
		validNextKinds: []TokenKind{

			NUMERIC,
			BOOLEAN,
			VARIABLE,
			FUNCTION,
			ACCESSOR,
			CLAUSE,
			CLAUSE_CLOSE,
		},
	},

	lexerState{

		kind:       TERNARY,
		isEOF:      false,
		isNullable: false,
		validNextKinds: []TokenKind{

			PREFIX,
			NUMERIC,
			BOOLEAN,
			STRING,
			TIME,
			VARIABLE,
			FUNCTION,
			ACCESSOR,
			CLAUSE,
			SEPARATOR,
		},
	},
	lexerState{

		kind:       FUNCTION,
		isEOF:      false,
		isNullable: false,
		validNextKinds: []TokenKind{
			CLAUSE,
		},
	},
	lexerState{

		kind:       ACCESSOR,
		isEOF:      true,
		isNullable: false,
		validNextKinds: []TokenKind{
			CLAUSE,
			MODIFIER,
			COMPARATOR,
			LOGICALOP,
			CLAUSE_CLOSE,
			TERNARY,
			SEPARATOR,
		},
	},
	lexerState{

		kind:       SEPARATOR,
		isEOF:      false,
		isNullable: true,
		validNextKinds: []TokenKind{

			PREFIX,
			NUMERIC,
			BOOLEAN,
			STRING,
			TIME,
			VARIABLE,
			FUNCTION,
			ACCESSOR,
			CLAUSE,
		},
	},
}

func (this lexerState) canTransitionTo(kind TokenKind) bool {

	for _, validKind := range this.validNextKinds {

		if validKind == kind {
			return true
		}
	}

	return false
}

// checkExpressionSyntax checks the token order. end is the position reported
// for an expression that ends too early.
func checkExpressionSyntax(tokens []ExpressionToken, end int) error {

	var state lexerState
	var lastToken ExpressionToken
	var err error

	state = validLexerStates[0]

	for _, token := range tokens {

		if !state.canTransitionTo(token.Kind) {

			// call out a specific error for tokens looking like they want to be functions.
			if lastToken.Kind == VARIABLE && token.Kind == CLAUSE {
				return &ExpressionError{Message: "Undefined function " + lastToken.Value.(string), Position: lastToken.Position}
			}

			firstStateName := fmt.Sprintf("%s [%v]", state.kind.String(), lastToken.Value)
			nextStateName := fmt.Sprintf("%s [%v]", token.Kind.String(), token.Value)

			return &ExpressionError{Message: "Cannot transition token types from " + firstStateName + " to " + nextStateName, Position: token.Position}
		}

		state, err = getLexerStateForToken(token.Kind)
		if err != nil {
			return withPosition(err, token.Position)
		}

		if !state.isNullable && token.Value == nil {

			errorMsg := fmt.Sprintf("Token kind '%v' cannot have a nil value", token.Kind.String())
			return &ExpressionError{Message: errorMsg, Position: token.Position}
		}

		lastToken = token
	}

	if !state.isEOF {
		return &ExpressionError{Message: "Unexpected end of expression", Position: end}
	}
	return nil
}

func getLexerStateForToken(kind TokenKind) (lexerState, error) {

	for _, possibleState := range validLexerStates {

		if possibleState.kind == kind {
			return possibleState, nil
		}
	}

	errorMsg := fmt.Sprintf("No lexer state found for token kind '%v'\n", kind.String())
	return validLexerStates[0], errors.New(errorMsg)
}
