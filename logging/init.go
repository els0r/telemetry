// Package logging supplies a global, structured logger
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
)

type loggingConfig struct {
	enableCaller bool
	stdOutput    io.Writer
	errsOutput   io.Writer
	initialAttr  map[string]slog.Attr
	replaceAttr  func(groups []string, a slog.Attr) slog.Attr
	closers      []io.Closer
}

const (
	initKeyName    = "name"
	initKeyVersion = "version"
)

// ShutdownFunc is a function that releases resources held by the logger (e.g., open file handles).
// It should be called when the logger is no longer needed.
type ShutdownFunc func() error

var noShutdown ShutdownFunc = func() error { return nil }

// Option denotes a functional option for the logging configuration
type Option func(*loggingConfig) error

// WithOutput sets the log output
func WithOutput(w io.Writer) Option {
	return func(lc *loggingConfig) error {
		lc.stdOutput = w
		return nil
	}
}

// WithErrorOutput sets the log output for level Error, Fatal and Panic. For the rest,
// the default output os.Stdout or the output set by `WithOutput` is chosen
func WithErrorOutput(w io.Writer) Option {
	return func(lc *loggingConfig) error {
		lc.errsOutput = w
		return nil
	}
}

var (
	errEmptyFilePath = errors.New("empty filepath provided")
)

const (
	devnullOutput = "devnull"
	stderrOutput  = "stderr"
	stdoutOutput  = "stdout"
)

// WithFileOutput sets the log output to a file. The filepath can be one of the following:
//
// - stdout: logs will be written to os.Stdout
// - stderr: logs will be written to os.Stderr
// - devnull: logs will be discarded
// - any other filepath: logs will be written to the file
//
// The special filepaths are case insensitive, e.g. DEVNULL works just as well.
// The file will be closed when the ShutdownFunc returned by New/Init is called.
func WithFileOutput(path string) Option {
	return func(lc *loggingConfig) error {
		var output io.Writer
		switch strings.ToLower(path) { // ToLower will allow users to pass STDERR for example
		case stdoutOutput:
			output = os.Stdout
		case stderrOutput:
			output = os.Stderr
		case devnullOutput:
			output = io.Discard
		case "":
			return errEmptyFilePath
		default:
			f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666) // #nosec G302
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			lc.closers = append(lc.closers, f)
			output = f
		}
		return WithOutput(output)(lc)
	}
}

// WithCaller sets whether the calling source should be logged, since the operation is
// computationally expensive
func WithCaller(b bool) Option {
	return func(lc *loggingConfig) error {
		lc.enableCaller = b
		return nil
	}
}

// WithName sets the application name as initial field present in all log messages
func WithName(name string) Option {
	return func(lc *loggingConfig) error {
		lc.initialAttr[initKeyName] = slog.String(initKeyName, name)
		return nil
	}
}

// WithVersion sets the application version as initial field present in all log messages
func WithVersion(version string) Option {
	return func(lc *loggingConfig) error {
		lc.initialAttr[initKeyVersion] = slog.String(initKeyVersion, version)
		return nil
	}
}

// WithReplaceAttr sets a custom attribute replacement function. This is applied on top
// of the default replacements (time key, level key, source key). It can be used to
// customize field names or transform values.
func WithReplaceAttr(fn func(groups []string, a slog.Attr) slog.Attr) Option {
	return func(lc *loggingConfig) error {
		lc.replaceAttr = fn
		return nil
	}
}

// globalLogger caches the *L wrapper around slog.Default() to avoid allocations
// on every Logger() call. It is invalidated (set to nil) when Init is called.
var globalLogger atomic.Pointer[L]

// globalShutdown holds the shutdown function for the current global logger.
var globalShutdown atomic.Pointer[ShutdownFunc]

// Init initializes the global logger. The `encoding` variable sets whether content should
// be printed for console output or in JSON (for machine consumption).
// Returns a ShutdownFunc that should be called to release resources (e.g., close file handles).
func Init(level slog.Level, encoding Encoding, opts ...Option) (ShutdownFunc, error) {
	// assign configured logger to slog's default logger
	logger, shutdown, err := New(level, encoding, opts...)
	if err != nil {
		return noShutdown, err
	}
	slog.SetDefault(logger.l)

	// close previous global logger's resources
	if prev := globalShutdown.Load(); prev != nil {
		_ = (*prev)()
	}

	// invalidate cached global logger so Logger() picks up the new default
	globalLogger.Store(nil)
	globalShutdown.Store(&shutdown)
	return shutdown, nil
}

