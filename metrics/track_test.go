package metrics_test

import (
	"errors"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/metrics"
)

func TestTrackCall_Success(t *testing.T) {
	r := metrics.NewRegistry()
	err := metrics.TrackCall(r, "x_calls_total", "x_errors_total", func() error { return nil })
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if v := r.Value("x_calls_total"); v != 1 {
		t.Errorf("calls = %d, want 1", v)
	}
	if v := r.Value("x_errors_total"); v != 0 {
		t.Errorf("errors = %d, want 0", v)
	}
}

func TestTrackCall_Failure(t *testing.T) {
	r := metrics.NewRegistry()
	boom := errors.New("boom")
	err := metrics.TrackCall(r, "x_calls_total", "x_errors_total", func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if v := r.Value("x_calls_total"); v != 1 {
		t.Errorf("calls = %d", v)
	}
	if v := r.Value("x_errors_total"); v != 1 {
		t.Errorf("errors = %d", v)
	}
}

func TestTrackCall_NilRegistry(t *testing.T) {
	boom := errors.New("boom")
	err := metrics.TrackCall(nil, "c", "e", func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("nil reg should still call fn, got %v", err)
	}
}

func TestTrackCallTimed_RecordsDuration(t *testing.T) {
	r := metrics.NewRegistry()
	err := metrics.TrackCallTimed(r, "c", "e", "latency_seconds", func() error {
		time.Sleep(3 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	// In non-prom (default) mode, TimerHandle.Stop writes gauge in ms.
	if g := r.Gauge("latency_seconds").Value(); g < 3.0 {
		t.Errorf("gauge in ms = %.2f, want >= 3.0", g)
	}
}
