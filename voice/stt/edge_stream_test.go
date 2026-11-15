package stt_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

func TestStreamSendBeforeConnect(t *testing.T) {
	t.Parallel()
	h := &mockHandler{}
	sc := stt.NewStreamClient("ws://127.0.0.1:1", stt.StreamParams{}, h)
	err := sc.Send([]byte{0x01})
	if err == nil {
		t.Fatal("expected error when sending before Connect")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("err = %q, want 'not connected'", err.Error())
	}
}

func TestStreamFinalizeBeforeConnect(t *testing.T) {
	t.Parallel()
	h := &mockHandler{}
	sc := stt.NewStreamClient("ws://127.0.0.1:1", stt.StreamParams{}, h)
	err := sc.Finalize()
	if err == nil {
		t.Fatal("expected error when calling Finalize before Connect")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("err = %q, want 'not connected'", err.Error())
	}
}

func TestStreamDoubleClose(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var ctrl map[string]string
		_ = json.Unmarshal(data, &ctrl)
		if ctrl["type"] == "CloseStream" {
			sendJSON(t, conn, stt.StreamEvent{Type: "CloseStream"})
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		}
		// drain until client fully closes
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := sc.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// Second Close() must return an error, not panic.
	if err := sc.Close(); err == nil {
		t.Error("expected error on second Close()")
	}
}

func TestStreamConnectUnreachable(t *testing.T) {
	t.Parallel()
	h := &mockHandler{}
	sc := stt.NewStreamClient("ws://127.0.0.1:1", stt.StreamParams{}, h)
	if err := sc.Connect(context.Background()); err == nil {
		t.Fatal("expected dial error to unreachable address")
	}
}

func TestStreamServerSendsInvalidJSON(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("not json"))
		conn.ReadMessage() //nolint:errcheck // drain
	})
	defer srv.Close()

	h := &mockHandler{}
	sc := stt.NewStreamClient(wsURL(srv.URL), stt.StreamParams{}, h)
	if err := sc.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sc.ForceClose()
	time.Sleep(80 * time.Millisecond)

	if errs := h.Errors(); len(errs) == 0 {
		t.Error("expected OnError to be called for invalid JSON frame")
	}
}

func TestChannelStreamEventsDropped(t *testing.T) {
	// Server sends 70 events (> buffer of 64); the non-blocking send in channelHandler
	// must not deadlock even when the channel is full.
	const eventCount = 70
	serverDone := make(chan struct{})

	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		defer close(serverDone)
		for range eventCount {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"Results"}`)); err != nil {
				return // client closed — that's fine
			}
		}
		// do NOT read — just return; connection closes via defer conn.Close() in newMockWSServer
	})
	defer srv.Close()

	cs, err := stt.StreamWithChannels(context.Background(), wsURL(srv.URL), stt.StreamParams{})
	if err != nil {
		t.Fatalf("StreamWithChannels: %v", err)
	}
	defer cs.ForceClose()

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server to finish sending")
	}
	time.Sleep(80 * time.Millisecond) // let readLoop process remaining frames
}

func TestStreamFileEmptyFile(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage {
				continue
			}
			var ctrl map[string]string
			_ = json.Unmarshal(data, &ctrl)
			switch ctrl["type"] {
			case "Finalize":
				sendJSON(t, conn, stt.StreamEvent{Type: "Results", IsFinal: true, FromFinalize: true})
			case "CloseStream":
				sendJSON(t, conn, stt.StreamEvent{Type: "CloseStream"})
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
		}
	})
	defer srv.Close()

	f, err := os.CreateTemp(t.TempDir(), "empty*.pcm")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_ = f.Close()

	_, err = stt.StreamFile(context.Background(), wsURL(srv.URL), f.Name())
	if err != nil {
		t.Fatalf("StreamFile on empty file: %v", err)
	}
}

func TestStreamFileNotFound(t *testing.T) {
	t.Parallel()
	_, err := stt.StreamFile(context.Background(), "ws://127.0.0.1:1", "/nonexistent/audio.pcm")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "read audio file") {
		t.Errorf("err = %q, want to contain 'read audio file'", err.Error())
	}
}
