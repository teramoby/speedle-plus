//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
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
	logger atomic.Pointer[slog.Logger]
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
	if cfg == nil {
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
	logger, err := NewLogger(&next)
	if err != nil {
		return err
	}
	s.config = next
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
	var config LogConfig
	if cfg != nil {
		config = *cfg
	}
	var writer io.Writer = os.Stderr
	if config.RotationConfig != nil {
		writer = config.RotationConfig
	}
	handler, err := newHandler(writer, config)
	if err != nil {
		return nil, err
	}
	return slog.New(handler), nil
}

func newHandler(writer io.Writer, cfg LogConfig) (slog.Handler, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	options := &slog.HandlerOptions{Level: level, AddSource: cfg.SetReportCaller, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && a.Key == slog.SourceKey && cfg.SetReportCaller {
			if source, ok := a.Value.Any().(*slog.Source); ok {
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
	caller  bool
	grouped bool
	text    bool
}

func (h *compatibleHandler) normalize(a slog.Attr, top bool) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		normalized := make([]slog.Attr, len(attrs))
		for i, child := range attrs {
			normalized[i] = h.normalize(child, top && a.Key == "")
		}
		a.Value = slog.GroupValue(normalized...)
	} else if err, ok := a.Value.Any().(error); ok {
		a.Value = slog.StringValue(err.Error())
	} else if h.text && a.Value.Kind() == slog.KindAny {
		// TextHandler uses %+v for arbitrary values; the existing audit contract
		// uses fmt.Sprint (%v), including the legacy protobuf response structs.
		a.Value = slog.StringValue(fmt.Sprint(a.Value.Any()))
	}
	if top && (a.Key == "time" || a.Key == "level" || a.Key == "msg" || h.caller && (a.Key == "file" || a.Key == "func")) {
		a.Key = "fields." + a.Key
	}
	return a
}

func (h *compatibleHandler) Handle(ctx context.Context, r slog.Record) error {
	record := slog.NewRecord(r.Time, r.Level, strings.TrimSuffix(r.Message, "\n"), r.PC)
	r.Attrs(func(a slog.Attr) bool { record.AddAttrs(h.normalize(a, !h.grouped)); return true })
	return h.Handler.Handle(ctx, record)
}

func (h *compatibleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	normalized := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		normalized[i] = h.normalize(a, !h.grouped)
	}
	return &compatibleHandler{Handler: h.Handler.WithAttrs(normalized), caller: h.caller, grouped: h.grouped, text: h.text}
}

func (h *compatibleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &compatibleHandler{Handler: h.Handler.WithGroup(name), caller: h.caller, grouped: true, text: h.text}
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
