package slogh_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/tracing/slogh"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// withRecorder spins up a TracerProvider that captures spans in memory
// — used to assert recordToSpanEvent populated the active span.
func withRecorder(t *testing.T) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	ctx, span := tp.Tracer("test").Start(context.Background(), "outer")
	t.Cleanup(func() {
		span.End()
		_ = tp.Shutdown(context.Background())
	})
	return ctx, rec
}

func TestHandle_NoSpan_PassesThroughUnchanged(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(slogh.NewHandler(base))

	logger.Info("hello", "k", "v")

	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Errorf("base record lost: %s", out)
	}
	if strings.Contains(out, "trace_id") {
		t.Errorf("trace_id injected without an active span: %s", out)
	}
}

func TestHandle_WithSpan_InjectsTraceID(t *testing.T) {
	ctx, _ := withRecorder(t)
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(slogh.NewHandler(base))

	logger.InfoContext(ctx, "with span", "k", "v")

	out := buf.String()
	if !strings.Contains(out, `"trace_id":"`) {
		t.Errorf("trace_id missing: %s", out)
	}
	if !strings.Contains(out, `"span_id":"`) {
		t.Errorf("span_id missing: %s", out)
	}
}

func TestHandle_ErrorLevel_AddsSpanEvent(t *testing.T) {
	ctx, rec := withRecorder(t)
	base := slog.NewJSONHandler(&bytes.Buffer{}, nil)
	logger := slog.New(slogh.NewHandler(base))

	logger.ErrorContext(ctx, "db failed", "err", errors.New("boom").Error())

	// Force flush
	spans := rec.Ended()
	// Active span hasn't ended yet — events live on the in-progress span.
	// We can inspect via OnStart/OnEnd recorder; tracetest captures both.
	started := rec.Started()
	if len(started) == 0 {
		t.Fatal("no spans started")
	}
	var found bool
	for _, s := range started {
		for _, e := range s.Events() {
			if e.Name == "db failed" {
				found = true
			}
		}
	}
	_ = spans
	if !found {
		t.Errorf("ERROR record did not become a span event")
	}
}

func TestEnabled_DelegatesToBase(t *testing.T) {
	base := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := slogh.NewHandler(base)
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("INFO should be filtered out by base level=WARN")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("ERROR should pass base level=WARN")
	}
}

func TestNilBase_NoOp(t *testing.T) {
	// Defensive: nil base handler must not panic, drops everything silently.
	h := slogh.NewHandler(nil)
	if h.Enabled(context.Background(), slog.LevelError) {
		t.Error("nil base must report disabled")
	}
	_ = h.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelError, "x", 0))
}
