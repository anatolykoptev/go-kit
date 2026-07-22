package breaker

import (
	"errors"
	"io"
	"testing"
	"time"
)

// TestExecutePanicDoesNotWedgeHalfOpen is the RED repro for the freeze_stall
// bug: a panicking fn inside Execute must not leave halfOpenInFlight pinned,
// which would make Allow() return false forever in half-open state.
//
// Without the recover guard in Execute, Record is skipped on panic, the probe
// slot is never released, and b.Allow() stays false permanently.
func TestExecutePanicDoesNotWedgeHalfOpen(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: 10 * time.Millisecond})
	b.ForceHalfOpen()

	// Execute must re-panic (preserve caller stack) but still record the failure.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Execute did not re-panic; got nil")
			}
		}()
		_, _ = Execute[int](b, func() (int, error) { panic("boom") })
	}()

	// Record(false) in half-open trips the breaker to Open — that is expected.
	// The wedge bug manifests AFTER the cooldown expires: the breaker transitions
	// back to half-open, but without the recover guard halfOpenInFlight stays
	// pinned at 1 (never decremented), so Allow() returns false forever.
	// Wait for the cooldown to elapse, then verify the slot was released.
	time.Sleep(20 * time.Millisecond)
	if !b.Allow() {
		t.Fatalf("Allow() = false after cooldown; half-open slot wedged by panic")
	}

	// The panic counter must have been incremented exactly once.
	if got := ExecutePanicRecovered.Load(); got != 1 {
		t.Fatalf("ExecutePanicRecovered = %d, want 1", got)
	}
}

// FuzzExecutePanic verifies that no matter where/whether a fn panics, the
// breaker never ends up with a permanently-false Allow() after Execute returns
// (via recover). The fuzzer exercises arbitrary panic/no-panic/error/success
// combinations.
func FuzzExecutePanic(f *testing.F) {
	f.Add(uint8(0)) // no panic, success
	f.Add(uint8(1)) // no panic, error
	f.Add(uint8(2)) // panic immediately
	f.Add(uint8(3)) // panic after returning a value

	f.Fuzz(func(t *testing.T, mode uint8) {
		b := New(Options{FailThreshold: 1, OpenDuration: 10 * time.Millisecond})
		b.ForceHalfOpen()

		fn := func() (int, error) {
			switch mode % 4 {
			case 0:
				return 42, nil
			case 1:
				return 0, io.EOF
			case 2:
				panic("fuzz boom")
			case 3:
				_ = 42
				panic("fuzz boom after work")
			}
			return 0, nil
		}

		func() {
			defer func() { _ = recover() }()
			_, _ = Execute[int](b, fn)
		}()

		// After Execute returns (or panics out), Allow() must not be
		// permanently false due to a wedged probe slot. We allow the breaker
		// to have tripped open (mode 1/2/3 record failures), but if it is
		// still half-open the slot must be released.
		if st := b.State(); st == StateHalfOpen {
			if !b.Allow() {
				t.Fatalf("Allow() = false while half-open after Execute; slot wedged (mode=%d)", mode)
			}
		}
	})
}

func TestExecute_RunsFunctionWhenClosed(t *testing.T) {
	b := New(Options{FailThreshold: 3, OpenDuration: time.Second})
	got, err := Execute(b, func() (int, error) { return 42, nil })
	if err != nil || got != 42 {
		t.Fatalf("got (%d, %v), want (42, nil)", got, err)
	}
}

func TestExecute_ReturnsErrOpenWhenTripped(t *testing.T) {
	b := New(Options{FailThreshold: 1, OpenDuration: time.Second})
	_, _ = Execute(b, func() (int, error) { return 0, io.EOF })
	_, err := Execute(b, func() (int, error) { return 42, nil })
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
}

func TestExecute_RecordsFailureOnError(t *testing.T) {
	b := New(Options{FailThreshold: 2, OpenDuration: time.Second})
	for range 2 {
		_, _ = Execute(b, func() (int, error) { return 0, io.EOF })
	}
	if b.State() != StateOpen {
		t.Fatalf("state = %s, want open after 2 errors", b.State())
	}
}
