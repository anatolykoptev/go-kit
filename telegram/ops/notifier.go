// Package ops provides operator notification utilities for Telegram bots.
//
// # Overview
//
// Notifier delivers structured messages (with inline buttons) to a single
// operator chat. It supports:
//
//   - Button callbacks: each Button carries a handler; HandleCallback dispatches
//     incoming CallbackQuery updates to the matching handler.
//   - Coalescing: when many messages arrive within a short window, individual
//     sends are suppressed after the threshold and replaced with a single
//     "📥 N pending" summary.
//   - Probe: an optional pre-send health check whose result is prepended to
//     every message body as ✅ (success) or ⚠️ (failure).
//
// # Usage
//
//	n := ops.NewNotifier(operatorChatID, sender,
//	    ops.WithCoalesce(5, 30*time.Second),
//	    ops.WithProbe(func(ctx context.Context) (string, error) {
//	        return pingService(ctx)
//	    }),
//	)
//
//	err := n.Send(ctx, ops.Request{
//	    Title: "Deploy",
//	    Body:  "v1.2.0 deployed",
//	    Buttons: []ops.Button{
//	        {Label: "Approve", CallbackData: "approve:deploy42", Handler: approveHandler},
//	        {Label: "Deny",    CallbackData: "deny:deploy42",    Handler: denyHandler},
//	    },
//	})
//
//	// In the Telegram update loop:
//	handled, err := n.HandleCallback(ctx, update.CallbackQuery)
package ops

import (
	"context"
	"fmt"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Sender is the interface through which Notifier delivers messages to Telegram.
// It is a minimal surface so callers can substitute test doubles.
type Sender interface {
	// Send delivers text to chatID. buttons may be nil/empty.
	Send(ctx context.Context, chatID int64, text string, buttons []Button) error
}

// ProbeFn is called before each Send to assess system health. It returns a
// short status string (e.g. "HTTP 200 OK") and an error. A nil error means
// healthy; a non-nil error means degraded.
type ProbeFn func(ctx context.Context) (text string, err error)

// Button is an inline keyboard button with an associated callback handler.
type Button struct {
	// Label is the button text shown to the operator.
	Label string
	// CallbackData is the opaque string sent back in the CallbackQuery.
	CallbackData string
	// Handler is invoked by HandleCallback when a query with this CallbackData
	// is received.
	Handler func(ctx context.Context, callbackData string) error
}

// ProbeReq is a per-call probe override. When set in Request.Probe, this
// ProbeFn is used instead of the notifier-level WithProbe function.
type ProbeReq struct {
	Fn ProbeFn
}

// Request carries the message content for a single Send call.
type Request struct {
	// Title is a short heading (e.g. "Deploy alert").
	Title string
	// Body is the main message body.
	Body string
	// Buttons is the list of inline keyboard buttons.
	Buttons []Button
	// Probe, if non-nil, overrides the notifier-level WithProbe for this send.
	Probe *ProbeReq
}

// Option configures a Notifier.
type Option func(*Notifier)

// WithCoalesce enables burst detection. When more than threshold Sends occur
// within window, subsequent individual sends within that window are suppressed
// and a single "📥 N pending" summary is sent instead when the window expires.
func WithCoalesce(threshold int, window time.Duration) Option {
	return func(n *Notifier) {
		n.coalesceThreshold = threshold
		n.coalesceWindow = window
	}
}

// WithProbe sets a notifier-level probe function that runs before each Send.
// Its result is prepended to the message body as "✅ <text>" or "⚠️ <err>".
func WithProbe(fn ProbeFn) Option {
	return func(n *Notifier) {
		n.probe = fn
	}
}

// Notifier delivers operator alerts to a single Telegram chat.
type Notifier struct {
	operatorChatID int64
	sender         Sender
	probe          ProbeFn

	// Button handler registry: callbackData → handler.
	mu       sync.Mutex
	handlers map[string]func(ctx context.Context, data string) error

	// Coalescing state (Notifier-level, non-re-entrant serial via mu).
	coalesceThreshold int
	coalesceWindow    time.Duration
	windowStart       time.Time
	windowCount       int
	pendingCount      int // suppressed sends after threshold
}

// NewNotifier creates a Notifier that delivers to operatorChatID via sender.
func NewNotifier(operatorChatID int64, sender Sender, opts ...Option) *Notifier {
	n := &Notifier{
		operatorChatID: operatorChatID,
		sender:         sender,
		handlers:       make(map[string]func(ctx context.Context, data string) error),
	}
	for _, o := range opts {
		o(n)
	}
	return n
}

// Send delivers req to the operator chat. If a probe is configured it runs
// first and its result is prepended to the body. If coalescing is active and
// the burst threshold has been exceeded, this call may be suppressed (or
// replaced by a summary).
func (n *Notifier) Send(ctx context.Context, req Request) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Coalescing check.
	if n.coalesceThreshold > 0 {
		now := time.Now()
		if n.windowStart.IsZero() || now.Sub(n.windowStart) >= n.coalesceWindow {
			// New window: reset counters. If there were pending suppressed
			// messages from the old window, they were already coalesced.
			n.windowStart = now
			n.windowCount = 0
			n.pendingCount = 0
		}
		n.windowCount++
		if n.windowCount > n.coalesceThreshold {
			// Burst threshold exceeded. Accumulate suppressed sends without
			// calling the Sender for each one. On the first suppressed send
			// we emit one "📥 N pending" summary; thereafter we suppress
			// silently until the window resets.
			n.pendingCount++
			if n.pendingCount == 1 {
				// First suppression: emit the summary once.
				summary := fmt.Sprintf("📥 %d pending", n.pendingCount)
				return n.sender.Send(ctx, n.operatorChatID, summary, nil)
			}
			// Subsequent suppressions: silently drop (no Sender call).
			return nil
		}
	}

	// Resolve probe function (per-call override takes precedence).
	probeFn := n.probe
	if req.Probe != nil && req.Probe.Fn != nil {
		probeFn = req.Probe.Fn
	}

	body := req.Body
	if probeFn != nil {
		probeText, probeErr := probeFn(ctx)
		if probeErr != nil {
			body = fmt.Sprintf("⚠️ %s\n%s", probeErr.Error(), body)
		} else {
			body = fmt.Sprintf("✅ %s\n%s", probeText, body)
		}
	}

	text := req.Title + "\n" + body

	// Register button handlers.
	for _, btn := range req.Buttons {
		if btn.Handler != nil {
			n.handlers[btn.CallbackData] = btn.Handler
		}
	}

	return n.sender.Send(ctx, n.operatorChatID, text, req.Buttons)
}

// HandleCallback dispatches an incoming CallbackQuery to the registered button
// handler whose CallbackData matches cq.Data.
//
// Returns (true, nil) if the handler was found and invoked successfully.
// Returns (true, err) if the handler returned an error.
// Returns (false, nil) if no handler is registered for cq.Data.
func (n *Notifier) HandleCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) (bool, error) {
	n.mu.Lock()
	handler, ok := n.handlers[cq.Data]
	n.mu.Unlock()

	if !ok {
		return false, nil
	}
	if err := handler(ctx, cq.Data); err != nil {
		return true, err
	}
	return true, nil
}
