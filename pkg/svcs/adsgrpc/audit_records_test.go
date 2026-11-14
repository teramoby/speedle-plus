package adsgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/logging"
)

// These records were captured by the original f2591e6b generated client calling
// that revision's native ADS, under both real audit formatters. They include
// nested diagnosis messages, repeated permissions, map order, UTF-8 and controls.
func TestLegacyProtoAuditRecords(t *testing.T) {
	raw, err := os.ReadFile("testdata/legacy-audit-records.json")
	if err != nil {
		t.Fatal(err)
	}
	type record struct {
		Message  string `json:"msg"`
		Response string `json:"response"`
		Value    any    `json:"responseValue"`
	}
	var fixtures struct {
		Records map[string][]record `json:"records"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	// Discovery is optional on other stores; this is an audit format contract.
	t.Setenv("STORE_TYPE", "file")
	service := newService(t)
	logger := logging.AuditLog()
	previousOut, previousFormatter, previousLevel := logger.Out, logger.Formatter, logger.GetLevel()
	t.Cleanup(func() {
		logger.SetOutput(previousOut)
		logger.SetFormatter(previousFormatter)
		logger.SetLevel(previousLevel)
	})
	logger.SetLevel(log.InfoLevel)
	attributes := map[string]string{"level": "high", "edge": "quote\"\\\n\t\x01中"}
	alice, bob := request("alice", "res1", "read", attributes), request("bob", "res1", "read", attributes)
	calls := []struct {
		name string
		run  func() error
	}{
		{"allow", func() error { _, err := service.IsAllowed(context.Background(), alice); return err }},
		{"roles", func() error { _, err := service.GetAllGrantedRoles(context.Background(), alice); return err }},
		{"permissions", func() error { _, err := service.GetAllPermissions(context.Background(), alice); return err }},
		{"diagnose allow", func() error { _, err := service.Diagnose(context.Background(), alice); return err }},
		{"deny", func() error { _, err := service.IsAllowed(context.Background(), bob); return err }},
		{"diagnose deny", func() error { _, err := service.Diagnose(context.Background(), bob); return err }},
		{"discover", func() error { _, err := service.Discover(context.Background(), bob); return err }},
	}
	field := regexp.MustCompile(`(?:^|\s)responseValue=("(?:\\.|[^"\\])*"|\S+)`)
	for _, format := range []string{"text", "json"} {
		expected := fixtures.Records[format]
		if len(expected) != len(calls) {
			t.Fatalf("%s fixture count=%d, want %d", format, len(expected), len(calls))
		}
		for i, call := range calls {
			t.Run(format+"/"+call.name, func(t *testing.T) {
				var output bytes.Buffer
				logger.SetOutput(&output)
				if format == "text" {
					logger.SetFormatter(&log.TextFormatter{DisableTimestamp: true, DisableColors: true})
				} else {
					logger.SetFormatter(&log.JSONFormatter{})
				}
				if err := call.run(); err != nil {
					t.Fatal(err)
				}
				want := expected[i]
				if format == "json" {
					var actual record
					if err := json.Unmarshal(output.Bytes(), &actual); err != nil {
						t.Fatal(err)
					}
					if call.name == "permissions" {
						actual.Value = normalizePermissionAuditOrder(actual.Value, format)
						want.Value = normalizePermissionAuditOrder(want.Value, format)
					}
					if !reflect.DeepEqual(actual, want) {
						t.Fatalf("audit JSON changed:\ngot %#v\nwant %#v", actual, want)
					}
				} else {
					var value any
					if match := field.FindStringSubmatch(output.String()); match != nil {
						text := match[1]
						if strings.HasPrefix(text, `"`) {
							text, err = strconv.Unquote(text)
							if err != nil {
								t.Fatal(err)
							}
						}
						value = text
					}
					if call.name == "permissions" {
						value = normalizePermissionAuditOrder(value, format)
						want.Value = normalizePermissionAuditOrder(want.Value, format)
					}
					if !strings.Contains(output.String(), want.Message) || !strings.Contains(output.String(), "response="+want.Response) || !reflect.DeepEqual(value, want.Value) {
						t.Fatalf("audit text changed:\ngot %#v\nwant %#v\nrecord %s", value, want.Value, output.String())
					}
				}
			})
		}
	}
}

// Permission enumeration was already unordered before this migration. Keep
// exact per-entry bytes, separators and the final space, normalizing only the
// order of the two simple fixture entries (whose strings contain no spaces).
func normalizePermissionAuditOrder(value any, format string) any {
	if format == "text" {
		text, ok := value.(string)
		if !ok || !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, " ]") {
			return value
		}
		entries := strings.Split(text[1:len(text)-2], "  ")
		sort.Strings(entries)
		return "[" + strings.Join(entries, "  ") + " ]"
	}
	entries, ok := value.([]any)
	if !ok {
		return value
	}
	ordered := append([]any(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].(map[string]any)["resource"].(string) < ordered[j].(map[string]any)["resource"].(string)
	})
	return ordered
}
