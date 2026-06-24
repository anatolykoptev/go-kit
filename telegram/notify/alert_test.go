package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/telegram/notify"
)

// ---------------------------------------------------------------------------
// Severity enum — compile-time governance
// ---------------------------------------------------------------------------

// TestSeverity_ValidValues ensures each defined constant has the correct
// underlying string value dozor expects.
// Red-on-revert: if Critical changed to "crit", dozor would silently route it
// as Warning (its default fallback).
func TestSeverity_ValidValues(t *testing.T) {
	tests := []struct {
		sev  notify.Severity
		want string
	}{
		{notify.Critical, "critical"},
		{notify.Warning, "warning"},
		{notify.Info, "info"},
	}
	for _, tc := range tests {
		if string(tc.sev) != tc.want {
			t.Errorf("Severity %v = %q, want %q", tc.sev, string(tc.sev), tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// v4 payload shape — verified against dozor/gateway_alertmanager.go:25-41,100
// ---------------------------------------------------------------------------

// alertmanagerV4Payload mirrors the shape decoded at the fake webhook.
type alertmanagerV4Payload struct {
	Version string `json:"version"`
	Status  string `json:"status"`
	Alerts  []struct {
		Status      string            `json:"status"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"alerts"`
}

// TestAlertSink_V4PayloadShape verifies the wire format matches
// dozor/gateway_alertmanager.go:25-41 exactly. Revert buildV4Payload and this
// test fails (version != "4" rejected by dozor :100).
func TestAlertSink_V4PayloadShape(t *testing.T) {
	a := notify.Alert{
		Name:     "disk_almost_full",
		Severity: notify.Critical,
		Service:  "node",
		Instance: "krolik-box",
		Summary:  "/ is at 92%",
	}

	var captured alertmanagerV4Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	sink := notify.NewAlertSink(notify.WithWebhookURL(srv.URL))
	if err := sink.Alert(context.Background(), a); err != nil {
		t.Fatalf("Alert: unexpected error: %v", err)
	}

	// version must be "4" — dozor rejects anything else (:100)
	if captured.Version != "4" {
		t.Errorf("version=%q, want \"4\"", captured.Version)
	}
	if len(captured.Alerts) != 1 {
		t.Fatalf("alerts count=%d, want 1", len(captured.Alerts))
	}
	item := captured.Alerts[0]
	if item.Labels["alertname"] != "disk_almost_full" {
		t.Errorf("alertname=%q, want %q", item.Labels["alertname"], "disk_almost_full")
	}
	if item.Labels["severity"] != "critical" {
		t.Errorf("severity=%q, want %q", item.Labels["severity"], "critical")
	}
	if item.Labels["service"] != "node" {
		t.Errorf("service=%q, want %q", item.Labels["service"], "node")
	}
	if item.Labels["instance"] != "krolik-box" {
		t.Errorf("instance=%q, want %q", item.Labels["instance"], "krolik-box")
	}
	if item.Annotations["summary"] != "/ is at 92%" {
		t.Errorf("summary=%q, want %q", item.Annotations["summary"], "/ is at 92%")
	}
	if item.Status != "firing" {
		t.Errorf("item.status=%q, want %q", item.Status, "firing")
	}
}

// TestAlertSink_ResolvedSetsStatusResolved verifies that Alert.Resolved=true
// produces status="resolved" in the payload (dozor downgrades to Info at :178).
func TestAlertSink_ResolvedSetsStatusResolved(t *testing.T) {
	var captured alertmanagerV4Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	sink := notify.NewAlertSink(notify.WithWebhookURL(srv.URL))
	err := sink.Alert(context.Background(), notify.Alert{
		Name:     "disk_almost_full",
		Severity: notify.Warning,
		Resolved: true,
	})
	if err != nil {
		t.Fatalf("Alert: %v", err)
	}
	if captured.Status != "resolved" {
		t.Errorf("payload.status=%q, want %q", captured.Status, "resolved")
	}
	if captured.Alerts[0].Status != "resolved" {
		t.Errorf("item.status=%q, want %q", captured.Alerts[0].Status, "resolved")
	}
}

// TestAlertSink_ExtraLabelsAreMerged verifies caller-supplied Labels are
// present in the payload but cannot override alertname/severity.
func TestAlertSink_ExtraLabelsAreMerged(t *testing.T) {
	var captured alertmanagerV4Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	sink := notify.NewAlertSink(notify.WithWebhookURL(srv.URL))
	err := sink.Alert(context.Background(), notify.Alert{
		Name:     "test_alert",
		Severity: notify.Info,
		Labels: map[string]string{
			"env":       "staging",
			"alertname": "SHOULD_NOT_OVERRIDE", // must not win
		},
	})
	if err != nil {
		t.Fatalf("Alert: %v", err)
	}
	lbl := captured.Alerts[0].Labels
	if lbl["env"] != "staging" {
		t.Errorf("env=%q, want %q", lbl["env"], "staging")
	}
	// Standard label wins over caller-supplied duplicate.
	if lbl["alertname"] != "test_alert" {
		t.Errorf("alertname=%q, want %q; caller must not override standard labels", lbl["alertname"], "test_alert")
	}
}

// TestAlertSink_EmptyNameReturnsError verifies programming-mistake guard.
func TestAlertSink_EmptyNameReturnsError(t *testing.T) {
	sink := notify.NewAlertSink()
	err := sink.Alert(context.Background(), notify.Alert{Severity: notify.Warning})
	if err == nil {
		t.Fatal("expected error for empty Name, got nil")
	}
}

// ---------------------------------------------------------------------------
// Failure handling — best-effort contract
// ---------------------------------------------------------------------------

// TestAlertSink_WebhookFailureBumpsMetricAndReturnsNil verifies that delivery
// failures are best-effort: the counter is bumped, but nil is returned to the
// caller (a dozor outage must not fail the main path).
// Red-on-revert: remove the best-effort nil return and this test fails.
func TestAlertSink_WebhookFailureBumpsMetricAndReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "downstream exploded", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	m := metrics.NewRegistry()
	sink := notify.NewAlertSink(
		notify.WithWebhookURL(srv.URL),
		notify.WithAlertMetrics(m),
	)

	err := sink.Alert(context.Background(), notify.Alert{
		Name:     "service_down",
		Severity: notify.Critical,
	})
	if err != nil {
		t.Fatalf("Alert should return nil on delivery failure (best-effort), got: %v", err)
	}

	key := metrics.Label(notify.MetricAlertTotal, "severity", "critical", "result", "error")
	if got := m.Value(key); got != 1 {
		t.Errorf("error counter=%d, want 1", got)
	}
}

// TestAlertSink_SuccessBumpsOkCounter verifies the ok counter path.
// Red-on-revert: remove the Incr("...result=ok") call and this test fails.
func TestAlertSink_SuccessBumpsOkCounter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	m := metrics.NewRegistry()
	sink := notify.NewAlertSink(
		notify.WithWebhookURL(srv.URL),
		notify.WithAlertMetrics(m),
	)
	if err := sink.Alert(context.Background(), notify.Alert{
		Name:     "cpu_spike",
		Severity: notify.Warning,
	}); err != nil {
		t.Fatalf("Alert: %v", err)
	}

	key := metrics.Label(notify.MetricAlertTotal, "severity", "warning", "result", "ok")
	if got := m.Value(key); got != 1 {
		t.Errorf("ok counter=%d, want 1", got)
	}
}

// TestAlertSink_BearerTokenSent verifies the Authorization header is set when
// WithBearer is used.
func TestAlertSink_BearerTokenSent(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	sink := notify.NewAlertSink(
		notify.WithWebhookURL(srv.URL),
		notify.WithBearer("supersecret"),
	)
	_ = sink.Alert(context.Background(), notify.Alert{Name: "x", Severity: notify.Info})
	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("Authorization=%q, want Bearer prefix", authHeader)
	}
	if authHeader != "Bearer supersecret" {
		t.Errorf("Authorization=%q, want %q", authHeader, "Bearer supersecret")
	}
}

// TestAlertSink_NetworkFailureIsNilReturn verifies unreachable server also
// returns nil (best-effort) and bumps the error counter.
func TestAlertSink_NetworkFailureIsNilReturn(t *testing.T) {
	m := metrics.NewRegistry()
	sink := notify.NewAlertSink(
		notify.WithWebhookURL("http://127.0.0.1:1"), // nothing listens here
		notify.WithAlertMetrics(m),
	)
	err := sink.Alert(context.Background(), notify.Alert{
		Name:     "network_test",
		Severity: notify.Critical,
	})
	if err != nil {
		t.Fatalf("expected nil (best-effort), got: %v", err)
	}
	key := metrics.Label(notify.MetricAlertTotal, "severity", "critical", "result", "error")
	if got := m.Value(key); got != 1 {
		t.Errorf("error counter=%d, want 1", got)
	}
}

// TestAlertSink_ContentTypeIsJSON verifies the Content-Type header.
func TestAlertSink_ContentTypeIsJSON(t *testing.T) {
	var ct string
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct = r.Header.Get("Content-Type")
		count.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	sink := notify.NewAlertSink(notify.WithWebhookURL(srv.URL))
	_ = sink.Alert(context.Background(), notify.Alert{Name: "x", Severity: notify.Warning})
	if count.Load() != 1 {
		t.Fatalf("server called %d times, want 1", count.Load())
	}
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type=%q, want application/json", ct)
	}
}
