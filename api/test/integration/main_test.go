//go:build integration

package integration

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/tracing"
)

// TestMain wires tracing for the whole integration suite.
//
// Why this exists: setupTestEnv builds the app through server.BuildApp, which
// never calls tracing.New -- that only happens in cmd/api/main.go. Without a
// TracerProvider registered, OTel silently falls back to a no-op
// implementation: every tracer.Start still returns a span, nothing errors, and
// not a single span ever reaches Jaeger. Tests would pass and the trace view
// would stay empty.
//
// It is opt-in via OTEL_EXPORTER_OTLP_ENDPOINT so that plain `go test` in CI
// does not need a collector running.
func TestMain(m *testing.M) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		os.Exit(m.Run())
	}

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	tp, err := tracing.New(ctx, &config.Config{
		OtelExporterEndpoint: endpoint,
		OtelServiceName:      envOr("OTEL_SERVICE_NAME", "order-api-integration"),
		// Always sample: the point of the run is to inspect every trace.
		OtelSampleRatio: 1.0,
	}, log)
	if err != nil {
		log.Error("integration tracing setup failed", "error", err)
		os.Exit(1)
	}

	code := m.Run()

	// Critical: the SDK batches spans and flushes on a timer. A test binary
	// exits far sooner than that, so without an explicit Shutdown the last
	// batch is dropped and the traces you actually care about never arrive.
	if err := tp.Shutdown(ctx); err != nil {
		log.Error("integration tracing shutdown failed", "error", err)
	}

	os.Exit(code)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
