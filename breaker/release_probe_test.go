// breaker/release_probe_test.go
package breaker

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tripAndCooldown drives the breaker into half-open: trip it once, then wait
// for the cooldown to elapse. The next Allow() transitions to half-open and
// consumes one probe slot.
func tripAndCooldown(t *testing.T, b *Breaker) {
	t.Helper()
	b.Record(false) // trip
	time.Sleep(20 * time.Millisecond)
}

// TestReleaseProbe_ClosedLeavesStreakIntact — StateClosed + non-zero
// consecutiveFails: ReleaseProbe MUST NOT touch the streak. This is the
// load-bearing case: an outcome not attributable to this breaker must not
// fabricate health.
func TestReleaseProbe_ClosedLeavesStreakIntact(t *testing.T) {
	b := New(Options{FailThreshold: 5, OpenDuration: time.Second})
	b.Record(false)
	b.Record(false)
	b.Record(false) // consecutiveFails == 3, under threshold
	before := b.consecutiveFails
	b.ReleaseProbe()
	if b.consecutiveFails != before {
		t.Fatalf("consecutiveFails = %d, want %d (streak must be untouched in StateClosed)",
			b.consecutiveFails, before)
	}
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed", b.State())
	}
}

// TestReleaseProbe_HalfOpenReleasesSlot — StateHalfOpen with MaxHalfOpenCalls=1:
// Allow() takes the slot, ReleaseProbe() frees it, the next Allow() succeeds.
func TestReleaseProbe_HalfOpenReleasesSlot(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 10 * time.Millisecond})
	tripAndCooldown(t, b)
	if !b.Allow() {
		t.Fatal("first Allow in half-open must succeed")
	}
	b.ReleaseProbe()
	if !b.Allow() {
		t.Fatal("Allow after ReleaseProbe must succeed — slot was not released")
	}
}

// TestReleaseProbe_HalfOpenStaysHalfOpen — ReleaseProbe MUST NOT call reset():
// state stays half-open and consecutiveOpens is unchanged.
func TestReleaseProbe_HalfOpenStaysHalfOpen(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 10 * time.Millisecond})
	tripAndCooldown(t, b)
	if !b.Allow() { // → half-open, consumes slot
		t.Fatal("Allow must succeed after cooldown")
	}
	opensBefore := b.consecutiveOpens
	b.ReleaseProbe()
	if b.state != StateHalfOpen {
		t.Fatalf("state = %s, want half-open (ReleaseProbe must not reset)", b.state)
	}
	if b.consecutiveOpens != opensBefore {
		t.Fatalf("consecutiveOpens = %d, want %d (must not reset backoff history)",
			b.consecutiveOpens, opensBefore)
	}
}

// TestReleaseProbe_HalfOpenDoesNotTrip — ReleaseProbe MUST NOT call tripToOpen().
func TestReleaseProbe_HalfOpenDoesNotTrip(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 10 * time.Millisecond})
	tripAndCooldown(t, b)
	if !b.Allow() {
		t.Fatal("Allow must succeed after cooldown")
	}
	b.ReleaseProbe()
	if b.state == StateOpen {
		t.Fatal("state = open; ReleaseProbe must not trip to open")
	}
}

// TestReleaseProbe_OpenIsNoop — StateOpen: no panic, no state change (mirrors
// Record's existing no-op in StateOpen).
func TestReleaseProbe_OpenIsNoop(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: time.Second})
	b.Record(false) // → open
	if b.state != StateOpen {
		t.Fatalf("precondition: state = %s, want open", b.state)
	}
	opensBefore := b.consecutiveOpens
	inflightBefore := b.halfOpenInFlight
	b.ReleaseProbe() // must not panic
	if b.state != StateOpen {
		t.Fatalf("state = %s, want open (ReleaseProbe must be a no-op in StateOpen)", b.state)
	}
	if b.consecutiveOpens != opensBefore {
		t.Fatalf("consecutiveOpens changed: %d -> %d", opensBefore, b.consecutiveOpens)
	}
	if b.halfOpenInFlight != inflightBefore {
		t.Fatalf("halfOpenInFlight changed: %d -> %d", inflightBefore, b.halfOpenInFlight)
	}
}

