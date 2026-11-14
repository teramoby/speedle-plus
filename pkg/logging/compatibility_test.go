package logging_test

import (
	"testing"

	"github.com/teramoby/speedle-plus/pkg/logging"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Go consumers explicitly migrate to the v2 import for the slog redesign.
func TestV2RotationConfigCompatibility(t *testing.T) {
	rotation := &lumberjack.Logger{MaxSize: 10, MaxBackups: 3}
	config := logging.LogConfig{RotationConfig: rotation}
	if config.RotationConfig != rotation {
		t.Fatal("rotation configuration did not retain the supplied logger")
	}
}
