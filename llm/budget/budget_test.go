package budget

import (
	"testing"
	"time"
)

func TestTracker_SessionLimit(t *testing.T) {
	tr := New(Options{
		PerSessionLimit: 1000,
		WarnThreshold:   0.8,
		SwitchModel:     "cheap-model",
	})

	// Under threshold — OK.
	tr.Add("s1", 500)
	if s := tr.Check("s1"); !s.OK || s.Warning || s.SwitchModel || s.HardStop {
		t.Fatalf("expected OK at 50%%: %+v", s)
	}

	// 80% — warning, not yet switch.
	tr.Add("s1", 300)
	if s := tr.Check("s1"); !s.OK || !s.Warning || s.SwitchModel || s.HardStop {
		t.Fatalf("expected warning at 80%%: %+v", s)
	}

	// 90% — switch model.
	tr.Add("s1", 100)
	if s := tr.Check("s1"); !s.OK || !s.Warning || !s.SwitchModel || s.HardStop {
		t.Fatalf("expected switch at 90%%: %+v", s)
	}

	// 100% — hard stop.
	tr.Add("s1", 100)
	if s := tr.Check("s1"); s.OK || !s.HardStop {
		t.Fatalf("expected hard stop at 100%%: %+v", s)
	}
}

func TestTracker_DailyLimit(t *testing.T) {
	tr := New(Options{
		PerDayLimit:   5000,
		WarnThreshold: 0.8,
	})

	// Multiple sessions contribute to one daily total.
	tr.Add("s1", 2000)
	tr.Add("s2", 2000)
	if s := tr.Check("s1"); !s.OK || !s.Warning {
		t.Fatalf("expected daily warning at 80%%: %+v", s)
	}

	tr.Add("s3", 1000)
	if s := tr.Check("s3"); s.OK || !s.HardStop {
		t.Fatalf("expected daily hard stop at 100%%: %+v", s)
	}
}

func TestTracker_DailyReset(t *testing.T) {
	tr := New(Options{
		PerDayLimit:   1000,
		WarnThreshold: 0.8,
	})

	tr.Add("s1", 900)
	if s := tr.Check("s1"); !s.SwitchModel {
		t.Fatalf("expected switch at 90%%: %+v", s)
	}

	// Inject a clock 25h forward to trigger the daily reset on the next Add.
	tr.SetClock(func() time.Time { return time.Now().Add(25 * time.Hour) })

	tr.Add("s1", 1)
	used, _ := tr.DailyUsage()
	if used != 1 {
		t.Fatalf("expected daily reset, got used=%d", used)
	}

	if s := tr.Check("s1"); !s.OK || s.Warning {
		t.Fatalf("expected OK after reset: %+v", s)
	}
}

func TestTracker_Unlimited(t *testing.T) {
	tr := New(Options{})
	tr.Add("s1", 999_999_999)
	if s := tr.Check("s1"); !s.OK || s.Warning || s.SwitchModel || s.HardStop {
		t.Fatalf("expected OK with no limits: %+v", s)
	}
}

func TestTracker_DefaultWarnThreshold(t *testing.T) {
	// Zero/negative WarnThreshold should normalise to 0.8.
	tr := New(Options{PerSessionLimit: 100})
	tr.Add("s1", 80)
	if s := tr.Check("s1"); !s.Warning {
		t.Fatalf("expected warn at 80%% via default threshold: %+v", s)
	}
}

func TestTracker_SessionUsage(t *testing.T) {
	tr := New(Options{PerSessionLimit: 1000})
	tr.Add("a", 300)
	tr.Add("b", 700)
	if used, lim := tr.SessionUsage("a"); used != 300 || lim != 1000 {
		t.Errorf("a: got (%d,%d) want (300,1000)", used, lim)
	}
	if used, lim := tr.SessionUsage("b"); used != 700 || lim != 1000 {
		t.Errorf("b: got (%d,%d) want (700,1000)", used, lim)
	}
}

func TestTracker_ResetClearsAll(t *testing.T) {
	tr := New(Options{PerSessionLimit: 1000, PerDayLimit: 5000})
	tr.Add("s1", 500)
	tr.Reset()
	if used, _ := tr.SessionUsage("s1"); used != 0 {
		t.Errorf("session not reset: %d", used)
	}
	if used, _ := tr.DailyUsage(); used != 0 {
		t.Errorf("daily not reset: %d", used)
	}
}
