# A Collection of Telemetry Go modules

[![Github Release](https://img.shields.io/github/release/els0r/telemetry.svg)](https://github.com/els0r/telemetry/releases)
[![Build/Test Status](https://github.com/els0r/telemetry/workflows/Go/badge.svg)](https://github.com/els0r/telemetry/actions?query=workflow%3AGo)
[![CodeQL](https://github.com/els0r/telemetry/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/els0r/telemetry/actions/workflows/codeql-analysis.yml)

This package contains the following Go modules:

|Module|Description|Documentation|Report|
|-|-|-|-|
| [logging](./logging) | A module providing convenience wrappers for [log/slog](https://pkg.go.dev/log/slog) | [![GoDoc](https://godoc.org/github.com/fels0r/telemetry/logging?status.svg)](https://godoc.org/github.com/els0r/telemetry/logging/) | [![Go Report Card](https://goreportcard.com/badge/github.com/els0r/telemetry/logging)](https://goreportcard.com/report/github.com/els0r/telemetry/logging) |
| [metrics](./metrics) | A module providing HTTP metrics middleware for [gin-gonic](github.com/gin-gonic/gin) | [![GoDoc](https://godoc.org/github.com/els0r/telemetry/metrics?status.svg)](https://godoc.org/github.com/els0r/telemetry/metrics/) | [![Go Report Card](https://goreportcard.com/badge/github.com/els0r/telemetry/metrics)](https://goreportcard.com/report/github.com/els0r/telemetry/metrics) |
| [tracing](./tracing) | A module providing convenience wrappers for tracing using the [OpenTelemetry SDK](go.opentelemetry.io/otel/sdk/trace) | [![GoDoc](https://godoc.org/github.com/els0r/telemetry/tracing?status.svg)](https://godoc.org/github.com/els0r/telemetry/tracing/) | [![Go Report Card](https://goreportcard.com/badge/github.com/els0r/telemetry/tracing)](https://goreportcard.com/report/github.com/els0r/telemetry/tracing) |

## Bug Reports

Please use the [issue tracker](https://github.com/els0r/telemetry/issues) for bugs and feature requests.

## License

See the [LICENSE](./LICENSE) file for usage conditions.
