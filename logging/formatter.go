package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

type formatter struct {
	l *slog.Logger

	exiter   exiter
	panicker panicker
}

type exiter interface {
	Exit(code int)
}

type defaultExiter struct{}

func (de defaultExiter) Exit(code int) {
	os.Exit(code)
}

type panicker interface {
	Panic(msg string)
}

type defaultPanicker struct{}

func (dp defaultPanicker) Panic(msg string) {
	panic(msg)
}

var enableCtx = context.Background()

// logAttrs is a helper that creates a record, adds args as slog key-value pairs, and
// dispatches to the handler. It follows slog's convention: args are alternating keys
// and values (e.g., "key1", val1, "key2", val2) or slog.Attr values.
func (f *formatter) logAttrs(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !f.l.Enabled(ctx, level) {
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.Add(args...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Debug emits a log message with level debug.
// Args follow slog convention: alternating key-value pairs or slog.Attr values.
//
//	logger.Debug("cache hit", "key", cacheKey, "ttl", ttl)
func (f *formatter) Debug(msg string, args ...any) {
	f.logAttrs(enableCtx, slog.LevelDebug, msg, args...)
}

// Debugf allows writing of formatted debug messages to the logger
func (f *formatter) Debugf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelDebug) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelDebug, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// DebugContext emits a debug message, passing ctx through to the handler chain.
// This enables context-aware handler middleware (e.g., trace ID injection) to read
// from the context. Use with FromContext for the full pattern:
//
//	logging.FromContext(ctx).DebugContext(ctx, "msg", slog.String("key", "val"))
//
// The attrs parameter allows attaching per-call structured fields without a
// separate .With() allocation.
func (f *formatter) DebugContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, slog.LevelDebug) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelDebug, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Info emits a log message with level info.
// Args follow slog convention: alternating key-value pairs or slog.Attr values.
//
//	logger.Info("request handled", "method", r.Method, "status", code)
func (f *formatter) Info(msg string, args ...any) {
	f.logAttrs(enableCtx, slog.LevelInfo, msg, args...)
}

// Infof allows writing of formatted info messages to the logger
func (f *formatter) Infof(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelInfo) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// InfoContext emits an info message, passing ctx through to the handler chain.
// This enables context-aware handler middleware (e.g., trace ID injection) to read
// from the context. Use with FromContext for the full pattern:
//
//	logging.FromContext(ctx).InfoContext(ctx, "msg", slog.String("key", "val"))
//
// The attrs parameter allows attaching per-call structured fields without a
// separate .With() allocation.
func (f *formatter) InfoContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, slog.LevelInfo) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Warn emits a log message with level warn.
// Args follow slog convention: alternating key-value pairs or slog.Attr values.
func (f *formatter) Warn(msg string, args ...any) {
	f.logAttrs(enableCtx, slog.LevelWarn, msg, args...)
}

// Warnf allows writing of formatted warning messages to the logger
func (f *formatter) Warnf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelWarn) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelWarn, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// WarnContext emits a warn message, passing ctx through to the handler chain.
// See InfoContext for usage pattern and rationale.
func (f *formatter) WarnContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, slog.LevelWarn) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelWarn, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Error emits a log message with level error.
// Args follow slog convention: alternating key-value pairs or slog.Attr values.
func (f *formatter) Error(msg string, args ...any) {
	f.logAttrs(enableCtx, slog.LevelError, msg, args...)
}

// Errorf allows writing of formatted error messages to the logger. Its variadic
// arguments will _not_ add key-value pairs to the message, but be used
// as part of the msg's format string
func (f *formatter) Errorf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelError) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelError, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// ErrorContext emits an error message, passing ctx through to the handler chain.
// See InfoContext for usage pattern and rationale.
func (f *formatter) ErrorContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, slog.LevelError) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelError, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Fatal emits a log message with level fatal and exits with a non-zero exit code.
// Args follow slog convention: alternating key-value pairs or slog.Attr values.
func (f *formatter) Fatal(msg string, args ...any) {
	if !f.l.Enabled(enableCtx, LevelFatal) {
		return
	}
	r := slog.NewRecord(time.Now(), LevelFatal, msg, 0)
	r.Add(args...)
	_ = f.l.Handler().Handle(enableCtx, r)
	f.exiter.Exit(1)
}

// Fatalf will emit a formatted log message with level fatal and exit with a non-zero exit code
func (f *formatter) Fatalf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, LevelFatal) {
		return
	}
	r := slog.NewRecord(time.Now(), LevelFatal, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
	f.exiter.Exit(1)
}

// FatalContext emits a fatal message, passing ctx through to the handler chain, then exits.
// See InfoContext for usage pattern and rationale.
func (f *formatter) FatalContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, LevelFatal) {
		return
	}
	r := slog.NewRecord(time.Now(), LevelFatal, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
	f.exiter.Exit(1)
}

// Panic emits a log message with level panic, then panics.
// Args follow slog convention: alternating key-value pairs or slog.Attr values.
func (f *formatter) Panic(msg string, args ...any) {
	if !f.l.Enabled(enableCtx, LevelPanic) {
		return
	}
	r := slog.NewRecord(time.Now(), LevelPanic, msg, 0)
	r.Add(args...)
	_ = f.l.Handler().Handle(enableCtx, r)
	f.panicker.Panic(msg)
}

// Panicf will emit a formatted log message with level panic and panic
func (f *formatter) Panicf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, LevelPanic) {
		return
	}
	msg := fmt.Sprintf(format, args...)

	r := slog.NewRecord(time.Now(), LevelPanic, msg, 0)
	_ = f.l.Handler().Handle(enableCtx, r)
	f.panicker.Panic(msg)
}

// PanicContext emits a panic message, passing ctx through to the handler chain, then panics.
// See InfoContext for usage pattern and rationale.
func (f *formatter) PanicContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, LevelPanic) {
		return
	}
	r := slog.NewRecord(time.Now(), LevelPanic, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
	f.panicker.Panic(msg)
}

// Slog returns the underlying *slog.Logger for direct access to the standard library API
func (f *formatter) Slog() *slog.Logger {
	return f.l
}
