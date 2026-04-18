# eventbus

In-process pub/sub message bus with dot-separated topics and wildcard pattern matching.
Not a replacement for Kafka or NATS — use this for intra-process event routing only.

```
go get github.com/anatolykoptev/go-kit/eventbus
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/eventbus"

bus := eventbus.New()

// Subscribe before publishing.
ch := bus.Subscribe("alerts.*")

// Publish from any goroutine.
bus.Publish("alerts.twitter", map[string]string{"msg": "login failed"})

// Consume.
ev := <-ch
fmt.Println(ev.Topic, ev.Payload)
```

## Topic pattern matching

Topics are dot-separated strings (`"alerts.twitter"`). Patterns follow these rules:

| Pattern | Matches | Does not match |
|---------|---------|----------------|
| `a.b` | `a.b` | `a.b.c`, `a.x` |
| `a.*` | `a.b`, `a.c` | `a.b.c` |
| `a.**` | `a.b`, `a.b.c.d` | `b.c` |
| `**` | any topic | — |
| `a.*.c` | `a.b.c`, `a.x.c` | `a.b.d`, `a.b.c.d` |

## SubscribeCtx — auto-unsubscribe

Use `SubscribeCtx` to tie a subscription's lifetime to a context. When the
context is cancelled, the subscriber is removed and its channel is closed.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

ch := bus.SubscribeCtx(ctx, "jobs.**")
for ev := range ch {
    // channel closes automatically when ctx is cancelled
    process(ev)
}
```

## Drop-on-full-buffer gotcha

Each subscriber gets a 64-slot buffered channel. If a subscriber cannot keep
up, events are **silently dropped** — the publisher never blocks. There is no
back-pressure, no error, no retry.

Design implications:
- Keep subscriber processing fast (offload heavy work to a worker pool).
- If you need guaranteed delivery, use a persistent queue.
- For monitoring: periodically compare published vs consumed event counts.

## Unsubscribe manually

```go
ch := bus.Subscribe("metrics.*")
// ... later
bus.Unsubscribe("metrics.*", ch) // closes ch
```
