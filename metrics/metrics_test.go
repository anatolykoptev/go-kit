package metrics_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
)

func TestIncr(t *testing.T) {
	r := metrics.NewRegistry()
	r.Incr("requests")
	r.Incr("requests")
	r.Incr("errors")

	snap := r.Snapshot()
	if snap["requests"] != 2 {
		t.Errorf("requests = %d, want 2", snap["requests"])
	}
	if snap["errors"] != 1 {
		t.Errorf("errors = %d, want 1", snap["errors"])
	}
}

func TestAdd(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("bytes", 1024)
	r.Add("bytes", 2048)

	snap := r.Snapshot()
	if snap["bytes"] != 3072 {
		t.Errorf("bytes = %d, want 3072", snap["bytes"])
	}
}

func TestValue(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("counter", 5)
	if got := r.Value("counter"); got != 5 {
		t.Errorf("Value = %d, want 5", got)
	}
}

func TestSnapshot_Empty(t *testing.T) {
	r := metrics.NewRegistry()
	snap := r.Snapshot()
	if len(snap) != 0 {
		t.Errorf("snapshot len = %d, want 0", len(snap))
	}
}

func TestReset(t *testing.T) {
	r := metrics.NewRegistry()
	r.Incr("a")
	r.Incr("b")
	r.Reset()

	snap := r.Snapshot()
	if len(snap) != 0 {
		t.Errorf("after reset, snapshot len = %d, want 0", len(snap))
	}
}

func TestTrackOperation_Success(t *testing.T) {
	r := metrics.NewRegistry()
	err := r.TrackOperation("calls", "errs", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := r.Snapshot()
	if snap["calls"] != 1 {
		t.Errorf("calls = %d, want 1", snap["calls"])
	}
	if v, ok := snap["errs"]; ok && v != 0 {
		t.Errorf("errs = %d, want 0", v)
	}
}

func TestTrackOperation_Error(t *testing.T) {
	r := metrics.NewRegistry()
	err := r.TrackOperation("calls", "errs", func() error {
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	snap := r.Snapshot()
	if snap["calls"] != 1 {
		t.Errorf("calls = %d, want 1", snap["calls"])
	}
	if snap["errs"] != 1 {
		t.Errorf("errs = %d, want 1", snap["errs"])
	}
}

func TestFormat(t *testing.T) {
	r := metrics.NewRegistry()
	r.Add("requests", 100)
	r.Add("errors", 3)

	s := r.Format()
	if s == "" {
		t.Error("Format returned empty string")
	}
	if !strings.Contains(s, "requests") || !strings.Contains(s, "errors") {
		t.Errorf("Format missing counter names: %q", s)
	}
}

func TestFormat_Empty(t *testing.T) {
	r := metrics.NewRegistry()
	if got := r.Format(); got != "" {
		t.Errorf("Format = %q, want empty", got)
	}
}
