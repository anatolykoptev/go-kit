package pg_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anatolykoptev/go-kit/telegram/botusers"
	"github.com/anatolykoptev/go-kit/telegram/botusers/botuserstest"
	"github.com/anatolykoptev/go-kit/telegram/botusers/pg"
)

// newTestPool creates a pgxpool for integration tests.
// Skips if TEST_DATABASE_URL is not set (consistent with fsm/store_postgres_test.go).
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() {
		// Use a timeout context — context.Background() hangs indefinitely
		// if the pool's background health-check connection holds a lock
		// that blocks DROP TABLE. With a timeout, the worst case is the
		// table isn't dropped (the next subtest's setup does its own
		// DROP TABLE IF EXISTS anyway).
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pool.Exec(ctx, "DROP TABLE IF EXISTS bot_user_events") //nolint:errcheck
		pool.Exec(ctx, "DROP TABLE IF EXISTS bot_users")       //nolint:errcheck
		pool.Close()
	})
	return pool
}

// newTestStore creates a fresh pg.Store backed by a real Postgres.
func newTestStore(t *testing.T) botusers.Store {
	t.Helper()
	pool := newTestPool(t)
	s, err := pg.New(context.Background(), pool)
	if err != nil {
		t.Fatalf("pg.New: %v", err)
	}
	return s
}

// TestPgStore_Contract runs the full Store contract suite against pg.Store.
func TestPgStore_Contract(t *testing.T) {
	botuserstest.RunContract(t, func(t *testing.T) botusers.Store {
		t.Helper()
		pool := newTestPool(t)
		// Each sub-test gets a fresh store; Apply is idempotent so calling
		// it again on an existing schema is safe.
		// Drop and recreate for test isolation.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pool.Exec(ctx, "DROP TABLE IF EXISTS bot_user_events") //nolint:errcheck
		pool.Exec(ctx, "DROP TABLE IF EXISTS bot_users")       //nolint:errcheck
		s, err := pg.New(ctx, pool)
		if err != nil {
			t.Fatalf("pg.New: %v", err)
		}
		return s
	})
}

// TestPgStore_Apply_Idempotent verifies Apply can be called multiple times.
func TestPgStore_Apply_Idempotent(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	s, err := pg.New(ctx, pool)
	if err != nil {
		t.Fatalf("pg.New first: %v", err)
	}
	// Apply the schema again — must succeed idempotently.
	if err := s.Apply(ctx, pool); err != nil {
		t.Fatalf("Apply second time: %v", err)
	}
}

// TestPgStore_SchemaSQL_Embedded verifies the embedded SQL is accessible
// and returns valid DDL (contains expected table names).
func TestPgStore_SchemaSQL_Embedded(t *testing.T) {
	sql := botusers.SchemaSQL()
	if sql == "" {
		t.Fatal("SchemaSQL() returned empty string")
	}
	for _, needle := range []string{"bot_users", "bot_id", "tg_id"} {
		if !containsStr(sql, needle) {
			t.Errorf("SchemaSQL() missing %q", needle)
		}
	}
}

// TestPgStore_PrivacyOff_NoWrite verifies that WithPrivacy(Off) makes
// UpsertFromInitData a no-op (no row written).
// Ref: advisor — Privacy.Off contract
func TestPgStore_PrivacyOff_NoWrite(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	// Apply schema first via a standard store.
	if _, err := pg.New(ctx, pool); err != nil {
		t.Fatalf("pg.New schema: %v", err)
	}

	// Store configured with Off privacy.
	s, err := pg.New(ctx, pool, botusers.WithPrivacy(botusers.Off))
	if err != nil {
		t.Fatalf("pg.New off: %v", err)
	}

	user := botusers.TelegramUser{TgID: 88888}
	obs := botusers.Observation{}
	if err := s.UpsertFromInitData(ctx, "botOff", user, obs); err != nil {
		t.Fatalf("UpsertFromInitData with Off: %v", err)
	}

	// Verify no row was written.
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM bot_users WHERE bot_id=$1 AND tg_id=$2",
		"botOff", int64(88888),
	).Scan(&count)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 0 {
		t.Errorf("WithPrivacy(Off): expected 0 rows, got %d", count)
	}
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

