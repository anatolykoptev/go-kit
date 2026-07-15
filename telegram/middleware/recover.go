package middleware

import (
	"context"
	"fmt"
	"runtime/debug"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// Recover catches panics in next(), calls log with the panic value, and returns a
// wrapped error. Place Recover as the outermost middleware in Chain.
func Recover(log func(any)) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) (retErr error) {
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					log(fmt.Sprintf("recover: panic in handler: %v\n%s", r, stack))
					retErr = fmt.Errorf("handler panic: %v", r)
				}
			}()
			return next(ctx, upd)
		}
	}
}
