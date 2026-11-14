//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	stderrlog "log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"

	"github.com/teramoby/speedle-plus/pkg/errors"
	"gopkg.in/natefinch/lumberjack.v2"
)

func testLogger(t *testing.T, cfg LogConfig) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	handler, err := newHandler(&out, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return slog.New(handler), &out
}

func TestLevelMappingAndThresholds(t *testing.T) {
	for _, test := range []struct {
		name   string
		level  slog.Level
		output string
	}{
		{"trace", slog.LevelDebug, "debug"}, {"DEBUG", slog.LevelDebug, "debug"},
		{"", slog.LevelInfo, "info"}, {"info", slog.LevelInfo, "info"},
		{"warn", slog.LevelWarn, "warning"}, {"Warning", slog.LevelWarn, "warning"},
		{"error", slog.LevelError, "error"}, {"FATAL", LevelFatal, "fatal"}, {"panic", LevelPanic, "panic"},
	} {
		t.Run(test.name, func(t *testing.T) {
			logger, out := testLogger(t, LogConfig{Formatter: "json", Level: test.name})
			if logger.Enabled(context.Background(), test.level-1) || !logger.Enabled(context.Background(), test.level) {
				t.Fatal("threshold changed")
			}
			logger.Log(context.Background(), test.level-1, "hidden")
			logger.Log(context.Background(), test.level, "visible")
			entry := lastEntry(t, out)
			if entry["level"] != test.output || entry["msg"] != "visible" || strings.Count(out.String(), "\n") != 1 {
				t.Fatalf("record %v", entry)
			}
		})
	}
}

func TestFormatsFieldsAndCaller(t *testing.T) {
	for _, format := range []string{"", "text", "TEXT", "json", "JSON"} {
		t.Run(format, func(t *testing.T) {
			logger, out := testLogger(t, LogConfig{Formatter: format, SetReportCaller: true})
			logger.With("level", "user", "error", errors.New(errors.LoggingError, "broken"), "time", "user-time").Info("hello", "msg", "user-message", "file", "user-file", "func", "user-func", "count", 3)
			if strings.EqualFold(format, "json") {
				entry := lastEntry(t, out)
				if entry["msg"] != "hello" || entry["level"] != "info" || entry["fields.level"] != "user" || entry["fields.msg"] != "user-message" || entry["fields.time"] != "user-time" || entry["fields.file"] != "user-file" || entry["fields.func"] != "user-func" || entry["count"] != float64(3) || !strings.Contains(entry["error"].(string), "broken") || !strings.Contains(entry["file"].(string), "config_test.go:") || !strings.Contains(entry["func"].(string), "TestFormatsFieldsAndCaller") {
					t.Fatalf("fields %v", entry)
				}
			} else {
				for _, want := range []string{"level=info", "msg=hello", "fields.level=user", "fields.msg=user-message", "count=3", "config_test.go:"} {
					if !strings.Contains(out.String(), want) {
						t.Errorf("missing %q: %s", want, out)
					}
				}
			}
		})
	}
	logger, out := testLogger(t, LogConfig{Formatter: "json"})
	logger.Info("no caller", "file", "user-file", "source", "user-source")
	entry := lastEntry(t, out)
	if entry["func"] != nil || entry["file"] != "user-file" || entry["source"] != "user-source" {
		t.Fatalf("caller disabled: %v", entry)
	}
}

