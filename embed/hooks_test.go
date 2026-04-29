package embed

import (
	"context"
	"testing"
	"time"
)

// TestNoopObserver_AllMethodsNoop verifies that noopObserver implements
// Observer and all methods can be called without panicking or side effects.
func TestNoopObserver_AllMethodsNoop(t *testing.T) {
	var obs Observer = noopObserver{}
	ctx := context.Background()

	// All 6 callbacks must not panic.
	obs.OnBeforeEmbed(ctx, "model", 5)
	obs.OnAfterEmbed(ctx, StatusOk, time.Millisecond, 5)
	obs.OnRetry(ctx, 1, nil)
	obs.OnCircuitTransition(ctx, CircuitClosed, CircuitOpen)
	obs.OnCacheHit(ctx, 3)
	obs.OnTruncate(ctx, 0, 512, 256)
}

// TestSafeCall_RecoversFromPanic verifies that a panicking observer callback
// does not propagate the panic to the caller.
func TestSafeCall_RecoversFromPanic(t *testing.T) {
	panicked := false
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("safeCall: panic escaped: %v", r)
		}
	}()

	safeCall(func() {
		panicked = true
		panic("observer panic")
	})

	if !panicked {
		t.Error("expected inner function to run before panic")
	}
}

// TestSafeCall_NormalFunctionRuns verifies that safeCall executes the function
// when no panic occurs.
func TestSafeCall_NormalFunctionRuns(t *testing.T) {
	ran := false
	safeCall(func() { ran = true })
	if !ran {
		t.Error("expected function to run")
	}
}

// countingObserver counts how many times each callback is invoked.
type countingObserver struct {
	beforeEmbed       int
	afterEmbed        int
	retry             int
	circuitTransition int
	cacheHit          int
	truncate          int
}

func (o *countingObserver) OnBeforeEmbed(_ context.Context, _ string, _ int) { o.beforeEmbed++ }
func (o *countingObserver) OnAfterEmbed(_ context.Context, _ Status, _ time.Duration, _ int) {
	o.afterEmbed++
}
func (o *countingObserver) OnRetry(_ context.Context, _ int, _ error) { o.retry++ }
func (o *countingObserver) OnCircuitTransition(_ context.Context, _, _ CircuitState) {
	o.circuitTransition++
}
func (o *countingObserver) OnCacheHit(_ context.Context, _ int)           { o.cacheHit++ }
func (o *countingObserver) OnTruncate(_ context.Context, _ int, _, _ int) { o.truncate++ }

// TestObserver_AllSixCallbacksFireable verifies that an instrumented observer
// correctly counts each of the 6 callbacks when invoked manually.
func TestObserver_AllSixCallbacksFireable(t *testing.T) {
	obs := &countingObserver{}
	ctx := context.Background()

	safeCall(func() { obs.OnBeforeEmbed(ctx, "model", 2) })
	safeCall(func() { obs.OnAfterEmbed(ctx, StatusOk, time.Millisecond, 2) })
	safeCall(func() { obs.OnRetry(ctx, 1, nil) })
	safeCall(func() { obs.OnCircuitTransition(ctx, CircuitClosed, CircuitOpen) })
	safeCall(func() { obs.OnCacheHit(ctx, 2) })
	safeCall(func() { obs.OnTruncate(ctx, 0, 512, 256) })

	if obs.beforeEmbed != 1 {
		t.Errorf("OnBeforeEmbed: want 1, got %d", obs.beforeEmbed)
	}
	if obs.afterEmbed != 1 {
		t.Errorf("OnAfterEmbed: want 1, got %d", obs.afterEmbed)
	}
	if obs.retry != 1 {
		t.Errorf("OnRetry: want 1, got %d", obs.retry)
	}
	if obs.circuitTransition != 1 {
		t.Errorf("OnCircuitTransition: want 1, got %d", obs.circuitTransition)
	}
	if obs.cacheHit != 1 {
		t.Errorf("OnCacheHit: want 1, got %d", obs.cacheHit)
	}
	if obs.truncate != 1 {
		t.Errorf("OnTruncate: want 1, got %d", obs.truncate)
	}
}

// TestCircuitState_String verifies all 3 defined values + unknown.
func TestCircuitState_String(t *testing.T) {
	cases := []struct {
		s    CircuitState
		want string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}
