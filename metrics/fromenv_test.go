package metrics

import "testing"

func TestFromEnv_NoVars(t *testing.T) {
	t.Setenv("PROM_NAMESPACE", "")
	t.Setenv("METRICS_PROM", "")
	r := FromEnv("myservice")
	if r.promBridge != nil {
		t.Fatal("expected in-memory registry, got prom-backed")
	}
}

func TestFromEnv_PromNamespaceSet(t *testing.T) {
	t.Setenv("PROM_NAMESPACE", "custom_ns")
	r := FromEnv("ignored")
	if r.promBridge == nil || r.promBridge.namespace != "custom_ns" {
		t.Fatalf("expected namespace=custom_ns")
	}
}

func TestFromEnv_MetricsPromFlag(t *testing.T) {
	t.Setenv("PROM_NAMESPACE", "")
	t.Setenv("METRICS_PROM", "1")
	r := FromEnv("myservice")
	if r.promBridge == nil || r.promBridge.namespace != "myservice" {
		t.Fatalf("expected namespace=myservice")
	}
}

func TestFromEnv_PanicsOnEmptyBoth(t *testing.T) {
	t.Setenv("PROM_NAMESPACE", "")
	t.Setenv("METRICS_PROM", "1")
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty defaultNamespace")
		}
	}()
	FromEnv("")
}
