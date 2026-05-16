package kb

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Registry composes multiple Keyboards into a single Dispatch entry point.
// Use it to replace fan-out chains of keyboard.Dispatch calls with one call.
//
// Register panics on duplicate prefix — duplicate registration is a programming
// error (like registering the same route twice in chi), caught at startup.
type Registry struct {
	keyboards map[string]*Keyboard
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{keyboards: make(map[string]*Keyboard)}
}

// Register adds k to the registry.
// Panics if a keyboard with the same prefix is already registered.
func (r *Registry) Register(k *Keyboard) {
	p := k.Prefix()
	if _, exists := r.keyboards[p]; exists {
		panic(fmt.Sprintf("kb: Registry.Register: duplicate prefix %q", p))
	}
	r.keyboards[p] = k
}

// Dispatch routes cq to whichever registered keyboard owns cq.Data's prefix.
// Returns (false, nil) if no keyboard matches — safe to chain with other handlers.
func (r *Registry) Dispatch(ctx context.Context, cq *tgbotapi.CallbackQuery) (handled bool, err error) {
	for _, k := range r.keyboards {
		handled, err = k.Dispatch(ctx, cq)
		if handled {
			return handled, err
		}
	}
	return false, nil
}
