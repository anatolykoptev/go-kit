package botusers

import (
	"log/slog"
	"time"
)

// Config holds the resolved options for a Store or RetentionSweeper.
// Callers should not construct this directly; use the With* functions.
type Config struct {
	// BotID is the default bot identifier used when the per-call botID
	// argument is empty.
	BotID string
	// UseEventsTable enables the optional bot_user_events append-log.
	UseEventsTable bool
	// StoreIP enables persisting the client IP from Observation.IP.
	StoreIP bool
	// Privacy controls the data-storage mode.
	Privacy Privacy
	// SweepInterval is the period used by RetentionSweeper.Run.
	SweepInterval time.Duration
	// InactivityWindow is the duration used by RetentionSweeper to identify
	// stale users. 0 means "delete all" when inactivityWindowSet is true.
	inactivityWindow time.Duration
	// inactivityWindowSet is true when WithInactivityWindow was called explicitly.
	// Distinguishes WithInactivityWindow(0) from "not set" (defaults to 90d).
	inactivityWindowSet bool
	// Clock returns the current time. Defaults to time.Now.
	Clock func() time.Time
	// Metrics is an optional prometheus-compatible counter/gauge emitter.
	// The package calls Metrics.Incr(name) and Metrics.Gauge(name, v)
	// when non-nil. Pass nil to disable.
	Metrics MetricsEmitter
	// appliedOptions records the names of all options applied to this Config.
	// Used to warn when a Store option is passed to NewRetentionSweeper and vice versa.
	appliedOptions []string
}

// Option is a functional option for Config.
type Option func(*Config)

// WithBotID sets the default bot identifier. Individual call arguments
// override this value when non-empty.
func WithBotID(id string) Option {
	return func(c *Config) { c.BotID = id; recordOption(c, "WithBotID") }
}

// WithEventsTable enables the optional bot_user_events append-log table.
// When enabled every Upsert writes an additional event row.
// Default: false (events table is not used).
func WithEventsTable(enabled bool) Option {
	return func(c *Config) { c.UseEventsTable = enabled; recordOption(c, "WithEventsTable") }
}

// WithStoreIP enables persisting the client IP address from Observation.IP.
// When false (default) the IP field is ignored and never stored.
// Subsequent upserts with this option disabled also clear any prior IP stored
// for the user — disabling WithStoreIP actively purges the value.
func WithStoreIP(enabled bool) Option {
	return func(c *Config) { c.StoreIP = enabled; recordOption(c, "WithStoreIP") }
}

// WithPrivacy sets the data-storage privacy mode. Default: SoftOptIn.
func WithPrivacy(p Privacy) Option {
	return func(c *Config) { c.Privacy = p; recordOption(c, "WithPrivacy") }
}

// WithSweepInterval sets the period for RetentionSweeper.Run.
// Default: 24 hours.
func WithSweepInterval(d time.Duration) Option {
	return func(c *Config) { c.SweepInterval = d; recordOption(c, "WithSweepInterval") }
}

// WithClock overrides the time source. Useful for deterministic tests.
// Default: time.Now.
func WithClock(fn func() time.Time) Option {
	return func(c *Config) { c.Clock = fn; recordOption(c, "WithClock") }
}

// WithMetrics attaches a MetricsEmitter. The package calls its methods on
// key events. Pass nil to disable (the default).
func WithMetrics(m MetricsEmitter) Option {
	return func(c *Config) { c.Metrics = m; recordOption(c, "WithMetrics") }
}

// MetricsEmitter is the minimal interface the package uses for emitting
// operational metrics. *metrics.Registry from go-kit/metrics satisfies
// this interface when it exposes Incr and Gauge.
type MetricsEmitter interface {
	// Incr increments the named counter by 1.
	Incr(name string)
	// Gauge sets the named gauge to value.
	Gauge(name string, value float64)
}

// defaultConfig returns a Config with production-safe defaults applied.
func defaultConfig(opts []Option) Config {
	cfg := Config{
		Privacy:       SoftOptIn,
		SweepInterval: 24 * time.Hour,
		Clock:         time.Now,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// recordOption appends name to cfg.appliedOptions so constructors can detect
// cross-domain option misuse.
func recordOption(cfg *Config, name string) {
	cfg.appliedOptions = append(cfg.appliedOptions, name)
}

// warnCrossDomainOptions logs a warning for each option that belongs to the
// wrong domain. domain is "store" or "sweeper".
func warnCrossDomainOptions(cfg Config, domain string) {
	// Options that belong only to the sweeper (not to Store).
	sweeperOnly := map[string]bool{
		"WithSweepInterval":    true,
		"WithInactivityWindow": true,
	}
	// Options that belong only to the Store (not to sweeper).
	storeOnly := map[string]bool{
		"WithStoreIP":     true,
		"WithEventsTable": true,
		"WithPrivacy":     true,
	}
	for _, name := range cfg.appliedOptions {
		if domain == "store" && sweeperOnly[name] {
			slog.Warn("botusers: option intended for RetentionSweeper passed to pg.New; it has no effect",
				"option", name)
		} else if domain == "sweeper" && storeOnly[name] {
			slog.Warn("botusers: option intended for pg.Store passed to NewRetentionSweeper; it has no effect",
				"option", name)
		}
	}
}

// WarnCrossDomainOptions is the exported entry point for sub-packages (e.g. pg)
// to trigger cross-domain option validation. domain is "store" or "sweeper".
func (c Config) WarnCrossDomainOptions(domain string) {
	warnCrossDomainOptions(c, domain)
}

// resolveBot returns botID if non-empty, falls back to cfg.BotID, or
// returns ("", ErrBotIDRequired).
func resolveBot(botID string, cfg Config) (string, error) {
	if botID != "" {
		return botID, nil
	}
	if cfg.BotID != "" {
		return cfg.BotID, nil
	}
	return "", ErrBotIDRequired
}
