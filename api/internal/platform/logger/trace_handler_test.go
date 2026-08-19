package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(NewTraceHandler(slog.NewJSONHandler(buf, nil)))
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("log bukan JSON valid: %v", err)
	}
	return out
}

func TestTraceHandlerAddsTraceIDInsideSpan(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	tp := trace.NewTracerProvider()
	ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	log.ErrorContext(ctx, "boom")

	fields := decode(t, &buf)
	got, ok := fields["trace_id"].(string)
	if !ok {
		t.Fatalf("trace_id tidak ada di log: %v", fields)
	}
	if want := span.SpanContext().TraceID().String(); got != want {
		t.Errorf("trace_id = %q, mau %q", got, want)
	}
	if fields["span_id"] != span.SpanContext().SpanID().String() {
		t.Errorf("span_id tidak cocok: %v", fields["span_id"])
	}
}

func TestTraceHandlerOutsideSpan(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(&buf).Info("startup")

	fields := decode(t, &buf)
	if _, ok := fields["trace_id"]; ok {
		t.Errorf("log di luar request tidak boleh punya trace_id: %v", fields)
	}
	if fields["msg"] != "startup" {
		t.Errorf("msg hilang: %v", fields)
	}
}

func TestTraceHandlerPreservesWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(&buf).With("service", "orders").Info("hello")

	if fields := decode(t, &buf); fields["service"] != "orders" {
		t.Errorf("atribut dari With() hilang: %v", fields)
	}
}
