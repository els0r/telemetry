# Telemetry SDK Review

Critical analysis of the `els0r/telemetry` repository covering SDK clarity/UX, performance, structured logging best practices, and OpenTelemetry semantic convention alignment.

Findings are organized by severity: **Critical** (bugs, correctness), **High** (design flaws, breaking UX), **Medium** (best-practice violations), **Low** (nits, polish).

---

## Critical

### C1. `ContextFrom*Header` functions mutate global propagator (tracing)

**File:** `tracing/traceparent.go:67`, `tracing/traceparent.go:85`

`ContextFromW3CTraceparentHeader` and `ContextFromB3SingleHeader` both call `otel.SetTextMapPropagator(...)`, mutating global state as a side effect of what looks like a pure extraction function. In a concurrent application, two goroutines calling these functions with different header formats will race on the global propagator.

```go
// traceparent.go:67 — mutates global state during extraction!
func ContextFromW3CTraceparentHeader(ctx context.Context, traceparentHeader string) context.Context {
    p := propagation.TraceContext{}
    otel.SetTextMapPropagator(p) // <-- global side effect
    return p.Extract(ctx, ...)
}
```

**Fix:** Remove the `otel.SetTextMapPropagator` calls. Use a local propagator instance for extraction only. If callers need to change the global propagator, they should do so explicitly.

**Breaking:** No.

---

### C2. Dead code / unreachable error check in `NewTracerProvider` (tracing)

**File:** `tracing/trace.go:144-147`

```go
if err != nil {  // err is always nil here — leftover from removed code
    return nil, fmt.Errorf("failed to create resource: %w", err)
}
```

The variable `err` was last assigned by the options loop (line 135-138) but is checked again at line 145. If any option fails, the function already returns at line 138. This dead branch is confusing and suggests a removed `resource.New(...)` call that was replaced by `resource.Default()` without cleaning up.

**Fix:** Delete the dead `if err != nil` block.

**Breaking:** No.

---

### C3. `ContextFromW3CTraceparentHeader` discards provided context when header is empty (tracing)

**File:** `tracing/traceparent.go:62-64`

```go
if traceparentHeader == "" {
    return context.Background()  // discards the ctx the caller passed in!
}
```

Same issue in `ContextFromB3SingleHeader`. If the header is empty, the function returns `context.Background()` instead of the caller's `ctx`, silently dropping all context values (deadlines, cancellation, trace context from other sources).

**Fix:** Return `ctx` instead of `context.Background()` when the header is empty.

**Breaking:** Behavioral change, but the current behavior is almost certainly a bug.

---

## High

### H1. Logger API uses `fmt.Sprint`/`fmt.Sprintf` — incompatible with slog's structured paradigm

**File:** `logging/formatter.go` (all methods)

The entire `L` API (`Debug`, `Debugf`, `Info`, `Infof`, etc.) uses `fmt.Sprint(args...)` for the message, which means:

1. **No structured key-value pairs on log calls.** The only way to attach fields is `.With(args...).Info(msg)`, which allocates a new logger per call.
2. **No `context.Context` parameter.** Every method uses a package-level `enableCtx = context.Background()`, which means handler middleware that reads from context (e.g., trace ID injection, request-scoped fields) does not work.
3. **`Debugf`/`Infof`/etc. are an anti-pattern in structured logging.** They encourage string interpolation over structured fields, producing messages that are impossible to parse or aggregate.

This is the single biggest UX issue: the SDK wraps `slog` but removes its most valuable feature — structured per-call attributes with context propagation.

**Fix:** Provide `slog`-style methods that accept `context.Context` and `...slog.Attr`:

```go
func (l *L) InfoContext(ctx context.Context, msg string, attrs ...slog.Attr) { ... }
```

Keep `Info(msg string, args ...any)` for convenience but route it through `slog.Logger` directly (which already does structured key-value parsing). Deprecate or remove `Infof` and friends.