// New returns a new logger and a ShutdownFunc to release its resources
func New(level slog.Level, encoding Encoding, opts ...Option) (*L, ShutdownFunc, error) {
	if level == LevelUnknown {
		return nil, noShutdown, fmt.Errorf("unknown log level provided: %s", level)
	}

	cfg := &loggingConfig{
		stdOutput:   os.Stdout,
		initialAttr: make(map[string]slog.Attr),
	}
	for _, opt := range opts {
		err := opt(cfg)
		if err != nil {
			return nil, noShutdown, err
		}
	}

	replaceFunc := func(groups []string, a slog.Attr) slog.Attr {
		// write time as ts
		switch a.Key {
		case slog.TimeKey:
			a.Key = "ts"
		case slog.LevelKey:
			// Handle custom level values
			level := a.Value.Any().(slog.Level)

			switch {
			case level < LevelInfo:
				a.Value = slog.StringValue(debugLevel)
			case level < LevelWarn:
				a.Value = slog.StringValue(infoLevel)
			case level < LevelError:
				a.Value = slog.StringValue(warnLevel)
			case level < LevelFatal:
				a.Value = slog.StringValue(errorLevel)
			case level < LevelPanic:
				a.Value = slog.StringValue(fatalLevel)
			default:
				a.Value = slog.StringValue(panicLevel)
			}
		case slog.SourceKey:
			a.Key = "caller"

			source := a.Value.Any().(*slog.Source)

			// only returns the pkg name, file and line number
			dir, file := filepath.Split(source.File)
			source.File = filepath.Join(filepath.Base(dir), file)
		}

		// apply user-provided replacement on top of defaults
		if cfg.replaceAttr != nil {
			a = cfg.replaceAttr(groups, a)
		}

		return a
	}

	hopts := slog.HandlerOptions{
		Level:       level,
		AddSource:   cfg.enableCaller,
		ReplaceAttr: replaceFunc,
	}

	th, err := getHandler(cfg.stdOutput, encoding, hopts)
	if err != nil {
		return nil, noShutdown, err
	}

	// inject a split level handler in case the error output is defined
	if cfg.errsOutput != nil {
		h, _ := getHandler(cfg.errsOutput, encoding, hopts)
		th = newLevelSplitHandler(th, h)
	}

	// assign initial attributes if there are any
	var attrs []slog.Attr
	for _, attr := range cfg.initialAttr {
		attrs = append(attrs, attr)
	}

	if len(attrs) > 0 {
		sort.SliceStable(attrs, func(i, j int) bool {
			return attrs[i].Key < attrs[j].Key
		})
		th = th.WithAttrs(attrs)
	}

	if cfg.enableCaller {
		// inject a caller handler in case the caller should be reported. It's important that
		// this one comes at the beginning of the chain
		th = &callerHandler{addSource: cfg.enableCaller, next: th}
	}

	shutdown := noShutdown
	if len(cfg.closers) > 0 {
		closers := cfg.closers
		shutdown = func() error {
			var errs []error
			for _, c := range closers {
				if err := c.Close(); err != nil {
					errs = append(errs, err)
				}
			}
			return errors.Join(errs...)
		}
	}

	// return a new L logger
	return newL(slog.New(th)), shutdown, nil
}

func getHandler(w io.Writer, encoding Encoding, hopts slog.HandlerOptions) (th slog.Handler, err error) {
	switch encoding {
	case EncodingJSON:
		th = slog.NewJSONHandler(w, &hopts)
	case EncodingLogfmt:
		th = slog.NewTextHandler(w, &hopts)
	case EncodingPlain:
		th = newPlainHandler(w, hopts.Level.Level())
	default:
		return nil, fmt.Errorf("unknown encoding %q", encoding)
	}
	return th, nil
}

// NewFromContext creates a new logger, deriving structured fields from the supplied context
func NewFromContext(ctx context.Context, level slog.Level, encoding Encoding, opts ...Option) (*L, ShutdownFunc, error) {
	logger, shutdown, err := New(level, encoding, opts...)
	if err != nil {
		return nil, noShutdown, err
	}
	return fromContext(ctx, logger), shutdown, nil
}

// Logger returns the cached global logger wrapping slog.Default().
// The result is cached to avoid allocations in performance-critical sections.
func Logger() *L {
	if l := globalLogger.Load(); l != nil {
		return l
	}
	l := newL(slog.Default())
	globalLogger.Store(l)
	return l
}

type loggerKeyType int

const (
	fieldsKey loggerKeyType = iota
)

// loggerFields stores context-accumulated structured attributes.
// Uses an immutable slice (copy-on-write) — no mutex needed.
type loggerFields struct {
	attrs []slog.Attr
}

// WithFields returns a context that has extra fields added.
//
// The method is meant to be used in conjunction with FromContext that selects
// the context-enriched logger.
//
// The strength of this approach is that labels set in parent context are accessible.
func WithFields(ctx context.Context, fields ...slog.Attr) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	var existing []slog.Attr
	if lf, ok := ctx.Value(fieldsKey).(loggerFields); ok {
		existing = lf.attrs
	}

	// create a new slice: copy existing + append new (copy-on-write)
	merged := make([]slog.Attr, 0, len(existing)+len(fields))
	merged = append(merged, existing...)
	merged = append(merged, fields...)

	return context.WithValue(ctx, fieldsKey, loggerFields{attrs: merged})
}

func fromContext(ctx context.Context, logger *L) *L {
	if ctx == nil {
		return logger
	}
	lf, ok := ctx.Value(fieldsKey).(loggerFields)
	if !ok || len(lf.attrs) == 0 {
		return logger
	}

	// convert attrs to interface{} slice for With()
	args := make([]interface{}, len(lf.attrs))
	for i, attr := range lf.attrs {
		args[i] = attr
	}
	return logger.With(args...)
}

// FromContext returns a global logger which has as much context set as possible
func FromContext(ctx context.Context) *L {
	return fromContext(ctx, Logger())
}
