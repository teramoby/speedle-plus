package eval

import (
	"fmt"
	"os"
	"runtime/debug"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

type recoveredEvaluationError struct{}

func (*recoveredEvaluationError) Error() string {
	return "SPDL-2001 authorization engine recovered an internal panic"
}

func (*recoveredEvaluationError) Code() errors.ErrorCode { return errors.EvalEngineError }

// recoverEvaluation must be deferred directly at an engine entry point. A
// condition/function panic must abort the whole decision, rather than turn a
// failed deny condition into a nonmatch and leave an unrelated grant active.
func recoverEvaluation(operation string, err *error) {
	if value := recover(); value != nil {
		*err = &recoveredEvaluationError{}
		logEvaluationPanic(operation, value)
	}
}

func logEvaluationPanic(operation string, value interface{}) {
	// Error reporting must not throw a second panic out of the recovery defer.
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "authorization engine recovered a panic; error logger failed")
		}
	}()
	// Do not expose panic payloads (which can contain tokens or attributes).
	log.WithFields(log.Fields{
		"operation":  operation,
		"panic_type": fmt.Sprintf("%T", value),
		"stack":      string(debug.Stack()),
	}).Error("authorization engine recovered a panic")
}
