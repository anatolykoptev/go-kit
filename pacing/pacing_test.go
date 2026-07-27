package pacing

import (
	"context"
	"testing"
	"time"
)

func TestJitter_Duration(t *testing.T) {
	t.Parallel()
	j := Jitter{Min: 100 * time.Millisecond, Max: 500 * time.Millisecond}
	for range 1000 {
		d := j.Duration()
		if d < 100*time.Millisecond || d >= 500*time.Millisecond {
			t.Fatalf("Duration out of range: %v", d)
		}
	}
}

func TestJitter_DurationDegenerate(t *testing.T) {
	t.Parallel()
	if (Jitter{Min: 100, Max: 100}).Duration() != 0 {
		t.Fatal("degenerate range should return 0")
	}
	if (Jitter{Min: 200, Max: 100}).Duration() != 0 {
		t.Fatal("inverted range should return 0")
	}
}

func TestJitter_Sleep(t *testing.T) {
	t.Parallel()
	j := Jitter{Min: 10 * time.Millisecond, Max: 30 * time.Millisecond}
	start := time.Now()
	if err := j.Sleep(context.Background()); err != nil {
		t.Fatalf("Sleep returned error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 10*time.Millisecond {
		t.Fatalf("Sleep returned too fast: %v", elapsed)
	}
}

func TestJitter_SleepCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j := Jitter{Min: 100 * time.Millisecond, Max: 200 * time.Millisecond}
	if err := j.Sleep(ctx); err == nil {
		t.Fatal("Sleep should return ctx.Err() on cancelled context")
	}
}

func TestJitter_SleepDegenerate(t *testing.T) {
	t.Parallel()
	j := Jitter{Min: 100, Max: 100}
	start := time.Now()
	if err := j.Sleep(context.Background()); err != nil {
		t.Fatalf("Sleep returned error: %v", err)
	}
	if time.Since(start) > 5*time.Millisecond {
		t.Fatal("degenerate Sleep should return immediately")
	}
}

func TestKeyedPacer_AllowFirstRequest(t *testing.T) {
	t.Parallel()
	p := NewKeyedPacer(100*time.Millisecond, 50*time.Millisecond)
	if !p.Allow("a") {
		t.Fatal("first request should be allowed")
	}
}

func TestKeyedPacer_AllowBlocksSecond(t *testing.T) {
	t.Parallel()
	p := NewKeyedPacer(100*time.Millisecond, 0)
	if !p.Allow("a") {
		t.Fatal("first request should be allowed")
	}
	if p.Allow("a") {
		t.Fatal("second immediate request should be blocked")
	}
}

func TestKeyedPacer_AllowIndependentKeys(t *testing.T) {
	t.Parallel()
	p := NewKeyedPacer(100*time.Millisecond, 0)
	if !p.Allow("a") {
		t.Fatal("first request for key a should be allowed")
	}
	if !p.Allow("b") {
		t.Fatal("first request for key b should be allowed (independent)")
	}
}

func TestKeyedPacer_Disabled(t *testing.T) {
	t.Parallel()
	p := NewKeyedPacer(0, 0)
	if !p.Allow("a") {
		t.Fatal("disabled pacer should always allow")
	}
	if !p.Allow("a") {
		t.Fatal("disabled pacer should always allow")
	}
	if err := p.Wait(context.Background(), "a"); err != nil {
		t.Fatalf("disabled pacer Wait should return nil: %v", err)
	}
}

func TestKeyedPacer_Wait(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	p := NewKeyedPacer(50*time.Millisecond, 0, WithPacerClock(clock.now))
	if !p.Allow("a") {
		t.Fatal("first Allow should succeed")
	}
	// Advance time past the delay.
	clock.advance(60 * time.Millisecond)
	if err := p.Wait(context.Background(), "a"); err != nil {
		t.Fatalf("Wait should succeed after delay: %v", err)
	}
}

func TestKeyedPacer_WaitCancelled(t *testing.T) {
	t.Parallel()
	p := NewKeyedPacer(1*time.Hour, 0)
	// Arm the pacer so the next Allow returns false, forcing Wait into the
	// poll loop where ctx cancellation is checked.
	if !p.Allow("a") {
		t.Fatal("first Allow should succeed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Wait(ctx, "a"); err == nil {
		t.Fatal("Wait should return ctx.Err() on cancelled context")
	}
}

func TestSymmetricJitter_Range(t *testing.T) {
	t.Parallel()
	base := 100 * time.Millisecond
	pct := 0.25 // ±25%
	min := time.Duration(float64(base) * (1 - pct))
	max := time.Duration(float64(base) * (1 + pct))
	for range 1000 {
		d := SymmetricJitter(base, pct)
		if d < min || d > max {
			t.Fatalf("SymmetricJitter out of range: %v (want [%v, %v])", d, min, max)
		}
	}
}

func TestSymmetricJitter_NoJitter(t *testing.T) {
	t.Parallel()
	base := 100 * time.Millisecond
	if d := SymmetricJitter(base, 0); d != base {
		t.Fatalf("SymmetricJitter with pct=0 should return base: got %v, want %v", d, base)
	}
	if d := SymmetricJitter(0, 0.25); d != 0 {
		t.Fatalf("SymmetricJitter with base=0 should return 0: got %v", d)
	}
	if d := SymmetricJitter(-1, 0.25); d != -1 {
		t.Fatalf("SymmetricJitter with base<0 should return base unchanged: got %v", d)
	}
}

func TestExponentialBackoff(t *testing.T) {
	t.Parallel()
	initial := 100 * time.Millisecond
	max := 10 * time.Second
	mult := 2.0
	pct := 0.0 // no jitter for deterministic check

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{10, 10 * time.Second}, // capped
	}
	for _, c := range cases {
		got := ExponentialBackoff(initial, max, mult, pct, c.attempt)
		if got != c.want {
			t.Fatalf("attempt %d: got %v, want %v", c.attempt, got, c.want)
		}
	}
}

func TestExponentialBackoff_WithJitter(t *testing.T) {
	t.Parallel()
	initial := 100 * time.Millisecond
	max := 10 * time.Second
	mult := 2.0
	pct := 0.3
	base := 200 * time.Millisecond // attempt 1
	min := time.Duration(float64(base) * (1 - pct))
	maxAllowed := time.Duration(float64(base) * (1 + pct))
	for range 100 {
		d := ExponentialBackoff(initial, max, mult, pct, 1)
		if d < min || d > maxAllowed {
			t.Fatalf("ExponentialBackoff with jitter out of range: %v (want [%v, %v])", d, min, maxAllowed)
		}
	}
}

// fakeClock allows deterministic time advancement in tests.
type fakeClock struct {
	t time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Now()}
}

func (f *fakeClock) now() time.Time { return f.t }

func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }
