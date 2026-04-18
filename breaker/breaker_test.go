// breaker/breaker_test.go
package breaker

import (
	"testing"
	"time"
)

func TestBreaker_ClosedAllowsAndRecords(t *testing.T) {
	b := New(Options{FailThreshold: 3, OpenDuration: 10 * time.Millisecond})
	if !b.Allow() {
		t.Fatal("closed breaker must allow")
	}
	b.Record(true)
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed", b.State())
	}
}

func TestBreaker_TripsAfterNConsecutiveFailures(t *testing.T) {
	b := New(Options{FailThreshold: 3, OpenDuration: time.Second})
	for range 3 {
		b.Record(false)
	}
	if b.State() != StateOpen {
		t.Fatalf("state = %s, want open", b.State())
	}
	if b.Allow() {
		t.Fatal("open breaker must not allow")
	}
}

func TestBreaker_SuccessResetsFailureCount(t *testing.T) {
	b := New(Options{FailThreshold: 3, OpenDuration: time.Second})
	b.Record(false)
	b.Record(false)
	b.Record(true) // reset
	b.Record(false)
	b.Record(false) // 2 consecutive, under threshold
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed", b.State())
	}
}

func TestBreaker_HalfOpenAfterCooldown(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 20 * time.Millisecond})
	b.Record(false)
	time.Sleep(30 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow one probe after cooldown")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("state = %s, want half-open", b.State())
	}
}

func TestBreaker_ProbeSlotBlocksAdditionalCalls(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 10 * time.Millisecond})
	b.Record(false)
	time.Sleep(20 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("first call in half-open must be allowed")
	}
	if b.Allow() {
		t.Fatal("second call in half-open must be blocked (MaxHalfOpenCalls=1 default)")
	}
}

func TestBreaker_ProbeSuccessClosesBreaker(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 10 * time.Millisecond})
	b.Record(false)
	time.Sleep(20 * time.Millisecond)
	b.Allow() // → half-open, probe used
	b.Record(true)
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed", b.State())
	}
}

func TestBreaker_ProbeFailureReopensWithHigherCooldown(t *testing.T) {
	b := New(Options{
		FailThreshold:     1,
		OpenDuration:      10 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxOpenDuration:   100 * time.Millisecond,
	})
	b.Record(false) // trip #1, cooldown = 10ms
	time.Sleep(20 * time.Millisecond)
	b.Allow()       // → half-open
	b.Record(false) // probe failed → open trip #2, cooldown = 20ms
	time.Sleep(15 * time.Millisecond)
	if b.Allow() {
		t.Fatal("should still be open (cooldown 20ms not yet elapsed)")
	}
	time.Sleep(10 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe after cooldown 2 elapses")
	}
}
