package hedge

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// Primary returns a value before the delay → fallback never starts.
func TestDoFallback_PrimaryFast(t *testing.T) {
	var fallbackCalls int32
	primary := func(_ context.Context) (string, error) {
		return "primary-ok", nil
	}
	fallback := func(_ context.Context) (string, error) {
		atomic.AddInt32(&fallbackCalls, 1)
		return "fallback-ok", nil
	}
	got, err := DoFallback(context.Background(), 50*time.Millisecond, primary, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "primary-ok" {
		t.Errorf("got %q, want primary-ok", got)
	}
	// Give any (incorrect) fallback launch time to fire.
	time.Sleep(80 * time.Millisecond)
	if c := atomic.LoadInt32(&fallbackCalls); c != 0 {
		t.Errorf("fallback called %d times; want 0", c)
	}
}

// Primary slow → after delay, fallback starts; fallback returns first → win.
func TestDoFallback_PrimarySlow_FallbackWins(t *testing.T) {
	primary := func(ctx context.Context) (string, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return "primary-late", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	fallback := func(_ context.Context) (string, error) {
		return "fallback-ok", nil
	}
	got, err := DoFallback(context.Background(), 30*time.Millisecond, primary, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback-ok" {
		t.Errorf("got %q, want fallback-ok", got)
	}
}

// Primary fails BEFORE delay → fallback starts immediately (this is the
// behaviour difference from Do, where re-running the same fn doesn't
// help on early failure).
func TestDoFallback_PrimaryFastFail_FallbackImmediate(t *testing.T) {
	var fallbackStartedAt time.Time
	primary := func(_ context.Context) (string, error) {
		return "", errors.New("primary boom")
	}
	fallback := func(_ context.Context) (string, error) {
		fallbackStartedAt = time.Now()
		return "fallback-ok", nil
	}
	start := time.Now()
	got, err := DoFallback(context.Background(), 500*time.Millisecond, primary, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback-ok" {
		t.Errorf("got %q, want fallback-ok", got)
	}
	// Fallback must start without waiting the full 500ms delay; allow
	// 100ms slack for scheduling.
	delta := fallbackStartedAt.Sub(start)
	if delta > 100*time.Millisecond {
		t.Errorf("fallback started after %v; expected <100ms (immediate)", delta)
	}
}

// Both fail → primary error returned (more diagnostic).
func TestDoFallback_BothFail_ReturnsPrimaryErr(t *testing.T) {
	primaryErr := errors.New("primary boom")
	fallbackErr := errors.New("fallback boom")
	primary := func(_ context.Context) (string, error) {
		return "", primaryErr
	}
	fallback := func(_ context.Context) (string, error) {
		return "", fallbackErr
	}
	_, err := DoFallback(context.Background(), 30*time.Millisecond, primary, fallback)
	if !errors.Is(err, primaryErr) {
		t.Errorf("got error %v; want primaryErr (%v)", err, primaryErr)
	}
}

// Primary slow + fails AFTER fallback has succeeded → fallback wins.
func TestDoFallback_PrimarySlowFail_FallbackSucceeds(t *testing.T) {
	primary := func(ctx context.Context) (string, error) {
		select {
		case <-time.After(150 * time.Millisecond):
			return "", errors.New("primary boom")
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	fallback := func(_ context.Context) (string, error) {
		return "fallback-ok", nil
	}
	got, err := DoFallback(context.Background(), 30*time.Millisecond, primary, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback-ok" {
		t.Errorf("got %q, want fallback-ok", got)
	}
}

// Context cancellation during execution surfaces ctx error.
func TestDoFallback_ContextCancelled(t *testing.T) {
	primary := func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	fallback := func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := DoFallback(ctx, 10*time.Millisecond, primary, fallback)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got error %v; want DeadlineExceeded", err)
	}
}

// delay <= 0 → primary called once, fallback never runs.
func TestDoFallback_ZeroDelay(t *testing.T) {
	var fallbackCalls int32
	primary := func(_ context.Context) (string, error) { return "primary", nil }
	fallback := func(_ context.Context) (string, error) {
		atomic.AddInt32(&fallbackCalls, 1)
		return "fallback", nil
	}
	got, err := DoFallback(context.Background(), 0, primary, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "primary" {
		t.Errorf("got %q, want primary", got)
	}
	if c := atomic.LoadInt32(&fallbackCalls); c != 0 {
		t.Errorf("fallback called %d times with zero delay; want 0", c)
	}
}

// Cancellation during the post-primary-fail wait for fallback.
func TestDoFallback_PrimaryFastFail_CtxCancelDuringFallback(t *testing.T) {
	primary := func(_ context.Context) (string, error) {
		return "", errors.New("primary boom")
	}
	fallback := func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := DoFallback(ctx, 200*time.Millisecond, primary, fallback)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got error %v; want DeadlineExceeded", err)
	}
}
