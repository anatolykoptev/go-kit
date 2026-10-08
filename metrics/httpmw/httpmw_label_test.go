package httpmw_test

import (
	"net/http"
	"net/http/httptest"
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
// RED-on-mutation: in Middleware (metrics/httpmw/httpmw.go) go back to
// histVec.WithLabelValues(r.Method, path).Observe(elapsed); the "/a\xff"
// request panics with "is not valid UTF-8".
func TestMiddleware_HistogramPathMatchesCounterAndNeverPanics(t *testing.T) {
	promReg := prometheus.NewRegistry()
	reg := metrics.NewPrometheusRegistry("t_histpath")
	mw := httpmw.Middleware(reg, "thpath",
		httpmw.WithDurationHistogram(promReg),
		httpmw.WithPathLabel(func(r *http.Request) string {
			if r.URL.Path == "/items/7" {
				return "GET /items/{id}"
			}
			return r.URL.Path
		}))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items/7", nil))
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
	if len(paths) != 1 || paths[0] != "GET /items/{id}" {
		t.Fatalf("histogram path labels = %q, want the counter's [GET /items/{id}]", paths)
	}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/a%ff", nil)) // must not panic
}
