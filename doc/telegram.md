# telegram

Production-grade toolbox for Telegram bots and Mini Apps built on top of
`go-telegram-bot-api/telegram-bot-api/v5`. Solves the recurring problems:
parse-mode jail, callback-data collisions, FSM persistence, per-chat
keyboards, broadcast pacing, rate-limit middleware, init-data validation,
and the awkward `tgbotapi` API surface for the new Bot API features.

```
go get github.com/anatolykoptev/go-kit/telegram
```

## Quick start — parse-mode pre-flight

The single biggest source of `Bad Request: can't parse entities` 400s in any
Telegram bot is sending text with the wrong `parse_mode`. `PrepareForTelegram`
auto-detects the safest mode (HTML, MarkdownV2, or plain) and returns the
sanitised body together with the right mode:

```go
import "github.com/anatolykoptev/go-kit/telegram"

text, mode := telegram.PrepareForTelegram(userGeneratedOrLLMOutput)
msg := tgbotapi.NewMessage(chatID, text)
msg.ParseMode = mode
_, _ = bot.Send(msg)
```

`SanitizeHTML(s)` is the underlying HTML sanitiser if you already know the
mode is HTML (Telegram's allow-list is tiny and very particular).

## Markdown -> HTML conversion

`MarkdownToHTML(text)` converts a wide subset of Markdown to Telegram-flavoured
HTML, including:

- Bold (`**`, `__`), italic (`*`, `_`), strikethrough (`~~`)
- Inline code and fenced code blocks (with language hint)
- Links (preserves URLs containing underscores)
- Headings (collapsed to bold)
- Blockquotes
- Strips Markdown image syntax (Telegram has no inline images in HTML)

```go
html := telegram.MarkdownToHTML(rawMD)
```

Companion helpers:

| Function | Notes |
|----------|-------|
| `StripMarkdown(s)` | Drop all formatting, keep just the text |
| `CloseUnclosedMarkdown(s)` | Auto-close dangling `**`, `_`, `~~`, ` ``` ` from truncated LLM output |
| `Detect(s) Format` | Return whether the text is HTML / Markdown / plain |

## Subpackages

| Subpkg | Purpose |
|--------|---------|
| `telegram/cmd` | Router for `/commands`, predicates (`PrivateChat`, `FromUser`, `RegexMatch`, `OnReaction`, …) |
| `telegram/middleware` | Composable middleware: `RateLimit`, `Recover`, `Metrics`, `OperatorOnly`, `NewChatQuota`, `WebhookSecret`, `AutoRespond`, `DeletePrev`, tracing, shadowban |
| `telegram/kb` | Inline + reply keyboards with prefix-tagged callbacks and a `Registry` for dispatch |
| `telegram/callback` | HMAC-signed typed callback codec (`EncodeTyped[T]` / `DecodeTyped[T]`) — no in-memory state, replay-safe |
| `telegram/fsm` | Conversation state machine; `MemoryStore` for single-process, `PostgresStore` for HA |
| `telegram/forum` | Forum-topic management (create / edit / close) |
| `telegram/broadcast` | Bulk-message pacer with RPS budget + dead-letter callback |
| `telegram/miniapp` | Mini App helpers: `ValidateInitData`, invoices, prepared inline messages |
| `telegram/tgapi5` | Thin adapters over `tgbotapi.BotAPI` to satisfy the small `Sender`/`Answerer`/`Deleter` interfaces other subpackages depend on |
| `telegram/ops` | Operator-notifier with coalescing and on-demand probes |

## Command router (`telegram/cmd`)

```go
r := cmd.NewRouter()
r.Handle("/start", cmd.PrivateChat(), startHandler)
r.Handle("/admin", cmd.And(cmd.FromAnyUser(adminIDs...), cmd.PrivateChat()), adminHandler)
r.HandleRegex(cmd.RegexMatch(`^report\s+(\d+)$`), reportHandler)
r.SetDefault(unknownHandler)
r.AutoHelp() // exposes "/help"

if err := r.Dispatch(ctx, upd); err != nil {
    slog.Error("dispatch", "err", err)
}
```

Predicates compose with `And`, `Or`, `Not`. Common ones:
`PrivateChat`, `GroupChat`, `ChannelChat`, `FromUser`, `FromAnyUser`,
`Has`, `InTopic`, `AnyTopic`, `OnReaction`, `RegexMatch`.

## Middleware

```go
import "github.com/anatolykoptev/go-kit/telegram/middleware"

mws := middleware.Chain(
    middleware.Recover(func(p any) { slog.Error("panic", "v", p) }),
    middleware.Metrics(reg, "tgbot"),
    middleware.RateLimit(keyLim, middleware.ByUserID, denyHandler),
    middleware.OperatorOnly(isOperator, denyHandler),
)

handler := mws(r.Dispatch)
```

`RateLimit` wraps a [`ratelimit.KeyLimiter`](ratelimit.md); pick `ByUserID`
or `ByChatID` for the keying strategy. `Metrics` wires per-update counters
into a [`metrics.Registry`](metrics.md).

## Mini App init-data validation

```go
import "github.com/anatolykoptev/go-kit/telegram/miniapp"

data, err := miniapp.ValidateInitDataWithMaxAge(initDataHeader, botToken, 24*time.Hour)
if err != nil {
    http.Error(w, "bad init data", http.StatusUnauthorized)
    return
}
slog.Info("authed", "uid", data.User.ID, "username", data.User.Username)
```

`ValidateInitData` does the HMAC dance per the Telegram spec; the `WithMaxAge`
variant additionally rejects stale tokens (replay protection).

## Typed callbacks (HMAC-signed, stateless)

```go
import "github.com/anatolykoptev/go-kit/telegram/callback"

c := callback.New([]byte(os.Getenv("TG_CALLBACK_SECRET")))

type vote struct{ MsgID int; Choice string }

data, _ := callback.EncodeTyped(c, "vote", vote{MsgID: 12, Choice: "yes"})
btn := tgbotapi.NewInlineKeyboardButtonData("Yes", data)

// On the receiving side:
prefix, v, err := callback.DecodeTyped[vote](c, cq.Data)
if err != nil {
    return // bad signature / corrupt
}
```

Stateless: no in-memory map of callback IDs to wear out across restarts;
HMAC-signed: no spoofing valid callback payloads.

## FSM — conversation state

```go
import "github.com/anatolykoptev/go-kit/telegram/fsm"

store := fsm.NewMemoryStore() // or fsm.NewPostgresStore(ctx, pool, 24*time.Hour)

m := fsm.New(store, askName, 30*time.Minute,
    fsm.WithCancelCmds("/cancel"),
    fsm.WithOnCancel(func(ctx context.Context, chatID int64) {
        send(chatID, "Cancelled.")
    }),
)

func askName(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
    send(e.ChatID, "What is your name?")
    return collectName, nil
}

func collectName(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
    storeName(e.ChatID, e.Text)
    send(e.ChatID, "Thanks!")
    return nil, nil // end of flow
}
```

`PostgresStore` lets multiple bot replicas share a conversation; the TTL is
enforced at the database level so dead conversations don't linger.

## Broadcast pacer

```go
import "github.com/anatolykoptev/go-kit/telegram/broadcast"

p := broadcast.NewPacer(
    func(ctx context.Context, chatID int64, msg string) error {
        _, err := bot.Send(tgbotapi.NewMessage(chatID, msg))
        return err
    },
    broadcast.WithRPS(25),                                         // Telegram bulk-send safe rate
    broadcast.WithDeadLetter(func(id int64, err error) {
        slog.Warn("undeliverable", "chatID", id, "err", err)
    }),
)

for _, id := range subscribers {
    p.Send(ctx, id, "Daily digest is ready.")
}
```

`Broadcaster` is the higher-level variant that uses `copyMessage` /
`deleteMessage` to operate on existing posts (e.g. announce-then-retract).

## Keyboards (`telegram/kb`)

`Keyboard` builds an inline-keyboard markup and registers handlers under a
shared `WithPrefix("kb:")` namespace so callbacks never collide across
features. `Registry` dispatches them.

```go
kb := kb.New(kb.WithPrefix("settings:"), kb.WithDeleteOnClick(true))
kb.Add("Dark mode", toggleDarkMode)
kb.Add("Notifications", toggleNotifications)
markup := kb.Markup()
```

## tgapi5 adapters

The subpackages above only depend on small interfaces (`Sender`,
`CallbackAnswerer`, `MessageDeleter`, `InlineAnswerer`, …). `telegram/tgapi5`
ships the production adapters that wrap a `*tgbotapi.BotAPI`:

```go
deleter := tgapi5.NewMessageDeleter(bot, reg)
answer  := tgapi5.NewCallbackAnswerer(bot)
inline  := tgapi5.NewInlineAnswerer(bot)
invoice := tgapi5.NewInvoiceSender(bot)
prepared:= tgapi5.NewPreparedSender(bot)
```

This indirection means the rest of `telegram/*` is testable without spinning
up an actual bot — pass a fake implementing the interface.

## Notes

- All subpackages tolerate `nil` responses from `tgbotapi.BotAPI` (defence in
  depth — older bot-api versions have surprising nil-on-success returns).
- The Telegram parse-mode landscape is brittle; default to
  `PrepareForTelegram` over hand-built `parse_mode` selection.
- For end-to-end Mini App auth, pair `miniapp.ValidateInitDataWithMaxAge`
  with a per-request CSRF token; init-data alone is replay-able within the
  max-age window.
- Test data and HardRed fixtures cover Unicode edge cases (RTL, ZWJ emoji
  sequences, combining characters) — if you find a parse-mode 400 that
  `PrepareForTelegram` misses, please file with a minimal repro for hardred.
