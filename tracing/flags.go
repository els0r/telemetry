package tracing

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/els0r/telemetry/tracing/internal/flagutil"
	flag "github.com/spf13/pflag"
)

const (
	otelGRPC = "otel-grpc"
	stdout   = "stdout"
)

var supportedCollectors = []string{
	otelGRPC,
	stdout,
}

// Tracing flag names. The '.' is the hierarchy delimiter.
const (
	TracingKey = "tracing"

	TracingEnabledArg  string = TracingKey + ".enabled"
	TracingEnabledHelp string = "enable tracing"

	TracingCollectorKey string = TracingKey + ".collector"

	TracingCollectorTypeArg string = TracingCollectorKey + ".type"
	TracingCollectorTypeHelp string = "tracing collector type"

	TracingCollectorEndpointArg  string = TracingCollectorKey + ".endpoint"
	TracingCollectorEndpointHelp string = "endpoint collecting traces"
)

type traceFlagsConfig struct {
	enabled           bool
	collectorType     string
	collectorEndpoint string
}

func (t traceFlagsConfig) options(ctx context.Context) (opts []Option) {
	if !t.enabled {
		return opts
	}

	switch t.collectorType {
	case stdout:
		opts = append(opts, WithStdoutTraceExporter(true))
	case otelGRPC:
		opts = append(opts, WithGRPCExporter(ctx, t.collectorEndpoint))
	default:
		return opts
	}
	return opts
}

var (
	registeredFlags *flag.FlagSet
	registered      bool
	registration    = sync.Once{}
)

// RegisterFlags registers CLI arguments into flags. Registration of the flags is done exactly once.
func RegisterFlags(flags *flag.FlagSet) {
	registration.Do(func() {
		registered = true
		registeredFlags = flags

		flags.Bool(TracingEnabledArg, false, TracingEnabledHelp)
		flags.String(TracingCollectorTypeArg, "", flagutil.WithSupported(TracingCollectorTypeHelp, supportedCollectors))
		flags.String(TracingCollectorEndpointArg, "", TracingCollectorEndpointHelp)
	})
}

// ShutdownFunc is a function that can be used to shutdown the tracing collector
type ShutdownFunc func(context.Context) error

var noShutdown ShutdownFunc = func(context.Context) error { return nil }

// InitFromFlags initializes tracing from the set of registered flags. Replaces the Init method.
func InitFromFlags(ctx context.Context) (ShutdownFunc, error) {
	if !registered || registeredFlags == nil {
		fmt.Fprintf(os.Stderr, "CLI flags have not been registered. Use RegisterFlags in your command definition. Defaulting to no tracing\n") //revive:disable-line
		return noShutdown, nil
	}

	enabled, _ := registeredFlags.GetBool(TracingEnabledArg)
	collectorType, _ := registeredFlags.GetString(TracingCollectorTypeArg)
	collectorEndpoint, _ := registeredFlags.GetString(TracingCollectorEndpointArg)

	traceFlags := traceFlagsConfig{
		enabled:           enabled,
		collectorType:     collectorType,
		collectorEndpoint: collectorEndpoint,
	}

	opts := traceFlags.options(ctx)
	if len(opts) == 0 {
		return noShutdown, nil
	}

	return Init(opts...)
}
