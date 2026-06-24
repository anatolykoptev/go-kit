// Package notify provides two opinionated, non-conflatable notification sinks
// for Go services in the krolik fleet.
//
// # Two sinks, one rule each
//
// AlertSink is the ONLY sanctioned way for Go code to raise an ops alert.
// It speaks directly to dozor's alertmanager-v4 webhook and exposes no method
// that can reach api.telegram.org. This makes the governed notification path
// non-bypassable by construction: a caller cannot emit a raw Telegram message
// through AlertSink, and the Severity enum makes the known-dead "warn"/"page"
// vocabulary un-typeable at compile time.
//
// ProductSink delivers per-event product notifications (new bounty, KOL
// mention, etc.) via an own bot token, with bounded-rate fan-out, retry, and
// dead-letter. It wraps the existing telegram/broadcast.Pacer and
// telegram/tgapi5.BotSender; HTML→plaintext fallback is applied per chunk
// (borrowed from go-hully's bot.go:283 pattern).
//
// # Metric names
//
// Both sinks bump counters on every attempt so failures are never silent:
//
//   - AlertSink:   "notify_alert_total{severity=<level>,result=<ok|error>}"
//   - ProductSink: "notify_product_total{result=<sent|failed>}"
//
// Pass a *metrics.Registry from metrics.FromEnv to wire into Prometheus.
// nil is accepted on every constructor (counters become no-ops).
//
// # Env vars
//
//	DOZOR_WEBHOOK_URL   AlertSink default: http://127.0.0.1:8765/webhook/alertmanager
//	DOZOR_WEBHOOK_TOKEN AlertSink bearer token (optional; loopback usually unenforced)
//	TELEGRAM_BOT_TOKEN  ProductSink bot token (required)
//	<PREFIX>_NOTIFY_CHAT_ID  ProductSink default chat ID, e.g. BOUNTY_NOTIFY_CHAT_ID
package notify
