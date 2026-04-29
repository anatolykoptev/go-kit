package embed

import "testing"

// TestStatus_String verifies all 4 defined values + unknown sentinel.
func TestStatus_String(t *testing.T) {
	cases := []struct {
		s    Status
		want string
	}{
		{StatusOk, "ok"},
		{StatusDegraded, "degraded"},
		{StatusFallback, "fallback"},
		{StatusSkipped, "skipped"},
		{Status(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

// TestResult_ZeroValueSafe verifies that a zero-value Result is usable
// without panicking (nil Vectors slice, zero Status == StatusOk).
func TestResult_ZeroValueSafe(t *testing.T) {
	var r Result
	if r.Status != StatusOk {
		t.Errorf("zero Status: want StatusOk (0), got %d", r.Status)
	}
	if r.Vectors != nil {
		t.Errorf("zero Vectors: want nil, got %v", r.Vectors)
	}
	if r.Err != nil {
		t.Errorf("zero Err: want nil, got %v", r.Err)
	}
	if r.Model != "" {
		t.Errorf("zero Model: want empty, got %q", r.Model)
	}
	if r.TokensUsed != 0 {
		t.Errorf("zero TokensUsed: want 0, got %d", r.TokensUsed)
	}
}

// TestVector_ZeroValueSafe verifies that a zero-value Vector is usable.
func TestVector_ZeroValueSafe(t *testing.T) {
	var v Vector
	if v.Status != StatusOk {
		t.Errorf("zero Status: want StatusOk (0), got %d", v.Status)
	}
	if v.Embedding != nil {
		t.Errorf("zero Embedding: want nil, got %v", v.Embedding)
	}
	if v.Dim != 0 {
		t.Errorf("zero Dim: want 0, got %d", v.Dim)
	}
	if v.TokenCount != 0 {
		t.Errorf("zero TokenCount: want 0, got %d", v.TokenCount)
	}
}

// TestStatus_IotaOrder verifies that the iota order matches the spec
// (StatusOk=0, StatusDegraded=1, StatusFallback=2, StatusSkipped=3).
// This is critical: callers may store Status as integers.
func TestStatus_IotaOrder(t *testing.T) {
	if StatusOk != 0 {
		t.Errorf("StatusOk must be 0, got %d", StatusOk)
	}
	if StatusDegraded != 1 {
		t.Errorf("StatusDegraded must be 1, got %d", StatusDegraded)
	}
	if StatusFallback != 2 {
		t.Errorf("StatusFallback must be 2, got %d", StatusFallback)
	}
	if StatusSkipped != 3 {
		t.Errorf("StatusSkipped must be 3, got %d", StatusSkipped)
	}
}
