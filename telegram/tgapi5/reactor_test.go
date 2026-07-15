package tgapi5_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"io"

	"github.com/anatolykoptev/go-kit/telegram/tgapi5"
)

// ─── Reactor tests ──────────────────────────────────────────────────────────

func TestReactor_SetReaction_Success(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.SetReaction(context.Background(), 100, 42, []string{"👍", "❤️"}, false)
	if err != nil {
		t.Fatalf("SetReaction: unexpected error: %v", err)
	}
}

func TestReactor_SetReaction_IsBig(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.SetReaction(context.Background(), 100, 42, []string{"🔥"}, true)
	if err != nil {
		t.Fatalf("SetReaction with IsBig: unexpected error: %v", err)
	}
}

func TestReactor_SetReaction_Error(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.SetReaction(context.Background(), 999, 42, []string{"👍"}, false)
	if err == nil {
		t.Fatal("SetReaction: expected error, got nil")
	}
}

// TestReactor_ClearBotReaction verifies the renamed method dispatches
// setMessageReaction with an empty reaction slice (Telegram bot-scope clear).
func TestReactor_ClearBotReaction_CallsSetReactionWithEmptySlice(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, getMeResponse)
			return
		}
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}))
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.ClearBotReaction(context.Background(), 100, 42)
	if err != nil {
		t.Fatalf("ClearBotReaction: unexpected error: %v", err)
	}
	if !strings.HasSuffix(capturedPath, "/setMessageReaction") {
		t.Errorf("ClearBotReaction used endpoint %q; want /setMessageReaction", capturedPath)
	}
}

// TestReactor_SetReaction_EmptyEmojis verifies empty slice is accepted.
func TestReactor_SetReaction_EmptyEmojis(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.SetReaction(context.Background(), 100, 42, []string{}, false)
	if err != nil {
		t.Fatalf("SetReaction empty emojis: unexpected error: %v", err)
	}
}

// TestReactor_AdminRemoveUserReaction verifies correct endpoint and userID plumbing.
func TestReactor_AdminRemoveUserReaction_CorrectEndpoint(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, getMeResponse)
			return
		}
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}))
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.AdminRemoveUserReaction(context.Background(), 100, 42, 9999)
	if err != nil {
		t.Fatalf("AdminRemoveUserReaction: unexpected error: %v", err)
	}
	if !strings.HasSuffix(capturedPath, "/deleteMessageReaction") {
		t.Errorf("AdminRemoveUserReaction used endpoint %q; want /deleteMessageReaction", capturedPath)
	}
}

// TestReactor_AdminRemoveAllUserReactions verifies correct endpoint.
func TestReactor_AdminRemoveAllUserReactions_CorrectEndpoint(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, getMeResponse)
			return
		}
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}))
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.AdminRemoveAllUserReactions(context.Background(), 100, 9999)
	if err != nil {
		t.Fatalf("AdminRemoveAllUserReactions: unexpected error: %v", err)
	}
	if !strings.HasSuffix(capturedPath, "/deleteAllMessageReactions") {
		t.Errorf("AdminRemoveAllUserReactions used endpoint %q; want /deleteAllMessageReactions", capturedPath)
	}
}

// TestReactor_AdminRemoveUserReaction_Error verifies error propagation.
func TestReactor_AdminRemoveUserReaction_Error(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/deleteMessageReaction": `{"ok":false,"error_code":403,"description":"Forbidden: not an admin"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.AdminRemoveUserReaction(context.Background(), 100, 42, 9999)
	if err == nil {
		t.Fatal("AdminRemoveUserReaction: expected error for non-admin, got nil")
	}
}
