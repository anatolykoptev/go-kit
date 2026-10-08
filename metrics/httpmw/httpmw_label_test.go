package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/metrics/httpmw"
	"github.com/prometheus/client_golang/prometheus"
)

// The duration histogram is observed directly on a prometheus vec, beside
// the Label-built counter. Both must carry the same path value, and a path
// the vec rejects (invalid UTF-8 from r.URL.Path) must be dropped and
// counted, not panic the request.
//
// RED-on-mutation (metrics/httpmw/httpmw.go, Middleware): go back to
// histVec.WithLabelValues(r.Method, path) — the "/a\xff" request panics;
// drop metrics.SanitizeLabelValue(path) — the "GET /a,b" histogram label
// stays "GET /a,b" while the counter says "GET /a_b"; delete
// metrics.RecordDroppedSample() — the dropped-sample delta stays 0.
func TestMiddleware_HistogramPathMatchesCounterAndNeverPanics(t *testing.T) {
	promReg := prometheus.NewRegistry()
	reg := metrics.NewPrometheusRegistry("t_histpath")
	mw := httpmw.Middleware(reg, "thpath",
		httpmw.WithDurationHistogram(promReg),
		httpmw.WithPathLabel(func(r *http.Request) string {
			switch r.URL.Path {
			case "/items/7":
				return "GET /items/{id}"
			case "/ab":
				return "GET /a,b"
			}
			return r.URL.Path
		}))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items/7", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ab", nil))
	if v := reg.Value(metrics.Label("thpath_requests_total", "method", "GET", "path", "GET /items/{id}", "code", "200")); v != 1 {
		t.Fatalf("counter for the route pattern = %d, want 1", v)
	}
	mfs, err := promReg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, mf := range mfs {
		if strings.Contains(mf.GetName(), "request_duration_histogram") {
			for _, m := range mf.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "path" {
						paths = append(paths, l.GetValue())
					}
				}
			}
		}
	}
	slices.Sort(paths)
	if want := []string{"GET /a_b", "GET /items/{id}"}; !slices.Equal(paths, want) {
		t.Fatalf("histogram path labels = %q, want the counter's %q", paths, want)
	}
	if v := reg.Value(metrics.Label("thpath_requests_total", "method", "GET", "path", "GET /a,b", "code", "200")); v != 1 {
		t.Fatalf("counter for GET /a,b = %d, want 1", v)
	}

	before := droppedSamples(t)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/a%ff", nil)) // must not panic
	// The histogram drop, plus the bridge's counter and gauge drops.
	if d := droppedSamples(t) - before; d < 3 {
		t.Fatalf("an invalid UTF-8 path must be dropped and counted on every series, dropped delta = %v, want >= 3", d)
	}
}

func droppedSamples(t *testing.T) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() == "gokit_metrics_dropped_samples_total" {
			return mf.GetMetric()[0].GetCounter().GetValue()
		}
	}
	return 0
}
