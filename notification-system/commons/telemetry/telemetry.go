// Package telemetry sets up tracing and metrics for the services.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// SetupTracing makes trace context travel with messages and, if endpoint is not empty,
// exports spans to the collector at that base URL over OTLP/HTTP under the given service
// name. Without an endpoint spans are created but not recorded.
//
// The returned function flushes pending spans; call it before the program exits.
func SetupTracing(ctx context.Context, service, endpoint string) (shutdown func(context.Context) error, err error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	// Traces go to /v1/traces under the collector's base URL.
	tracesURL, err := url.JoinPath(endpoint, "v1", "traces")
	if err != nil {
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT: %v", err)
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(tracesURL))
	if err != nil {
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT: %v", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(semconv.ServiceName(service))),
	)
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

// NewRegistry returns a metrics registry that already reports on the Go runtime and
// the process.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
}

// MetricsRouter returns a handler that serves the metrics of reg at /metrics.
func MetricsRouter(reg *prometheus.Registry) http.Handler {
	r := chi.NewRouter()
	r.Method(http.MethodGet, "/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	return r
}
