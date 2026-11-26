//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teramoby/speedle-plus/pkg/errors"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	FORMATTER_TEXT            = "text"
	FORMATTER_JSON            = "json"
	LevelFatal     slog.Level = 12
	LevelPanic     slog.Level = 16
)

// LogConfig retains the Engine's logging flags and JSON configuration keys.
// RotationConfig uses lumberjack v2; consumers of the former v1 type must
// update their import to gopkg.in/natefinch/lumberjack.v2.
type LogConfig struct {
	Level           string             `json:"level,omitempty"`
	Formatter       string             `json:"formatter,omitempty"`
	SetReportCaller bool               `json:"setReportCaller,omitempty"`
	RotationConfig  *lumberjack.Logger `json:"rotationConfig,omitempty"`
}

type loggerState struct {
	mu     sync.Mutex
	config LogConfig
	writer *rotationWriter
	logger atomic.Pointer[slog.Logger]
}

// rotationWriter preserves a pre-existing regular log file's permission bits
// across lumberjack rotations when the process umask is more restrictive.
// LogConfig.RotationConfig exposes *lumberjack.Logger directly (callers may
// invoke Rotate or Close on it) and lumberjack.v2 offers no custom file-open
// hook; on Unix, Write performs the open/create, O_NOFOLLOW verification,
// File.Chmod, size-based rotation, and write through the same verified file
// descriptor instead of delegating path-based writes to lumberjack.
type rotationWriter struct {
	mu           sync.Mutex
	logger       *lumberjack.Logger
	filename     string
	active       os.FileInfo
	perm         os.FileMode
	uid          uint32
	uidSet       bool
	preservePerm bool
	checked      bool
	maintained   bool
	maxSizeBytes int64
	chmod        func(*os.File, os.FileMode) error
	afterOpen    func(string)
}

func newRotationWriter(l *lumberjack.Logger, prev *rotationWriter) *rotationWriter {
	filename := l.Filename
	if filename == "" {
		filename = filepath.Join(os.TempDir(), filepath.Base(os.Args[0])+"-lumberjack.log")
	}
	if prev != nil && prev.logger == l && prev.filename == filename {
		return prev
	}
	w := &rotationWriter{logger: l, filename: filename}
	w.initPlatform()
	return w
}

func (w *rotationWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writePlatform(p)
}

func (w *rotationWriter) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotatePlatform()
}

// Rotate triggers a synchronized rotation on the application logger's
// configured rotation writer. When no rotation config is installed, it is a no-op.
func Rotate() error { return application.rotate() }

func (s *loggerState) rotate() error {
	s.mu.Lock()
	w := s.writer
	s.mu.Unlock()
	if w == nil {
		return nil
	}
	return w.Rotate()
}

func newState() *loggerState {
	s := &loggerState{}
	l, _ := NewLogger(nil)
	s.logger.Store(l)
	return s
}

var application = newState()
var audit = newState()

// Default returns the Engine application logger. It is independent of both
// slog.Default and the host application's standard-library log logger.
func Default() *slog.Logger { return application.logger.Load() }

// SetDefault installs an application logger, for example a host-owned handler.
// It does not change slog.Default or redirect the standard-library log package.
// A nil logger is a programming error. Configuration from InitLog is retained.
func SetDefault(logger *slog.Logger) {
	if logger == nil {
		panic("logging: nil application logger")
	}
	application.mu.Lock()
	defer application.mu.Unlock()
	application.logger.Store(logger)
}

// SetAuditLog installs an independent audit logger. A nil logger is a
// programming error. Configuration from InitAuditLog is retained.
func SetAuditLog(logger *slog.Logger) {
	if logger == nil {
		panic("logging: nil audit logger")
	}
	audit.mu.Lock()
	defer audit.mu.Unlock()
	audit.logger.Store(logger)
}

// InitLog configures the Engine logger without changing the host's loggers.
// Unspecified fields retain their previous values; nil is a no-op.
func InitLog(cfg *LogConfig) error { return application.init(cfg) }

