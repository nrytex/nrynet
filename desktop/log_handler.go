package main

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const maxLogs = 500

type memoryLogHandler struct {
	mu      sync.Mutex
	entries []LogEntry
}

func newMemoryLogHandler() *memoryLogHandler {
	return &memoryLogHandler{entries: make([]LogEntry, 0, 128)}
}

func (h *memoryLogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *memoryLogHandler) Handle(_ context.Context, r slog.Record) error {
	entry := LogEntry{Time: r.Time, Level: r.Level.String(), Message: r.Message}
	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	fields := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = logFieldValue(a.Value)
		return true
	})
	entry.Fields = fields
	h.append(entry)
	return nil
}

func logFieldValue(value slog.Value) any {
	if value.Kind() == slog.KindLogValuer {
		return logFieldValue(value.Resolve())
	}
	if value.Kind() != slog.KindAny {
		return value.Any()
	}
	field := value.Any()
	if err, ok := field.(error); ok {
		return err.Error()
	}
	return field
}

func (h *memoryLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &prefixedLogHandler{base: h, attrs: attrs}
}

func (h *memoryLogHandler) WithGroup(string) slog.Handler {
	return h
}

func (h *memoryLogHandler) Snapshot() []LogEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]LogEntry, len(h.entries))
	copy(out, h.entries)
	return out
}

func (h *memoryLogHandler) append(entry LogEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, entry)
	if len(h.entries) > maxLogs {
		h.entries = h.entries[len(h.entries)-maxLogs:]
	}
}

type prefixedLogHandler struct {
	base  *memoryLogHandler
	attrs []slog.Attr
}

func (h *prefixedLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *prefixedLogHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, a := range h.attrs {
		r.AddAttrs(a)
	}
	return h.base.Handle(ctx, r)
}

func (h *prefixedLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := append([]slog.Attr{}, h.attrs...)
	next = append(next, attrs...)
	return &prefixedLogHandler{base: h.base, attrs: next}
}

func (h *prefixedLogHandler) WithGroup(string) slog.Handler {
	return h
}