func TestHandlerContract(t *testing.T) {
	var out bytes.Buffer
	handler, err := newHandler(&out, LogConfig{Formatter: "json", Level: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	err = slogtest.TestHandler(handler, func() []map[string]any {
		var results []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			var result map[string]any
			if err := json.Unmarshal([]byte(line), &result); err != nil {
				t.Fatal(err)
			}
			results = append(results, result)
		}
		return results
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInitializationIsAtomicAndPartial(t *testing.T) {
	oldApp, oldAudit := application, audit
	application, audit = newState(), newState()
	t.Cleanup(func() { application, audit = oldApp, oldAudit })
	rotation := &lumberjack.Logger{Filename: filepath.Join(t.TempDir(), "app.log")}
	defer rotation.Close()
	if err := InitLog(&LogConfig{Level: "warn", Formatter: "json", SetReportCaller: true, RotationConfig: rotation}); err != nil {
		t.Fatal(err)
	}
	if err := InitLog(&LogConfig{Level: "debug"}); err != nil {
		t.Fatal(err)
	}
	if application.config.Formatter != "json" || !application.config.SetReportCaller || application.config.RotationConfig != rotation {
		t.Fatalf("partial config %+v", application.config)
	}
	previous := Default()
	for _, cfg := range []*LogConfig{nil, {Level: "loud"}, {Formatter: "xml"}, {Level: "error", Formatter: "xml"}} {
		err := InitLog(cfg)
		if cfg != nil && errors.Code(err) != errors.LoggingError {
			t.Errorf("invalid config %v", err)
		}
		if Default() != previous {
			t.Fatal("invalid/nil config replaced logger")
		}
	}
	Default().Debug("retained writer")
	raw, err := os.ReadFile(rotation.Filename)
	if err != nil || !bytes.Contains(raw, []byte(`"msg":"retained writer"`)) {
		t.Fatalf("writer %s %v", raw, err)
	}
	if err := InitAuditLog(&LogConfig{Level: "error", Formatter: "text"}); err != nil {
		t.Fatal(err)
	}
	auditBefore := AuditLog()
	if err := InitAuditLog(&LogConfig{Level: "wrong"}); err == nil || AuditLog() != auditBefore {
		t.Fatal("invalid audit initialization changed logger")
	}
	if err := InitAuditLog(nil); err != nil || AuditLog() != auditBefore {
		t.Fatal("nil audit config changed logger")
	}
	if !Default().Enabled(context.Background(), slog.LevelDebug) || AuditLog().Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("audit/application settings coupled")
	}
	for _, cfg := range []*LogConfig{{Level: " old "}, {Formatter: "invalid"}} {
		if _, err := NewLogger(cfg); errors.Code(err) != errors.LoggingError || !strings.Contains(err.Error(), strings.TrimSpace(cfg.Level+cfg.Formatter)) {
			t.Fatalf("error missing config value: %v", err)
		}
	}
	if _, err := NewLogger(nil); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryDoesNotRedirectHostLogging(t *testing.T) {
	beforeSlog, beforeWriter, beforeFlags, beforePrefix := slog.Default(), stderrlog.Writer(), stderrlog.Flags(), stderrlog.Prefix()
	previous := Default()
	previousConfig := application.config
	t.Cleanup(func() { SetDefault(previous); application.config = previousConfig })
	if err := InitLog(&LogConfig{Level: "panic", Formatter: "json"}); err != nil {
		t.Fatal(err)
	}
	if slog.Default() != beforeSlog || stderrlog.Writer() != beforeWriter || stderrlog.Flags() != beforeFlags || stderrlog.Prefix() != beforePrefix {
		t.Fatal("library initialization modified host logging")
	}
}

func TestRotationAndConfigRoundTrip(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "engine.log")
			if existing {
				if err := os.WriteFile(filename, []byte("existing\n"), 0640); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filename, 0640); err != nil {
					t.Fatal(err)
				}
			}
			cfg := LogConfig{Level: "debug", Formatter: "json", RotationConfig: &lumberjack.Logger{Filename: filename, MaxSize: 1, MaxAge: 2, MaxBackups: 3, LocalTime: true, Compress: false}}
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var decoded LogConfig
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.RotationConfig.Filename != filename || decoded.RotationConfig.MaxSize != 1 || decoded.RotationConfig.MaxAge != 2 || decoded.RotationConfig.MaxBackups != 3 || !decoded.RotationConfig.LocalTime {
				t.Fatalf("round trip %+v", decoded.RotationConfig)
			}
			logger, err := NewLogger(&decoded)
			if err != nil {
				t.Fatal(err)
			}
			defer decoded.RotationConfig.Close()
			logger.Info("first")
			info, err := os.Stat(filename)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" {
				if existing && info.Mode().Perm() != 0640 {
					t.Fatalf("existing mode %o", info.Mode().Perm())
				}
				if !existing && info.Mode().Perm()&0077 != 0 {
					t.Fatalf("new log readable by non-owner: %o", info.Mode().Perm())
				}
			}
			mode := info.Mode().Perm()
			if err := decoded.RotationConfig.Rotate(); err != nil {
				t.Fatal(err)
			}
			logger.Info("second")
			if err := decoded.RotationConfig.Close(); err != nil {
				t.Fatal(err)
			}
			logger.Info("reopened")
			data, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("first")) || !bytes.Contains(data, []byte("second")) || !bytes.Contains(data, []byte("reopened")) {
				t.Fatalf("rotation %s", data)
			}
			info, err = os.Stat(filename)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != mode {
				t.Fatalf("rotated mode %o, want %o", info.Mode().Perm(), mode)
			}
			backups, err := filepath.Glob(strings.TrimSuffix(filename, ".log") + "-*.log")
			if err != nil || len(backups) != 1 {
				t.Fatalf("backups %v %v", backups, err)
			}
			data, err = os.ReadFile(backups[0])
			if err != nil || !bytes.Contains(data, []byte("first")) {
				t.Fatalf("backup %s %v", data, err)
			}
		})
	}
}

