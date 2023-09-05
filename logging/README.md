logging - A module providing convenience wrappers for log/slog
===========
[![GoDoc](https://godoc.org/github.com/fels0r/telemetry/logging?status.svg)](https://godoc.org/github.com/els0r/telemetry/logging/)
[![Go Report Card](https://goreportcard.com/badge/github.com/els0r/telemetry/logging)](https://goreportcard.com/report/github.com/els0r/telemetry/logging)

This module wraps the Go standard library logger [log/slog](https://pkg.go.dev/log/slog) (introduced in Go 1.21), providing a simplified interaction model and additional capabilities & convenience functions.

## Features
- Structured logging allowing for context-based cumulative field enrichment
- Support for format-based output (such as `Infof(...)`)
- Support for various (extensible) output encodings (`console`, `logfmt`, `JSON`, ...)
- Extension of standard logging levels by `Fatal[f](...)` and `Panic[f](...)` logging events (triggering the respective actions)
- Performance-oriented approach to log level handling

## Installation
```bash
go get -u github.com/els0r/telemetry/logging
```

## Examples
Instantiate a new simple / plain console logger with INFO level, writing to STDOUT:
```Go
logger, err := logging.New(
    logging.LevelInfo,
    logging.EncodingPlain,
)
if err != nil {
	// Handle error, e.g. abort
}
```
Instantiate a new formatting logger with WARNING level, writing to STDOUT (ERROR level events and higher to STDERR) and adding the caller and a structured version log field to all emitted messages:
```Go
logger, err := logging.New(
	logging.LevelWarn,
	logging.EncodingLogfmt,
	logging.WithOutput(os.Stdout),
	logging.WithErrorOutput(os.Stderr),
	logging.WithVersion("v1.0.0"),
    logging.WithCaller(true),
)
if err != nil {
	// Handle error, e.g. abort
}
```
Enrich an exising context / logger in a function / method (labels set in parent context are present):
```Go
func handle(ctx context.Context, iface string) {
	ctx = logging.WithFields(ctx, slog.String("iface", iface))
	logger := logging.FromContext(ctx)

    logging.FromContext(ctx).Info("performing action") // will throw [... iface=XYZ] and all labels from parent context

    // ...
}
```
Some simple log emission examples:
```Go
// INFO message, no formatting directive
logger.Info("this is a plain INFO message")

// DEBUG message, no formatting directive (will only show if log level is logging.LevelDebug)
logger.Debug("this is a plain DEBUG message")

// FATAL message (terminating the program) with formatting directive
err := errors.New("this is a critical error")
logger.Fatalf("critical error encountered: %s", err)

// PANIC message (throwing a stack trace / panic) with formatting directive
logger.Panicf("really critical error encountered: %s", err)
```