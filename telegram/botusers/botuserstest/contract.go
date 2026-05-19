// Package botuserstest provides a reusable contract test suite for
// botusers.Store implementations.
//
// Usage in an implementation package:
//
//	func TestMyStore_Contract(t *testing.T) {
//	    botuserstest.RunContract(t, func(t *testing.T) botusers.Store {
//	        // return a fresh, empty store backed by whatever backend
//	        return newMyStore(t)
//	    })
//	}
package botuserstest

import (
	"context"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/telegram/botusers"
)

// RunContract runs the full Store contract test suite against a Store
// produced by newStore. newStore is called once per sub-test to ensure
// isolation.
func RunContract(t *testing.T, newStore func(t *testing.T) botusers.Store) {
	t.Helper()

	t.Run("UpsertFromInitData_BasicRoundTrip", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		const botID = "bot1"
		user := botusers.TelegramUser{
			TgID:      12345,
			Username:  "alice",
			FirstName: "Alice",
			LastName:  "Smith",
			Lang:      "en",
		}
		obs := botusers.Observation{Source: botusers.SourceMiniAppInit, At: time.Now()}
		if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
			t.Fatalf("UpsertFromInitData: %v", err)
		}
		got, err := s.Get(ctx, botID, user.TgID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got == nil {
			t.Fatal("Get: expected non-nil user")
		}
		if got.TgID != user.TgID {
			t.Errorf("TgID: want %d got %d", user.TgID, got.TgID)
		}
		if got.Username != user.Username {
			t.Errorf("Username: want %q got %q", user.Username, got.Username)
		}
		if got.FirstName != user.FirstName {
			t.Errorf("FirstName: want %q got %q", user.FirstName, got.FirstName)
		}
		if got.TotalObservations != 1 {
			t.Errorf("TotalObservations: want 1 got %d", got.TotalObservations)
		}

	})

	t.Run("UpsertFromInitData_FirstSeenAtImmutable", func(t *testing.T) {
		// Ref: advisor — classic bug: ON CONFLICT sets first_seen_at = EXCLUDED.first_seen_at
		s := newStore(t)
		ctx := context.Background()
		const botID = "bot1"
		user := botusers.TelegramUser{TgID: 111}
		obs := botusers.Observation{At: time.Now()}

		if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
			t.Fatalf("first UpsertFromInitData: %v", err)
		}
		first, err := s.Get(ctx, botID, user.TgID)
		if err != nil || first == nil {
			t.Fatalf("Get after first upsert: err=%v user=%v", err, first)
		}
		firstSeenAt := first.FirstSeenAt

		// Wait a tiny bit to ensure clock advances, then upsert again.
		time.Sleep(2 * time.Millisecond)
		if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
			t.Fatalf("second UpsertFromInitData: %v", err)
		}
		second, err := s.Get(ctx, botID, user.TgID)
		if err != nil || second == nil {
			t.Fatalf("Get after second upsert: err=%v user=%v", err, second)
		}

		// first_seen_at must not change across re-upserts.
		if !second.FirstSeenAt.Equal(firstSeenAt) {
			t.Errorf("FirstSeenAt changed: was %v now %v", firstSeenAt, second.FirstSeenAt)
		}
	})

	t.Run("UpsertFromInitData_ObservationCounterIncrements", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		const botID = "bot1"
		user := botusers.TelegramUser{TgID: 222}
		obs := botusers.Observation{At: time.Now()}

		for i := 1; i <= 3; i++ {
			if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
				t.Fatalf("UpsertFromInitData #%d: %v", i, err)
			}
			got, err := s.Get(ctx, botID, user.TgID)
			if err != nil || got == nil {
				t.Fatalf("Get after upsert #%d: err=%v", i, err)
			}
			if got.TotalObservations != int64(i) {
				t.Errorf("after upsert #%d: TotalObservations want %d got %d", i, i, got.TotalObservations)
			}
		}
	})

	t.Run("UpsertFromInitData_LastSeenAtUpdates", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		const botID = "bot1"
		user := botusers.TelegramUser{TgID: 333}

		obs1 := botusers.Observation{At: time.Now()}
		if err := s.UpsertFromInitData(ctx, botID, user, obs1); err != nil {
			t.Fatalf("first upsert: %v", err)
		}
		first, _ := s.Get(ctx, botID, user.TgID)
		firstLastSeen := first.LastSeenAt

		time.Sleep(2 * time.Millisecond)
		obs2 := botusers.Observation{At: time.Now()}
		if err := s.UpsertFromInitData(ctx, botID, user, obs2); err != nil {
			t.Fatalf("second upsert: %v", err)
		}
		second, _ := s.Get(ctx, botID, user.TgID)

		if !second.LastSeenAt.After(firstLastSeen) {
			t.Errorf("LastSeenAt did not advance: first=%v second=%v", firstLastSeen, second.LastSeenAt)
		}
	})

	t.Run("UpsertFromCommand_BasicRoundTrip", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		const botID = "bot1"
		obs := botusers.Observation{Source: botusers.SourceBotCommand, At: time.Now()}
		if err := s.UpsertFromCommand(ctx, botID, 99999, "ru", obs); err != nil {
			t.Fatalf("UpsertFromCommand: %v", err)
		}
		got, err := s.Get(ctx, botID, 99999)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got == nil {
			t.Fatal("Get: nil")
		}
		if got.Lang != "ru" {
			t.Errorf("Lang: want ru got %q", got.Lang)
		}
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		_, err := s.Get(ctx, "bot1", 9999999)
		if err == nil {
			t.Fatal("expected ErrNotFound, got nil")
		}
		if !isNotFound(err) {
			t.Fatalf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("MultiTenancy_SameTgIDDifferentBotIDs", func(t *testing.T) {
		// Ref: advisor — composite PK test
		s := newStore(t)
		ctx := context.Background()
		user := botusers.TelegramUser{TgID: 55555, Username: "shared"}
		obs := botusers.Observation{At: time.Now()}

		if err := s.UpsertFromInitData(ctx, "botA", user, obs); err != nil {
			t.Fatalf("upsert botA: %v", err)
		}
		if err := s.UpsertFromInitData(ctx, "botB", user, obs); err != nil {
			t.Fatalf("upsert botB: %v", err)
		}

		gA, err := s.Get(ctx, "botA", user.TgID)
		if err != nil || gA == nil {
			t.Fatalf("Get botA: err=%v", err)
		}
		gB, err := s.Get(ctx, "botB", user.TgID)
		if err != nil || gB == nil {
			t.Fatalf("Get botB: err=%v", err)
		}
		if gA.BotID != "botA" {
			t.Errorf("gA.BotID: want botA got %q", gA.BotID)
		}
		if gB.BotID != "botB" {
			t.Errorf("gB.BotID: want botB got %q", gB.BotID)
		}
	})

	t.Run("ErrBotIDRequired_EmptyBotID", func(t *testing.T) {
		// Ref: advisor — ErrBotIDRequired when no per-call and no default.
		s := newStore(t)
		ctx := context.Background()
		user := botusers.TelegramUser{TgID: 1}
		obs := botusers.Observation{At: time.Now()}

		err := s.UpsertFromInitData(ctx, "", user, obs)
		if err == nil {
			t.Fatal("expected ErrBotIDRequired, got nil")
		}
		if !isBotIDRequired(err) {
			t.Fatalf("expected ErrBotIDRequired, got: %v", err)
		}
	})

	t.Run("Forget_DeletesUser", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		const botID = "bot1"
		user := botusers.TelegramUser{TgID: 777}
		obs := botusers.Observation{At: time.Now()}
		if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		if err := s.Forget(ctx, botID, user.TgID); err != nil {
			t.Fatalf("Forget: %v", err)
		}
		_, err := s.Get(ctx, botID, user.TgID)
		if !isNotFound(err) {
			t.Fatalf("expected ErrNotFound after Forget, got: %v", err)
		}
	})

	t.Run("Forget_Idempotent", func(t *testing.T) {
		// Ref: advisor — Forget on missing row; contract says ErrNotFound.
		s := newStore(t)
		ctx := context.Background()
		err := s.Forget(ctx, "bot1", 9999888)
		if err == nil {
			t.Fatal("expected ErrNotFound for non-existent user, got nil")
		}
		if !isNotFound(err) {
			t.Fatalf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("Aggregate_Counts", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		const botID = "botAgg"

		// Insert 3 users.
		for i := int64(1); i <= 3; i++ {
			user := botusers.TelegramUser{TgID: i}
			obs := botusers.Observation{At: time.Now()}
			if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
				t.Fatalf("upsert %d: %v", i, err)
			}
		}

		agg, err := s.Aggregate(ctx, botID)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		if agg.Total != 3 {
			t.Errorf("Total: want 3 got %d", agg.Total)
		}
		if agg.Active1D != 3 {
			t.Errorf("Active1D: want 3 got %d", agg.Active1D)
		}
		if agg.Active7D != 3 {
			t.Errorf("Active7D: want 3 got %d", agg.Active7D)
		}
		if agg.Active30D != 3 {
			t.Errorf("Active30D: want 3 got %d", agg.Active30D)
		}
	})

	t.Run("List_BasicPagination", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		const botID = "botList"

		for i := int64(1); i <= 5; i++ {
			user := botusers.TelegramUser{TgID: i}
			obs := botusers.Observation{At: time.Now()}
			time.Sleep(time.Millisecond) // ensure distinct last_seen_at
			if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
				t.Fatalf("upsert %d: %v", i, err)
			}
		}

		// Page 1: 3 users.
		page1, cursor, err := s.List(ctx, botusers.Filter{BotID: botID, Limit: 3})
		if err != nil {
			t.Fatalf("List page 1: %v", err)
		}
		if len(page1) != 3 {
			t.Fatalf("List page 1: want 3 users, got %d", len(page1))
		}
		if cursor.IsZero() {
			t.Fatal("List page 1: expected non-zero cursor")
		}

		// Page 2: remaining 2 users.
		page2, cursor2, err := s.List(ctx, botusers.Filter{BotID: botID, Limit: 3, Cursor: &cursor})
		if err != nil {
			t.Fatalf("List page 2: %v", err)
		}
		if len(page2) != 2 {
			t.Fatalf("List page 2: want 2 users, got %d", len(page2))
		}
		_ = cursor2

		// Verify no overlap.
		seen := map[int64]bool{}
		for _, u := range page1 {
			seen[u.TgID] = true
		}
		for _, u := range page2 {
			if seen[u.TgID] {
				t.Errorf("TgID %d appears in both pages", u.TgID)
			}
		}
	})

	t.Run("DeleteInactive_RemovesOldUsers", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()
		const botID = "botRetain"
		user := botusers.TelegramUser{TgID: 42}
		obs := botusers.Observation{At: time.Now()}
		if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
			t.Fatalf("upsert: %v", err)
		}

		// Delete users inactive for more than 0 time — the just-inserted user
		// has last_seen_at=now so should NOT be deleted (edge case: 0 duration
		// means "older than exactly now").
		deleted, err := s.DeleteInactive(ctx, botID, 24*time.Hour)
		if err != nil {
			t.Fatalf("DeleteInactive: %v", err)
		}
		// The user was just inserted; should survive a 24h inactivity window.
		if deleted != 0 {
			t.Errorf("DeleteInactive(24h): expected 0 deleted, got %d", deleted)
		}

		// Confirm user still exists.
		got, err := s.Get(ctx, botID, user.TgID)
		if err != nil || got == nil {
			t.Fatalf("Get after DeleteInactive: err=%v got=%v", err, got)
		}
	})

	t.Run("UpsertFromCommand_PreservesPriorNames", func(t *testing.T) {
		// C1: UpsertFromCommand must not clobber names written by UpsertFromInitData.
		// When called with empty names, existing username/first_name/last_name must be preserved.
		s := newStore(t)
		ctx := context.Background()
		const botID = "bot1"

		// Step 1: upsert via init_data (has full names).
		userFull := botusers.TelegramUser{
			TgID:      88888,
			Username:  "carol",
			FirstName: "Carol",
			LastName:  "Jones",
			Lang:      "en",
		}
		obs1 := botusers.Observation{Source: botusers.SourceMiniAppInit, At: time.Now()}
		if err := s.UpsertFromInitData(ctx, botID, userFull, obs1); err != nil {
			t.Fatalf("UpsertFromInitData: %v", err)
		}

		// Step 2: upsert via command (no names in this path).
		time.Sleep(time.Millisecond)
		obs2 := botusers.Observation{Source: botusers.SourceBotCommand, At: time.Now()}
		if err := s.UpsertFromCommand(ctx, botID, userFull.TgID, "ru", obs2); err != nil {
			t.Fatalf("UpsertFromCommand: %v", err)
		}

		// Step 3: names must be preserved, lang must update.
		got, err := s.Get(ctx, botID, userFull.TgID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Username != "carol" {
			t.Errorf("Username: want %q got %q", "carol", got.Username)
		}
		if got.FirstName != "Carol" {
			t.Errorf("FirstName: want %q got %q", "Carol", got.FirstName)
		}
		if got.LastName != "Jones" {
			t.Errorf("LastName: want %q got %q", "Jones", got.LastName)
		}
		if got.Lang != "ru" {
			t.Errorf("Lang: want %q got %q", "ru", got.Lang)
		}
	})

	t.Run("Aggregate_TopCountries_Typed", func(t *testing.T) {
		// M3: TopCountries must return typed CountryCount values, not [2]any.
		s := newStore(t)
		ctx := context.Background()
		const botID = "botCC"

		// Insert users with distinct countries.
		for i, country := range []string{"US", "US", "DE"} {
			user := botusers.TelegramUser{TgID: int64(9000 + i)}
			obs := botusers.Observation{Country: country, At: time.Now()}
			if err := s.UpsertFromInitData(ctx, botID, user, obs); err != nil {
				t.Fatalf("upsert %d: %v", i, err)
			}
		}

		agg, err := s.Aggregate(ctx, botID)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		if len(agg.TopCountries) == 0 {
			t.Fatal("expected non-empty TopCountries")
		}
		// Access typed fields — compile error if type is still [2]any.
		top := agg.TopCountries[0]
		if top.Code == "" {
			t.Error("TopCountries[0].Code must not be empty")
		}
		if top.Count <= 0 {
			t.Errorf("TopCountries[0].Count must be > 0, got %d", top.Count)
		}
		// US appears twice — must be first.
		if top.Code != "US" {
			t.Errorf("TopCountries[0].Code: want US got %q", top.Code)
		}
		if top.Count != 2 {
			t.Errorf("TopCountries[0].Count: want 2 got %d", top.Count)
		}
	})


	t.Run("CursorTampered_ReturnsErrCursor", func(t *testing.T) {
		// MED5: a tampered/invalid cursor string must return a wrapped ErrCursor,
		// not a raw base64 or JSON error.
		s := newStore(t)
		ctx := context.Background()
		badCursor := botusers.CursorFromString("!!!not-valid-base64!!!")
		_, _, err := s.List(ctx, botusers.Filter{
			BotID:  "bot1",
			Cursor: &badCursor,
		})
		if err == nil {
			t.Fatal("expected error for tampered cursor, got nil")
		}
		if !isCursorErr(err) {
			t.Fatalf("expected ErrCursor-wrapped error, got: %v", err)
		}
	})

}

// isNotFound reports whether err is or wraps botusers.ErrNotFound.
func isNotFound(err error) bool {
	return isErr(err, botusers.ErrNotFound)
}

// isBotIDRequired reports whether err is or wraps botusers.ErrBotIDRequired.
func isBotIDRequired(err error) bool {
	return isErr(err, botusers.ErrBotIDRequired)
}

// isCursorErr reports whether err wraps ErrCursor.
func isCursorErr(err error) bool {
	return isErr(err, botusers.ErrCursor)
}

func isErr(err, target error) bool {
	if err == nil {
		return false
	}
	// errors.Is handles wrapping.
	type iser interface{ Is(error) bool }
	// Use errors package via loop — avoid import cycle.
	for err != nil {
		if err == target {
			return true
		}
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
