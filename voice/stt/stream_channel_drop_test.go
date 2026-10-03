package stt_test

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/goleak"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestChannelStreamDroppedEventsCounted verifies that when the events channel
// buffer overflows (slow consumer), dropped events are counted and exposed via
// DroppedEvents() instead of being silently lost.
func TestChannelStreamDroppedEventsCounted(t *testing.T) {
	const eventCount = 70 // > buffer of 64
	serverDone := make(chan struct{})

	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		defer close(serverDone)
		for range eventCount {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"Results"}`)); err != nil {
				return // client closed — that's fine
			}
		}
	})
	defer srv.Close()

	cs, err := stt.StreamWithChannels(context.Background(), wsURL(srv.URL), stt.StreamParams{})
	if err != nil {
		t.Fatalf("StreamWithChannels: %v", err)
	}
	opts := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, opts) })
	defer cs.ForceClose()

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server to finish sending")
	}
	// Let readLoop process all frames. Don't drain the events channel —
	// we want the buffer to overflow.
	time.Sleep(200 * time.Millisecond)

	dropped := cs.DroppedEvents()
	if dropped == 0 {
		t.Errorf("expected dropped events > 0 (sent %d, buffer %d), got 0", eventCount, 64)
	}
}
