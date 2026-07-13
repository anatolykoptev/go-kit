package botusers_test

import (
	"context"
	"testing"

	"github.com/anatolykoptev/go-kit/telegram/botusers"
	"github.com/anatolykoptev/go-kit/telegram/botusers/botuserstest"
)

func TestEmitGauges_CallsEmitter(t *testing.T) {
	store := botuserstest.NewMemStore()
	emitter := &fakeEmitter{gauges: map[string]float64{}}

	// EmitGauges should call Aggregate and emit 4 gauges.
	if err := botusers.EmitGauges(context.Background(), "bot1", store, emitter); err != nil {
		// Empty store — aggregate returns zeros; error unlikely.
		t.Fatalf("EmitGauges: %v", err)
	}

	for _, name := range []string{
		botusers.GaugeTotal,
		botusers.GaugeActive1D,
		botusers.GaugeActive7D,
		botusers.GaugeActive30D,
	} {
		if _, ok := emitter.gauges[name]; !ok {
			t.Errorf("EmitGauges did not emit gauge %q", name)
		}
	}
}

func TestGaugeConstants_Unique(t *testing.T) {
	names := []string{
		botusers.GaugeTotal,
		botusers.GaugeActive1D,
		botusers.GaugeActive7D,
		botusers.GaugeActive30D,
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("duplicate gauge constant: %q", n)
		}
		seen[n] = true
		if n == "" {
			t.Error("gauge constant must not be empty string")
		}
	}
}

type fakeEmitter struct {
	incrs  []string
	gauges map[string]float64
}

func (f *fakeEmitter) Incr(name string)                 { f.incrs = append(f.incrs, name) }
func (f *fakeEmitter) Gauge(name string, value float64) { f.gauges[name] = value }
