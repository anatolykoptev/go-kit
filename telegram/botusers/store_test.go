package botusers_test

import (
	"errors"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/telegram/botusers"
	"github.com/anatolykoptev/go-kit/telegram/botusers/botuserstest"
)

// TestSchemaSQL verifies that SchemaSQL() returns a non-empty string
// containing the main table DDL.
func TestSchemaSQL(t *testing.T) {
	sql := botusers.SchemaSQL()
	if sql == "" {
		t.Fatal("SchemaSQL() returned empty string")
	}
	if !containsStr(sql, "bot_users") {
		t.Error("SchemaSQL() does not mention bot_users table")
	}
}

// TestSentinelErrors verifies the sentinel errors are distinct.
func TestSentinelErrors(t *testing.T) {
	if errors.Is(botusers.ErrBotIDRequired, botusers.ErrNotFound) {
		t.Error("ErrBotIDRequired and ErrNotFound must be distinct")
	}
	if errors.Is(botusers.ErrNotFound, botusers.ErrBotIDRequired) {
		t.Error("ErrNotFound and ErrBotIDRequired must be distinct")
	}
}

// TestPrivacyEnum verifies the two privacy values are distinct.
func TestPrivacyEnum(t *testing.T) {
	vals := []botusers.Privacy{botusers.Off, botusers.SoftOptIn}
	for i, a := range vals {
		for j, b := range vals {
			if i != j && a == b {
				t.Errorf("Privacy values at index %d and %d are equal: %v", i, j, a)
			}
		}
	}
}

// TestCursorEncodeDecode verifies the cursor codec round-trips correctly.
func TestCursorEncodeDecode(t *testing.T) {
	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	c := botusers.EncodeCursor(ts, 99999)
	if c.IsZero() {
		t.Fatal("EncodeCursor returned zero cursor")
	}
	gotTs, gotID, err := botusers.DecodeCursor(c)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if !gotTs.Equal(ts) {
		t.Errorf("time: want %v got %v", ts, gotTs)
	}
	if gotID != 99999 {
		t.Errorf("tgID: want 99999 got %d", gotID)
	}
}

// TestCursorFromString verifies the string round-trip.
func TestCursorFromString(t *testing.T) {
	c := botusers.EncodeCursor(time.Now(), 12345)
	s := c.String()
	c2 := botusers.CursorFromString(s)
	if c.String() != c2.String() {
		t.Errorf("CursorFromString: want %q got %q", c.String(), c2.String())
	}
}

// TestCursorIsZero verifies zero-value detection.
func TestCursorIsZero(t *testing.T) {
	var c botusers.Cursor
	if !c.IsZero() {
		t.Error("zero-value Cursor.IsZero() must return true")
	}
	c2 := botusers.EncodeCursor(time.Now(), 1)
	if c2.IsZero() {
		t.Error("non-zero Cursor.IsZero() must return false")
	}
}

// TestContract_InMemory runs the full Store contract suite against the
// in-memory reference implementation from botuserstest.
func TestContract_InMemory(t *testing.T) {
	botuserstest.RunContract(t, func(t *testing.T) botusers.Store {
		t.Helper()
		return botuserstest.NewMemStore()
	})
}

func containsStr(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
