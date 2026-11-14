package adsgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/logging"
)

func TestIsAllowedLegacyAuditFormat(t *testing.T) {
	service := newService(t)
	logger := logging.AuditLog()
	previousOut, previousFormatter, previousLevel := logger.Out, logger.Formatter, logger.GetLevel()
	t.Cleanup(func() {
		logger.SetOutput(previousOut)
		logger.SetFormatter(previousFormatter)
		logger.SetLevel(previousLevel)
	})
	logger.SetLevel(log.InfoLevel)
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
				var output bytes.Buffer
				logger.SetOutput(&output)
				if format == "text" {
					logger.SetFormatter(&log.TextFormatter{DisableTimestamp: true, DisableColors: true})
				} else {
					logger.SetFormatter(&log.JSONFormatter{})
				}
				response, err := service.IsAllowed(context.Background(), request(test.user, "res1", "read", nil))
				if err != nil || response.Allowed != test.allowed {
					t.Fatalf("decision=%v err=%v", response, err)
				}
				if format == "text" {
					if !strings.Contains(output.String(), `responseValue="`+test.text+`"`) || !strings.Contains(output.String(), "[AUDIT][gRPC]IsAllowed") {
						t.Fatalf("legacy text audit changed: %s", output.String())
					}
				} else {
					var record map[string]any
					if err := json.Unmarshal(output.Bytes(), &record); err != nil {
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
