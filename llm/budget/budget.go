// Package budget tracks LLM token usage per session and per day with
// soft warning, model-switch, and hard-stop thresholds. Pure stdlib;
// safe for concurrent use; daily counter resets at UTC midnight.
//
// Typical wiring in an agent loop:
//
//	tracker := budget.New(budget.Options{
//	    PerSessionLimit: 50_000,
//	    PerDayLimit:     1_000_000,
//	    WarnThreshold:   0.8,
//	    SwitchModel:     "gemini-2.5-flash-lite",
//	})
//	// before each LLM call:
//	if s := tracker.Check(sessionKey); s.HardStop {
//	    return errors.New(s.Message)
//	} else if s.SwitchModel {
//	    model = tracker.Options.SwitchModel
//	}
//	// after the call:
//	tracker.Add(sessionKey, usage.TotalTokens)
package budget

import (
	"fmt"
	"sync"
	"time"
)

// Options defines per-session and per-day token limits.
//
// A zero limit means "unbounded" — Check returns OK regardless of usage.
// WarnThreshold defaults to 0.8 (80%) when <= 0; SwitchModel is purely
// advisory (set on Status when usage >= 90% of any limit).
type Options struct {
	// PerSessionLimit caps tokens per session key. Zero = unbounded.
	PerSessionLimit int

	// PerDayLimit caps tokens across all sessions per UTC day. Zero = unbounded.
	PerDayLimit int

	// WarnThreshold is the fraction of the limit that flips Status.Warning.
	// Defaults to 0.8 if <= 0.
	WarnThreshold float64

	// SwitchModel is the cheaper model name to use when Status.SwitchModel
	// is true (>= 90% of any limit). Purely informational; the caller
	// decides whether to honor it.
	SwitchModel string
}

// Status reports the current budget state for a session.
type Status struct {
	// OK is true unless any limit is fully exhausted.
	OK bool

	// Warning is true at or above Options.WarnThreshold of any limit.
	Warning bool

	// SwitchModel is true at or above 90% of any limit — caller
	// should consider routing to Options.SwitchModel.
	SwitchModel bool

	// HardStop is true at or above 100% of any limit — caller MUST
	// refuse the request to avoid blowing the budget.
	HardStop bool

	// Message is a human-readable status line ("session tokens at
	// 85% (850/1000)" or "daily token budget exceeded (1000/1000)").
	Message string
}

// Tracker tracks token usage per session key and across all sessions
// per UTC day. Safe for concurrent use.
type Tracker struct {
	Options Options

	mu         sync.Mutex
	sessions   map[string]int
	dailyTotal int
	dailyReset time.Time
	nowFunc    func() time.Time
}

// New creates a tracker with the given options.
func New(opts Options) *Tracker {
	if opts.WarnThreshold <= 0 {
		opts.WarnThreshold = 0.8
	}
	return &Tracker{
		Options:    opts,
		sessions:   make(map[string]int),
		dailyReset: startOfDay(time.Now()),
		nowFunc:    time.Now,
	}
}

// Add records token usage for a session. Resets the daily counter
// when the UTC day boundary has passed since the last Add or Check.
func (t *Tracker) Add(sessionKey string, tokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFunc()
	if now.After(t.dailyReset.Add(24 * time.Hour)) {
		t.dailyTotal = 0
		t.dailyReset = startOfDay(now)
	}

	t.sessions[sessionKey] += tokens
	t.dailyTotal += tokens
}

// Check returns the budget status for a session. Pure read — does not
// mutate counters.
func (t *Tracker) Check(sessionKey string) Status {
	t.mu.Lock()
	defer t.mu.Unlock()

	status := Status{OK: true}

	if t.Options.PerSessionLimit > 0 {
		used := t.sessions[sessionKey]
		ratio := float64(used) / float64(t.Options.PerSessionLimit)
		if ratio >= 1.0 {
			status.OK = false
			status.HardStop = true
			status.Message = fmt.Sprintf("session token budget exceeded (%d/%d)", used, t.Options.PerSessionLimit)
			return status
		}
		if ratio >= 0.9 {
			status.SwitchModel = true
			status.Warning = true
			status.Message = fmt.Sprintf("session tokens at %.0f%% (%d/%d)", ratio*100, used, t.Options.PerSessionLimit)
		} else if ratio >= t.Options.WarnThreshold {
			status.Warning = true
			status.Message = fmt.Sprintf("session tokens at %.0f%% (%d/%d)", ratio*100, used, t.Options.PerSessionLimit)
		}
	}

	if t.Options.PerDayLimit > 0 {
		ratio := float64(t.dailyTotal) / float64(t.Options.PerDayLimit)
		if ratio >= 1.0 {
			status.OK = false
			status.HardStop = true
			status.Message = fmt.Sprintf("daily token budget exceeded (%d/%d)", t.dailyTotal, t.Options.PerDayLimit)
			return status
		}
		if ratio >= 0.9 {
			status.SwitchModel = true
			status.Warning = true
			if status.Message == "" {
				status.Message = fmt.Sprintf("daily tokens at %.0f%% (%d/%d)", ratio*100, t.dailyTotal, t.Options.PerDayLimit)
			}
		} else if ratio >= t.Options.WarnThreshold {
			status.Warning = true
			if status.Message == "" {
				status.Message = fmt.Sprintf("daily tokens at %.0f%% (%d/%d)", ratio*100, t.dailyTotal, t.Options.PerDayLimit)
			}
		}
	}

	return status
}

// DailyUsage returns the current daily total and the configured limit.
// Useful for /metrics emission or status commands.
func (t *Tracker) DailyUsage() (used, limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dailyTotal, t.Options.PerDayLimit
}

// SessionUsage returns the current session total and the configured
// session limit.
func (t *Tracker) SessionUsage(key string) (used, limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessions[key], t.Options.PerSessionLimit
}

// Reset clears all session and daily counters. Intended for tests
// and operator commands; production callers should rely on the
// automatic daily reset.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions = make(map[string]int)
	t.dailyTotal = 0
	t.dailyReset = startOfDay(t.nowFunc())
}

// SetClock injects a clock function for tests. Production callers
// should leave the default (time.Now). Concurrent-safe.
func (t *Tracker) SetClock(now func() time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now == nil {
		now = time.Now
	}
	t.nowFunc = now
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
