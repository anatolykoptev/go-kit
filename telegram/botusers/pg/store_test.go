package pg_test

import (
	"context"
	"os"
	"testing"

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
		// Clean up tables so tests are repeatable.
		pool.Exec(context.Background(), "DROP TABLE IF EXISTS bot_user_events") //nolint:errcheck
		pool.Exec(context.Background(), "DROP TABLE IF EXISTS bot_users")       //nolint:errcheck
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
		pool.Exec(context.Background(), "DROP TABLE IF EXISTS bot_user_events") //nolint:errcheck
		pool.Exec(context.Background(), "DROP TABLE IF EXISTS bot_users")       //nolint:errcheck
		s, err := pg.New(context.Background(), pool)
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

// Compile-time check.
var _ = newTestStore
