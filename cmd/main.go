package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func initTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	r, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithAttributes(
			resource.Default().Attributes()...,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(r),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

func example(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return
	}
	fmt.Println(string(body))
	fmt.Fprintf(w, "ok")
}

func health(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "ok")
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tp, err := initTracer(ctx)
	if err != nil {
		fmt.Printf("failed to initialize tracer: %+v\n", err)
		os.Exit(1)
	}
	defer tp.Shutdown(ctx)

	http.Handle("/v1/example", otelhttp.NewHandler(http.HandlerFunc(example), "example"))
	http.Handle("/health", otelhttp.NewHandler(http.HandlerFunc(health), "health"))
	fmt.Println("server is listening on 8090")
	http.ListenAndServe(":8090", nil)
}
