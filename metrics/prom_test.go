package metrics

import (
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestParseLabeled(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantKeys []string
		wantVals []string
	}{
		{"wp_rest_calls", "wp_rest_calls", nil, nil},
		{"rpc{method=login}", "rpc", []string{"method"}, []string{"login"}},
		{"rpc{service=auth,method=login}", "rpc", []string{"service", "method"}, []string{"auth", "login"}},
		{"malformed{", "malformed{", nil, nil},                    // invalid → treat as plain
		{"rpc{method=}", "rpc", []string{"method"}, []string{""}}, // empty value allowed
		{"rpc{method=,timeout=5s}", "rpc", []string{"method", "timeout"}, []string{"", "5s"}},
	}
	for _, c := range cases {
		gotName, gotKeys, gotVals := parseLabeled(c.in)
		if gotName != c.wantName {
			t.Errorf("%q: name = %q, want %q", c.in, gotName, c.wantName)
		}
		if len(gotKeys) != len(c.wantKeys) {
			t.Errorf("%q: keys = %v, want %v", c.in, gotKeys, c.wantKeys)
			continue
		}
		for i := range gotKeys {
			if gotKeys[i] != c.wantKeys[i] || gotVals[i] != c.wantVals[i] {
				t.Errorf("%q: [%d] = (%q,%q), want (%q,%q)", c.in, i, gotKeys[i], gotVals[i], c.wantKeys[i], c.wantVals[i])
			}
		}
	}
}

func TestNewPrometheusRegistry_Constructs(t *testing.T) {
	reg := NewPrometheusRegistry("testsvc")
	if reg == nil {
		t.Fatal("nil registry")
	}
	if reg.promBridge == nil {
		t.Fatal("promBridge not initialized")
	}
	if reg.promBridge.namespace != "testsvc" {
		t.Fatalf("namespace = %q, want %q", reg.promBridge.namespace, "testsvc")
	}
}

func TestMetricsHandler_Serves(t *testing.T) {
	h := MetricsHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()
	_ = prometheus.DefaultRegisterer
}

func TestNewPrometheusRegistry_PanicsOnBadNamespace(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	NewPrometheusRegistry("bad-name")
}
