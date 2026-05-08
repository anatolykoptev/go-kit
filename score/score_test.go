package score

import (
	"math"
	"testing"
)

func TestConfidenceFromScore_DefaultThresholds(t *testing.T) {
	cases := []struct {
		name string
		s    float64
		want ConfidenceLevel
	}{
		{"negative", -1.0, ConfidenceLow},
		{"zero", 0.0, ConfidenceLow},
		{"just_under_low_boundary", 0.19999, ConfidenceLow},
		{"exact_low_boundary", 0.2, ConfidenceMedium},
		{"middle_of_medium", 0.5, ConfidenceMedium},
		{"just_under_high_boundary", 0.69999, ConfidenceMedium},
		{"exact_high_boundary", 0.7, ConfidenceHigh},
		{"saturated", 5.0, ConfidenceHigh},
		{"infinity", math.Inf(1), ConfidenceHigh},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ConfidenceFromScore(c.s); got != c.want {
				t.Errorf("ConfidenceFromScore(%v) = %q, want %q", c.s, got, c.want)
			}
		})
	}
}

func TestConfidenceFromScoreThresholds_CustomCutoffs(t *testing.T) {
	// Cosine similarity range [0, 1] — different cutoffs.
	got := ConfidenceFromScoreThresholds(0.4, 0.3, 0.6)
	if got != ConfidenceMedium {
		t.Errorf("got %q, want %q", got, ConfidenceMedium)
	}

	got = ConfidenceFromScoreThresholds(0.7, 0.3, 0.6)
	if got != ConfidenceHigh {
		t.Errorf("got %q, want %q", got, ConfidenceHigh)
	}

	got = ConfidenceFromScoreThresholds(0.2, 0.3, 0.6)
	if got != ConfidenceLow {
		t.Errorf("got %q, want %q", got, ConfidenceLow)
	}
}

func TestConfidenceFromScoreThresholds_PanicOnFlipped(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on flipped thresholds")
		}
	}()
	_ = ConfidenceFromScoreThresholds(0.5, 0.7, 0.3) // low > medium
}

func TestBucket_3Way(t *testing.T) {
	// Mirror ConfidenceFromScore via Bucket.
	thr := []float64{DefaultLowMax, DefaultMediumMax}
	labels := []string{"low", "medium", "high"}

	cases := []struct {
		s    float64
		want string
	}{
		{-0.5, "low"},
		{0.0, "low"},
		{0.19999, "low"},
		{0.2, "medium"},
		{0.5, "medium"},
		{0.69999, "medium"},
		{0.7, "high"},
		{10.0, "high"},
	}
	for _, c := range cases {
		got := Bucket(c.s, thr, labels)
		if got != c.want {
			t.Errorf("Bucket(%v) = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestBucket_4WaySeverity(t *testing.T) {
	thr := []float64{0.25, 0.5, 0.75}
	labels := []string{"info", "warning", "error", "critical"}

	cases := []struct {
		s    float64
		want string
	}{
		{0.0, "info"},
		{0.24, "info"},
		{0.25, "warning"},
		{0.49, "warning"},
		{0.5, "error"},
		{0.74, "error"},
		{0.75, "critical"},
		{0.99, "critical"},
	}
	for _, c := range cases {
		got := Bucket(c.s, thr, labels)
		if got != c.want {
			t.Errorf("Bucket(%v, severity) = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestBucket_2Way(t *testing.T) {
	got := Bucket(0.4, []float64{0.5}, []string{"below", "above"})
	if got != "below" {
		t.Errorf("got %q, want %q", got, "below")
	}
	got = Bucket(0.6, []float64{0.5}, []string{"below", "above"})
	if got != "above" {
		t.Errorf("got %q, want %q", got, "above")
	}
}

func TestBucket_PanicsOnMismatchedLabels(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on len(labels) != len(thresholds)+1")
		}
	}()
	_ = Bucket(0.5, []float64{0.3, 0.6}, []string{"a", "b"}) // labels=2, expected 3
}

func TestBucket_PanicsOnEmptyLabels(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty labels")
		}
	}()
	_ = Bucket(0.5, nil, nil)
}

func TestBucket_PanicsOnUnsortedThresholds(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on unsorted thresholds")
		}
	}()
	_ = Bucket(0.5, []float64{0.5, 0.3, 0.7}, []string{"a", "b", "c", "d"})
}

func TestBucket_NegativeInfinity(t *testing.T) {
	got := Bucket(math.Inf(-1), []float64{0.2, 0.7}, []string{"low", "med", "high"})
	if got != "low" {
		t.Errorf("got %q for -∞, want %q", got, "low")
	}
}
