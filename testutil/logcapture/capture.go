//Copyright (c) 2026, The Speedle Authors. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package logcapture provides a concurrency-safe slog handler for record-level
// assertions in Engine conformance and recovery tests.
package logcapture

import (
	"context"
	"log/slog"
	"sync"
)

type records struct {
	sync.Mutex
	entries []slog.Record
}

// Handler captures all levels, including attributes attached using With.
// Use one handler per test; its derived handlers share the captured records.
type Handler struct {
	once  sync.Once
	state *records
	attrs []slog.Attr
	group string
}

func (h *Handler) init() {
	h.once.Do(func() {
		if h.state == nil {
			h.state = &records{}
		}
	})
}
func (h *Handler) Enabled(context.Context, slog.Level) bool { return true }
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	h.init()
	record := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	record.AddAttrs(h.attrs...)
	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool { attrs = append(attrs, a); return true })
	if h.group != "" {
		record.AddAttrs(slog.Attr{Key: h.group, Value: slog.GroupValue(attrs...)})
	} else {
		record.AddAttrs(attrs...)
	}
	h.state.Lock()
	defer h.state.Unlock()
	h.state.entries = append(h.state.entries, record)
	return nil
}
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.init()
	copyAttrs := append([]slog.Attr(nil), h.attrs...)
	if h.group != "" {
		copyAttrs = append(copyAttrs, slog.Attr{Key: h.group, Value: slog.GroupValue(attrs...)})
	} else {
		copyAttrs = append(copyAttrs, attrs...)
	}
	return &Handler{state: h.state, attrs: copyAttrs, group: h.group}
}
func (h *Handler) WithGroup(name string) slog.Handler {
	h.init()
	return &Handler{state: h.state, attrs: h.attrs, group: name}
}
func (h *Handler) Records() []slog.Record {
	h.init()
	h.state.Lock()
	defer h.state.Unlock()
	return append([]slog.Record(nil), h.state.entries...)
}
func (h *Handler) Reset() {
	h.init()
	h.state.Lock()
	defer h.state.Unlock()
	h.state.entries = nil
}
