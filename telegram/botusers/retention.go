package botusers

import (
	"context"
	"log/slog"
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
	return func(c *Config) {
		c.inactivityWindow = d
		c.inactivityWindowSet = true
	}
}

// NewRetentionSweeper creates a RetentionSweeper. opts are the standard
// botusers functional options; WithBotID is required (Run returns
// ErrBotIDRequired if not set). WithSweepInterval controls sweep frequency
// (default 24h). WithInactivityWindow controls the deletion threshold
// (default 90 days).
func NewRetentionSweeper(store Store, opts ...Option) *RetentionSweeper {
	cfg := defaultConfig(opts)
	inactivityWindow := cfg.inactivityWindow
	// When WithInactivityWindow was not called, default to 90 days.
	// When called explicitly with 0, use 0 (delete all users on each sweep).
	if !cfg.inactivityWindowSet {
		inactivityWindow = 90 * 24 * time.Hour
	}
	return &RetentionSweeper{
		store:            store,
		cfg:              cfg,
		inactivityWindow: inactivityWindow,
	}
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
	n, err := s.store.DeleteInactive(ctx, bid, s.inactivityWindow)
	if err != nil {
		slog.Warn("bot_users sweep failed", "bot_id", bid, "err", err)
		if s.cfg.Metrics != nil {
			s.cfg.Metrics.Incr("bot_users.sweep_error")
		}
		return
	}
	if s.cfg.Metrics != nil {
		s.cfg.Metrics.Gauge("bot_users.last_sweep_deleted", float64(n))
	}
}
