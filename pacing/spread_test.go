package pacing

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

const spreadDraws = 100_000

func seeded() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

// normCDF is the standard normal CDF, the analytic oracle for log-normal
// quantiles: P(X <= x) = Phi((ln x - mu) / sigma).
func normCDF(z float64) float64 { return 0.5 * (1 + math.Erf(z/math.Sqrt2)) }

// lnCDF is the CDF of a log-normal with the given unclamped mean and sigma.
func lnCDF(x, mean time.Duration, sigma float64) float64 {
	mu := math.Log(float64(mean)) - sigma*sigma/2
	return normCDF((math.Log(float64(x)) - mu) / sigma)
}

func draws(s Spread, n int) []time.Duration {
	r := seeded()
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = s.DurationFrom(r)
	}
	return out
}

func fraction(ds []time.Duration, pred func(time.Duration) bool) float64 {
	c := 0
	for _, d := range ds {
		if pred(d) {
			c++
		}
	}
	return float64(c) / float64(len(ds))
}

func TestSpread_WithinBounds(t *testing.T) {
	t.Parallel()
	s := Spread{Mean: 3 * time.Hour, Sigma: 1, Min: 30 * time.Minute, Max: 6 * time.Hour}
	ds := draws(s, spreadDraws)
	for _, d := range ds {
		if d < s.Min || d > s.Max {
			t.Fatalf("draw %v outside [%v, %v]", d, s.Min, s.Max)
		}
	}
	// The clamps must actually bite, with analytic mass at each end.
	wantLo := lnCDF(s.Min, s.Mean, 1)
	wantHi := 1 - lnCDF(s.Max, s.Mean, 1)
	if got := fraction(ds, func(d time.Duration) bool { return d == s.Min }); math.Abs(got-wantLo) > 0.01 {
		t.Errorf("mass at Min = %.4f, want %.4f", got, wantLo)
	}
	if got := fraction(ds, func(d time.Duration) bool { return d == s.Max }); math.Abs(got-wantHi) > 0.01 {
		t.Errorf("mass at Max = %.4f, want %.4f", got, wantHi)
	}
}

func TestSpread_MeanUnclamped(t *testing.T) {
	t.Parallel()
	s := Spread{Mean: time.Hour, Sigma: 0.5}
	var sum float64
	for _, d := range draws(s, spreadDraws) {
		sum += float64(d)
	}
	got := sum / spreadDraws
	if rel := math.Abs(got-float64(s.Mean)) / float64(s.Mean); rel > 0.03 {
		t.Fatalf("empirical mean %v, want %v within 3%% (rel err %.4f)",
			time.Duration(got), s.Mean, rel)
	}
}

func TestSpread_IsHeavyTailed(t *testing.T) {
	t.Parallel()
	s := Spread{Mean: 3 * time.Hour, Sigma: 1, Min: 2 * time.Minute, Max: 12 * time.Hour}
	ds := draws(s, spreadDraws)

	// Clamps sit far outside both thresholds, so the analytic unclamped CDF
	// gives the expected fractions (~9.8% short, ~15.6% long).
	wantShort := lnCDF(30*time.Minute, s.Mean, 1)
	wantLong := 1 - lnCDF(5*time.Hour, s.Mean, 1)
	short := fraction(ds, func(d time.Duration) bool { return d < 30*time.Minute })
	long := fraction(ds, func(d time.Duration) bool { return d > 5*time.Hour })

	if math.Abs(short-wantShort) > 0.01 {
		t.Errorf("share under 30m = %.4f, want %.4f +-0.01", short, wantShort)
	}
	if math.Abs(long-wantLong) > 0.01 {
		t.Errorf("share over 5h = %.4f, want %.4f +-0.01", long, wantLong)
	}
	if short < 0.05 || long < 0.10 {
		t.Errorf("not wide enough: short=%.4f (want >=0.05) long=%.4f (want >=0.10)", short, long)
	}
}

func TestSpread_NotUniformBand(t *testing.T) {
	t.Parallel()
	// A uniform(0.5,1.5)*Mean draw never leaves [1.5h, 4.5h] for Mean=3h.
	s := Spread{Mean: 3 * time.Hour, Sigma: 1}
	ds := draws(s, spreadDraws)
	outside := fraction(ds, func(d time.Duration) bool { return d < 90*time.Minute || d > 270*time.Minute })
	if outside < 0.5 {
		t.Fatalf("only %.2f of gaps outside the 90-270m band, want >= 0.5", outside)
	}
}

func TestSpread_Degenerate(t *testing.T) {
	t.Parallel()
	r := seeded()
	nan, inf := math.NaN(), math.Inf(1)
	cases := []struct {
		name string
		s    Spread
		min  time.Duration // inclusive bounds the result must satisfy
		max  time.Duration
	}{
		{"zero value", Spread{}, 0, 0},
		{"mean zero returns floor", Spread{Min: time.Minute}, time.Minute, time.Minute},
		{"mean negative", Spread{Mean: -time.Hour, Min: time.Minute, Max: time.Hour}, time.Minute, time.Minute},
		{"negative min", Spread{Mean: time.Hour, Min: -time.Hour, Max: 2 * time.Hour}, 0, 2 * time.Hour},
		{"min above max, floor wins", Spread{Mean: time.Hour, Min: 2 * time.Hour, Max: time.Hour}, 2 * time.Hour, 2 * time.Hour},
		{"sigma NaN", Spread{Mean: time.Hour, Sigma: nan, Max: 10 * time.Hour}, 0, 10 * time.Hour},
		{"sigma negative", Spread{Mean: time.Hour, Sigma: -3, Max: 10 * time.Hour}, 0, 10 * time.Hour},
		{"sigma +Inf", Spread{Mean: time.Hour, Sigma: inf, Max: 10 * time.Hour}, 0, 10 * time.Hour},
		{"sigma huge, capped", Spread{Mean: time.Hour, Sigma: 1e300, Max: 10 * time.Hour}, 0, 10 * time.Hour},
		{"overflow, no cap", Spread{Mean: math.MaxInt64, Sigma: 10}, 0, math.MaxInt64},
		{"overflow, capped", Spread{Mean: math.MaxInt64, Sigma: 10, Max: time.Hour}, 0, time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for range 2000 {
				d := tc.s.DurationFrom(r)
				if d < 0 || d < tc.min || d > tc.max {
					t.Fatalf("got %v, want within [%v, %v]", d, tc.min, tc.max)
				}
			}
		})
	}
}

func TestSpread_DefaultSigmaEqualsExplicit(t *testing.T) {
	t.Parallel()
	a := draws(Spread{Mean: time.Hour}, 100)
	b := draws(Spread{Mean: time.Hour, Sigma: DefaultSpreadSigma}, 100)
	if !slices.Equal(a, b) {
		t.Fatal("Sigma 0 must behave as DefaultSpreadSigma")
	}
}

func TestSpread_DeterministicAndGlobal(t *testing.T) {
	t.Parallel()
	s := Spread{Mean: time.Hour, Sigma: 1, Min: time.Minute, Max: 5 * time.Hour}
	if !slices.Equal(draws(s, 50), draws(s, 50)) {
		t.Fatal("same seed must give same sequence")
	}
	for range 1000 {
		if d := s.Duration(); d < s.Min || d > s.Max {
			t.Fatalf("Duration() %v outside bounds", d)
		}
	}
	if d := s.DurationFrom(nil); d < s.Min || d > s.Max {
		t.Fatalf("DurationFrom(nil) %v outside bounds", d)
	}
}
