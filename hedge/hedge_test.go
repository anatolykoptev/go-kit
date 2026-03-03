package hedge_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/hedge"
)

func TestDo_PrimaryFast(t *testing.T) {
	result, err := hedge.Do(context.Background(), 100*time.Millisecond, func(_ context.Context) (string, error) {
		return "fast", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fast" {
		t.Errorf("result = %q, want %q", result, "fast")
	}
}

func TestDo_HedgeSucceeds(t *testing.T) {
	var calls atomic.Int32

	result, err := hedge.Do(context.Background(), 20*time.Millisecond, func(_ context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			// Primary: slow.
			time.Sleep(200 * time.Millisecond)
			return "primary", nil
		}
		// Hedge: fast.
		return "hedge", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hedge" {
		t.Errorf("result = %q, want %q", result, "hedge")
	}
}

func TestDo_BothFail(t *testing.T) {
	errBoom := errors.New("boom")

	_, err := hedge.Do(context.Background(), 10*time.Millisecond, func(_ context.Context) (string, error) {
		return "", errBoom
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := hedge.Do(ctx, 100*time.Millisecond, func(ctx context.Context) (string, error) {
		return "ok", nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestDo_ZeroDelay(t *testing.T) {
	result, err := hedge.Do(context.Background(), 0, func(_ context.Context) (string, error) {
		return "direct", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "direct" {
		t.Errorf("result = %q, want %q", result, "direct")
	}
}

func TestDo_NegativeDelay(t *testing.T) {
	result, err := hedge.Do(context.Background(), -time.Second, func(_ context.Context) (string, error) {
		return "direct", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "direct" {
		t.Errorf("result = %q, want %q", result, "direct")
	}
}

func TestDo_PrimaryFailsBeforeDelay(t *testing.T) {
	errFail := errors.New("primary failed")

	_, err := hedge.Do(context.Background(), time.Second, func(_ context.Context) (string, error) {
		return "", errFail
	})
	if !errors.Is(err, errFail) {
		t.Errorf("err = %v, want %v", err, errFail)
	}
}

func TestDo_LoserGetsCancelled(t *testing.T) {
	var cancelled atomic.Bool
	var calls atomic.Int32

	_, _ = hedge.Do(context.Background(), 10*time.Millisecond, func(ctx context.Context) (string, error) {
		n := calls.Add(1)
		if n == 1 {
			// Primary: block until cancelled.
			<-ctx.Done()
			cancelled.Store(true)
			return "", ctx.Err()
		}
		// Hedge: succeed immediately.
		return "hedge", nil
	})

	time.Sleep(50 * time.Millisecond)
	if !cancelled.Load() {
		t.Error("loser goroutine should have been cancelled")
	}
}
