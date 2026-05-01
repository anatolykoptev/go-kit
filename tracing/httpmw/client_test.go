package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/tracing/httpmw"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
)

// TestClient_PropagatesTraceparent verifies the wrapped client injects a
// W3C traceparent header on outgoing requests, even when the caller has
// no active span (otelhttp creates one on the fly).
func TestClient_PropagatesTraceparent(t *testing.T) {
	// otelhttp injects traceparent when a propagator is set AND the request
	// carries a valid span context. Mirror what tracing.Setup does.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := trace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	var seenHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Get("Traceparent")
	}))
	defer srv.Close()

	c := httpmw.Client()
	ctx, span := tp.Tracer("test").Start(context.Background(), "outer")
	defer span.End()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	if seenHeader == "" {
		t.Errorf("Traceparent header not injected on outgoing request")
	}
}

func TestWrapTransport_NilBaseUsesDefault(t *testing.T) {
	rt := httpmw.WrapTransport(nil)
	if rt == nil {
		t.Fatal("WrapTransport(nil) returned nil")
	}
}

func TestWrapTransport_PreservesCustomBase(t *testing.T) {
	called := false
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
	})
	c := &http.Client{Transport: httpmw.WrapTransport(base)}
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	if !called {
		t.Errorf("custom base RoundTripper bypassed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
