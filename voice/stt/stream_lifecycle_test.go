package stt_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/goleak"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestCloseBlocksForever_ServerHangs verifies that Close() returns within a
// bounded timeout when the server hangs without sending a close frame. Before
// the fix, Close() blocked forever on <-sc.done because readLoop's
// conn.ReadMessage() never returned.
func TestCloseBlocksForever_ServerHangs(t *testing.T) {
	// Server upgrades then holds the connection open without ever sending a
	// close frame and without reading client messages.
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		// Block forever; never close, never send a close frame.
		<-time.After(30 * time.Second)
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{Language: "en"}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Ignore server-side goroutines (the mock server handler is blocked on
	// time.After and won't exit until srv.Close() tears it down). We only
	// care about client-side readLoop leaks.
	opts := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, opts) })
	// Give the readLoop a moment to enter conn.ReadMessage().
	time.Sleep(50 * time.Millisecond)

	done := make(chan error, 1)
	go func() {
		done <- sc.Close()
	}()

	select {
	case err := <-done:
		// Close() returned within the deadline — success.
		_ = err
	case <-time.After(6 * time.Second):
		t.Fatal("Close() blocked forever waiting for server close frame")
	}
}

// TestForceCloseConcurrentReadLoop_Race verifies that ForceClose() called
// concurrently with an active readLoop does not race on the underlying conn
// (no data race, no panic). Run with -race.
func TestForceCloseConcurrentReadLoop_Race(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		// Hold the connection open so readLoop blocks in ReadMessage.
		<-time.After(10 * time.Second)
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{Language: "en"}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	opts := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, opts) })
	// Let readLoop enter the blocking ReadMessage call.
	time.Sleep(50 * time.Millisecond)

	// Hammer ForceClose from multiple goroutines concurrently with readLoop.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc.ForceClose()
		}()
	}
	wg.Wait()

	// readLoop must terminate; assert done channel closes within a deadline.
	select {
	case <-sc.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("readLoop did not terminate after ForceClose (goroutine leak)")
	}
}

// TestConnectTwiceNoPanic verifies that calling Connect() twice on the same
// StreamClient does not panic (previously the second readLoop deferred
// close(sc.done) on an already-closed channel → panic: close of closed channel).
func TestConnectTwiceNoPanic(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		<-time.After(5 * time.Second)
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{Language: "en"}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	opts := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, opts) })

	// Second Connect must return an error (already connected) and NOT panic.
	secondErr := sc.Connect(context.Background())
	if secondErr == nil {
		t.Error("expected second Connect() to return an error, got nil")
	}

	// Clean up.
	sc.ForceClose()
	select {
	case <-sc.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("readLoop did not terminate")
	}
}