func (s *loggerState) init(cfg *LogConfig) error {
	if cfg == nil || cfg.Level == "" && cfg.Formatter == "" && !cfg.SetReportCaller && cfg.RotationConfig == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.config
	if cfg.Level != "" {
		next.Level = cfg.Level
	}
	if cfg.Formatter != "" {
		next.Formatter = cfg.Formatter
	}
	if cfg.SetReportCaller {
		next.SetReportCaller = true
	}
	if cfg.RotationConfig != nil {
		next.RotationConfig = cfg.RotationConfig
	}
	logger, writer, err := newLogger(&next, s.writer)
	if err != nil {
		return err
	}
	s.config = next
	s.writer = writer
	s.logger.Store(logger)
	return nil
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "trace", "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	case "fatal":
		return LevelFatal, nil
	case "panic":
		return LevelPanic, nil
	default:
		return 0, errors.Errorf(errors.LoggingError, "failed to parse log level: %q", value)
	}
}

// NewLogger constructs an independent slog logger. Its defaults are info, text,
// stderr and no caller reporting. Invalid values never install a partial config.
func NewLogger(cfg *LogConfig) (*slog.Logger, error) {
	logger, _, err := newLogger(cfg, nil)
	return logger, err
}

func newLogger(cfg *LogConfig, prev *rotationWriter) (*slog.Logger, *rotationWriter, error) {
	var config LogConfig
	if cfg != nil {
		config = *cfg
	}
	var writer io.Writer = os.Stderr
	var rotWriter *rotationWriter
	if config.RotationConfig != nil {
		rotWriter = newRotationWriter(config.RotationConfig, prev)
		writer = rotWriter
	}
	handler, err := newHandler(writer, config)
	if err != nil {
		return nil, nil, err
	}
	return slog.New(handler), rotWriter, nil
}

func newHandler(writer io.Writer, cfg LogConfig) (slog.Handler, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	options := &slog.HandlerOptions{Level: level, AddSource: cfg.SetReportCaller, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && a.Key == slog.SourceKey && cfg.SetReportCaller {
			if source, ok := a.Value.Any().(*slog.Source); ok && source != nil {
				return slog.Group("", slog.String("file", source.File+":"+strconv.Itoa(source.Line)), slog.String("func", source.Function))
			}
		}
		if len(groups) == 0 && a.Key == slog.LevelKey {
			switch a.Value.Any().(slog.Level) {
			case slog.LevelDebug:
				a.Value = slog.StringValue("debug")
			case slog.LevelInfo:
				a.Value = slog.StringValue("info")
			case slog.LevelWarn:
				a.Value = slog.StringValue("warning")
			case slog.LevelError:
				a.Value = slog.StringValue("error")
			case LevelFatal:
				a.Value = slog.StringValue("fatal")
			case LevelPanic:
				a.Value = slog.StringValue("panic")
			}
		}
		return a
	}}
	var handler slog.Handler
	switch strings.ToLower(cfg.Formatter) {
	case "", FORMATTER_TEXT:
		handler = slog.NewTextHandler(writer, options)
	case FORMATTER_JSON:
		handler = slog.NewJSONHandler(writer, options)
	default:
		return nil, errors.Errorf(errors.LoggingError, "unknown formatter type: %q", cfg.Formatter)
	}
	return &compatibleHandler{Handler: handler, caller: cfg.SetReportCaller, text: !strings.EqualFold(cfg.Formatter, FORMATTER_JSON)}, nil
}

// compatibleHandler keeps logrus metadata names without duplicating those keys
// when user attributes collide. HandlerOptions.ReplaceAttr alone cannot tell
// user-supplied time/level/msg apart from the handler's own metadata.
type compatibleHandler struct {
	slog.Handler
	caller bool
	text   bool
	attrs  []slog.Attr
	groups []string
}

