// Package kb provides a fluent inline keyboard builder for Telegram bots,
// with co-located callback handler registration.
//
// Vendored and adapted from github.com/go-telegram/ui/keyboard/inline
// (MIT License, Copyright (c) 2022 negasus — see LICENSE.go-telegram).
// Modifications: *bot.Bot receiver removed; handler signature changed to
// func(ctx, *tgbotapi.CallbackQuery) error; dispatch is self-contained;
// data validated eagerly at Button() call time.
package kb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// prefixEntropyBytes is the number of crypto/rand bytes used for a random prefix.
	prefixEntropyBytes = 16
	// maxDataBytes is the Telegram callback_data field size limit per the Bot API spec.
	// Enforced eagerly at Button() call time on the raw data payload.
	maxDataBytes = 64
)

// Handler is the callback fired when a button is clicked.
type Handler func(ctx context.Context, cq *tgbotapi.CallbackQuery) error

// Option configures a Keyboard.
type Option func(*Keyboard)

// WithPrefix sets the namespace prefix for this keyboard's callback data.
// Default: 32-character hex string (16 bytes entropy via crypto/rand).
func WithPrefix(p string) Option {
	return func(k *Keyboard) { k.prefix = p }
}

// WithDeleteOnClick controls whether the originating message should be
// deleted after a button is clicked. Default: true (matches go-telegram/ui).
// Dispatch itself does not delete; callers read ShouldDeleteOnClick().
func WithDeleteOnClick(b bool) Option {
	return func(k *Keyboard) { k.deleteOnClick = b }
}

// WithOnError sets the error handler for validation and dispatch failures.
// Default: log.Printf.
func WithOnError(fn func(error)) Option {
	return func(k *Keyboard) { k.onError = fn }
}

// handlerData pairs stored payload bytes with the click handler.
type handlerData struct {
	data    []byte
	handler Handler
}

// Keyboard is a fluent inline keyboard builder.
// The zero value is not usable; construct via New.
type Keyboard struct {
	prefix        string
	deleteOnClick bool
	onError       func(error)
	handlers      []handlerData
	markup        [][]tgbotapi.InlineKeyboardButton
}

// New creates a Keyboard with the given options applied.
func New(opts ...Option) *Keyboard {
	k := &Keyboard{
		prefix:        randomPrefix(),
		deleteOnClick: true,
		onError:       defaultOnError,
		markup:        [][]tgbotapi.InlineKeyboardButton{{}},
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// randomPrefix returns a 32-char hex string from prefixEntropyBytes of crypto/rand.
// Panics on entropy failure — startup-time fatal condition.
func randomPrefix() string {
	b := make([]byte, prefixEntropyBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("kb: randomPrefix: crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}

func defaultOnError(err error) {
	log.Printf("[kb] error: %v", err)
}

// Prefix returns this keyboard's namespace prefix.
func (k *Keyboard) Prefix() string { return k.prefix }

// ShouldDeleteOnClick reports whether the message should be deleted on click.
// PR 3 middleware reads this flag; Dispatch does not delete.
func (k *Keyboard) ShouldDeleteOnClick() bool { return k.deleteOnClick }

// Row starts a new button row. No-op if the current row is already empty.
func (k *Keyboard) Row() *Keyboard {
	if len(k.markup[len(k.markup)-1]) > 0 {
		k.markup = append(k.markup, []tgbotapi.InlineKeyboardButton{})
	}
	return k
}

// Button appends an inline button that invokes onClick when clicked.
// The callback_data sent to Telegram is k.prefix+strconv.Itoa(buttonIndex),
// which is always short. data is stored server-side for the handler.
// Validation: len(data) > maxDataBytes calls onError and skips adding the button.
func (k *Keyboard) Button(label string, data []byte, onClick Handler) *Keyboard {
	if len(data) > maxDataBytes {
		k.onError(fmt.Errorf("kb: Button %q: data length %d exceeds %d-byte limit", label, len(data), maxDataBytes))
		return k
	}
	idx := len(k.handlers)
	k.handlers = append(k.handlers, handlerData{data: data, handler: onClick})
	k.markup[len(k.markup)-1] = append(k.markup[len(k.markup)-1], tgbotapi.InlineKeyboardButton{
		Text:         label,
		CallbackData: strPtr(k.prefix + strconv.Itoa(idx)),
	})
	return k
}

// URL appends an inline button that opens a URL (no callback handler).
func (k *Keyboard) URL(label, rawURL string) *Keyboard {
	k.markup[len(k.markup)-1] = append(k.markup[len(k.markup)-1], tgbotapi.InlineKeyboardButton{
		Text: label,
		URL:  strPtr(rawURL),
	})
	return k
}

// Markup returns the InlineKeyboardMarkup for use in tgbotapi send configs.
// Returns a deep copy so callers cannot mutate the keyboard's internal state.
func (k *Keyboard) Markup() tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, len(k.markup))
	for i, row := range k.markup {
		cp := make([]tgbotapi.InlineKeyboardButton, len(row))
		copy(cp, row)
		rows[i] = cp
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// Dispatch routes cq to the button handler whose index is encoded in cq.Data.
// Returns (false, nil) if cq.Data does not start with k.prefix (unknown keyboard).
// Returns (true, err) if the button was found; err is the handler's return value.
func (k *Keyboard) Dispatch(ctx context.Context, cq *tgbotapi.CallbackQuery) (handled bool, err error) {
	if !strings.HasPrefix(cq.Data, k.prefix) {
		return false, nil
	}
	indexStr := strings.TrimPrefix(cq.Data, k.prefix)
	idx, parseErr := strconv.Atoi(indexStr)
	if parseErr != nil || idx < 0 || idx >= len(k.handlers) {
		return true, fmt.Errorf("kb: Dispatch: invalid callback data %q", cq.Data)
	}
	h := k.handlers[idx].handler
	if h == nil {
		return true, nil
	}
	return true, h(ctx, cq)
}

// strPtr returns a pointer to s — helper for tgbotapi fields that use *string.
func strPtr(s string) *string { return &s }