**Breaking:** Yes — removing `Infof`, `Debugf`, etc. Alternatively, deprecate them and add the new methods alongside.

---

### H2. `Logger()` allocates on every call

**File:** `logging/init.go:243-245`

```go
func Logger() *L {
    return newL(slog.Default())  // allocates *L and *formatter every time
}
```

The comment says "low allocation logger for performance critical sections", but `newL` allocates a `*L` wrapping a `*formatter` on every call. In hot paths (e.g., `FromContext` which calls `Logger()`), this adds unnecessary GC pressure.

**Fix:** Cache the global `*L` instance (protected by `sync.Once` or atomic pointer) and invalidate it only when `Init` is called.

**Breaking:** No.

---

### H3. `loggerFields` uses `map[string]interface{}` with mutex — unnecessary overhead

**File:** `logging/init.go:253-263`

Context fields are stored as `map[string]interface{}` with a `sync.RWMutex`. Every `WithFields` call copies the entire map. Every `FromContext` call sorts the keys. This is heavyweight for what should be a lightweight operation.

Problems:
- The map stores `slog.Attr` values but is typed as `interface{}`, losing type safety.
- Full map copy on every `WithFields` call in a deep call stack is O(n) per level.
- Sorting keys on every `FromContext` is O(n log n) per log call.

**Fix:** Use a linked-list / slice-based approach (append-only). Store `[]slog.Attr` instead of `map[string]interface{}`. Deduplication can happen at log time or be skipped entirely (slog handlers tolerate duplicate keys).

**Breaking:** Internal, no public API change.

---

### H4. `plainHandler` silently drops all attributes and groups

**File:** `logging/plain-handler.go:44-49`

```go
func (ph *plainHandler) WithAttrs(_ []slog.Attr) slog.Handler { return ph }
func (ph *plainHandler) WithGroup(_ string) slog.Handler { return ph }
```

This means `logger.With("key", "val").Info("msg")` silently discards `key=val` when using `EncodingPlain`. There's no warning or documentation that this encoding loses data. If a user switches from JSON to Plain for local dev, they lose all context silently.

**Fix:** Document this explicitly. Consider printing attributes after the message in a simple `key=val` format, or at minimum log a warning when attributes are attached to a plain handler.

**Breaking:** No (additive).

---

### H5. `WithGRPCExporter` blocks on dial with a 1-second timeout (tracing)

**File:** `tracing/trace.go:55-63`

```go
dialCtx, cancel := context.WithTimeout(ctx, time.Second)
defer cancel()
conn, err := grpc.DialContext(dialCtx, collectorEndpoint,
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithBlock(),  // blocks until connected or timeout
)
```

Issues:
1. `grpc.DialContext` and `grpc.WithBlock()` are **deprecated** in modern gRPC-Go (since v1.63). Use `grpc.NewClient` instead.
2. Blocking on dial during initialization means the application hangs for 1 second if the collector is unreachable. OpenTelemetry best practice is to connect lazily.
3. Hard-coded `insecure.NewCredentials()` with no option to enable TLS.

**Fix:** Switch to `grpc.NewClient` (lazy connection). Remove `WithBlock`. Let the exporter handle reconnection. Add a `WithTLS` option.

**Breaking:** Yes — removes the synchronous "fail if collector is down" behavior. Applications relying on `Init` failing when the collector is unreachable would need to add health checks instead.

---

### H6. Hardcoded instrumentation scope name from a different repository (tracing)

**File:** `tracing/trace.go:29`

```go
const instrumentationCodeProviderName = "github.com/els0r/goProbe/pkg/tracing"
```

This is the path of a *different repository* (`goProbe`), not this SDK. Every span created via `tracing.Start()` will report a misleading instrumentation scope. This is a copy-paste artifact.

**Fix:** Change to `"github.com/els0r/telemetry/tracing"` or derive it dynamically.

**Breaking:** Changes span metadata (instrumentation scope name visible in tracing backends).

---

