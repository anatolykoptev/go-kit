package botusers

import (
	"context"
	"fmt"
)

// Gauge name constants. Callers use these as metric names when configuring
// dashboards, alerts, and Prometheus relabelling rules.
//
// Format: "bot_users.<metric>" to match the plan's specification.
const (
	// GaugeTotal is emitted as the total count of stored users for the bot.
	GaugeTotal = "bot_users.total"
	// GaugeActive1D is the count of users active in the last 24 hours.
	GaugeActive1D = "bot_users.active_1d"
	// GaugeActive7D is the count of users active in the last 7 days.
	GaugeActive7D = "bot_users.active_7d"
	// GaugeActive30D is the count of users active in the last 30 days.
	GaugeActive30D = "bot_users.active_30d"
)

// EmitGauges calls Aggregate for botID against store, then emits the four
// standard gauges (total, active_1d, active_7d, active_30d) via emitter.
//
// The caller decides how often to call EmitGauges — the package never runs
// a goroutine or ticker internally. A typical pattern is to call it inside a
// periodic health-check or metrics-collection loop.
//
// ctx is passed to Aggregate and may be used for deadline propagation. Pass
// context.Background() if no deadline is required.
//
// emitter must not be nil. Pass a no-op implementation if metrics are disabled.
func EmitGauges(ctx context.Context, botID string, store Store, emitter MetricsEmitter) error {
	if ctx == nil {
		ctx = context.Background()
	}
	agg, err := store.Aggregate(ctx, botID)
	if err != nil {
		return fmt.Errorf("botusers: EmitGauges aggregate: %w", err)
	}
	emitter.Gauge(GaugeTotal, float64(agg.Total))
	emitter.Gauge(GaugeActive1D, float64(agg.Active1D))
	emitter.Gauge(GaugeActive7D, float64(agg.Active7D))
	emitter.Gauge(GaugeActive30D, float64(agg.Active30D))
	return nil
}