func TestAuditMergeAndErrorFields(t *testing.T) {
	out := captureAudit(t)
	SetTenantID("tenant")
	defer SetTenantID("")
	WriteSucceededAuditLog("merge", map[string]interface{}{"same": "request", "response": "wrong", "tenantid": "wrong", "msg": "field"}, map[string]interface{}{"same": "response", "error": os.ErrNotExist})
	entry := lastEntry(t, out)
	if entry["same"] != "response" || entry["tenantid"] != "tenant" || entry["response"] != "succeeded" || entry["fields.msg"] != "field" || entry["error"] != os.ErrNotExist.Error() {
		t.Fatalf("merge %v", entry)
	}
}

func TestTermination(t *testing.T) {
	if mode := os.Getenv("SPEEDLE_TEST_TERMINATION"); mode != "" {
		level := "debug"
		if strings.Contains(mode, "filtered") {
			level = "panic"
		}
		if err := InitLog(&LogConfig{Level: level, Formatter: "json", SetReportCaller: true}); err != nil {
			panic(err)
		}
		if mode == "audit-fatal" {
			if err := InitAuditLog(&LogConfig{Formatter: "json"}); err != nil {
				panic(err)
			}
			writeAuditLog("terminal", nil, nil, LevelFatal)
			return
		}
		if strings.HasPrefix(mode, "fatal") {
			Fatal("terminal")
			return
		}
		// A native slog handler can filter panic too; termination still must occur.
		if strings.HasSuffix(mode, "filtered") {
			SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: LevelPanic + 1})))
		}
		Panic("terminal")
		return
	}
	for _, mode := range []string{"fatal", "fatal-filtered", "panic", "panic-filtered", "audit-fatal"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestTermination$")
			cmd.Env = append(os.Environ(), "SPEEDLE_TEST_TERMINATION="+mode)
			output, err := cmd.CombinedOutput()
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("termination %v %s", err, output)
			}
			want := 1
			if strings.HasPrefix(mode, "panic") {
				want = 2
			}
			if exit.ExitCode() != want {
				t.Fatalf("exit %d want %d: %s", exit.ExitCode(), want, output)
			}
			hasRecord := bytes.Contains(output, []byte(`"msg":"terminal"`)) || bytes.Contains(output, []byte(`"msg":"[AUDIT]terminal"`))
			if hasRecord == strings.Contains(mode, "filtered") {
				t.Fatalf("filter: %s", output)
			}
			if hasRecord && mode != "audit-fatal" && (!bytes.Contains(output, []byte(`config_test.go:`)) || !bytes.Contains(output, []byte(`TestTermination`))) {
				t.Fatalf("caller: %s", output)
			}
		})
	}
}

func TestConcurrentLoggerReplacement(t *testing.T) {
	previous := Default()
	defer SetDefault(previous)
	logger := slog.New(slog.DiscardHandler)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				SetDefault(logger)
				Default().Info("concurrent")
			}
		})
	}
	wg.Wait()
}
