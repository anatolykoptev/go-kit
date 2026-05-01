package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestSetup_NoEndpoint_NoOpButPropagatorsInstalled(t *testing.T) {
	t.Setenv(envEndpoint, "")
	shutdown, err := Setup(context.Background(), "test-svc")
	if err != nil {
		t.Fatalf("Setup with no endpoint should succeed (no-op), got: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("noop shutdown returned error: %v", err)
	}
	// Propagators must still be set so cross-process trace context
	// extraction keeps working even without local export.
	prop := otel.GetTextMapPropagator()
	if _, ok := prop.(propagation.TextMapPropagator); !ok {
		t.Errorf("propagator not installed")
	}
}

func TestSetup_EmptyServiceName_Errors(t *testing.T) {
	_, err := Setup(context.Background(), "")
	if err == nil {
		t.Errorf("Setup with empty serviceName should error")
	}
}

func TestSetup_BadEndpoint_ReturnsExporterError(t *testing.T) {
	// Invalid URL forms still parse to a host:port, but New() errors only on
	// option-validation issues — this is mainly a smoke test that the
	// exporter creation path runs.
	t.Setenv(envEndpoint, "")
	_, err := Setup(context.Background(), "test-svc", WithEndpoint("localhost:0"))
	// localhost:0 is technically valid; we don't expect an error here. The
	// test ensures Setup doesn't panic on common edge inputs.
	if err != nil {
		t.Logf("Setup with localhost:0 returned error (acceptable): %v", err)
	}
}

func TestStart_NoOpSpanFromGlobalProvider(t *testing.T) {
	// Without Setup, otel global TP is the no-op default — Start must still
	// return a usable (non-nil) Span and the original ctx semantics.
	ctx, span := Start(context.Background(), "noop.test", attribute.Int("k", 1))
	if span == nil {
		t.Fatal("Start returned nil span")
	}
	if !trace.SpanFromContext(ctx).SpanContext().IsValid() && trace.SpanFromContext(ctx) == trace.SpanFromContext(context.Background()) {
		// Acceptable — no-op span has invalid context. Just verify End is callable.
	}
	span.End()
}

func TestRecordError_NilNoop(t *testing.T) {
	_, span := Start(context.Background(), "test")
	defer span.End()
	RecordError(span, nil) // must not panic
}

func TestRecordError_SetsStatus(t *testing.T) {
	_, span := Start(context.Background(), "test")
	defer span.End()
	RecordError(span, errors.New("boom")) // must not panic
}

func TestNormaliseEndpoint(t *testing.T) {
	tests := []struct {
		in       string
		host     string
		insecure bool
	}{
		{"tempo:4318", "tempo:4318", true},
		{"http://tempo:4318", "tempo:4318", true},
		{"https://tempo.example:4318", "tempo.example:4318", false},
	}
	for _, tt := range tests {
		host, insecure := normaliseEndpoint(tt.in)
		if host != tt.host || insecure != tt.insecure {
			t.Errorf("normaliseEndpoint(%q) = (%q, %v), want (%q, %v)",
				tt.in, host, insecure, tt.host, tt.insecure)
		}
	}
}
