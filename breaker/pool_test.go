// breaker/pool_test.go
package breaker

import "testing"

func TestPool_ReturnsSameBreakerPerKey(t *testing.T) {
	p := NewPool(func(name string) Options {
		return Options{Name: name, FailThreshold: 3}
	})
	a1 := p.Get("twitter")
	a2 := p.Get("twitter")
	b := p.Get("linkedin")
	if a1 != a2 {
		t.Fatal("same key must return same breaker")
	}
	if a1 == b {
		t.Fatal("different keys must return different breakers")
	}
}

func TestPool_Snapshot(t *testing.T) {
	p := NewPool(func(name string) Options { return Options{Name: name} })
	p.Get("a")
	p.Get("b")
	snap := p.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot size = %d, want 2", len(snap))
	}
	if _, ok := snap["a"]; !ok {
		t.Fatal("snapshot missing key a")
	}
}
