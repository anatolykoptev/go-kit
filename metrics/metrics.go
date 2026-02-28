// Package metrics provides lightweight atomic counters for operational observability.
// All operations are safe for concurrent use. Zero external dependencies.
// Each Registry is independent — use NewRegistry() per component or share globally.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Registry holds named atomic counters.
type Registry struct {
	store sync.Map
}

// NewRegistry creates a new empty counter registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// counter returns the *atomic.Int64 for name, creating it on first access.
func (r *Registry) counter(name string) *atomic.Int64 {
	v, _ := r.store.LoadOrStore(name, new(atomic.Int64))
	return v.(*atomic.Int64) //nolint:forcetypeassert // invariant: only *atomic.Int64 stored
}

// Incr increments the named counter by 1.
func (r *Registry) Incr(name string) {
	r.counter(name).Add(1)
}

// Add adds delta to the named counter.
func (r *Registry) Add(name string, delta int64) {
	r.counter(name).Add(delta)
}

// Value returns the current value of the named counter.
func (r *Registry) Value(name string) int64 {
	return r.counter(name).Load()
}

// Snapshot returns a copy of all counters with their current values.
// Only counters that have been written at least once are included.
func (r *Registry) Snapshot() map[string]int64 {
	m := make(map[string]int64)
	r.store.Range(func(k, v any) bool {
		m[k.(string)] = v.(*atomic.Int64).Load() //nolint:forcetypeassert // invariant
		return true
	})
	return m
}

// Reset clears all counters. Intended for tests.
func (r *Registry) Reset() {
	r.store.Range(func(k, _ any) bool {
		r.store.Delete(k)
		return true
	})
}

// Format returns a human-readable summary of all counters, sorted by name.
func (r *Registry) Format() string {
	snap := r.Snapshot()
	if len(snap) == 0 {
		return ""
	}

	names := make([]string, 0, len(snap))
	for name := range snap {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, name := range names {
		fmt.Fprintf(&sb, "%s=%d\n", name, snap[name])
	}
	return sb.String()
}

// TrackOperation increments callCounter, runs fn, and increments errCounter
// if fn returns a non-nil error. The error from fn is always returned unchanged.
func (r *Registry) TrackOperation(callCounter, errCounter string, fn func() error) error {
	r.Incr(callCounter)
	if err := fn(); err != nil {
		r.Incr(errCounter)
		return err
	}
	return nil
}
