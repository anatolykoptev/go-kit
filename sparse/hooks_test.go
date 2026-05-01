package sparse

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestNoopObserver_AllNoOp verifies the default observer's methods
// don't panic on any input. They are called from inside safeCall so a
// panic would be swallowed, but we want to confirm the noop itself is
// a clean no-op.
func TestNoopObserver_AllNoOp(t *testing.T) {
	var o noopObserver
	ctx := context.Background()
	o.OnBeforeEmbed(ctx, "m", 1)
	o.OnAfterEmbed(ctx, StatusOk, time.Millisecond, 1)
	o.OnRetry(ctx, 1, errors.New("x"))
	o.OnCircuitTransition(ctx, CircuitClosed, CircuitOpen)
	o.OnCacheHit(ctx, 1)
}

// TestSafeCall_RecoversPanic verifies safeCall swallows any panic.
// A panic in user observer code must not kill an in-flight request.
func TestSafeCall_RecoversPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic escaped safeCall: %v", r)
		}
	}()
	safeCall(func() { panic("boom") })
}

// TestSafeCall_RunsFn verifies safeCall actually invokes the function
// when no panic occurs.
func TestSafeCall_RunsFn(t *testing.T) {
	called := false
	safeCall(func() { called = true })
	if !called {
		t.Errorf("safeCall did not invoke fn")
	}
}

// TestCircuitState_String verifies the labels are stable Prometheus values.
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
			t.Errorf("CircuitState(%d): want %q, got %q", tc.s, tc.want, got)
		}
	}
}
