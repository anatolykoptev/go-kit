package stt_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestChannelStreamReceive connects, sends audio, and reads from Events().
func TestChannelStreamReceive(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		sendJSON(t, conn, stt.StreamEvent{Type: "Metadata", RequestID: "ch-1", Channels: 1})
		conn.ReadMessage() //nolint:errcheck // drain
	})
	defer srv.Close()

	cs, err := stt.StreamWithChannels(context.Background(), wsURL(srv.URL), stt.StreamParams{Language: "en"})
	if err != nil {
		t.Fatalf("StreamWithChannels: %v", err)
	}
	defer cs.ForceClose()

	var got stt.StreamEvent
	select {
	case e, ok := <-cs.Events():
		if !ok {
			t.Fatal("events channel closed unexpectedly")
		}
		got = e
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	if got.Type != "Metadata" {
		t.Errorf("type = %q, want Metadata", got.Type)
	}
	if got.RequestID != "ch-1" {
		t.Errorf("request_id = %q, want ch-1", got.RequestID)
	}
}

// TestChannelStreamClose verifies that Close() causes the Events channel to close.
func TestChannelStreamClose(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var ctrl map[string]string
		_ = json.Unmarshal(data, &ctrl)
		if ctrl["type"] == "CloseStream" {
			sendJSON(t, conn, stt.StreamEvent{Type: "CloseStream"})
			conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		}
	})
	defer srv.Close()

	cs, err := stt.StreamWithChannels(context.Background(), wsURL(srv.URL), stt.StreamParams{})
	if err != nil {
		t.Fatalf("StreamWithChannels: %v", err)
	}

	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Drain remaining events then verify channel is closed.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-cs.Events():
			if !ok {
				return // channel closed — success
			}
		case <-timeout:
			t.Fatal("timeout: Events() channel not closed after Close()")
		}
	}
}

// TestStreamFile creates a temp file with fake PCM data, streams it, and verifies events.
func TestStreamFile(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		// Drain all binary frames until Finalize control.
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt == websocket.TextMessage {
				var ctrl map[string]string
				_ = json.Unmarshal(data, &ctrl)
				if ctrl["type"] == "Finalize" {
					break
				}
				if ctrl["type"] == "CloseStream" {
					sendJSON(t, conn, stt.StreamEvent{Type: "CloseStream"})
					conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return
				}
			}
		}
		sendJSON(t, conn, map[string]any{
			"type": "Results", "is_final": true, "from_finalize": true,
			"channel": map[string]any{
				"alternatives": []map[string]any{
					{"transcript": "test audio", "confidence": 0.95},
				},
			},
		})
		// Drain CloseStream.
		conn.ReadMessage() //nolint:errcheck
		sendJSON(t, conn, stt.StreamEvent{Type: "CloseStream"})
		conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	})
	defer srv.Close()

	// Write 1600 bytes of fake PCM (50ms of s16le at 16kHz).
	f, err := os.CreateTemp(t.TempDir(), "audio*.pcm")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_, _ = f.Write(make([]byte, 1600))
	_ = f.Close()

	events, err := stt.StreamFile(
		context.Background(),
		wsURL(srv.URL),
		f.Name(),
		stt.WithStreamLanguage("en"),
		stt.WithChunkDuration(50*time.Millisecond),
		stt.WithStreamSampleRate(16000),
		stt.WithInterim(false),
	)
	if err != nil {
		t.Fatalf("StreamFile: %v", err)
	}

	var found bool
	for _, e := range events {
		if e.Type == "Results" && e.FromFinalize {
			found = true
			if e.Transcript() != "test audio" {
				t.Errorf("transcript = %q, want 'test audio'", e.Transcript())
			}
		}
	}
	if !found {
		t.Errorf("no from_finalize Results event in %v", events)
	}
}

// TestStreamFileCancel cancels context mid-stream and verifies clean shutdown.
func TestStreamFileCancel(t *testing.T) {
	srv := newMockWSServer(t, func(conn *websocket.Conn) {
		// Just drain messages and ignore everything — client will cancel.
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	// 320000 bytes = 10s of 16kHz s16le audio — guarantees context cancels mid-stream.
	f, err := os.CreateTemp(t.TempDir(), "audio*.pcm")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_, _ = f.Write(make([]byte, 320000))
	_ = f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err = stt.StreamFile(
		ctx,
		wsURL(srv.URL),
		f.Name(),
		stt.WithChunkDuration(100*time.Millisecond),
		stt.WithStreamSampleRate(16000),
	)
	if err == nil {
		t.Error("expected error due to context cancellation, got nil")
	}
}
