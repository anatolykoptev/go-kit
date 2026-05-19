package botusers

import "time"

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
	// stale users. 0 means "not set" (NewRetentionSweeper uses 90d default).
	inactivityWindow time.Duration
	// Clock returns the current time. Defaults to time.Now.
	Clock func() time.Time
	// Metrics is an optional prometheus-compatible counter/gauge emitter.
	// The package calls Metrics.Incr(name) and Metrics.Gauge(name, v)
	// when non-nil. Pass nil to disable.
	Metrics MetricsEmitter
}

// Option is a functional option for Config.
type Option func(*Config)

// WithBotID sets the default bot identifier. Individual call arguments
// override this value when non-empty.
func WithBotID(id string) Option {
	return func(c *Config) { c.BotID = id }
}

// WithEventsTable enables the optional bot_user_events append-log table.
// When enabled every Upsert writes an additional event row.
// Default: false (events table is not used).
func WithEventsTable(enabled bool) Option {
	return func(c *Config) { c.UseEventsTable = enabled }
}

// WithStoreIP enables persisting the client IP address from Observation.IP.
// When false (default) the IP field is ignored and never stored.
// Subsequent upserts with this option disabled also clear any prior IP stored
// for the user — disabling WithStoreIP actively purges the value.
func WithStoreIP(enabled bool) Option {
	return func(c *Config) { c.StoreIP = enabled }
}

// WithPrivacy sets the data-storage privacy mode. Default: SoftOptIn.
func WithPrivacy(p Privacy) Option {
	return func(c *Config) { c.Privacy = p }
}

// WithSweepInterval sets the period for RetentionSweeper.Run.
// Default: 24 hours.
func WithSweepInterval(d time.Duration) Option {
	return func(c *Config) { c.SweepInterval = d }
}

// WithClock overrides the time source. Useful for deterministic tests.
// Default: time.Now.
func WithClock(fn func() time.Time) Option {
	return func(c *Config) { c.Clock = fn }
}

// WithMetrics attaches a MetricsEmitter. The package calls its methods on
// key events. Pass nil to disable (the default).
func WithMetrics(m MetricsEmitter) Option {
	return func(c *Config) { c.Metrics = m }
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
