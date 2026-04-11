// Package telemetry configures OpenTelemetry tracing with an OTLP exporter.
package telemetry

import (
	"context"
	"fmt"
	"os"

	otelpyroscope "github.com/grafana/otel-profiling-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Setup initializes OpenTelemetry tracing with an OTLP gRPC exporter when
// OTEL_EXPORTER_OTLP_ENDPOINT is set. When pyroscopeEnabled is true, the
// TracerProvider is wrapped with otelpyroscope to tag profiling samples with
// span IDs for trace-to-profile correlation. Returns a shutdown function.
func Setup(ctx context.Context, serviceName string, pyroscopeEnabled bool) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	r, err := resource.New(ctx,
		resource.WithAttributes(
			resource.Default().Attributes()...,
		),
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(r),
	)

	if pyroscopeEnabled {
		otel.SetTracerProvider(otelpyroscope.NewTracerProvider(tp))
	} else {
		otel.SetTracerProvider(tp)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