### H7. Metrics module is tightly coupled to Gin (metrics)

**File:** `metrics/middleware.go`

The entire metrics module only works with `gin-gonic/gin`. There's no `net/http` middleware, no framework-agnostic interface. This limits adoption and is unusual for a "telemetry" SDK.

Additionally:
- Uses the **global Prometheus registry** (`prometheus.MustRegister`) — panics if metrics are registered twice (e.g., in tests).
- `pathLabelFromContext` is never set (no public API to configure it), making the feature dead code.
- `reqSz` and `resSz` use `Summary` without quantile objectives, making them almost useless (they only track count and sum).

**Fix:**
1. Accept a `*prometheus.Registry` instead of using the global one.
2. Provide `net/http` middleware alongside Gin.
3. Either expose `pathLabelFromContext` via a `With` option or remove it.
4. Replace `Summary` with `Histogram` for request/response sizes.

**Breaking:** Yes — constructor signature change for custom registry.

---

## Medium

### M1. No correlation between logging and tracing

There is no built-in mechanism to inject `trace_id` / `span_id` into log records. This is table-stakes for observability — without it, correlating logs to traces requires manual plumbing by every consumer.

**Fix:** Provide an `slog.Handler` middleware that extracts the span context from `context.Context` and adds `trace_id`, `span_id`, and `trace_flags` attributes. This requires the logging methods to accept a `context.Context` (see H1).

**Breaking:** Depends on H1 resolution.

---

### M2. Non-standard log field names (logging)

**File:** `logging/init.go:136-163`

| slog default | SDK override | OTel semantic convention |
|---|---|---|
| `time` | `ts` | `Timestamp` (in log data model) |
| `level` | lowercase (`info`) | `SeverityText` (uppercase: `INFO`) |
| `source` | `caller` | `code.filepath`, `code.lineno`, `code.function` |
| — | `name` | `service.name` (resource attr) |
| — | `version` | `service.version` (resource attr) |

The SDK renames standard fields to non-standard names. `ts` instead of `time` or `timestamp` is unusual. Lowercase level values (`info` vs `INFO`) conflict with OTel's `SeverityText` convention and common log aggregator expectations.

`name` and `version` are set as log record attributes, but in OTel they belong on the **Resource**, not individual log records. This causes them to be repeated on every line.

**Fix:** Align with OTel semantic conventions or at minimum make field renaming configurable. Move `name`/`version` to resource-level concerns.

**Breaking:** Yes — output format changes.

---

### M3. `AlwaysSample()` sampler with no configuration (tracing)

**File:** `tracing/trace.go:152`

```go
sdktrace.WithSampler(sdktrace.AlwaysSample()),
```

There is no option to configure sampling. In production, `AlwaysSample` generates excessive trace data. Users should be able to set `TraceIDRatioBased`, `ParentBased`, or custom samplers.

**Fix:** Add `WithSampler(sampler sdktrace.Sampler) Option`. Default to `sdktrace.ParentBased(sdktrace.AlwaysSample())` which is the OTel default and respects parent sampling decisions.

**Breaking:** Changes default sampling behavior (parent-based vs always).

---

### M4. `convert.ToKeyVals` silently drops `KindDuration` (tracing/internal)

**File:** `tracing/internal/convert/convert.go:29`

```go
case slog.KindDuration:
    // falls through to return empty kvs!
case slog.KindFloat64:
```

`KindDuration` has no implementation — the switch case is empty and falls through. Duration attributes are silently dropped. No test covers this.

**Fix:** Convert durations to float64 seconds (OTel convention) or string representation:

```go
case slog.KindDuration:
    return []attribute.KeyValue{attribute.Float64(key, val.Duration().Seconds())}
```

**Breaking:** No — currently produces nothing, any output is an improvement.

---

### M5. `WithFileOutput` leaks file descriptors (logging)

**File:** `logging/init.go:80`

```go
f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
```

