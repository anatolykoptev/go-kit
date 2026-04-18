package metrics_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
)

func BenchmarkIncr(b *testing.B) {
	r := metrics.NewRegistry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Incr("requests")
	}
}

func BenchmarkGauge_Set(b *testing.B) {
	r := metrics.NewRegistry()
	g := r.Gauge("latency")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Set(42.0)
	}
}

func BenchmarkIncr_NilRegistry(b *testing.B) {
	var r *metrics.Registry
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Incr("x")
	}
}

func BenchmarkGauge_Set_NilRegistry(b *testing.B) {
	var r *metrics.Registry
	g := r.Gauge("x")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Set(42.0)
	}
}
