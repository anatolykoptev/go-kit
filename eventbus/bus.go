// Package eventbus provides an in-process pub/sub message bus with topic wildcard matching.
//
// Topics are dot-separated strings (e.g. "alerts.twitter"). Patterns support:
//   - Exact match: "alerts.twitter"
//   - Single-segment wildcard: "alerts.*" matches "alerts.twitter" but not "alerts.a.b"
//   - Multi-segment tail wildcard: "alerts.**" matches "alerts.twitter", "alerts.a.b.c"
//   - Global wildcard: "**" matches any topic
package eventbus

import (
	"context"
	"sync"
)

// subscriberBufSize is the buffer size for subscriber channels.
// Large enough to absorb bursts without blocking the publisher.
const subscriberBufSize = 64

// Event carries a topic and arbitrary payload.
type Event struct {
	Topic   string
	Payload any
}

// Bus is an in-process pub/sub message bus with topic wildcard matching.
type Bus struct {
	mu   sync.RWMutex
	subs map[string][]chan Event
}

// New creates a new Bus.
func New() *Bus {
	return &Bus{subs: make(map[string][]chan Event)}
}

// Subscribe returns a buffered channel that receives events matching pattern.
// Supports exact match or wildcard patterns ("alerts.*", "alerts.**", "**").
func (b *Bus) Subscribe(pattern string) <-chan Event {
	ch := make(chan Event, subscriberBufSize)
	b.mu.Lock()
	b.subs[pattern] = append(b.subs[pattern], ch)
	b.mu.Unlock()
	return ch
}

// SubscribeCtx returns a channel that auto-unsubscribes when ctx is cancelled.
func (b *Bus) SubscribeCtx(ctx context.Context, pattern string) <-chan Event {
	ch := b.Subscribe(pattern)
	go func() {
		<-ctx.Done()
		b.Unsubscribe(pattern, ch)
	}()
	return ch
}

// Unsubscribe removes a subscription channel and closes it.
func (b *Bus) Unsubscribe(pattern string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[pattern]
	for i, c := range list {
		if c == ch {
			b.subs[pattern] = append(list[:i], list[i+1:]...)
			close(c)
			return
		}
	}
}

// Publish sends an event to all matching subscribers (non-blocking, drops if buffer full).
func (b *Bus) Publish(topic string, payload any) {
	ev := Event{Topic: topic, Payload: payload}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for subPattern, chans := range b.subs {
		if matchTopic(subPattern, topic) {
			for _, ch := range chans {
				select {
				case ch <- ev:
				default: // drop if subscriber is slow
				}
			}
		}
	}
}
