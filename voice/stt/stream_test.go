package stt_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// mockHandler collects events and errors for assertions.
type mockHandler struct {
	mu     sync.Mutex
	events []stt.StreamEvent
	errors []error
}

func (h *mockHandler) OnEvent(e stt.StreamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e)
}

func (h *mockHandler) OnError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errors = append(h.errors, err)
}

func (h *mockHandler) Events() []stt.StreamEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]stt.StreamEvent, len(h.events))
	copy(out, h.events)
	return out
}

func (h *mockHandler) Errors() []error {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]error, len(h.errors))
	copy(out, h.errors)
	return out
}

func newMockWSServer(t *testing.T, handler func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		handler(conn)
	}))
	return srv
}

func sendJSON(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Errorf("server write: %v", err)
	}
}

func wsURL(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1)
}

// TestStreamConnect verifies that a Metadata event is received on connect.
func TestStreamConnect(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		sendJSON(t, conn, stt.StreamEvent{Type: "Metadata", RequestID: "req-1", Channels: 1})
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
		t.Fatal("expected at least one event")
	}
	if evts[0].Type != "Metadata" {
		t.Errorf("event type = %q, want Metadata", evts[0].Type)
	}
	if evts[0].RequestID != "req-1" {
		t.Errorf("request_id = %q, want req-1", evts[0].RequestID)
	}
}

// TestStreamSendAndFinalize verifies binary send + Finalize + Results response.
func TestStreamSendAndFinalize(t *testing.T) {
	serverDone := make(chan struct{})
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		defer close(serverDone)
		mt, _, err := conn.ReadMessage()
		if err != nil || mt != websocket.BinaryMessage {
			t.Errorf("expected binary frame: type=%d err=%v", mt, err)
			return
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read finalize: %v", err)
			return
		}
		var ctrl map[string]string
		_ = json.Unmarshal(data, &ctrl)
		if ctrl["type"] != "Finalize" {
			t.Errorf("control type = %q, want Finalize", ctrl["type"])
		}
		sendJSON(t, conn, map[string]any{
			"type": "Results", "is_final": true, "from_finalize": true,
			"channel": map[string]any{
				"alternatives": []map[string]any{
					{"transcript": "hello world", "confidence": 0.98},
				},
			},
		})
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{Language: "en"}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sc.ForceClose()
	if err := sc.Send([]byte{0x00, 0x01}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := sc.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server")
	}
	time.Sleep(50 * time.Millisecond)

	evts := h.Events()
	var result *stt.StreamEvent
	for i := range evts {
		if evts[i].Type == "Results" {
			result = &evts[i]
		}
	}
	if result == nil {
		t.Fatal("no Results event received")
	}
	if !result.IsFinal {
		t.Error("expected is_final=true")
	}
	if !result.FromFinalize {
		t.Error("expected from_finalize=true")
	}
	if result.Transcript() != "hello world" {
		t.Errorf("transcript = %q, want 'hello world'", result.Transcript())
	}
}
