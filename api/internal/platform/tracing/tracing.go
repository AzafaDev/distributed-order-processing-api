package tracing

import (
	"context"
	"log/slog"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*trace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(cfg.OtelExporterEndpoint))
	if err != nil {
		return nil, err
	}

	resc, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(cfg.OtelServiceName)))
	if err != nil {
		return nil, err
	}

	sampler := trace.ParentBased(trace.TraceIDRatioBased(cfg.OtelSampleRatio))

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resc),
		trace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.Baggage{},
			propagation.TraceContext{},
		),
	)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(cause error) {
		log.Error("new tracing", "error", cause)
	}))

	return tp, nil
}
