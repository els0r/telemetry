package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"unicode"
)

type plainHandler struct {
	mu    sync.Mutex
	w     io.Writer
	level slog.Level
	attrs []slog.Attr
	group string
}

func newPlainHandler(w io.Writer, level slog.Level) *plainHandler {
	return &plainHandler{
		mu:    sync.Mutex{},
		w:     w,
		level: level,
	}
}

func (ph *plainHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= ph.level
}

func (ph *plainHandler) Handle(_ context.Context, r slog.Record) error {
	runes := []rune(r.Message)

	// upper case the first letter
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}

	// collect all attributes (handler-level + record-level)
	var allAttrs []slog.Attr
	allAttrs = append(allAttrs, ph.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// build output: message followed by key=value pairs
	var buf []byte
	buf = append(buf, []byte(string(runes))...)

	for _, attr := range allAttrs {
		key := attr.Key
		if ph.group != "" {
			key = ph.group + "." + key
		}
		buf = append(buf, ' ')
		buf = append(buf, []byte(fmt.Sprintf("%s=%v", key, attr.Value.Any()))...)
	}
	buf = append(buf, '\n')

	ph.mu.Lock()
	_, err := ph.w.Write(buf)
	ph.mu.Unlock()
	return err
}

func (ph *plainHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(ph.attrs)+len(attrs))
	copy(newAttrs, ph.attrs)
	copy(newAttrs[len(ph.attrs):], attrs)
	return &plainHandler{
		w:     ph.w,
		level: ph.level,
		attrs: newAttrs,
		group: ph.group,
	}
}

func (ph *plainHandler) WithGroup(name string) slog.Handler {
	group := name
	if ph.group != "" {
		group = ph.group + "." + name
	}
	return &plainHandler{
		w:     ph.w,
		level: ph.level,
		attrs: ph.attrs,
		group: group,
	}
}
