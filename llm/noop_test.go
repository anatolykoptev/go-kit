package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// compile-time check: *Client must satisfy Completer
var _ llm.Completer = (*llm.Client)(nil)

func TestNoOp_CompleteReturnsErrUnavailable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	got, err := llm.NoOp{}.Complete(ctx, "sys", "user")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

func TestNewOptional_EmptyKeyReturnsNoOp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, ok := llm.NewOptional("http://example", "", "model")
	if ok {
		t.Errorf("expected ok=false for empty key, got true")
	}
	got, err := c.Complete(ctx, "s", "u")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

func TestNewOptional_NonEmptyKeyReturnsClient(t *testing.T) {
	t.Parallel()
	c, ok := llm.NewOptional("http://example", "key", "model")
	if !ok {
		t.Errorf("expected ok=true for non-empty key, got false")
	}
	if c == nil {
		t.Fatal("expected non-nil Completer")
	}
	if _, isClient := c.(*llm.Client); !isClient {
		t.Errorf("expected *llm.Client, got %T", c)
	}
}

func TestNoOp_TolerateChatOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// NoOp must silently ignore all ChatOptions and return ErrUnavailable without panicking.
	got, err := llm.NoOp{}.Complete(ctx, "s", "u",
		llm.WithChatMaxTokens(100),
		llm.WithChatTemperature(0.5),
	)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}
