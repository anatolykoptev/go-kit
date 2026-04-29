package rerank

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestApproxTokens_Cyrillic(t *testing.T) {
	// 100 Cyrillic runes → approx 100/1.5 ≈ 66 tokens
	text := strings.Repeat("я", 100)
	got := approxTokens(text)
	// Allow ±5 for rounding
	if got < 60 || got > 72 {
		t.Errorf("100 Cyrillic runes → %d tokens, want 60-72", got)
	}
}

func TestApproxTokens_Latin(t *testing.T) {
	// 100 ASCII chars → approx 100/4 = 25 tokens
	text := strings.Repeat("a", 100)
	got := approxTokens(text)
	if got < 20 || got > 30 {
		t.Errorf("100 Latin runes → %d tokens, want 20-30", got)
	}
}

func TestApproxTokens_Mixed(t *testing.T) {
	// 50 Cyrillic + 50 Latin
	text := strings.Repeat("я", 50) + strings.Repeat("a", 50)
	got := approxTokens(text)
	// 50/1.5 ≈ 33 + 50/4 = 12 → ~45 ± tolerance
	if got < 35 || got > 55 {
		t.Errorf("mixed text → %d tokens, want 35-55", got)
	}
}

func TestTruncateToTokens_RuneSafe(t *testing.T) {
	// Build a string with multi-byte Cyrillic runes.
	text := strings.Repeat("привет", 20) // 120 Cyrillic runes, each is 2 bytes
	truncated, before, after := truncateToTokens(text, 10)

	// Verify no split rune — must be valid UTF-8.
	if !utf8.ValidString(truncated) {
		t.Error("truncated string is not valid UTF-8")
	}
	if after > before {
		t.Errorf("afterTok %d > beforeTok %d", after, before)
	}
	if after > 10 {
		t.Errorf("afterTok %d exceeds maxTokens 10", after)
	}
}

func TestTruncateToTokens_OnTruncateHookFires(t *testing.T) {
	var firedDocID string
	var firedBefore, firedAfter int

	obs := &truncateObserver{
		onTruncate: func(ctx context.Context, docID string, before, after int) {
			firedDocID = docID
			firedBefore = before
			firedAfter = after
		},
	}

	// Server that accepts anything and returns a score.
	srv := v1TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		v1JSONResp(w, cohereResponse{Results: []cohereResult{{Index: 0, RelevanceScore: 0.9}}})
	})

	c := NewClient(srv.URL,
		WithModel("m"),
		WithTimeout(time.Second),
		WithMaxTokensPerDoc(5), // very small limit to force truncation
		WithObserver(obs),
	)
	docs := []Doc{{ID: "myDoc", Text: strings.Repeat("hello world ", 50)}}
	_, _ = c.RerankWithResult(context.Background(), "q", docs)

	if firedDocID != "myDoc" {
		t.Errorf("OnTruncate docID: got %q want %q", firedDocID, "myDoc")
	}
	if firedBefore <= firedAfter {
		t.Errorf("OnTruncate before (%d) should be > after (%d)", firedBefore, firedAfter)
	}
}

// truncateObserver implements Observer with a configurable OnTruncate callback.
type truncateObserver struct {
	noopObserver
	onTruncate func(ctx context.Context, docID string, before, after int)
}

func (o *truncateObserver) OnTruncate(ctx context.Context, docID string, before, after int) {
	if o.onTruncate != nil {
		o.onTruncate(ctx, docID, before, after)
	}
}
