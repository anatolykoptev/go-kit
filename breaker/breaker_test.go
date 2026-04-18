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
	b := New(Options{FailThreshold: 1, OpenDuration: 100 * time.Millisecond})
	b.Record(false)
	time.Sleep(150 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow one probe after cooldown")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("state = %s, want half-open", b.State())
	}
}

func TestBreaker_ProbeSlotBlocksAdditionalCalls(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 50 * time.Millisecond})
	b.Record(false)
	time.Sleep(80 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("first call in half-open must be allowed")
	}
	if b.Allow() {
		t.Fatal("second call in half-open must be blocked (MaxHalfOpenCalls=1 default)")
	}
}

func TestBreaker_ProbeSuccessClosesBreaker(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 50 * time.Millisecond})
	b.Record(false)
	time.Sleep(80 * time.Millisecond)
	b.Allow() // → half-open, probe used
	b.Record(true)
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed", b.State())
	}
}

func TestBreaker_ProbeFailureReopensWithHigherCooldown(t *testing.T) {
	b := New(Options{
		FailThreshold:     1,
		OpenDuration:      50 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxOpenDuration:   500 * time.Millisecond,
	})
	b.Record(false) // trip #1, cooldown = 50ms
	time.Sleep(80 * time.Millisecond)
	b.Allow()       // → half-open
	b.Record(false) // probe failed → open trip #2, cooldown = 100ms
	time.Sleep(60 * time.Millisecond)
	if b.Allow() {
		t.Fatal("should still be open (cooldown 100ms not yet elapsed)")
	}
	time.Sleep(60 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe after cooldown 2 elapses")
	}
}

func TestRecordCycle_TripsOnFailRate(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: time.Second, FailRateThreshold: 0.5})
	b.RecordCycle(10, 6) // 60% > 50%
	if b.State() != StateOpen {
		t.Fatalf("state = %s, want open", b.State())
	}
}

func TestRecordCycle_StaysClosedBelowThreshold(t *testing.T) {
	b := New(Options{FailRateThreshold: 0.5, OpenDuration: time.Second})
	b.RecordCycle(10, 4) // 40%
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed", b.State())
	}
}

func TestRecordCycle_NoopWithoutRateThreshold(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: time.Second})
	b.RecordCycle(10, 10) // FailRateThreshold == 0 → no-op
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed (RecordCycle must no-op without FailRateThreshold)", b.State())
	}
}

func TestRecordCycle_HalfOpenRecoversOnCleanCycle(t *testing.T) {
	b := New(Options{
		FailThreshold:     1,
		OpenDuration:      100 * time.Millisecond,
		FailRateThreshold: 0.5,
	})
	b.RecordCycle(10, 6) // trip
	time.Sleep(150 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow probe after cooldown")
	}
	b.RecordCycle(10, 0) // clean probe cycle
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed after clean probe cycle", b.State())
	}
}

func TestOnTrip_FiresOnOpen(t *testing.T) {
	fired := make(chan string, 1)
	b := New(Options{
		Name:          "test-svc",
		FailThreshold: 1,
		OpenDuration:  time.Second,
		OnTrip:        func(name string) { fired <- name },
	})
	b.Record(false)
	select {
	case name := <-fired:
		if name != "test-svc" {
			t.Errorf("got %q, want test-svc", name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("OnTrip did not fire")
	}
}

func TestOnRecover_FiresOnClose(t *testing.T) {
	fired := make(chan string, 1)
	b := New(Options{
		Name:          "test-svc",
		FailThreshold: 1,
		OpenDuration:  50 * time.Millisecond,
		OnRecover:     func(name string) { fired <- name },
	})
	b.Record(false)
	time.Sleep(80 * time.Millisecond)
	b.Allow()      // → half-open
	b.Record(true) // → closed, OnRecover fires
	select {
	case name := <-fired:
		if name != "test-svc" {
			t.Errorf("got %q, want test-svc", name)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("OnRecover did not fire")
	}
}

func TestMaxHalfOpenCalls_AllowsConfiguredParallelism(t *testing.T) {
	b := New(Options{
		FailThreshold:    1,
		OpenDuration:     50 * time.Millisecond,
		MaxHalfOpenCalls: 3,
	})
	b.Record(false)
	time.Sleep(80 * time.Millisecond)
	// Now in open → first Allow transitions to half-open.
	for i := 1; i <= 3; i++ {
		if !b.Allow() {
			t.Fatalf("call %d/3 must be allowed in half-open", i)
		}
	}
	if b.Allow() {
		t.Fatal("call 4/3 must be blocked (slot exhausted)")
	}
}
