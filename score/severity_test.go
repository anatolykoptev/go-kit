package score

import "testing"

func TestSeverityRank_OrderedAscending(t *testing.T) {
	cases := []struct {
		s    Severity
		want int
	}{
		{SeverityInfo, 0},
		{SeverityLow, 1},
		{SeverityMedium, 2},
		{SeverityHigh, 3},
		{SeverityCritical, 4},
	}
	for _, c := range cases {
		if got := SeverityRank(c.s); got != c.want {
			t.Errorf("SeverityRank(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestSeverityRank_UnknownReturnsNegative(t *testing.T) {
	if got := SeverityRank(Severity("garbage")); got != -1 {
		t.Errorf("expected -1 for unknown, got %d", got)
	}
	if got := SeverityRank(""); got != -1 {
		t.Errorf("expected -1 for empty, got %d", got)
	}
}

func TestParseSeverity_Canonical(t *testing.T) {
	cases := []struct {
		in   string
		want Severity
		ok   bool
	}{
		{"info", SeverityInfo, true},
		{"INFO", SeverityInfo, true},
		{"  info  ", SeverityInfo, true},
		{"informational", SeverityInfo, true},
		{"low", SeverityLow, true},
		{"medium", SeverityMedium, true},
		{"med", SeverityMedium, true},
		{"warning", SeverityMedium, true},
		{"warn", SeverityMedium, true},
		{"high", SeverityHigh, true},
		{"error", SeverityHigh, true},
		{"critical", SeverityCritical, true},
		{"crit", SeverityCritical, true},
		{"CRITICAL", SeverityCritical, true},
		{"unknown", SeverityInfo, false},
		{"", SeverityInfo, false},
	}
	for _, c := range cases {
		got, ok := ParseSeverity(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseSeverity(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestSeverityFromScore_Boundaries(t *testing.T) {
	cases := []struct {
		score float64
		want  Severity
	}{
		{-1.0, SeverityInfo},
		{0.0, SeverityInfo},
		{0.099, SeverityInfo},
		{0.1, SeverityLow},
		{0.39, SeverityLow},
		{0.4, SeverityMedium},
		{0.69, SeverityMedium},
		{0.7, SeverityHigh},
		{0.89, SeverityHigh},
		{0.9, SeverityCritical},
		{1.0, SeverityCritical},
		{10.0, SeverityCritical},
	}
	for _, c := range cases {
		if got := SeverityFromScore(c.score); got != c.want {
			t.Errorf("SeverityFromScore(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestSeverityAtLeast(t *testing.T) {
	cases := []struct {
		s, threshold Severity
		want         bool
	}{
		{SeverityCritical, SeverityHigh, true},
		{SeverityHigh, SeverityHigh, true},
		{SeverityMedium, SeverityHigh, false},
		{SeverityInfo, SeverityCritical, false},
		{SeverityCritical, SeverityInfo, true},
		{Severity("bogus"), SeverityHigh, false},
		{SeverityHigh, Severity("bogus"), false},
	}
	for _, c := range cases {
		if got := SeverityAtLeast(c.s, c.threshold); got != c.want {
			t.Errorf("SeverityAtLeast(%q, %q) = %v, want %v", c.s, c.threshold, got, c.want)
		}
	}
}

func TestSeverity_String(t *testing.T) {
	if SeverityHigh.String() != "high" {
		t.Errorf("got %q, want %q", SeverityHigh.String(), "high")
	}
}
