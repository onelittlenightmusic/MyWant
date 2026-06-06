//go:build ios

package mywant

import (
	"context"

	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
)

// iOS stub: OTEL/gRPC exporting is disabled on iOS.

type OTELShutdownFunc func(context.Context) error

func InitOTEL(_ context.Context, _ string) (OTELShutdownFunc, error) {
	return func(context.Context) error { return nil }, nil
}

func IsOTELEnabled() bool { return false }

func otelTracer() trace.Tracer { return nil }

func (n *Want) otelOnStatusChange(_, _ WantStatus)           {}
func (n *Want) otelEmitWantInfo(_ string)                    {}
func (n *Want) otelEmitWantLog(_ otellog.Severity, _ string) {}
func (n *Want) otelEmitStateChange(_ string, _ any)          {}

func otelEmitLog(_ context.Context, _ otellog.Severity, _ string, _ ...otellog.KeyValue) {}
