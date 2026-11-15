//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// captureAudit sends the production audit handler to a buffer as JSON.
func captureAudit(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	previous := AuditLog()
	handler, err := newHandler(&out, LogConfig{Formatter: "json", Level: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	SetAuditLog(slog.New(handler))
	t.Cleanup(func() { SetAuditLog(previous) })
	return &out
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
	for _, test := range []struct {
		level slog.Level
		name  string
	}{
		{slog.LevelWarn, "warning"}, {slog.LevelDebug, "debug"},
	} {
		writeAuditLog("api", nil, nil, test.level)
		if got := lastEntry(t, out)["level"]; got != test.name {
			t.Errorf("level %s logged as %v", test.name, got)
		}
	}
}

func TestAuditPanic(t *testing.T) {
	out := captureAudit(t)
	defer func() {
		if got := recover(); got != "[AUDIT]api" {
			t.Fatalf("panic = %v", got)
		}
		if entry := lastEntry(t, out); entry["level"] != "panic" {
			t.Fatalf("panic record %v", entry)
		}
	}()
	writeAuditLog("api", nil, nil, LevelPanic)
}