The opened file is never closed. There's no `Close()` method on `L` or `loggingConfig`, and no shutdown hook. If `Init` is called multiple times (e.g., config reload), each call leaks a file descriptor.

**Fix:** Return a cleanup/shutdown function from `New`/`Init` (similar to tracing's `ShutdownFunc`), or track the file and close it on re-initialization.

**Breaking:** Yes — `Init` / `New` return signature would need to include a closer.

---

### M6. No `WithResource` option (tracing)

**File:** `tracing/trace.go:154`

```go
sdktrace.WithResource(resource.Default()),
```

The resource is hardcoded to `resource.Default()`. Users cannot set `service.name`, `service.version`, or `deployment.environment` explicitly — they must rely on environment variables (`OTEL_SERVICE_NAME`, etc.). This is inflexible and poorly documented.

**Fix:** Add `WithResource(*resource.Resource) Option` and/or `WithServiceName(string) Option`.

**Breaking:** No.

---

### M7. Viper global state in flags.go (tracing)

**File:** `tracing/flags.go:81`

```go
_ = viper.BindPFlags(flags)
```

Uses viper's global instance, which conflicts if the consuming application also uses viper. The error from `BindPFlags` is silently ignored.

**Fix:** Accept a `*viper.Viper` instance or remove the viper dependency entirely (read flag values directly from `pflag`).

**Breaking:** Yes — if callers rely on viper bindings.

---

## Low

### L1. `GetSpanID` checks `HasTraceID()` instead of `HasSpanID()` (tracing)

**File:** `tracing/traceparent.go:137-138`

```go
func GetSpanID(ctx context.Context) (spanID string) {
    sc := trace.SpanContextFromContext(ctx)
    if sc.HasTraceID() {  // should be sc.HasSpanID() or sc.IsValid()
        return sc.SpanID().String()
    }
```

Functionally equivalent in practice (a valid span context always has both), but semantically incorrect.

**Fix:** Use `sc.IsValid()` or `sc.HasSpanID()`.

---

### L2. Typo in metrics help text (metrics)

**File:** `metrics/middleware.go:122`

```go
Help: "HTTP reponse sizes in bytes",  // "reponse" → "response"
```

---

### L3. Unnecessary `collector := collector` in range loop (metrics)

**File:** `metrics/middleware.go:139`

```go
for _, collector := range toRegister {
    collector := collector  // unnecessary since Go 1.22 (loop var semantics changed)
```

This was needed before Go 1.22 only if the variable was captured in a closure. Here it's passed directly to `MustRegister`, so it was never needed.

---

### L4. `testLogger.Write` returns `n=0` always (logging)

**File:** `logging/logging_test.go:24-27`

```go
func (tl *testLogger) Write(data []byte) (n int, err error) {
    tl.t.Log(strings.TrimRight(string(data), string('\n')))
    return n, err  // n is always 0, err is always nil
}
```

While test-only, returning `n=0` from `Write` technically signals "no bytes written" which could cause issues with some writers that check return values.

---

### L5. Exported flag constants are verbose and mix concerns (tracing)

```go
TracingEnabledArg     = "tracing.enabled"
TracingEnabledDefault = false
TracingEnabledHelp    = "enable tracing"
```

Help text and defaults are exported as package constants. These are implementation details that don't belong in the public API.

---

## Summary Table

| ID | Severity | Module | Breaking? | Summary |
|----|----------|--------|-----------|---------|
| C1 | Critical | tracing | No | `ContextFrom*Header` mutates global propagator |
| C2 | Critical | tracing | No | Dead unreachable error check |
| C3 | Critical | tracing | ~No | Empty header discards caller's context |
| H1 | High | logging | Yes | No context or structured attrs on log methods |
| H2 | High | logging | No | `Logger()` allocates on every call |
| H3 | High | logging | No | Context fields use heavyweight map+mutex+sort |
| H4 | High | logging | No | Plain handler silently drops all attributes |
| H5 | High | tracing | Yes | Deprecated blocking gRPC dial |
| H6 | High | tracing | ~Yes | Wrong instrumentation scope name |
| H7 | High | metrics | Yes | Gin-only, global registry, dead code, useless summaries |
| M1 | Medium | cross | Depends | No log-trace correlation |
| M2 | Medium | logging | Yes | Non-standard field names |
| M3 | Medium | tracing | ~Yes | No sampler configuration |
| M4 | Medium | tracing | No | Duration attributes silently dropped |
| M5 | Medium | logging | Yes | File descriptor leak |
| M6 | Medium | tracing | No | No resource configuration |
| M7 | Medium | tracing | Yes | Viper global state |
| L1 | Low | tracing | No | Wrong Has check for SpanID |
| L2 | Low | metrics | No | Typo in help text |
| L3 | Low | metrics | No | Unnecessary loop var copy |
| L4 | Low | logging | No | Test writer returns 0 bytes |
| L5 | Low | tracing | No | Over-exported flag constants |

---

## Proposed Fix Plan

### Phase 1: Bug fixes (no breaking changes)

Fix C1, C2, C3, H2, H6, M4, L1, L2, L3.

These are all safe, non-breaking fixes that correct bugs and dead code:

1. **C1** — Remove `otel.SetTextMapPropagator()` from `ContextFromW3CTraceparentHeader` and `ContextFromB3SingleHeader`. Use local propagator for `Extract` only.
2. **C2** — Delete the dead `if err != nil` block at `trace.go:144-147`.
3. **C3** — Return `ctx` instead of `context.Background()` when header is empty.
4. **H2** — Cache the global `*L` in a `sync.Pointer`/atomic, invalidate on `Init()`.
5. **H6** — Change `instrumentationCodeProviderName` to `"github.com/els0r/telemetry/tracing"`.
6. **M4** — Implement `KindDuration` → `attribute.Float64(key, dur.Seconds())`.
7. **L1** — Change `HasTraceID()` to `IsValid()` in `GetSpanID`.
8. **L2** — Fix "reponse" typo.
9. **L3** — Remove unnecessary loop variable copy.

### Phase 2: Logging API improvements (breaking)

Fix H1, H3, H4, M1, M2, M5.

1. **H1** — Add context-aware methods: `InfoContext(ctx, msg, ...slog.Attr)`, etc. Deprecate `Infof`/`Debugf`/etc. (remove in next major).
2. **H3** — Replace `map[string]interface{}` with `[]slog.Attr` (append-only, no mutex needed for immutable slices via copy-on-write).
3. **H4** — Document plainHandler data loss. Optionally print attrs in `key=val` format after message.
4. **M1** — Add a `TraceContextHandler` that injects `trace_id`/`span_id` from `context.Context` into log records. Wire it into the handler chain when tracing is active.
5. **M2** — Make field renaming configurable via `WithReplaceAttr`. Default to OTel-aligned names (`timestamp`, `severity`, `body`).
6. **M5** — Return `ShutdownFunc` from `New`/`Init` to close file handles.

### Phase 3: Tracing improvements (breaking)

Fix H5, M3, M6, M7.

1. **H5** — Replace `grpc.DialContext` + `WithBlock` with `grpc.NewClient`. Add `WithTLSCredentials` option.
2. **M3** — Add `WithSampler(sdktrace.Sampler) Option`. Default to `ParentBased(AlwaysSample())`.
3. **M6** — Add `WithResource(*resource.Resource) Option`.
4. **M7** — Remove viper dependency. Read values directly from pflag `FlagSet.GetBool`/`GetString`.

### Phase 4: Metrics redesign (breaking)

Fix H7.

1. Accept `*prometheus.Registry` instead of using the global one.
2. Add `net/http` middleware (framework-agnostic).
3. Expose or remove `pathLabelFromContext`.
4. Replace `Summary` with `Histogram` for request/response sizes.
5. Consider migrating to OTel metrics SDK for consistency with the tracing module.
