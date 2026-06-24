package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/anatolykoptev/go-kit/env"
	"github.com/anatolykoptev/go-kit/metrics"
)

// defaultWebhookURL is the canonical dozor alertmanager endpoint on the krolik box.
const defaultWebhookURL = "http://127.0.0.1:8765/webhook/alertmanager"

// defaultHTTPTimeout is used by the AlertSink HTTP client when none is supplied.
const defaultHTTPTimeout = 10 * time.Second

// MetricAlertTotal is the counter bumped on every AlertSink.Alert call.
// Labels: severity={critical,warning,info}, result={ok,error}.
const MetricAlertTotal = "notify_alert_total"

// Severity is a CLOSED enum of the three values dozor's alertmanager webhook
// understands. Using a named string type prevents callers from passing the
// known-dead "warn" or "page" strings — those values are un-typeable here.
type Severity string

const (
	// Critical maps to immediate push, hourly re-page in dozor's route tree.
	Critical Severity = "critical"
	// Warning maps to 5-minute group hold, 12-hour re-page.
	Warning Severity = "warning"
	// Info routes to the null receiver (logged, no Telegram message).
	Info Severity = "info"
)

// Alert is one ops alert. Its fields map 1:1 onto the alertmanager-v4 schema
// that dozor's gateway_alertmanager.go expects.
type Alert struct {
	// Name becomes labels.alertname.
	Name string
	// Severity governs dozor's routing tier. Uses the typed enum so "warn"/"page"
	// are impossible to emit.
	Severity Severity
	// Service becomes labels.service (used as the group key).
	Service string
	// Instance becomes labels.instance (inhibit key for storm suppression).
	Instance string
	// Summary becomes annotations.summary (the short human-readable headline).
	Summary string
	// Labels carries additional label key-value pairs merged on top of the
	// standard set. May be nil.
	Labels map[string]string
	// Resolved, when true, sets status="resolved" in the payload; dozor
	// downgrades resolved alerts to Info-level (gateway_alertmanager.go:178).
	Resolved bool
}

// AlertSink is the only sanctioned way for Go code to raise an ops alert.
// The only thing it can do is POST an alertmanager-v4 payload to dozor's
// webhook — there is no method that reaches api.telegram.org.
type AlertSink interface {
	// Alert fires an alert. It is best-effort by design: a dozor outage must
	// never fail the caller's main path (mirrors the leading-'-' semantics of
	// git-sync-notify.service). Delivery failures are logged and counted but
	// return nil to the caller.
	//
	// Returns a non-nil error only on programming mistakes (e.g. empty Name).
	Alert(ctx context.Context, a Alert) error
}

// AlertOption configures an alertSink.
type AlertOption func(*alertSink)

// WithWebhookURL overrides the dozor webhook URL.
func WithWebhookURL(url string) AlertOption {
	return func(s *alertSink) { s.webhookURL = url }
}

// WithBearer sets the Authorization bearer token sent in each request.
func WithBearer(token string) AlertOption {
	return func(s *alertSink) { s.bearerToken = token }
}

// WithAlertMetrics wires a metrics registry into the sink.
// The sink bumps notify_alert_total{severity,result} on every call.
func WithAlertMetrics(m *metrics.Registry) AlertOption {
	return func(s *alertSink) { s.m = m }
}

// WithHTTPClient replaces the default 10-second-timeout HTTP client.
func WithHTTPClient(c *http.Client) AlertOption {
	return func(s *alertSink) { s.client = c }
}

