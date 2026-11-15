package stt_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestStreamClose verifies that Close() sends CloseStream and waits for connection end.
func TestStreamClose(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		var ctrl map[string]string
		_ = json.Unmarshal(data, &ctrl)
		if ctrl["type"] != "CloseStream" {
			t.Errorf("control type = %q, want CloseStream", ctrl["type"])
		}
		sendJSON(t, conn, stt.StreamEvent{Type: "CloseStream"})
		conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := sc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-sc.Done():
	default:
		t.Error("Done() channel should be closed after Close()")
	}
}

// TestStreamTranscript verifies the Transcript() helper on StreamEvent.
func TestStreamTranscript(t *testing.T) {
	t.Run("with alternatives", func(t *testing.T) {
		e := stt.StreamEvent{
			Channel: &stt.StreamChannel{
				Alternatives: []stt.StreamAlternative{
					{Transcript: "hello", Confidence: 0.9},
				},
			},
		}
		if got := e.Transcript(); got != "hello" {
			t.Errorf("Transcript() = %q, want hello", got)
		}
	})
	t.Run("no channel", func(t *testing.T) {
		e := stt.StreamEvent{}
		if got := e.Transcript(); got != "" {
			t.Errorf("Transcript() = %q, want empty", got)
		}
	})
	t.Run("empty alternatives", func(t *testing.T) {
		e := stt.StreamEvent{Channel: &stt.StreamChannel{}}
		if got := e.Transcript(); got != "" {
			t.Errorf("Transcript() = %q, want empty", got)
		}
	})
}

// TestStreamServerError verifies that a server Error event is delivered via OnEvent.
func TestStreamServerError(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		sendJSON(t, conn, stt.StreamEvent{Type: "Error", Message: "bad audio format"})
		conn.ReadMessage() //nolint:errcheck // drain
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sc.ForceClose()
	time.Sleep(50 * time.Millisecond)

	evts := h.Events()
	var found bool
	for _, e := range evts {
		if e.Type == "Error" {
			found = true
			if e.Message != "bad audio format" {
				t.Errorf("message = %q, want 'bad audio format'", e.Message)
			}
		}
	}
	if !found {
		t.Error("expected Error event in handler")
	}
}
