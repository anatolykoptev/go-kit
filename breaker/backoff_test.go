// breaker/backoff_test.go
package breaker

import (
	"testing"
	"time"
)

func TestComputeBackoff_ConstantDefault(t *testing.T) {
	o := Options{OpenDuration: 10 * time.Second, BackoffMultiplier: 1.0}
	for n := 1; n <= 5; n++ {
		if got := computeBackoff(o, n); got != 10*time.Second {
			t.Errorf("n=%d got %s, want 10s", n, got)
		}
	}
}

func TestComputeBackoff_ExponentialGrowth(t *testing.T) {
	o := Options{OpenDuration: 5 * time.Minute, BackoffMultiplier: 2.0, MaxOpenDuration: 1 * time.Hour}
	cases := []struct {
		n    int
		want time.Duration
	}{
		{1, 5 * time.Minute},
		{2, 10 * time.Minute},
		{3, 20 * time.Minute},
		{4, 40 * time.Minute},
		{5, 1 * time.Hour}, // capped
		{6, 1 * time.Hour}, // capped
	}
	for _, c := range cases {
		if got := computeBackoff(o, c.n); got != c.want {
			t.Errorf("n=%d got %s, want %s", c.n, got, c.want)
		}
	}
}

func TestComputeBackoff_MultiplierBelowOneCollapses(t *testing.T) {
	o := Options{OpenDuration: 10 * time.Second, BackoffMultiplier: 0.5}
	if got := computeBackoff(o, 3); got != 10*time.Second {
		t.Errorf("got %s, want constant 10s when multiplier<1", got)
	}
}