// NewAlertSink builds an AlertSink with the supplied options.
// Use NewAlertSinkFromEnv for production code; this constructor is primarily
// for tests that need to inject a custom HTTP client or URL.
func NewAlertSink(opts ...AlertOption) AlertSink {
	s := &alertSink{
		webhookURL: defaultWebhookURL,
		client:     &http.Client{Timeout: defaultHTTPTimeout},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// NewAlertSinkFromEnv reads configuration from the environment and returns a
// production-ready AlertSink.
//
//   - DOZOR_WEBHOOK_URL  (default: http://127.0.0.1:8765/webhook/alertmanager)
//   - DOZOR_WEBHOOK_TOKEN (optional bearer token)
//
// m may be nil; nil registry makes counters no-ops.
func NewAlertSinkFromEnv(m *metrics.Registry) AlertSink {
	return NewAlertSink(
		WithWebhookURL(env.Str("DOZOR_WEBHOOK_URL", defaultWebhookURL)),
		WithBearer(env.Str("DOZOR_WEBHOOK_TOKEN", "")),
		WithAlertMetrics(m),
	)
}

// alertSink is the concrete AlertSink implementation.
type alertSink struct {
	webhookURL  string
	bearerToken string
	client      *http.Client
	m           *metrics.Registry
}

// Alert implements AlertSink.
func (s *alertSink) Alert(ctx context.Context, a Alert) error {
	if a.Name == "" {
		return errors.New("notify: Alert.Name must not be empty")
	}

	payload, err := buildV4Payload(a)
	if err != nil {
		return fmt.Errorf("notify: build payload: %w", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("notify: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.bearerToken)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		slog.Warn("notify: alert webhook delivery failed",
			slog.String("alert", a.Name),
			slog.String("severity", string(a.Severity)),
			slog.Any("error", err))
		s.m.Incr(metrics.Label(MetricAlertTotal, "severity", string(a.Severity), "result", "error"))
		return nil // best-effort: caller must not fail on dozor outage
	}
	defer resp.Body.Close() //nolint:errcheck // response body drain; read-only

	if resp.StatusCode >= http.StatusBadRequest {
		slog.Warn("notify: alert webhook non-2xx",
			slog.String("alert", a.Name),
			slog.String("severity", string(a.Severity)),
			slog.Int("status", resp.StatusCode))
		s.m.Incr(metrics.Label(MetricAlertTotal, "severity", string(a.Severity), "result", "error"))
		return nil // best-effort
	}

	s.m.Incr(metrics.Label(MetricAlertTotal, "severity", string(a.Severity), "result", "ok"))
	return nil
}

// ---------------------------------------------------------------------------
// v4 payload construction
// ---------------------------------------------------------------------------

// alertmanagerV4Payload is the alertmanager webhook v4 shape that
// dozor/cmd/dozor/gateway_alertmanager.go expects at POST /webhook/alertmanager.
type alertmanagerV4Payload struct {
	Version string                  `json:"version"`
	Status  string                  `json:"status"`
	Alerts  []alertmanagerV4Item    `json:"alerts"`
}

// alertmanagerV4Item is one alert inside the v4 payload.
type alertmanagerV4Item struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

// standardLabelCount is the number of fixed labels AlertSink stamps onto every
// payload: alertname, severity, service, instance.
const standardLabelCount = 4

// buildV4Payload converts an Alert into the alertmanager-v4 wire format.
// Verified against dozor/cmd/dozor/gateway_alertmanager.go:25-41 and :100-127.
func buildV4Payload(a Alert) (alertmanagerV4Payload, error) {
	status := "firing"
	if a.Resolved {
		status = "resolved"
	}

	// Merge caller-supplied extra labels, then stamp the standard ones so
	// they can never be overridden by the caller.
	labels := make(map[string]string, len(a.Labels)+standardLabelCount)
	for k, v := range a.Labels {
		labels[k] = v
	}
	labels["alertname"] = a.Name
	labels["severity"] = string(a.Severity)
	if a.Service != "" {
		labels["service"] = a.Service
	}
	if a.Instance != "" {
		labels["instance"] = a.Instance
	}

	annotations := map[string]string{}
	if a.Summary != "" {
		annotations["summary"] = a.Summary
	}

	return alertmanagerV4Payload{
		Version: "4",
		Status:  status,
		Alerts: []alertmanagerV4Item{
			{
				Status:      status,
				Labels:      labels,
				Annotations: annotations,
			},
		},
	}, nil
}
