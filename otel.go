package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// setupOTEL bootstraps the OpenTelemetry pipeline.
// If it returns an error, the pipeline is not set up.
// It returns a shutdown function that should be called when the application exits.
func setupOTEL(ctx context.Context) (shutdown func(context.Context) error, prop propagation.TextMapPropagator, err error) {
	if os.Getenv("ENABLE_OTEL") != "true" {
		return func(context.Context) error { return nil }, nil, nil
	}

	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = fn(ctx) // Simple error handling, in prod maybe join errors
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = fmt.Errorf("failed to setup OTEL: %w", inErr)
		shutdown(ctx)
	}

	// Set up propagator.
	prop = newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTraceProvider(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Instrument the default transport so that all clients (including libdns/cloudflare) are instrumented
	oldTransport := http.DefaultTransport
	http.DefaultTransport = otelhttp.NewTransport(oldTransport)
	shutdownFuncs = append(shutdownFuncs, func(context.Context) error {
		http.DefaultTransport = oldTransport
		return nil
	})

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider(ctx context.Context) (*trace.TracerProvider, error) {
	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.NewWithAttributes(
			"",
			semconv.ServiceName("slogtest"), // Fallback service name
		),
		resource.Default(), // Contains OTEL_SERVICE_NAME if set
	)
	if err != nil {
		return nil, err
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	)
	return traceProvider, nil
}
