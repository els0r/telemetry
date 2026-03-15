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

// Debug will emit a log message with level debug
func (f *formatter) Debug(args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelDebug) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelDebug, fmt.Sprint(args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// Debugf allows writing of formatted debug messages to the logger.
//
// Deprecated: Use DebugContext with structured attributes instead.
func (f *formatter) Debugf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelDebug) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelDebug, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// DebugContext emits a debug message with context and structured attributes
func (f *formatter) DebugContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, slog.LevelDebug) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelDebug, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Info will emit a log message with level info
func (f *formatter) Info(args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelInfo) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprint(args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// Infof allows writing of formatted info messages to the logger.
//
// Deprecated: Use InfoContext with structured attributes instead.
func (f *formatter) Infof(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelInfo) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// InfoContext emits an info message with context and structured attributes
func (f *formatter) InfoContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, slog.LevelInfo) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Warn will emit a log message with level warn
func (f *formatter) Warn(args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelWarn) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelWarn, fmt.Sprint(args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// Warnf allows writing of formatted warning messages to the logger.
//
// Deprecated: Use WarnContext with structured attributes instead.
func (f *formatter) Warnf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelWarn) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelWarn, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// WarnContext emits a warn message with context and structured attributes
func (f *formatter) WarnContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, slog.LevelWarn) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelWarn, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Error will emit a log message with level error
func (f *formatter) Error(args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelError) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelError, fmt.Sprint(args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// Errorf allows writing of formatted error messages to the logger. It's variadic
// arguments will _not_ add key-value pairs to the message, but be used
// as part of the msg's format string.
//
// Deprecated: Use ErrorContext with structured attributes instead.
func (f *formatter) Errorf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, slog.LevelError) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelError, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
}

// ErrorContext emits an error message with context and structured attributes
func (f *formatter) ErrorContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, slog.LevelError) {
		return
	}
	r := slog.NewRecord(time.Now(), slog.LevelError, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
}

// Fatal will emit a log message with level fatal and exit with a non-zero exit code
func (f *formatter) Fatal(args ...interface{}) {
	if !f.l.Enabled(enableCtx, LevelFatal) {
		return
	}
	r := slog.NewRecord(time.Now(), LevelFatal, fmt.Sprint(args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
	f.exiter.Exit(1)
}

// Fatalf will emit a formatted log message with level fatal and exit with a non-zero exit code.
//
// Deprecated: Use FatalContext with structured attributes instead.
func (f *formatter) Fatalf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, LevelFatal) {
		return
	}
	r := slog.NewRecord(time.Now(), LevelFatal, fmt.Sprintf(format, args...), 0)
	_ = f.l.Handler().Handle(enableCtx, r)
	f.exiter.Exit(1)
}

// FatalContext emits a fatal message with context and structured attributes, then exits
func (f *formatter) FatalContext(ctx context.Context, msg string, attrs ...slog.Attr) {
	if !f.l.Enabled(ctx, LevelFatal) {
		return
	}
	r := slog.NewRecord(time.Now(), LevelFatal, msg, 0)
	r.AddAttrs(attrs...)
	_ = f.l.Handler().Handle(ctx, r)
	f.exiter.Exit(1)
}

// Panic will emit a log message with level panic and panic
func (f *formatter) Panic(args ...interface{}) {
	if !f.l.Enabled(enableCtx, LevelPanic) {
		return
	}
	msg := fmt.Sprint(args...)

	r := slog.NewRecord(time.Now(), LevelPanic, msg, 0)
	_ = f.l.Handler().Handle(enableCtx, r)
	f.panicker.Panic(msg)
}

// Panicf will emit a formatted log message with level panic and panic.
//
// Deprecated: Use PanicContext with structured attributes instead.
func (f *formatter) Panicf(format string, args ...interface{}) {
	if !f.l.Enabled(enableCtx, LevelPanic) {
		return
	}
	msg := fmt.Sprintf(format, args...)

	r := slog.NewRecord(time.Now(), LevelPanic, msg, 0)
	_ = f.l.Handler().Handle(enableCtx, r)
	f.panicker.Panic(msg)
}

// PanicContext emits a panic message with context and structured attributes, then panics
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
