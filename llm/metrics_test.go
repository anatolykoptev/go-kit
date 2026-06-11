package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/llm"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestChainMetrics_ServedCounter_PrimarySuccess: a primary 200 increments
// llm_chain_served_total{model="primary",position="0"} by exactly 1, and the
// attempt counter records one ok attempt.
func TestChainMetrics_ServedCounter_PrimarySuccess(t *testing.T) {
	srv := httptest.NewServer(okChatHandler(t, "ok"))
	defer srv.Close()

	reg := prometheus.NewRegistry()
	m := llm.NewChainMetrics(reg)

	eps := []llm.Endpoint{
		{URL: srv.URL, Key: "k", Model: "primary"},
		{URL: srv.URL, Key: "k", Model: "fallback"},
	}
	c := llm.NewClient("", "", "",
		llm.WithEndpoints(eps),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(m.EndpointObserver(eps)),
	)

	if _, err := c.Complete(context.Background(), "", "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	served := testutil.ToFloat64(m.ServedTotal().WithLabelValues("primary", "0"))
	if served != 1 {
		t.Errorf("served{primary,0} = %v, want 1", served)
	}
	if other := testutil.ToFloat64(m.ServedTotal().WithLabelValues("fallback", "1")); other != 0 {
		t.Errorf("served{fallback,1} = %v, want 0 (primary served)", other)
	}
	if att := testutil.ToFloat64(m.AttemptTotal().WithLabelValues("primary", "ok")); att != 1 {
		t.Errorf("attempt{primary,ok} = %v, want 1", att)
	}
}

// TestChainMetrics_ServedCounter_Fallback: a primary failure then a fallback 200
// increments served{fallback,1}; attempt records one primary error + one fallback ok.
func TestChainMetrics_ServedCounter_Fallback(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer dead.Close()
	ok := httptest.NewServer(okChatHandler(t, "from-fallback"))
	defer ok.Close()

	reg := prometheus.NewRegistry()
	m := llm.NewChainMetrics(reg)

	eps := []llm.Endpoint{
		{URL: dead.URL, Key: "k", Model: "primary"},
		{URL: ok.URL, Key: "k", Model: "fallback"},
	}
	c := llm.NewClient("", "", "",
		llm.WithEndpoints(eps),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(m.EndpointObserver(eps)),
	)

	if _, err := c.Complete(context.Background(), "", "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if served := testutil.ToFloat64(m.ServedTotal().WithLabelValues("fallback", "1")); served != 1 {
		t.Errorf("served{fallback,1} = %v, want 1", served)
	}
	if served := testutil.ToFloat64(m.ServedTotal().WithLabelValues("primary", "0")); served != 0 {
		t.Errorf("served{primary,0} = %v, want 0 (primary failed)", served)
	}
	if att := testutil.ToFloat64(m.AttemptTotal().WithLabelValues("primary", "error")); att != 1 {
		t.Errorf("attempt{primary,error} = %v, want 1", att)
	}
	if att := testutil.ToFloat64(m.AttemptTotal().WithLabelValues("fallback", "ok")); att != 1 {
		t.Errorf("attempt{fallback,ok} = %v, want 1", att)
	}
}

// TestChainMetrics_CooldownActiveGauge: the CooldownObserver wired via
// WithModelCooldownObserver drives llm_model_cooldown_active{model}: it reads 1
// on cooldown entry and 0 on recovery.
func TestChainMetrics_CooldownActiveGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := llm.NewChainMetrics(reg)

	dead := httptest.NewServer(quotaHandler(""))
	defer dead.Close()
	ok := httptest.NewServer(okChatHandler(t, "fallback"))
	defer ok.Close()

	// Synchronise on the gauge transition: the observer is async (P1 fires it in
	// its own goroutine). Re-derive the gauge value until it flips, bounded.
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "primary"},
			{URL: ok.URL, Key: "k", Model: "fallback"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
		llm.WithModelCooldownObserver(m.CooldownObserver()),
	)

	if _, err := c.Complete(context.Background(), "", "hi"); err != nil {
		t.Fatalf("call: %v", err)
	}

	// Wait (bounded) for the async cooldown-entry observer to set the gauge to 1.
	active := m.CooldownActive().WithLabelValues("primary")
	deadline := time.Now().Add(2 * time.Second)
	for testutil.ToFloat64(active) != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("cooldown_active{primary} never reached 1: got %v", testutil.ToFloat64(active))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestChainMetrics_UnknownModelPosition: a served model NOT present in the chain
// the observer was built with must still record (position "unknown"), never panic
// or silently drop — defends against a chain/observer mismatch at wire time.
func TestChainMetrics_UnknownModelPosition(t *testing.T) {
	srv := httptest.NewServer(okChatHandler(t, "ok"))
	defer srv.Close()

	reg := prometheus.NewRegistry()
	m := llm.NewChainMetrics(reg)

	// Observer built with a DIFFERENT chain than the client actually runs.
	wrongChain := []llm.Endpoint{{URL: srv.URL, Key: "k", Model: "other"}}
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: srv.URL, Key: "k", Model: "real"},
		}),
		llm.WithMaxRetries(1),
		llm.WithEndpointAttemptObserver(m.EndpointObserver(wrongChain)),
	)

	if _, err := c.Complete(context.Background(), "", "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if served := testutil.ToFloat64(m.ServedTotal().WithLabelValues("real", "unknown")); served != 1 {
		t.Errorf("served{real,unknown} = %v, want 1 (model not in observer chain)", served)
	}
}
