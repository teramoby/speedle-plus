# Engine slog migration design (#389)

Status: approved for implementation for the release after v1.8.0.
Authorization behavior is outside scope.

## Coordinator: design approved

Approved. Implement the design with the following clarifications: keep the
Engine logger independent of the host's standard logger, retain the stricter
lumberjack v2 file permissions, and document Go API and collector migrations
under Unreleased. The coordinator runs the full regression.

## Logging API

Use native `log/slog` at Engine call sites. `pkg/logging` owns configuration,
handler construction, audit helpers and the few fatal/panic helpers needed to
preserve existing termination behavior. Do not build a replacement logrus API.

- Retain `InitLog(*LogConfig) error` and `InitAuditLog(*LogConfig) error`.
  Initialize independent application and audit handlers; `InitLog` installs the
  application logger through `logging.Default()` without calling
  `slog.SetDefault` (which would also redirect the host's standard `log`).
  `SetDefault(*slog.Logger)` and `SetAuditLog(*slog.Logger)` allow host-owned
  handlers and scoped test capture without changing process-wide loggers.
- Add `NewLogger(*LogConfig) (*slog.Logger, error)` for independent instances.
  Replace the logrus-specific `InitLogInstance` API with this constructor;
  callers assign its returned logger instead of mutating a logrus instance.
- `AuditLog()` returns `*slog.Logger`. Keep `SetTenantID`, audit constants and
  the existing `Write*AuditLog` function signatures.
- Preserve partial initialization: absent settings retain the initialized
  logger's settings; `InitLog(nil)`/`InitAuditLog(nil)` are no-ops. Fresh loggers
  default to info, text, stderr and caller reporting disabled. Validate settings
  before publishing a replacement so errors leave the previous logger intact.
- Existing fatal calls must still log then exit with status 1; panic calls must
  still log then panic, including when their output is filtered. Implement
  small `logging.Fatal`/`logging.Panic` helpers using slog records with the
  original caller location. Verify exit behavior in subprocess tests.

This deliberately changes the Go source API where it exposes logrus or the old
rotation type. It does not claim source compatibility for those consumers.

## Levels and fields

Accept the existing case-insensitive level names, including `warning` and
`trace`. Unknown nonempty values return `errors.LoggingError` naming the value.

| Existing level | slog level | Output `level` |
| --- | --- | --- |
| trace, debug | Debug (-4) | debug |
| info | Info (0) | info |
| warn, warning | Warn (4) | warning |
| error | Error (8) | error |
| fatal | custom (12) | fatal |
| panic | custom (16) | panic |

Preserve severity filtering and existing messages, with one explicit exception:
trace and debug share the debug threshold, so selecting debug also emits former
trace messages. Convert printf calls with
`fmt.Sprintf`, variadic message calls with `fmt.Sprint`, `WithField(s)` to slog
attributes and `WithError` to an `error` string attribute. Keep structured values
structured. Preserve `time`, `level`, `msg`, and caller `file`/`func` keys; caller
`file` includes its line number. Reserved-key user fields retain logrus-style
`fields.` prefixes rather than creating duplicate metadata keys.

Audit logs keep `[AUDIT]` directly before the API name and preserve
`tenantid`, `target_index` (`audit-<tenant>-YYYYMM`), `response`, `reason`,
`requestValue`, `responseValue` and request/response field merging. Keep success
at info and failure at error; do not change which operations emit audit events.

## Formats and configuration

Retain every `--log-*`/`--auditlog-*` flag, corresponding `SPDL_*` environment
variable, precedence rule and JSON key (`logConfig`, `auditLogConfig`, `level`,
`formatter`, `setReportCaller`, `rotationConfig` and rotation fields).
`text`/`json` remain case-insensitive; reject unknown nonempty formatter values
clearly. Keep existing command defaults, including audit defaults.

Use `slog.NewTextHandler` and `slog.NewJSONHandler` with shared attribute
normalization for compatible keys and lowercase levels. JSON remains one object
per line; text remains key=value. Timestamps stay RFC3339-compatible. Text
quoting/order, color behavior and timestamp precision may differ from logrus;
byte-for-byte formatting is not promised. Document examples and these collector
implications in the logging configuration pages during implementation.

## File rotation and Go consumer migration

Promote the already indirect `gopkg.in/natefinch/lumberjack.v2` to a direct
dependency; use its `*lumberjack.Logger` as `LogConfig.RotationConfig`. Preserve
filename, size, backup count, age, local-time and compression settings, existing
JSON decoding, and stderr when no rotation writer is configured.

Go consumers migrate their import from `github.com/natefinch/lumberjack` to
`gopkg.in/natefinch/lumberjack.v2`; struct literals keep the same rotation fields.
Existing v1 pointers no longer compile. Replace #405's external-package legacy
assignment test with a v2 consumer test only after this explicit break is
approved. Test real writes, close/reopen, rotation and config round trips.
New log files use v2's owner-only `0600` default (subject to umask), rather
than v1's `0644`. Existing file modes are preserved through rotation. Keep
these stricter defaults; run collectors as the log owner or explicitly provision
appropriate access before switching versions.

## Dependencies and scope

Remove Engine logrus imports, test hooks, demo usage and the old lumberjack
module. Migrate the playground adapter's logrus use too so its module can tidy
away logrus; update its logging capture without changing evaluation results.
Do not hand-edit generated website assets. Read `website/DESIGN.md` before any
website change and follow the repository generation process.

On this base, `spctl` already uses standard `log`; the only direct Go zap import
is the etcd TLS test's `transport.SelfCert(zap.NewNop(), ...)`. Replace that
fixture with a standard-library certificate fixture preserving its assertions.
Remove zap and logrus as direct requirements. `go mod why -m` confirms that
etcd retains zap and also retains logrus through
`github.com/tmc/grpc-websocket-proxy/wsproxy`; neither can disappear from the
root dependency graph while the current embedded-etcd dependency needs it.
The browser module can remove both entirely from its actual build graph.
Replace stray Engine standard-log calls with slog while preserving their exit
and panic behavior. No authorization, policy, recovery or store logic changes.

## Validation after approval

Adapt existing logging/audit output tests and log-capture assertions rather than
discarding them. Add table tests for every accepted/rejected level and format,
thresholds, caller fields, metadata collisions, error values, atomic invalid
initialization, partial initialization and independent audit/application config.
Retain flag/config precedence and rotation-option coverage. Run targeted logging,
flags, evaluator and audit regression tests, then vet, builds, module tidiness and playground checks. Dispatch full CI
on this branch without changing queue labels; the coordinator runs the full
regression. This worker never changes labels or merges.
Report exact-head CI and ReviewBot evidence separately from local results.

Validation results and exact-head CI/review evidence are maintained in the PR
description as implementation proceeds.
