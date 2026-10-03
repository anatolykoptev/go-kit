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

// finalizeAcceptingServer is a WS handler that accepts audio chunks, responds to
// Finalize with a from_finalize Results event, and handles CloseStream cleanly.
func finalizeAcceptingServer(t *testing.T) func(*websocket.Conn) {
	t.Helper()
	return func(conn *websocket.Conn) {
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
	}
}

func TestChunkSizeBytesEdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		sampleRate int
		duration   time.Duration
		fileBytes  int
	}{
		{"16kHz_100ms", 16000, 100 * time.Millisecond, 3200},
		{"44100Hz_50ms", 44100, 50 * time.Millisecond, 4410},
		{"16kHz_50ms", 16000, 50 * time.Millisecond, 1600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newMockWSServer(t, finalizeAcceptingServer(t))
			defer srv.Close()

			f, err := os.CreateTemp(t.TempDir(), "pcm*.raw")
			if err != nil {
				t.Fatalf("create temp: %v", err)
			}
			_, _ = f.Write(make([]byte, tt.fileBytes))
			_ = f.Close()

			events, err := stt.StreamFile(context.Background(), wsURL(srv.URL), f.Name(),
				stt.WithStreamSampleRate(tt.sampleRate),
				stt.WithChunkDuration(tt.duration),
			)
			if err != nil {
				t.Fatalf("StreamFile: %v", err)
			}
			var hasFinal bool
			for _, e := range events {
				if e.FromFinalize {
					hasFinal = true
				}
			}
			if !hasFinal {
				t.Errorf("no from_finalize event for sampleRate=%d duration=%v", tt.sampleRate, tt.duration)
			}
		})
	}
}