// TestReleaseProbe_RaceInvariants — under -race, concurrent Allow /
// ReleaseProbe / Record on one breaker must keep halfOpenInFlight within
// [0, MaxHalfOpenCalls] at every sampled instant.
func TestReleaseProbe_RaceInvariants(t *testing.T) {
	const max uint32 = 4
	b := New(Options{
		FailThreshold:    1,
		OpenDuration:     5 * time.Millisecond,
		MaxHalfOpenCalls: max,
	})

	var minSample atomic.Uint32
	minSample.Store(max)
	var maxSample atomic.Uint32

	var stop atomic.Bool
	var wg sync.WaitGroup

	// Monitor: sample halfOpenInFlight under the lock for the whole run.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			b.mu.Lock()
			v := b.halfOpenInFlight
			b.mu.Unlock()
			if v < minSample.Load() {
				minSample.Store(v)
			}
			if v > maxSample.Load() {
				maxSample.Store(v)
			}
		}
	}()

	// Workers: mix Allow + (ReleaseProbe | Record).
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				if b.Allow() {
					// Resolve the slot: half the time neutrally, half via Record.
					if time.Now().UnixNano()&1 == 0 {
						b.ReleaseProbe()
					} else {
						b.Record(time.Now().UnixNano()&3 != 0)
					}
				}
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	stop.Store(true)
	wg.Wait()

	// Cap check: halfOpenInFlight must never exceed MaxHalfOpenCalls.
	if got := maxSample.Load(); got > max {
		t.Fatalf("halfOpenInFlight exceeded MaxHalfOpenCalls: sampled max=%d, cap=%d", got, max)
	}
	// Floor check: a missing guard lets the counter decrement past zero,
	// wrapping to a huge uint32 — which then dominates maxSample.
	if got := maxSample.Load(); got > max {
		t.Fatalf("halfOpenInFlight went negative (uint32 wrap): sampled max=%d, cap=%d", got, max)
	}
	// Final settled state must also be in range.
	b.mu.Lock()
	final := b.halfOpenInFlight
	b.mu.Unlock()
	if final > max {
		t.Fatalf("final halfOpenInFlight = %d, exceeds cap %d", final, max)
	}
}

// TestReleaseProbe_NoCallbacksFired — ReleaseProbe MUST NOT fire OnTrip or
// OnRecover in any state. Callbacks fire in a goroutine, so a brief wait is
// needed after each ReleaseProbe before checking the flags.
func TestReleaseProbe_NoCallbacksFired(t *testing.T) {
	var tripped, recovered atomic.Bool
	b := New(Options{
		Name:          "cb",
		FailThreshold: 1,
		OpenDuration:  10 * time.Millisecond,
		OnTrip:        func(string) { tripped.Store(true) },
		OnRecover:     func(string) { recovered.Store(true) },
	})

	// StateClosed.
	tripped.Store(false)
	recovered.Store(false)
	b.ReleaseProbe()
	time.Sleep(30 * time.Millisecond)
	if tripped.Load() || recovered.Load() {
		t.Fatal("ReleaseProbe fired a callback in StateClosed")
	}

	// StateHalfOpen. The setup trip fires OnTrip, so reset flags right before
	// the call under test.
	tripAndCooldown(t, b)
	if !b.Allow() {
		t.Fatal("Allow must succeed after cooldown")
	}
	tripped.Store(false)
	recovered.Store(false)
	b.ReleaseProbe()
	time.Sleep(30 * time.Millisecond)
	if tripped.Load() || recovered.Load() {
		t.Fatal("ReleaseProbe fired a callback in StateHalfOpen")
	}

	// StateOpen.
	b2 := New(Options{
		Name:          "cb2",
		FailThreshold: 1,
		OpenDuration:  time.Second,
		OnTrip:        func(string) { tripped.Store(true) },
		OnRecover:     func(string) { recovered.Store(true) },
	})
	b2.Record(false) // → open (fires OnTrip)
	// Drain the async OnTrip goroutine before resetting flags, so a late
	// callback from the setup trip is not misattributed to ReleaseProbe.
	time.Sleep(30 * time.Millisecond)
	tripped.Store(false)
	recovered.Store(false)
	b2.ReleaseProbe()
	time.Sleep(30 * time.Millisecond)
	if tripped.Load() || recovered.Load() {
		t.Fatal("ReleaseProbe fired a callback in StateOpen")
	}
}
