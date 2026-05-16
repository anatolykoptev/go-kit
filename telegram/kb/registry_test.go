package kb_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/anatolykoptev/go-kit/telegram/kb"
)

// --- Test: Registry dispatches to the right keyboard ---

func TestRegistryDispatchRoutes(t *testing.T) {
	calledA, calledB := false, false

	kA := kb.New(kb.WithPrefix("aaa"))
	kA.Button("A", nil, func(_ context.Context, _ *tgbotapi.CallbackQuery) error {
		calledA = true
		return nil
	})

	kB := kb.New(kb.WithPrefix("bbb"))
	kB.Button("B", nil, func(_ context.Context, _ *tgbotapi.CallbackQuery) error {
		calledB = true
		return nil
	})

	reg := kb.NewRegistry()
	reg.Register(kA)
	reg.Register(kB)

	// Route to kB
	handled, err := reg.Dispatch(context.Background(), makeQuery("bbb0"))
	if !handled {
		t.Fatal("should be handled by kB")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledA {
		t.Fatal("kA handler should not be called")
	}
	if !calledB {
		t.Fatal("kB handler should be called")
	}
}

// --- Test: unknown prefix is not handled by registry ---

func TestRegistryDispatchUnknownPrefix(t *testing.T) {
	k := kb.New(kb.WithPrefix("known"))
	k.Button("btn", nil, nil)

	reg := kb.NewRegistry()
	reg.Register(k)

	handled, err := reg.Dispatch(context.Background(), makeQuery("unknown0"))
	if handled {
		t.Fatal("should not handle unknown prefix")
	}
	if err != nil {
		t.Fatalf("should return nil error for unknown prefix, got: %v", err)
	}
}

// --- Test: prefix collision panics (programming error) ---

func TestRegistryPrefixCollisionPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate prefix registration")
		}
	}()

	kA := kb.New(kb.WithPrefix("same"))
	kB := kb.New(kb.WithPrefix("same"))

	reg := kb.NewRegistry()
	reg.Register(kA)
	reg.Register(kB) // should panic
}

// --- Test H1: Registry is safe under concurrent Register and Dispatch ---
// Ref: ~/deploy/krolik-server/reports/go-kit/architecture/2026-05-16-v0.56-quality-review.md H1

func TestRegistry_ConcurrentRegisterAndDispatch(t *testing.T) {
	reg := kb.NewRegistry()

	const goroutines = 10
	const opsEach = 100

	// Pre-register some keyboards so Dispatch has something to iterate over.
	for i := 0; i < goroutines; i++ {
		prefix := fmt.Sprintf("pre%02d", i)
		k := kb.New(kb.WithPrefix(prefix))
		k.Button("btn", nil, func(_ context.Context, _ *tgbotapi.CallbackQuery) error { return nil })
		reg.Register(k)
	}

	var wg sync.WaitGroup
	// 10 goroutines Register new keyboards concurrently.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < opsEach; j++ {
				// Each goroutine registers a unique prefix (no collision).
				prefix := fmt.Sprintf("dyn%02d_%04d", i, j)
				k := kb.New(kb.WithPrefix(prefix))
				k.Button("btn", nil, func(_ context.Context, _ *tgbotapi.CallbackQuery) error { return nil })
				reg.Register(k)
			}
		}()
	}
	// 10 goroutines Dispatch concurrently.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < opsEach; j++ {
				// Route to one of the pre-registered keyboards.
				prefix := fmt.Sprintf("pre%02d", i%goroutines)
				_, _ = reg.Dispatch(context.Background(), makeQuery(prefix+"0"))
			}
		}()
	}
	wg.Wait()
	// If no data race or panic: test passes. Run under -race to catch H1.
}

// --- Test: longest-prefix-match correctness (item 1.2 — v0.57 polish) ---
// When two registered prefixes share a common prefix (e.g. "a" and "abc"),
// a callback_data "abc0" must route to "abc", not "a".
// Map iteration order is non-deterministic, so without sorting this test
// could fail ~50% of the time before the fix.
func TestRegistryDispatch_LongestPrefixMatch(t *testing.T) {
	calledShort, calledLong := false, false

	kShort := kb.New(kb.WithPrefix("a"))
	kShort.Button("short", nil, func(_ context.Context, _ *tgbotapi.CallbackQuery) error {
		calledShort = true
		return nil
	})

	kLong := kb.New(kb.WithPrefix("abc"))
	kLong.Button("long", nil, func(_ context.Context, _ *tgbotapi.CallbackQuery) error {
		calledLong = true
		return nil
	})

	reg := kb.NewRegistry()
	reg.Register(kShort)
	reg.Register(kLong)

	// "abc0" starts with both "a" and "abc"; must match "abc" (longest).
	handled, err := reg.Dispatch(context.Background(), makeQuery("abc0"))
	if !handled {
		t.Fatal("expected handled=true for abc0")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledShort {
		t.Error("short keyboard handler must NOT be called for abc0")
	}
	if !calledLong {
		t.Error("long keyboard handler must be called for abc0")
	}
}
