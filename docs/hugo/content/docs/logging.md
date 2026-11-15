+++
title = "Logging Framework"
description = "Speedle logging"
weight = 320
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "logging"]
categories = ["docs"]
bref = ""
lastmod = 2026-11-08T08:33:54+08:00
+++

## Overview

For the release after v1.8.0, Speedle uses Go `log/slog` for structured application and audit logs and `gopkg.in/natefinch/lumberjack.v2` for file rotation. v1.8.0 and older releases use the previous logging API.

## Logging configuration

All existing `--log-*` and `--auditlog-*` flags, `SPDL_*` environment variables and JSON configuration keys remain supported. Explicit flags override environment variables, which override the configuration file. Application defaults are info, text, stderr and no caller metadata; audit defaults are info and JSON. Set the corresponding `--auditlog-*` option to configure audit output independently.

```text
--log-level string
--log-formatter string
--log-reportcaller bool
--log-filename string
--log-maxsize int
--log-maxbackups int
--log-maxage int
--log-localtime bool
--log-compress bool
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--log-level` | `info` | Minimum severity |
| `--log-formatter` | `text` | `text` or `json` |
| `--log-reportcaller` | `false` | Include caller file, line and function |
| `--log-filename` | unset | Stderr unless a file is configured |
| `--log-maxsize` | `100` | Maximum file size in MB |
| `--log-maxbackups` | `0` | Backup count; 0 retains all, subject to max age |
| `--log-maxage` | `0` | Backup age in days; 0 disables age deletion |
| `--log-localtime` | `false` | Local time for backup filenames, otherwise UTC |
| `--log-compress` | `false` | Compress backups |

```json
{
  "logConfig": {
    "level": "info",
    "formatter": "text",
    "setReportCaller": false,
    "rotationConfig": {
      "filename": "/mnt/logs/speedle.log",
      "maxSize": 20,
      "maxBackups": 5,
      "maxAge": 0,
      "LocalTime": false,
      "compress": false
    }
  },
  "auditLogConfig": {
    "level": "info",
    "formatter": "json"
  }
}
```


## Levels and output formats

Level and formatter names are case-insensitive. Accepted levels are `trace`, `debug`, `info`, `warn`, `warning`, `error`, `fatal` and `panic`. Trace maps to debug; selecting debug therefore also emits former trace messages. The remaining thresholds retain their order. Unknown nonempty values fail with an error naming the supplied value.

Use `text` for key=value records or `json` for one JSON object per line. Metadata keys remain `time`, `level`, `msg` and, when caller reporting is enabled, `file` (with line number) and `func`. Level values are lowercase, with `warning` for warn. Errors remain strings and other structured fields remain structured. User fields colliding with metadata receive a `fields.` prefix.

```text
time=2026-11-15T09:00:00.000+08:00 level=info msg="Started observing beach" animal=walrus number=8
```

```json
{"time":"2026-11-15T09:00:00+08:00","level":"info","msg":"Started observing beach","animal":"walrus","number":8}
```


## Migration for log collectors

Text quoting, field order, color and timestamp precision can differ from logrus. Text has no ANSI colors; timestamps are RFC3339-compatible. Check existing parsers before upgrading, or use JSON and parse keys rather than field order. Audit messages retain `[AUDIT]` followed directly by the API name, plus `tenantid`, `target_index`, request/response values and success/error severity.

New rotated log files default to owner-only `0600` permissions, subject to umask, instead of the former `0644`. Rotation preserves the mode of an existing file. Run the collector as the log owner or explicitly provision access before upgrading; do not assume a separate collector account can read newly created files.

## Go API migration

Replace `InitLogInstance(logrusLogger, config)` with `logger, err := logging.NewLogger(config)`. `AuditLog()` now returns `*slog.Logger`; use native slog methods and `SetAuditLog(logger)` instead of logrus setters/hooks. Change the lumberjack import to v2 when assigning `LogConfig.RotationConfig`; v1 pointers no longer compile. `InitLog` and `InitAuditLog` retain unspecified settings and treat nil or an empty config as a no-op. Invalid configuration leaves the installed logger unchanged.

Use `logging.Default()` for the Engine logger or `logging.SetDefault(logger)` to install a host-owned slog handler. These functions do not change `slog.Default()` or redirect the host application’s standard `log` output. `logging.Fatal(message)` logs then exits with status 1; `logging.Panic(message)` logs then panics, even when their records are filtered.

```go
import (
    "github.com/teramoby/speedle-plus/pkg/logging"
    "gopkg.in/natefinch/lumberjack.v2"
)

logger, err := logging.NewLogger(&logging.LogConfig{
    Level: "info", Formatter: "json",
    RotationConfig: &lumberjack.Logger{Filename: "engine.log", MaxSize: 20},
})
if err != nil {
    return err
}
logger.Info("Started observing beach", "animal", "walrus", "number", 8)
```


## Container logging

For containers, leave the filename unset and use the container runtime’s log rotation. No lumberjack file configuration is needed. Embedded etcd may still pull logrus and zap transitively, but Engine code uses slog.