// resolve copies group slices as well: WithAttrs must not retain mutable
// caller-owned slices. Collision handling happens only after all scopes merge.
func (h *compatibleHandler) resolve(a slog.Attr) slog.Attr {
	if a.Equal(slog.Attr{}) {
		return a
	}
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		copied := make([]slog.Attr, len(attrs))
		for i, child := range attrs {
			copied[i] = h.resolve(child)
		}
		a.Value = slog.GroupValue(copied...)
	} else if err, ok := a.Value.Any().(error); ok {
		a.Value = slog.StringValue(err.Error())
	} else if h.text && a.Value.Kind() == slog.KindAny {
		// TextHandler uses %+v; legacy audit values use fmt.Sprint (%v).
		a.Value = slog.StringValue(fmt.Sprint(a.Value.Any()))
	} else if source, ok := a.Value.Any().(*slog.Source); ok {
		// Do not confuse a user-supplied Source with AddSource's metadata.
		if source == nil {
			a.Value = slog.AnyValue(nil)
		} else {
			a.Value = slog.GroupValue(slog.String("function", source.Function), slog.String("file", source.File), slog.Int("line", source.Line))
		}
	}
	return a
}

func scopedAttrs(groups []string, attrs []slog.Attr) []slog.Attr {
	if len(attrs) == 0 {
		return nil
	}
	for i := len(groups) - 1; i >= 0; i-- {
		attrs = []slog.Attr{{Key: groups[i], Value: slog.GroupValue(attrs...)}}
	}
	return attrs
}

// coalesce retains map-style last-write precedence. Reserved fields overwrite
// an explicitly supplied fields.<key> alias, as the old logrus formatter did.
// Buffering WithAttrs until Handle allows this to work across With chains too.
func (h *compatibleHandler) coalesce(attrs []slog.Attr, top bool) []slog.Attr {
	result := make([]slog.Attr, 0, len(attrs))
	positions := make(map[string]int, len(attrs))
	var add func(slog.Attr)
	add = func(a slog.Attr) {
		if a.Equal(slog.Attr{}) {
			return
		}
		if a.Value.Kind() == slog.KindGroup {
			children := h.coalesce(a.Value.Group(), false)
			// Empty groups are absent in slog, so they must not overwrite a prior
			// scalar or a populated group before the underlying handler drops them.
			if len(children) == 0 {
				return
			}
			a.Value = slog.GroupValue(children...)
			if a.Key == "" {
				for _, child := range children {
					add(child)
				}
				return
			}
		}
		if i, ok := positions[a.Key]; ok {
			if result[i].Value.Kind() == slog.KindGroup && a.Value.Kind() == slog.KindGroup {
				children := append([]slog.Attr(nil), result[i].Value.Group()...)
				a.Value = slog.GroupValue(h.coalesce(append(children, a.Value.Group()...), false)...)
			}
			result[i] = a
		} else {
			positions[a.Key] = len(result)
			result = append(result, a)
		}
	}
	for _, a := range attrs {
		add(a)
	}
	if top {
		reserved := []string{"time", "msg", "level", "logrus_error"}
		if h.caller {
			reserved = append(reserved, "file", "func")
		}
		for _, key := range reserved {
			i, ok := positions[key]
			if !ok {
				continue
			}
			a := result[i]
			a.Key = "fields." + key
			if j, ok := positions[a.Key]; ok {
				result[j] = a
				result[i] = slog.Attr{}
			} else {
				result[i] = a
			}
		}
	}
	return result
}

func (h *compatibleHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := append([]slog.Attr(nil), h.attrs...)
	var current []slog.Attr
	r.Attrs(func(a slog.Attr) bool { current = append(current, h.resolve(a)); return true })
	attrs = append(attrs, scopedAttrs(h.groups, current)...)
	record := slog.NewRecord(r.Time, r.Level, strings.TrimSuffix(r.Message, "\n"), r.PC)
	record.AddAttrs(h.coalesce(attrs, true)...)
	return h.Handler.Handle(ctx, record)
}

func (h *compatibleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copied := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		copied[i] = h.resolve(a)
	}
	next := *h
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), scopedAttrs(h.groups, copied)...)
	return &next
}

func (h *compatibleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

// Fatal logs at the fatal level and exits with status 1 even when filtered.
func Fatal(message string) {
	logTermination(LevelFatal, message)
	os.Exit(1)
}

// Panic logs at the panic level and panics even when filtered.
func Panic(message string) {
	logTermination(LevelPanic, message)
	panic(message)
}

func logTermination(level slog.Level, message string) {
	logger := Default()
	ctx := context.Background()
	if !logger.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	_ = logger.Handler().Handle(ctx, slog.NewRecord(time.Now(), level, message, pcs[0]))
}
