package kb_test

import (
	"context"
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
