//go:build !ios

package mywant

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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

// otelOnStatusChange manages the OTEL lifecycle span for a want.
//   - First active status (idle → anything) → start a new span
//   - Every status change → add a span event
//   - Terminal status → end the span
func (n *Want) otelOnStatusChange(oldStatus, newStatus WantStatus) {
	if !IsOTELEnabled() {
		return
	}

	terminalStatuses := map[WantStatus]bool{
		WantStatusAchieved:            true,
		WantStatusAchievedWithWarning: true,
		WantStatusFailed:              true,
		WantStatusCancelled:           true,
		WantStatusTerminated:          true,
	}

	if n.otelSpan == nil && oldStatus == WantStatusIdle {
		tracer := otelTracer()
		if tracer != nil {
			ctx, span := tracer.Start(context.Background(), "want/"+n.Metadata.Name,
				trace.WithAttributes(
					attribute.String("want.name", n.Metadata.Name),
					attribute.String("want.type", n.Metadata.Type),
					attribute.String("want.id", n.Metadata.ID),
				),
			)
			n.otelSpan = span
			n.otelSpanCtx = ctx
		}
	}

	if n.otelSpan == nil {
		return
	}

	n.otelSpan.AddEvent("status_change", trace.WithAttributes(
		attribute.String("want.status.old", string(oldStatus)),
		attribute.String("want.status.new", string(newStatus)),
	))

	if terminalStatuses[newStatus] {
		n.otelSpan.End()
		n.otelSpan = nil
		n.otelSpanCtx = nil
	}
}

// otelEmitWantInfo emits an Info-level log record with the want's identity attributes.
// Use this from non-otel files to avoid importing otellog severity constants.
func (n *Want) otelEmitWantInfo(body string) {
	n.otelEmitWantLog(otellog.SeverityInfo, body)
}

// otelEmitWantLog emits a log record with the want's identity attributes attached.
func (n *Want) otelEmitWantLog(severity otellog.Severity, body string) {
	ctx := n.otelSpanCtx
	if ctx == nil {
		ctx = context.Background()
	}
	otelEmitLog(ctx, severity, body,
		otellog.String("want.name", n.Metadata.Name),
		otellog.String("want.type", n.Metadata.Type),
		otellog.String("want.status", string(n.Status)),
	)
}

// otelEmitStateChange records a single state key/value pair as a span event (traces)
// and as a JSON log line (Loki), so Grafana can filter and unwrap any field by name.
func (n *Want) otelEmitStateChange(key string, value any) {
	if !IsOTELEnabled() {
		return
	}
	if n.otelSpan != nil {
		valStr := fmt.Sprintf("%v", value)
		if len(valStr) > 256 {
			valStr = valStr[:256] + "…"
		}
		n.otelSpan.AddEvent("state_change", trace.WithAttributes(
			attribute.String("state.key", key),
			attribute.String("state.value", valStr),
		))
	}
	valJSON, err := json.Marshal(value)
	var body string
	if err != nil || len(valJSON) > 512 {
		body = fmt.Sprintf(`{"event":"state","key":%q,"value":%q}`, key, fmt.Sprintf("%v", value))
	} else {
		body = fmt.Sprintf(`{"event":"state","key":%q,"value":%s}`, key, string(valJSON))
	}
	n.otelEmitWantLog(otellog.SeverityDebug, body)
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
