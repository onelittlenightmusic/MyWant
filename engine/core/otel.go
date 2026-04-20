package mywant

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const otelInstrumentationName = "mywant"

var (
	globalOTELMu      sync.RWMutex
	globalOTELEnabled bool
	globalTracer      trace.Tracer
	globalOTELLogger  otellog.Logger
)

// OTELShutdownFunc is returned by InitOTEL and must be called on server shutdown.
type OTELShutdownFunc func(context.Context) error

// InitOTEL initialises a TracerProvider and LoggerProvider, both exporting via
// OTLP/gRPC to endpoint (e.g. "localhost:4317").
// If endpoint is empty the function checks the OTEL_EXPORTER_OTLP_ENDPOINT env var.
// Returns a no-op if neither is set.
func InitOTEL(ctx context.Context, endpoint string) (OTELShutdownFunc, error) {
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if endpoint == "" {
		// No endpoint configured — disable silently.
		return func(context.Context) error { return nil }, nil
	}

	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: grpc dial %s: %w", endpoint, err)
	}

	// ── Shared resource (service.name label used by Tempo and Loki) ─────────
	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			semconv.ServiceName("mywant"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: resource: %w", err)
	}

	// ── Trace provider ──────────────────────────────────────────────────────
	traceExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("otel: trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// ── Log provider ────────────────────────────────────────────────────────
	logExp, err := otlploggrpc.New(ctx, otlploggrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("otel: log exporter: %w", err)
	}
	lp := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExp)),
		log.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	// Store references for use inside Want
	globalOTELMu.Lock()
	globalOTELEnabled = true
	globalTracer = tp.Tracer(otelInstrumentationName)
	globalOTELLogger = lp.Logger(otelInstrumentationName)
	globalOTELMu.Unlock()

	InfoLog("[OTEL] Initialised — exporting to %s", endpoint)

	shutdown := func(ctx context.Context) error {
		var errs []error
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("trace provider shutdown: %w", err))
		}
		if err := lp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("log provider shutdown: %w", err))
		}
		if len(errs) > 0 {
			return fmt.Errorf("otel shutdown errors: %v", errs)
		}
		return nil
	}
	return shutdown, nil
}

// IsOTELEnabled returns true when an OTLP endpoint has been configured.
func IsOTELEnabled() bool {
	globalOTELMu.RLock()
	defer globalOTELMu.RUnlock()
	return globalOTELEnabled
}

// otelTracer returns the global tracer (nil-safe).
func otelTracer() trace.Tracer {
	globalOTELMu.RLock()
	defer globalOTELMu.RUnlock()
	return globalTracer
}

// otelEmitLog emits a single log record via the OTEL LoggerProvider.
// attrs is a flat list of key, value pairs (both string).
func otelEmitLog(ctx context.Context, severity otellog.Severity, body string, attrs ...otellog.KeyValue) {
	globalOTELMu.RLock()
	logger := globalOTELLogger
	enabled := globalOTELEnabled
	globalOTELMu.RUnlock()

	if !enabled || logger == nil {
		return
	}

	var r otellog.Record
	r.SetTimestamp(time.Now())
	r.SetSeverity(severity)
	r.SetBody(otellog.StringValue(body))
	r.AddAttributes(attrs...)
	logger.Emit(ctx, r)
}
