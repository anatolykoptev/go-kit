package middleware

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// ErrNewChatQuotaExceeded is returned by NewChatQuota when a previously-unseen
// chat key arrives and the window's distinct-chat count has reached limit.
var ErrNewChatQuotaExceeded = errors.New("new chat quota exceeded")

// NewChatQuota drops updates from unseen chat keys beyond limit per window.
// It maintains a rolling window of distinct keys first seen; once the count
// reaches limit, any update whose key is not already in the map is rejected.
//
// keyFn extracts the grouping key from the update (use ByChatID for the
// common case of per-chat throttling).
//
// GC of expired entries runs lazily on each invocation — no background goroutine
// is started, so no cleanup is required by the caller.
func NewChatQuota(window time.Duration, limit int, keyFn func(*tgbotapi.Update) string) Middleware {
	var mu sync.Mutex
	seen := make(map[string]time.Time) // key → time.Time first seen in current window

	gc := func(now time.Time) {
		cutoff := now.Add(-window)
		for k, t := range seen {
			if t.Before(cutoff) {
				delete(seen, k)
			}
		}
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			key := keyFn(upd)
			now := time.Now()

			mu.Lock()
			gc(now) // remove entries outside the window before counting

			if _, known := seen[key]; known {
				// Already counted in this window — let through.
				mu.Unlock()
				return next(ctx, upd)
			}

			if len(seen) >= limit {
				// New key but quota full.
				mu.Unlock()
				slog.DebugContext(ctx, "newchatquota: dropped", "key", key, "limit", limit)
				return ErrNewChatQuotaExceeded
			}

			// New key within quota — record and let through.
			seen[key] = now
			mu.Unlock()
			return next(ctx, upd)
		}
	}
}
