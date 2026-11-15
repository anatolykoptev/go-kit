package stt_test

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/goleak"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestWSReadLimitRejectsHugeMessage verifies that the WebSocket read limit is
// enforced: when a server sends a message larger than the configured limit
// (defaultWSReadLimit = 10MB), readLoop must surface the error via
// handler.OnError and terminate the connection — rather than allocating the
// full payload into memory (memory bomb).
//
// The test sends an 11MB text frame (just over the 10MB limit) so the client
// rejects it without ever materializing a 1GB allocation. The server-side
// write itself is fine because gorilla streams the frame; the client-side
// ReadMessage must abort with a read-limit error.
func TestWSReadLimitRejectsHugeMessage(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		// Send a single text message slightly larger than the 10MB read limit.
		// 11MB = 11 * 1024 * 1024 bytes. The client must reject this.
		huge := make([]byte, 11*1024*1024)
		if err := conn.WriteMessage(websocket.TextMessage, huge); err != nil {
			// The client will close the connection on read-limit error; a
			// write error here is expected and not a test failure.
			return
		}
		// Hold the connection open so the client has time to process.
		<-time.After(5 * time.Second)
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{Language: "en"}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	opts := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, opts) })

	// readLoop must terminate (connection torn down) and OnError must be
	// called with a read-limit error — NOT allocate the full payload.
	select {
	case <-sc.Done():
		// Connection terminated as expected.
	case <-time.After(5 * time.Second):
		t.Fatal("readLoop did not terminate after oversized message (read limit not enforced)")
	}

	errs := h.Errors()
	if len(errs) == 0 {
		t.Fatal("expected handler.OnError to be called for oversized message, got no errors")
	}
}

// TestWSReadLimitAllowsNormalMessage verifies that messages under the read
// limit are still processed normally (no false positives).
func TestWSReadLimitAllowsNormalMessage(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		sendJSON(t, conn, stt.StreamEvent{Type: "Metadata", RequestID: "req-ok"})
		conn.ReadMessage() //nolint:errcheck // drain until client closes
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{Language: "en"}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sc.ForceClose()
	time.Sleep(50 * time.Millisecond)

	evts := h.Events()
	if len(evts) == 0 {
		t.Fatal("expected at least one event for normal-sized message")
	}
	if evts[0].Type != "Metadata" {
		t.Errorf("event type = %q, want Metadata", evts[0].Type)
	}
}
