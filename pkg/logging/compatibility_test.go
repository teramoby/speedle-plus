package logging_test

import (
	"testing"

	"github.com/natefinch/lumberjack"
	"github.com/teramoby/speedle-plus/pkg/logging"
)

// Existing consumers must be able to construct the exported configuration
// with the original concrete logger type during routine dependency upgrades.
func TestLegacyRotationConfigCompatibility(t *testing.T) {
	rotation := &lumberjack.Logger{MaxSize: 10, MaxBackups: 3}
	config := logging.LogConfig{RotationConfig: rotation}
	if config.RotationConfig != rotation {
		t.Fatal("rotation configuration did not retain the supplied logger")
	}
}
