package sparse

import "testing"

// TestStatus_String verifies each Status renders to a stable label.
// These labels are emitted into Prometheus metric label values and
// caller logs — changing them is a breaking observability change.
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
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.s.String(); got != tc.want {
				t.Errorf("Status(%d).String() = %q, want %q", tc.s, got, tc.want)
			}
		})
	}
}

// TestVector_ZeroValueIsEmpty verifies a default-constructed Vector has
// an empty SparseVector.
func TestVector_ZeroValueIsEmpty(t *testing.T) {
	var v Vector
	if !v.Sparse.IsEmpty() {
		t.Errorf("zero Vector.Sparse should be empty: %+v", v.Sparse)
	}
	if v.Status != StatusOk {
		t.Errorf("zero Vector.Status = %s, want StatusOk (the zero value)", v.Status)
	}
}

// TestResult_ZeroValueOk verifies a zero Result has StatusOk and no Err.
// (The constructor in client_v2 always sets Status explicitly; this
// guards against a future refactor that relies on the zero value.)
func TestResult_ZeroValueOk(t *testing.T) {
	var r Result
	if r.Status != StatusOk {
		t.Errorf("zero Result.Status = %s, want StatusOk", r.Status)
	}
	if r.Err != nil {
		t.Errorf("zero Result.Err = %v, want nil", r.Err)
	}
	if r.Vectors != nil {
		t.Errorf("zero Result.Vectors = %v, want nil", r.Vectors)
	}
}
