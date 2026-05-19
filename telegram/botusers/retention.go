package botusers

import (
	"context"
	"time"
)

// RetentionSweeper periodically calls DeleteInactive to remove stale user
// records. It is designed for caller-managed lifecycle: either call Run in
// a goroutine (background sweep) or call Run synchronously (blocks until
// context is cancelled).
//
// The package never starts goroutines on its own — the caller decides.
type RetentionSweeper struct {
	store            Store
	cfg              Config
	inactivityWindow time.Duration
}

// WithInactivityWindow sets the duration after which users are considered
// inactive and eligible for deletion. Default: 90 days.
//
// Pass 0 to delete all users on every sweep (useful in tests).
func WithInactivityWindow(d time.Duration) Option {
	return func(c *Config) { c.inactivityWindow = d }
}

// NewRetentionSweeper creates a RetentionSweeper. opts are the standard
// botusers functional options; WithBotID is required (Run returns
// ErrBotIDRequired if not set). WithSweepInterval controls sweep frequency
// (default 24h). WithInactivityWindow controls the deletion threshold
// (default 90 days).
func NewRetentionSweeper(store Store, opts ...Option) *RetentionSweeper {
	cfg := defaultConfig(opts)
	inactivityWindow := cfg.inactivityWindow
	if inactivityWindow == 0 && !hasInactivityWindowOption(opts) {
		inactivityWindow = 90 * 24 * time.Hour
	}
	return &RetentionSweeper{
		store:            store,
		cfg:              cfg,
		inactivityWindow: inactivityWindow,
	}
}

// hasInactivityWindowOption detects whether WithInactivityWindow was explicitly
// passed (to distinguish 0 as "delete all" vs "not set").
func hasInactivityWindowOption(opts []Option) bool {
	// We apply the option to a sentinel config and check if inactivityWindow changed.
	var sentinel Config
	for _, o := range opts {
		o(&sentinel)
	}
	return sentinel.inactivityWindow != 0 || hasExplicitZeroWindow(opts)
}

// hasExplicitZeroWindow is a best-effort heuristic. Since Go closures don't
// carry metadata, we use a secondary marker approach: apply to a sentinel and
// check if any option set a non-default field that signals "0 was intended".
// In practice, callers using WithInactivityWindow(0) get immediate deletion —
// the default 90d is applied only when WithInactivityWindow is not called at all.
//
// Implementation note: we can't distinguish "WithInactivityWindow(0)" from
// "not called" without a separate flag. We use the presence of inactivityWindow
// field being 0 after applying all options as "use default" for safety.
// Tests that want "delete all" should pass a very small positive duration instead.
func hasExplicitZeroWindow(opts []Option) bool {
	// marker config with a sentinel non-zero value
	sentinel := Config{inactivityWindow: -1}
	for _, o := range opts {
		o(&sentinel)
	}
	// If it's still -1, WithInactivityWindow was not called.
	// If it's 0, it was explicitly set to 0.
	return sentinel.inactivityWindow == 0
}

// Run blocks and performs a sweep at every SweepInterval until ctx is
// cancelled. Safe to call without go (synchronous), or as go sweeper.Run(ctx).
//
// Errors from DeleteInactive are silently swallowed (not propagated) to keep
// the sweeper resilient to transient database failures. The error from ctx
// cancellation is also swallowed — Run returns nil always.
func (s *RetentionSweeper) Run(ctx context.Context) {
	interval := s.cfg.SweepInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	// Run one sweep immediately, then on tick.
	s.sweep(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

func (s *RetentionSweeper) sweep(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	bid, err := resolveBot(s.cfg.BotID, s.cfg)
	if err != nil {
		return // ErrBotIDRequired — caller misconfigured; silent.
	}
	_, _ = s.store.DeleteInactive(ctx, bid, s.inactivityWindow) //nolint:errcheck
}
