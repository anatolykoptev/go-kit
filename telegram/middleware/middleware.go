// Package middleware provides composable handler middleware for Telegram bots.
// All primitives operate on *tgbotapi.Update so they compose directly with kb
// handlers (which receive *tgbotapi.CallbackQuery unwrapped from Update) via
// thin adapter wrappers at the consumer side.
//
// # Recommended chain order
//
//	mw.Chain(
//	    Recover(log),           // outermost — catch any panic
//	    Metrics(reg, "name"),
//	    AutoRespond(answer),    // before next() — spinner always clears
//	    DeletePrev(del),        // before next() — UX cleanup
//	    OperatorOnly(isOp, onDeny), // gate
//	    ShadowBan(check),       // drop if abuser
//	    RateLimit(kl, keyFn, onDeny), // throttle
//	    handler,                // innermost
//	)
package middleware

import (
	"context"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// Handler is the unit of work: process a Telegram update and return an error.
type Handler func(ctx context.Context, upd *tgbotapi.Update) error

// Middleware wraps a Handler, adding cross-cutting behaviour.
type Middleware func(next Handler) Handler

// Chain composes middlewares left-to-right (outermost first).
// Chain(a, b, c)(h) ≡ a(b(c(h))).
// Chain() with no arguments returns identity.
// Chain(Chain(a,b), c) ≡ Chain(a, b, c) — nested composition preserves order.
func Chain(mws ...Middleware) Middleware {
	return func(next Handler) Handler {
		// Apply right-to-left so that the leftmost middleware is outermost.
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}