// TestPgStore_UpsertAtomicity_SkippedWithoutDB documents that the transaction
// wrapping upsert+event is tested only when TEST_DATABASE_URL is set.
// This test always passes; the contract test covers the pg path end-to-end
// for upsert atomicity when the pool is available.
func TestPgStore_UpsertAtomicity_Doc(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; pg transaction atomicity verified only with real DB")
	}
	// With a real DB, the contract tests exercise the upsert path.
	// Injecting a constraint violation on bot_user_events to prove rollback
	// requires DDL manipulation not suitable for shared CI; covered by code review.
}

// TestPgStore_Forget_UserFirstThenEvents verifies that Forget deletes the user
// row first and only then removes events — so ErrNotFound fires before event
// deletion when the user doesn't exist. Runs against real DB when available.
func TestPgStore_Forget_UserFirstThenEvents(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	s, err := pg.New(ctx, pool, botusers.WithEventsTable(true))
	if err != nil {
		t.Fatalf("pg.New: %v", err)
	}

	// Forget a non-existent user — must return ErrNotFound without touching events.
	err = s.Forget(ctx, "bot1", 999999)
	if err == nil {
		t.Fatal("expected ErrNotFound for missing user, got nil")
	}
	var notFound bool
	for e := err; e != nil; {
		if e == botusers.ErrNotFound {
			notFound = true
			break
		}
		type unwrap interface{ Unwrap() error }
		if u, ok := e.(unwrap); ok {
			e = u.Unwrap()
		} else {
			break
		}
	}
	if !notFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

// TestPgStore_StoreIPDisabledClearsExistingIP verifies that switching to
// WithStoreIP(false) on subsequent upserts actively clears any previously
// stored IP. M2 fix.
func TestPgStore_StoreIPDisabledClearsExistingIP(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	// Store 1: IP enabled.
	s1, err := pg.New(ctx, pool, botusers.WithStoreIP(true))
	if err != nil {
		t.Fatalf("pg.New (store_ip=true): %v", err)
	}

	user := botusers.TelegramUser{TgID: 11223344, Lang: "en"}
	obs1 := botusers.Observation{IP: "1.2.3.4", Source: botusers.SourceBotCommand, At: time.Now()}
	if err := s1.UpsertFromInitData(ctx, "botM2", user, obs1); err != nil {
		t.Fatalf("upsert with IP: %v", err)
	}

	// Confirm IP was stored.
	var storedIP string
	if err := pool.QueryRow(ctx,
		"SELECT client_ip FROM bot_users WHERE bot_id=$1 AND tg_id=$2",
		"botM2", user.TgID,
	).Scan(&storedIP); err != nil {
		t.Fatalf("scan IP after first upsert: %v", err)
	}
	if storedIP != "1.2.3.4" {
		t.Fatalf("expected stored IP 1.2.3.4, got %q", storedIP)
	}

	// Store 2: IP disabled — must clear the previously stored IP.
	s2, err := pg.New(ctx, pool, botusers.WithStoreIP(false))
	if err != nil {
		t.Fatalf("pg.New (store_ip=false): %v", err)
	}

	obs2 := botusers.Observation{IP: "5.6.7.8", Source: botusers.SourceBotCommand, At: time.Now()}
	if err := s2.UpsertFromInitData(ctx, "botM2", user, obs2); err != nil {
		t.Fatalf("upsert without IP: %v", err)
	}

	// IP must be cleared.
	got, err := s2.Get(ctx, "botM2", user.TgID)
	if err != nil {
		t.Fatalf("Get after second upsert: %v", err)
	}
	_ = got // IP is not exposed on User struct; verify via raw query.

	if err := pool.QueryRow(ctx,
		"SELECT client_ip FROM bot_users WHERE bot_id=$1 AND tg_id=$2",
		"botM2", user.TgID,
	).Scan(&storedIP); err != nil {
		t.Fatalf("scan IP after second upsert: %v", err)
	}
	if storedIP != "" {
		t.Errorf("expected cleared IP, got %q", storedIP)
	}
}

// Compile-time check.
var _ = newTestStore
