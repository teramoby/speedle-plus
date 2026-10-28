//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/natefinch/lumberjack"
	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

// captureAudit sends the audit log to a buffer as JSON at debug level.
func captureAudit(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	restore := saveLogger(auditLogger)
	auditLogger.Out = &out
	auditLogger.Formatter = &log.JSONFormatter{}
	auditLogger.Level = log.DebugLevel
	auditLogger.ExitFunc = func(int) {}
	t.Cleanup(restore)
	return &out
}

// saveLogger returns a function restoring the logger's settings.
func saveLogger(logger *log.Logger) func() {
	out, formatter, level, exit, caller := logger.Out, logger.Formatter, logger.Level, logger.ExitFunc, logger.ReportCaller
	return func() {
		logger.Out, logger.Formatter, logger.Level, logger.ExitFunc, logger.ReportCaller = out, formatter, level, exit, caller
	}
}

func lastEntry(t *testing.T, out *bytes.Buffer) map[string]interface{} {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatalf("audit entry %q: %v", lines[len(lines)-1], err)
	}
	return entry
}

func TestAuditEntries(t *testing.T) {
	out := captureAudit(t)
	SetTenantID("t1")
	defer SetTenantID("")

	WriteSimpleSucceededAuditLog("CreateService", "svc", 3)
	entry := lastEntry(t, out)
	if entry["msg"] != "[AUDIT]CreateService" || entry["level"] != "info" || entry["response"] != Response_Succeeded ||
		entry[Request_default_key] != "svc" || entry[Response_default_key] != float64(3) || entry[TenantID_key] != "t1" ||
		!strings.HasPrefix(entry[Target_Index_Key].(string), "audit-t1-") {
		t.Fatalf("succeeded entry %v", entry)
	}

	WriteSimpleSucceededAuditLog("ListServices", nil, nil)
	if entry := lastEntry(t, out); entry[Request_default_key] != nil || entry["response"] != Response_Succeeded {
		t.Fatalf("entry without fields %v", entry)
	}

	WriteSimpleFailedAuditLog("DeleteService", "svc", "not found")
	entry = lastEntry(t, out)
	if entry["level"] != "error" || entry["response"] != Response_Failed || entry["reason"] != "not found" {
		t.Fatalf("failed entry %v", entry)
	}
	WriteSimpleFailedAuditLog("DeleteServices", nil, "store down")
	if entry := lastEntry(t, out); entry[Request_default_key] != nil || entry["reason"] != "store down" {
		t.Fatalf("failed entry without request %v", entry)
	}
}

func TestAuditLevels(t *testing.T) {
	out := captureAudit(t)
	for _, level := range []log.Level{log.WarnLevel, log.DebugLevel, log.FatalLevel} {
		writeAuditLog("api", nil, nil, level)
		if got := lastEntry(t, out)["level"]; got != level.String() {
			t.Errorf("level %s logged as %v", level, got)
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("a panic-level audit entry must panic")
			}
		}()
		writeAuditLog("api", nil, nil, log.PanicLevel)
	}()
}

func TestInitLogInstance(t *testing.T) {
	logger := log.New()
	rotation := &lumberjack.Logger{Filename: filepath.Join(t.TempDir(), "a.log")}
	if err := InitLogInstance(logger, &LogConfig{Level: "debug", Formatter: "TEXT", SetReportCaller: true, RotationConfig: rotation}); err != nil {
		t.Fatal(err)
	}
	if logger.Level != log.DebugLevel || !logger.ReportCaller || logger.Out != rotation {
		t.Fatalf("logger %+v", logger)
	}
	if _, ok := logger.Formatter.(*log.TextFormatter); !ok {
		t.Fatalf("formatter %T", logger.Formatter)
	}
	if err := InitLogInstance(logger, &LogConfig{Formatter: "json"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := logger.Formatter.(*log.JSONFormatter); !ok {
		t.Fatalf("formatter %T", logger.Formatter)
	}
	if err := InitLogInstance(logger, nil); err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]*LogConfig{"level": {Level: "loud"}, "formatter": {Formatter: "xml"}} {
		if err := InitLogInstance(logger, cfg); errors.Code(err) != errors.LoggingError {
			t.Errorf("bad %s: %v", name, err)
		}
	}
	if err := InitLogInstance(nil, nil); errors.Code(err) != errors.LoggingError {
		t.Errorf("nil logger: %v", err)
	}
	if err := InitAuditLog(&LogConfig{Level: "nope"}); err == nil {
		t.Error("InitAuditLog must reject a bad level")
	}
}

func TestInitLog(t *testing.T) {
	std := log.StandardLogger()
	defer saveLogger(std)()

	rotation := &lumberjack.Logger{Filename: filepath.Join(t.TempDir(), "b.log")}
	if err := InitLog(&LogConfig{Level: "warn", Formatter: "json", SetReportCaller: true, RotationConfig: rotation}); err != nil {
		t.Fatal(err)
	}
	if std.Level != log.WarnLevel || !std.ReportCaller || std.Out != rotation {
		t.Fatalf("standard logger %+v", std)
	}
	if err := InitLog(&LogConfig{Formatter: "text"}); err != nil {
		t.Fatal(err)
	}
	if err := InitLog(nil); err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]*LogConfig{"level": {Level: "loud"}, "formatter": {Formatter: "xml"}} {
		if err := InitLog(cfg); errors.Code(err) != errors.LoggingError {
			t.Errorf("bad %s: %v", name, err)
		}
	}
}
