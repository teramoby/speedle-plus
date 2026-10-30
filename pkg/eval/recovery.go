package eval

import (
	"fmt"
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
		// Do not expose panic payloads (which can contain tokens or attributes).
		log.WithFields(log.Fields{
			"operation":  operation,
			"panic_type": fmt.Sprintf("%T", value),
			"stack":      string(debug.Stack()),
		}).Error("authorization engine recovered a panic")
		*err = &recoveredEvaluationError{}
	}
}
