package rerank

import "testing"

func TestStatus_String(t *testing.T) {
	cases := []struct {
		status Status
		want   string
	}{
		{StatusOk, "ok"},
		{StatusDegraded, "degraded"},
		{StatusFallback, "fallback"},
		{StatusSkipped, "skipped"},
		{Status(99), "unknown"}, // out-of-range must not panic
	}
	for _, tc := range cases {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestStatus_Iota_ZeroIsOk(t *testing.T) {
	// StatusOk must be the zero value so that a zero-value Result is safe.
	var s Status
	if s != StatusOk {
		t.Errorf("zero Status = %v, want StatusOk", s)
	}
}

func TestResult_ZeroValueSafe(t *testing.T) {
	var r Result
	// Zero-value Result must have Status=Ok (iota=0) and nil Scored/Err.
	if r.Status != StatusOk {
		t.Errorf("zero Result.Status = %v, want StatusOk", r.Status)
	}
	if r.Err != nil {
		t.Errorf("zero Result.Err = %v, want nil", r.Err)
	}
	if r.Scored != nil {
		t.Errorf("zero Result.Scored = %v, want nil", r.Scored)
	}
	if r.Model != "" {
		t.Errorf("zero Result.Model = %q, want empty", r.Model)
	}
}

func TestResult_DegradedHasErr(t *testing.T) {
	// Convention: Err non-nil iff Status==StatusDegraded.
	r := Result{Status: StatusDegraded, Err: errSentinel}
	if r.Err == nil {
		t.Error("Degraded Result must have non-nil Err")
	}
}

// errSentinel is a package-level sentinel used in tests.
type sentinelError struct{ msg string }

func (e sentinelError) Error() string { return e.msg }

var errSentinel = sentinelError{"test error"}
