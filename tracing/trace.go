// Package tracing supplies access to tracer providers.
//
// It uses the [otel](https://opentelemetry.io/docs/instrumentation/go/getting-started/) tracing library, connecting
// to an OpenTelemetry collector.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	instrumentationCodeProviderName = "github.com/els0r/telemetry/tracing"
)

type tracingConfig struct {
	exporter    sdktrace.SpanExporter
	sampler     sdktrace.Sampler
	resource    *resource.Resource
	credentials credentials.TransportCredentials
}

// Option allows to configure the tracing setup
type Option func(*tracingConfig) error

var (
	errorNoCollector    = errors.New("no collector endpoint set")
	errorNoSpanExporter = errors.New("no span exporter set")
)

// WithGRPCExporter sets up an exporter using a gRPC connection to the trace collector.
// The connection is established lazily — the application will not block during initialization
// if the collector is unreachable. Use WithTransportCredentials to configure TLS.
func WithGRPCExporter(_ context.Context, collectorEndpoint string) Option {
	return func(tc *tracingConfig) error {
		if collectorEndpoint == "" {
			return errorNoCollector
		}

		creds := tc.credentials
		if creds == nil {
			creds = insecure.NewCredentials()
		}

		// Use grpc.NewClient for lazy, non-blocking connection establishment.
		// The deprecated grpc.DialContext+WithBlock pattern was removed to avoid
		// blocking application startup when the collector is unreachable.
		conn, err := grpc.NewClient(collectorEndpoint,
			grpc.WithTransportCredentials(creds),
		)
		if err != nil {
			return fmt.Errorf("failed to create gRPC client for collector: %w", err)
		}

		traceExporter, err := otlptracegrpc.New(context.Background(), otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			return fmt.Errorf("failed to create gRPC trace exporter: %w", err)
		}
		tc.exporter = traceExporter
		return nil
	}
}

// WithTransportCredentials sets the transport credentials for gRPC connections.
// Must be called before WithGRPCExporter to take effect. If not set, insecure
// credentials are used by default.
func WithTransportCredentials(creds credentials.TransportCredentials) Option {
	return func(tc *tracingConfig) error {
		tc.credentials = creds
		return nil
	}
}

// WithStdoutTraceExporter sets the exporter to write to stdout. It's not recommended to
// use this exporter in production and only for testing and validation.
//
// A writer can be optionally provided. The default case is stdout.
func WithStdoutTraceExporter(prettyPrint bool, w ...io.Writer) Option { //revive:disable-line
	return func(tc *tracingConfig) error {
		var opts []stdouttrace.Option
		if len(w) > 0 {
			opts = append(opts, stdouttrace.WithWriter(w[0]))
		}
		if prettyPrint {
			opts = append(opts, stdouttrace.WithPrettyPrint())
		}
		exp, err := stdouttrace.New(opts...)
		if err != nil {
			return fmt.Errorf("failed to create stdout trace exporter: %w", err)
		}
		tc.exporter = exp
		return nil
	}
}

// WithSpanExporter sets the exporter
func WithSpanExporter(exporter sdktrace.SpanExporter) Option {
	return func(tc *tracingConfig) error {
		tc.exporter = exporter
		return nil
	}
}

// WithSampler sets the trace sampler. The default is ParentBased(AlwaysSample()),
// which respects the parent span's sampling decision and samples all root spans.
func WithSampler(sampler sdktrace.Sampler) Option {
	return func(tc *tracingConfig) error {
		tc.sampler = sampler
		return nil
	}
}

// WithResource sets the resource describing the entity producing telemetry.
// Use this to set service.name, service.version, deployment.environment, etc.
// If not set, resource.Default() is used which auto-detects from environment variables.
func WithResource(r *resource.Resource) Option {
	return func(tc *tracingConfig) error {
		tc.resource = r
		return nil
	}
}

// Init initializes the tracer provider. The function is meant to be called once upon
// program setup.
func Init(opts ...Option) (ShutdownFunc, error) {
	tracerProvider, err := NewTracerProvider(opts...)
	if err != nil {
		return noShutdown, fmt.Errorf("failed to create tracer provider: %w", err)
	}

	// it is imperative that this is set, since Start relies on the global tracer provider to be set.
	// Otherwise, trace propagation will not work
	otel.SetTracerProvider(tracerProvider)

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Shutdown will flush any remaining spans and shut down the exporter.
	return tracerProvider.Shutdown, nil
}

// NewTracerProvider creates a new tracer provider. The options can and should be used
// to supply a span exporter. There is no default set on purpose, since the choice of
// exporter highly depends on the environment the application is deployed in.
func NewTracerProvider(opts ...Option) (tp *sdktrace.TracerProvider, err error) {
	// apply options
	tracingCfg := &tracingConfig{}
	for _, opt := range opts {
		err = opt(tracingCfg)
		if err != nil {
			return nil, err
		}
	}
	exporter := tracingCfg.exporter
	if exporter == nil {
		return nil, errorNoSpanExporter
	}

	// default sampler: ParentBased(AlwaysSample) — respects parent sampling decisions
	sampler := tracingCfg.sampler
	if sampler == nil {
		sampler = sdktrace.ParentBased(sdktrace.AlwaysSample())
	}

	// default resource: auto-detect from environment
	res := tracingCfg.resource
	if res == nil {
		res = resource.Default()
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tp = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	return tp, nil
}

// Start gets the tracer provider and starts a span with the provided name and options. Additional observability attributes
// from the context are added to the span with trace.WithAttributes
func Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.GetTracerProvider().Tracer(instrumentationCodeProviderName)

	if ctx == nil {
		return tracer.Start(context.Background(), spanName, opts...)
	}

	return tracer.Start(ctx, spanName, opts...)
}

// Error records the error as an event and sets the span status to error
func Error(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
