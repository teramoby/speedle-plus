package adsgrpc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/logging"
	"gopkg.in/natefinch/lumberjack.v2"
)

func TestIsAllowedLegacyAuditFormat(t *testing.T) {
	service := newService(t)
	previous := logging.AuditLog()
	t.Cleanup(func() { logging.SetAuditLog(previous) })
	for _, format := range []string{"text", "json"} {
		for _, test := range []struct {
			name, user, text string
			allowed          bool
			json             map[string]any
		}{
			{"allow", "alice", "{true 0 }", true, map[string]any{"allowed": true}},
			{"deny", "bob", "{false 3 }", false, map[string]any{"reason": float64(3)}},
		} {
			t.Run(format+"/"+test.name, func(t *testing.T) {
				rotation := &lumberjack.Logger{Filename: filepath.Join(t.TempDir(), "audit.log")}
				t.Cleanup(func() { _ = rotation.Close() })
				logger, err := logging.NewLogger(&logging.LogConfig{Formatter: format, RotationConfig: rotation})
				if err != nil {
					t.Fatal(err)
				}
				logging.SetAuditLog(logger)
				response, err := service.IsAllowed(context.Background(), request(test.user, "res1", "read", nil))
				if err != nil || response.Allowed != test.allowed {
					t.Fatalf("decision=%v err=%v", response, err)
				}
				rawOutput, err := os.ReadFile(rotation.Filename)
				if err != nil {
					t.Fatal(err)
				}
				output := string(rawOutput)
				if format == "text" {
					if !strings.Contains(output, `responseValue="`+test.text+`"`) || !strings.Contains(output, "[AUDIT][gRPC]IsAllowed") {
						t.Fatalf("legacy text audit changed: %s", output)
					}
				} else {
					var record map[string]any
					if err := json.Unmarshal([]byte(output), &record); err != nil {
						t.Fatal(err)
					}
					if record["msg"] != "[AUDIT][gRPC]IsAllowed" || record["response"] != "succeeded" || !reflect.DeepEqual(record["responseValue"], test.json) {
						t.Fatalf("legacy JSON audit changed: %#v", record)
					}
				}
			})
		}
	}
}
