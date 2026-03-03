package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrencyLimiter_AcquireRelease(t *testing.T) {
	cl := NewConcurrencyLimiter(3)
	if cl.Size() != 3 {
		t.Errorf("Size = %d, want 3", cl.Size())
	}
	if cl.Available() != 3 {
		t.Errorf("Available = %d, want 3", cl.Available())
	}

	release, err := cl.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if cl.Available() != 2 {
		t.Errorf("Available after acquire = %d, want 2", cl.Available())
	}

	release()
	if cl.Available() != 3 {
		t.Errorf("Available after release = %d, want 3", cl.Available())
	}
}

func TestConcurrencyLimiter_AcquireBlocks(t *testing.T) {
	cl := NewConcurrencyLimiter(1)
	release, err := cl.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	// Second acquire should block.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = cl.Acquire(ctx)
	if err == nil {
		t.Fatal("second Acquire should have timed out")
	}

	release()
	// Now should succeed.
	release2, err := cl.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire after release failed: %v", err)
	}
	release2()
}

func TestConcurrencyLimiter_TryAcquire(t *testing.T) {
	cl := NewConcurrencyLimiter(1)
	release, ok := cl.TryAcquire()
	if !ok {
		t.Fatal("first TryAcquire should succeed")
	}

	_, ok = cl.TryAcquire()
	if ok {
		t.Fatal("second TryAcquire should fail when full")
	}

	release()
	release2, ok := cl.TryAcquire()
	if !ok {
		t.Fatal("TryAcquire after release should succeed")
	}
	release2()
}

func TestConcurrencyLimiter_Concurrent(t *testing.T) {
	const maxConcurrent = 5
	cl := NewConcurrencyLimiter(maxConcurrent)

	var peak atomic.Int32
	var current atomic.Int32
	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := cl.Acquire(context.Background())
			if err != nil {
				t.Errorf("Acquire failed: %v", err)
				return
			}
			defer release()

			n := current.Add(1)
			// Track peak.
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			current.Add(-1)
		}()
	}
	wg.Wait()

	if p := peak.Load(); p > maxConcurrent {
		t.Errorf("peak concurrency = %d, want <= %d", p, maxConcurrent)
	}
	if cl.Available() != maxConcurrent {
		t.Errorf("Available after all done = %d, want %d", cl.Available(), maxConcurrent)
	}
}

func TestConcurrencyLimiter_ContextCancel(t *testing.T) {
	cl := NewConcurrencyLimiter(1)
	release, _ := cl.Acquire(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := cl.Acquire(ctx)
	if err == nil {
		t.Fatal("Acquire with cancelled context should fail")
	}

	release()
}

func TestConcurrencyLimiter_PanicOnZero(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on zero maxConcurrent")
		}
	}()
	NewConcurrencyLimiter(0)
}

func TestConcurrencyLimiter_PanicOnNegative(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on negative maxConcurrent")
		}
	}()
	NewConcurrencyLimiter(-1)
}
