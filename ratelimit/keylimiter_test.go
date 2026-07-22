package ratelimit

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// The sliding-window tests (TestSlidingWindow_FailOpenOnRedisError,
	// TestSlidingWindow_FailClosedOnRedisError) dial an unreachable Redis
	// address. go-redis v9's CircuitBreakerManager spawns a background
	// cleanupLoop goroutine per client that lingers after the test; ignore
	// it so goleak focuses on OUR goroutine leaks.
	goleak.VerifyTestMain(m,
		goleak.IgnoreAnyFunction("github.com/redis/go-redis/v9/maintnotifications.(*CircuitBreakerManager).cleanupLoop"),
	)
}

// TestKeyLimiterGoroutineLeakOnNoClose verifies that a KeyLimiter whose
// Close() is never explicitly called does not leak its background cleanup
// goroutine. NewKeyLimiter registers a runtime.SetFinalizer that calls
// Close() when the KeyLimiter is garbage-collected, stopping the goroutine.
// Without the finalizer (and the weak-pointer cleanup goroutine that does
// not hold a strong reference to the KeyLimiter), the goroutine would leak
// for the process lifetime.
func TestKeyLimiterGoroutineLeakOnNoClose(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Create a KeyLimiter in a nested scope so it becomes unreachable as
	// soon as the scope exits. The finalizer registered by NewKeyLimiter
	// must call Close() to stop the background cleanup goroutine.
	func() {
		kl := NewKeyLimiter(10, 5)
		kl.StartCleanup(time.Second, time.Minute)
		_ = kl
	}()

	// Finalizers run asynchronously in a dedicated goroutine after GC, so
	// we poll: trigger GC repeatedly until the finalizer has run and the
	// cleanup goroutine has exited.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
		if err := goleak.Find(); err == nil {
			return
		}
	}
	// Final deferred VerifyNone will surface the failure detail.
}

// TestKeyLimiterUnboundedGrowth verifies that Allow() grows the limiters map
// by exactly one entry per unique key when no cleanup is running. This
// documents the unbounded-growth behavior and confirms Len() reflects the
// true count. The mitigation for unbounded growth is WithAutoCleanup /
// StartCleanup (tested below), which reaps idle entries.
func TestKeyLimiterUnboundedGrowth(t *testing.T) {
	defer goleak.VerifyNone(t)

	kl := NewKeyLimiter(10, 5)
	// No StartCleanup, no Close — the finalizer will reap the goroutine-less
	// KeyLimiter on GC (there is no goroutine to leak here).
	for i := range 10000 {
		kl.Allow(fmt.Sprintf("key-%d", i))
	}
	if got := kl.Len(); got != 10000 {
		t.Fatalf("Len = %d, want 10000", got)
	}
	kl = nil
	runtime.GC()
	runtime.GC()
}

// TestKeyLimiterAutoCleanupBoundsGrowth verifies that the WithAutoCleanup
// option starts a background cleanup goroutine that reaps idle keys, bounding
// map growth for high-cardinality transient-key workloads.
func TestKeyLimiterAutoCleanupBoundsGrowth(t *testing.T) {
	defer goleak.VerifyNone(t)

	now := time.Now()
	kl := NewKeyLimiter(10, 5, WithAutoCleanup(10*time.Millisecond, time.Millisecond))
	kl.now = func() time.Time { return now }
	defer kl.Close()

	// Add many keys.
	for i := range 1000 {
		kl.Allow(fmt.Sprintf("key-%d", i))
	}
	if got := kl.Len(); got != 1000 {
		t.Fatalf("Len before cleanup = %d, want 1000", got)
	}

	// Advance time past the idle threshold and wait for the auto-cleanup
	// goroutine to reap all idle entries.
	kl.now = func() time.Time { return now.Add(time.Hour) }
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if kl.Len() == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Len = %d after auto-cleanup, want 0 (idle keys not reaped)", kl.Len())
}

// TestKeyLimiterFinalizerClearsOnClose verifies that an explicit Close() does
// not trigger a second Close() from the finalizer (double-close panic). This
// exercises the runtime.SetFinalizer(kl, nil) clear in Close().
func TestKeyLimiterFinalizerClearsOnClose(t *testing.T) {
	defer goleak.VerifyNone(t)

	kl := NewKeyLimiter(10, 5)
	kl.StartCleanup(time.Second, time.Minute)
	kl.Close()
	// Drop the reference; the finalizer (now cleared) must NOT call Close again.
	kl = nil
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
}
