package pgutil

// migrate_test.go — integration tests for RunMigrations.
//
// Requires a live Postgres instance. Set MIGRATE_TEST_DSN to the admin DSN
// before running:
//
//	MIGRATE_TEST_DSN=postgres://user:pass@host:5432/admindb go test ./pgutil/
//
// Individual PG* env vars are also accepted as a fallback:
//
//	PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE
//
// If neither is set the tests are skipped. Each test creates a unique scratch
// database, runs its scenario, then drops the database. No prod databases are
// touched.

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// adminDSN returns the admin DSN for creating/dropping scratch databases.
// Requires MIGRATE_TEST_DSN env var, or individual PG* vars.
// Returns "" when neither is set (caller should t.Skip).
func adminDSN() string {
	if v := os.Getenv("MIGRATE_TEST_DSN"); v != "" {
		return v
	}
	host := os.Getenv("PGHOST")
	port := os.Getenv("PGPORT")
	user := os.Getenv("PGUSER")
	pass := os.Getenv("PGPASSWORD")
	db := os.Getenv("PGDATABASE")
	if host == "" || user == "" || pass == "" || db == "" {
		return ""
	}
	if port == "" {
		port = "5432"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, pass, host, port, db)
}

// requireTestDSN skips the test when no DSN is configured.
func requireTestDSN(t *testing.T) string {
	t.Helper()
	dsn := adminDSN()
	if dsn == "" {
		t.Skip("set MIGRATE_TEST_DSN (or PGHOST/PGUSER/PGPASSWORD/PGDATABASE) to run migration tests")
	}
	return dsn
}

// scratchDB creates a throw-away database, returns its DSN, and registers
// cleanup that drops the database when the test finishes.
func scratchDB(t *testing.T) string {
	t.Helper()
	dsn := requireTestDSN(t)
	ctx := context.Background()
	name := fmt.Sprintf("migrate_test_%d_%d", os.Getpid(), time.Now().UnixNano())

	adminPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer adminPool.Close()

	if _, err := adminPool.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatalf("create scratch db %s: %v", name, err)
	}

	// Build DSN for the scratch db.
	parts := strings.Split(dsn, "/")
	scratchDSN := strings.Join(parts[:len(parts)-1], "/") + "/" + name

	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ap, err := pgxpool.New(dropCtx, dsn)
		if err != nil {
			t.Logf("cleanup: connect admin: %v", err)
			return
		}
		defer ap.Close()
		// Terminate existing connections before dropping.
		_, _ = ap.Exec(dropCtx,
			`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`,
			name)
		if _, err := ap.Exec(dropCtx, `DROP DATABASE IF EXISTS `+name); err != nil {
			t.Logf("cleanup: drop %s: %v", name, err)
		}
	})
	return scratchDSN
}

// openPool opens a pgxpool against dsn and registers Close in cleanup.
func openPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// simpleFS builds a minimal fs.FS from a map[filename]sql_content.
func simpleFS(files map[string]string) fs.FS {
	m := make(fstest.MapFS)
	for name, content := range files {
		m[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

// countApplied returns the number of rows in the tracking table.
func countApplied(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("countApplied: %v", err)
	}
	return n
}

// tableExists returns true when the named table exists in the public schema.
func tableExists(t *testing.T, pool *pgxpool.Pool, tableName string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)`, tableName).Scan(&ok); err != nil {
		t.Fatalf("tableExists %s: %v", tableName, err)
	}
	return ok
}

// warnCapture builds a *slog.Logger that appends all Warn messages to a slice.
type warnCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (w *warnCapture) logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&warnWriter{w}, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

type warnWriter struct{ wc *warnCapture }

func (wr *warnWriter) Write(p []byte) (int, error) {
	line := string(bytes.TrimRight(p, "\n"))
	if strings.Contains(line, "level=WARN") {
		wr.wc.mu.Lock()
		wr.wc.msgs = append(wr.wc.msgs, line)
		wr.wc.mu.Unlock()
	}
	return len(p), nil
}

// ─── tests ──────────────────────────────────────────────────────────────────

// TestMigrate_FreshDB — empty tracking, Baseline=nil: all files run in order.
func TestMigrate_FreshDB(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	fsys := simpleFS(map[string]string{
		"0001_create_users.sql": `CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);`,
		"0002_add_email.sql":    `ALTER TABLE users ADD COLUMN email TEXT;`,
	})

	if err := RunMigrations(ctx, pool, fsys, MigrateOptions{}); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if !tableExists(t, pool, "users") {
		t.Fatal("expected users table to exist after fresh migration")
	}

	// Verify email column was added (migration 0002).
	var hasEmail bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'email'
		)`).Scan(&hasEmail); err != nil {
		t.Fatalf("check email column: %v", err)
	}
	if !hasEmail {
		t.Fatal("expected email column from 0002_add_email.sql")
	}

	if n := countApplied(t, pool, "schema_migrations"); n != 2 {
		t.Fatalf("expected 2 tracking rows, got %d", n)
	}
}

// TestMigrate_DirtyProdBackfill — schema present, tracking empty, Baseline=true:
// ZERO files executed, ALL files marked applied. This is the migration-008
// incident class: a non-idempotent ALTER that would fail on re-run must NOT run.
func TestMigrate_DirtyProdBackfill(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	// Simulate a "dirty prod": the schema already exists (accounts table).
	if _, err := pool.Exec(ctx, `CREATE TABLE accounts (id SERIAL PRIMARY KEY);`); err != nil {
		t.Fatalf("seed accounts table: %v", err)
	}

	// The second migration is NON-idempotent: would fail if run on a DB that
	// already has accounts_old. This is the canary — if it executes, the test fails.
	fsys := simpleFS(map[string]string{
		"0001_create_accounts.sql": `CREATE TABLE IF NOT EXISTS accounts (id SERIAL PRIMARY KEY);`,
		"0002_non_idempotent.sql":  `ALTER TABLE accounts RENAME TO accounts_old;`,
	})

	canaryExecuted := false

	baseline := func(_ context.Context, conn *pgxpool.Conn) (bool, error) {
		var ok bool
		err := conn.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = 'accounts'
			)`).Scan(&ok)
		return ok, err
	}

	if err := RunMigrations(ctx, pool, fsys, MigrateOptions{Baseline: baseline}); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// accounts_old must NOT exist — meaning 0002 never ran.
	if tableExists(t, pool, "accounts_old") {
		canaryExecuted = true
	}
	if canaryExecuted {
		t.Fatal("BACKFILL SAFETY VIOLATED: non-idempotent migration was executed on dirty-prod DB")
	}

	// All files must be recorded in the tracking table.
	if n := countApplied(t, pool, "schema_migrations"); n != 2 {
		t.Fatalf("expected 2 tracking rows after baseline, got %d", n)
	}

	// accounts (original) must still exist intact.
	if !tableExists(t, pool, "accounts") {
		t.Fatal("accounts table should still exist after baseline (no rename ran)")
	}
}

// TestMigrate_Incremental — some files applied, new file added: only new file runs.
func TestMigrate_Incremental(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	// Phase 1: run first migration.
	fsys1 := simpleFS(map[string]string{
		"0001_create_items.sql": `CREATE TABLE items (id SERIAL PRIMARY KEY);`,
	})
	if err := RunMigrations(ctx, pool, fsys1, MigrateOptions{}); err != nil {
		t.Fatalf("phase 1 RunMigrations: %v", err)
	}
	if n := countApplied(t, pool, "schema_migrations"); n != 1 {
		t.Fatalf("expected 1 tracking row after phase 1, got %d", n)
	}

	// Phase 2: add a second migration.
	fsys2 := simpleFS(map[string]string{
		"0001_create_items.sql": `CREATE TABLE items (id SERIAL PRIMARY KEY);`,
		"0002_add_qty.sql":      `ALTER TABLE items ADD COLUMN qty INT NOT NULL DEFAULT 0;`,
	})
	if err := RunMigrations(ctx, pool, fsys2, MigrateOptions{}); err != nil {
		t.Fatalf("phase 2 RunMigrations: %v", err)
	}

	// qty column must exist.
	var hasQty bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'items' AND column_name = 'qty'
		)`).Scan(&hasQty); err != nil {
		t.Fatalf("check qty column: %v", err)
	}
	if !hasQty {
		t.Fatal("expected qty column from 0002_add_qty.sql")
	}
	if n := countApplied(t, pool, "schema_migrations"); n != 2 {
		t.Fatalf("expected 2 tracking rows after phase 2, got %d", n)
	}
}

// TestMigrate_ChecksumDrift — applied file's content changed: Warn emitted, NOT re-run.
func TestMigrate_ChecksumDrift(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	// Phase 1: apply migration.
	fsys1 := simpleFS(map[string]string{
		"0001_create_logs.sql": `CREATE TABLE logs (id SERIAL PRIMARY KEY, msg TEXT);`,
	})
	if err := RunMigrations(ctx, pool, fsys1, MigrateOptions{}); err != nil {
		t.Fatalf("initial apply: %v", err)
	}

	// Phase 2: same filename, different content (checksum drift).
	wc := &warnCapture{}
	fsys2 := simpleFS(map[string]string{
		// Content changed — would fail if re-run (duplicate CREATE TABLE).
		"0001_create_logs.sql": `CREATE TABLE logs (id SERIAL PRIMARY KEY, msg TEXT, ts TIMESTAMPTZ);`,
	})
	if err := RunMigrations(ctx, pool, fsys2, MigrateOptions{Logger: wc.logger()}); err != nil {
		t.Fatalf("drift run: %v", err)
	}

	// Must NOT have added the ts column (migration was not re-run).
	var hasTS bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'logs' AND column_name = 'ts'
		)`).Scan(&hasTS); err != nil {
		t.Fatalf("check ts column: %v", err)
	}
	if hasTS {
		t.Fatal("drift migration must NOT be re-applied")
	}

	// A Warn must have been emitted.
	wc.mu.Lock()
	warns := wc.msgs
	wc.mu.Unlock()
	found := false
	for _, m := range warns {
		if strings.Contains(m, "checksum drift") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a checksum drift Warn log, got: %v", warns)
	}
}

// TestMigrate_ConcurrentBoots — two RunMigrations in parallel goroutines:
// advisory lock serializes, no double-apply, no error.
func TestMigrate_ConcurrentBoots(t *testing.T) {
	dsn := scratchDB(t)
	ctx := context.Background()

	// Two pools simulate two independent boot processes.
	pool1 := openPool(t, dsn)
	pool2 := openPool(t, dsn)

	fsys := simpleFS(map[string]string{
		"0001_create_events.sql": `CREATE TABLE events (id SERIAL PRIMARY KEY, kind TEXT);`,
	})

	var (
		wg   sync.WaitGroup
		errs [2]error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = RunMigrations(ctx, pool1, fsys, MigrateOptions{})
	}()
	go func() {
		defer wg.Done()
		errs[1] = RunMigrations(ctx, pool2, fsys, MigrateOptions{})
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}

	// Exactly one tracking row — no double-apply.
	// Use a fresh pool (pool1/pool2 are closed by t.Cleanup, but still open here).
	verifyPool := openPool(t, dsn)
	if n := countApplied(t, verifyPool, "schema_migrations"); n != 1 {
		t.Fatalf("expected exactly 1 tracking row after concurrent boots, got %d", n)
	}

	if !tableExists(t, verifyPool, "events") {
		t.Fatal("events table should exist")
	}
}

// TestMigrate_PreMigrate — PreMigrate hook runs before migrations and its effect
// is visible inside migrations (e.g. a GUC set by PreMigrate persists on the conn).
func TestMigrate_PreMigrate(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	preMigrateCalled := false

	// PreMigrate creates a schema; the migration then creates a table inside it.
	fsys := simpleFS(map[string]string{
		"0001_create_thing.sql": `CREATE TABLE app.thing (id SERIAL PRIMARY KEY);`,
	})

	opts := MigrateOptions{
		PreMigrate: func(ctx context.Context, conn *pgxpool.Conn) error {
			preMigrateCalled = true
			// Create the schema so the migration's CREATE TABLE succeeds.
			_, err := conn.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS app`)
			return err
		},
	}

	if err := RunMigrations(ctx, pool, fsys, opts); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if !preMigrateCalled {
		t.Fatal("PreMigrate was not called")
	}

	// app.thing must exist.
	var hasThing bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'app' AND table_name = 'thing'
		)`).Scan(&hasThing); err != nil {
		t.Fatalf("check app.thing: %v", err)
	}
	if !hasThing {
		t.Fatal("expected app.thing table created via PreMigrate + migration")
	}
}

// TestMigrate_Idempotent — RunMigrations called twice on an already-migrated DB:
// second call is a no-op, no error.
func TestMigrate_Idempotent(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	fsys := simpleFS(map[string]string{
		"0001_create_widgets.sql": `CREATE TABLE widgets (id SERIAL PRIMARY KEY);`,
	})

	for i := range 2 {
		if err := RunMigrations(ctx, pool, fsys, MigrateOptions{}); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	if n := countApplied(t, pool, "schema_migrations"); n != 1 {
		t.Fatalf("expected 1 tracking row, got %d", n)
	}
}

// TestMigrate_InvalidTableName — applyDefaults rejects SQL-injection vectors in TableName.
// Uses the exported-for-test helper validateTableName to avoid needing a live DB
// for pure identifier validation.
func TestMigrate_InvalidTableName(t *testing.T) {
	cases := []struct {
		tableName string
		wantErr   bool
	}{
		{"schema_migrations", false},
		{"myschema.schema_migrations", false},
		{"memos_graph.schema_migrations", false},
		{"", false}, // defaults to schema_migrations
		{"bad-name", true},
		{"bad name", true},
		{"'; DROP TABLE users; --", true},
		{"schema.table.extra", true},
		{"123invalid", true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q", tc.tableName), func(t *testing.T) {
			opts := MigrateOptions{TableName: tc.tableName}
			err := opts.applyDefaults()
			if tc.wantErr && err == nil {
				t.Fatalf("expected validation error for TableName %q, got nil", tc.tableName)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for TableName %q: %v", tc.tableName, err)
			}
		})
	}
}

// TestMigrate_LockKey — custom LockKey is accepted; two concurrent boots with
// the same custom key still serialize correctly (no double-apply).
func TestMigrate_LockKey(t *testing.T) {
	dsn := scratchDB(t)
	ctx := context.Background()

	pool1 := openPool(t, dsn)
	pool2 := openPool(t, dsn)

	fsys := simpleFS(map[string]string{
		"0001_create_orders.sql": `CREATE TABLE orders (id SERIAL PRIMARY KEY);`,
	})

	const customKey int64 = 42

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = RunMigrations(ctx, pool1, fsys, MigrateOptions{LockKey: customKey})
	}()
	go func() {
		defer wg.Done()
		errs[1] = RunMigrations(ctx, pool2, fsys, MigrateOptions{LockKey: customKey})
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}

	verifyPool := openPool(t, dsn)
	if n := countApplied(t, verifyPool, "schema_migrations"); n != 1 {
		t.Fatalf("expected 1 tracking row with custom lock key, got %d", n)
	}
}

// TestMigrate_ConcurrentBaseline — two boots both on the baseline path (-race):
// exactly N rows in tracking table, zero SQL executed.
func TestMigrate_ConcurrentBaseline(t *testing.T) {
	dsn := scratchDB(t)
	ctx := context.Background()

	// Seed the live schema so Baseline predicate returns true.
	setupPool := openPool(t, dsn)
	if _, err := setupPool.Exec(ctx, `CREATE TABLE live_table (id SERIAL PRIMARY KEY);`); err != nil {
		t.Fatalf("seed live_table: %v", err)
	}

	const numFiles = 3
	files := map[string]string{
		"0001_live.sql": `CREATE TABLE IF NOT EXISTS live_table (id SERIAL PRIMARY KEY);`,
		"0002_live.sql": `ALTER TABLE live_table ADD COLUMN IF NOT EXISTS val TEXT;`,
		"0003_live.sql": `CREATE INDEX IF NOT EXISTS live_val_idx ON live_table(val);`,
	}
	fsys := simpleFS(files)

	// Baseline predicate: live_table exists → prod schema present.
	baseline := func(_ context.Context, conn *pgxpool.Conn) (bool, error) {
		var ok bool
		err := conn.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = 'live_table'
			)`).Scan(&ok)
		return ok, err
	}

	// Two pools simulate two independent boot processes both hitting the baseline path.
	pool1 := openPool(t, dsn)
	pool2 := openPool(t, dsn)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = RunMigrations(ctx, pool1, fsys, MigrateOptions{Baseline: baseline})
	}()
	go func() {
		defer wg.Done()
		errs[1] = RunMigrations(ctx, pool2, fsys, MigrateOptions{Baseline: baseline})
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}

	// Exactly numFiles rows — no duplicates, no missing entries.
	verifyPool := openPool(t, dsn)
	if n := countApplied(t, verifyPool, "schema_migrations"); n != numFiles {
		t.Fatalf("expected %d tracking rows after concurrent baseline, got %d", numFiles, n)
	}

	// live_table.val must NOT exist: the ALTER was never executed.
	var hasVal bool
	if err := verifyPool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'live_table' AND column_name = 'val'
		)`).Scan(&hasVal); err != nil {
		t.Fatalf("check val column: %v", err)
	}
	if hasVal {
		t.Fatal("baseline must NOT execute SQL — val column must not exist")
	}
}

// TestIsSoftMigration — pure unit test for the "-- soft" first-line detector.
// No DB required. Mirrors go-job's hand-rolled runMigrations detection:
// strings.HasPrefix(strings.TrimSpace(string(data)), "-- soft").
func TestIsSoftMigration(t *testing.T) {
	cases := []struct {
		name string
		data string
		want bool
	}{
		{"plain first line", "-- soft\nCREATE EXTENSION age;\n", true},
		{"first line no trailing newline", "-- soft", true},
		{"leading whitespace before marker", "   \t\n  -- soft\nSELECT 1;\n", true},
		{"leading blank lines then marker", "\n\n-- soft\nSELECT 1;\n", true},
		{"marker with trailing content on same line", "-- soft: optional AGE extension\nCREATE EXTENSION age;\n", true},
		{"no space after dashes", "--soft\nSELECT 1;\n", false},
		{"soft not on first line", "SELECT 1;\n-- soft\n", false},
		{"empty file", "", false},
		{"only whitespace", "   \n\t\n", false},
		// "-- softer" matches the literal fleet convention
		// (strings.HasPrefix(..., "-- soft")) — it IS treated as soft.
		// A real file is vanishingly unlikely to start with that exact
		// string, and matching it is the documented go-job behaviour.
		{"longer word starting with soft", "-- softer\nSELECT 1;\n", true},
		{"case sensitive", "-- Soft\nSELECT 1;\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSoftMigration([]byte(tc.data)); got != tc.want {
				t.Fatalf("isSoftMigration(%q) = %v, want %v", tc.data, got, tc.want)
			}
		})
	}
}

// TestMigrate_SoftFailsContinues — a "-- soft" migration whose SQL fails
// (e.g. references a guaranteed-absent extension) must NOT abort the run:
// RunMigrations returns nil, later normal files apply, the soft file is NOT
// recorded (so a later run retries it — self-healing once the extension lands).
func TestMigrate_SoftFailsContinues(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	wc := &warnCapture{}
	fsys := simpleFS(map[string]string{
		// Soft file that will fail: extension name guaranteed absent.
		"0001_soft_missing_ext.sql": "-- soft\nCREATE EXTENSION \"definitely_not_installed_xyz\";\n",
		// Normal file AFTER the failing soft one — must still apply.
		"0002_create_anchor.sql": "CREATE TABLE soft_anchor (id SERIAL PRIMARY KEY);",
	})

	if err := RunMigrations(ctx, pool, fsys, MigrateOptions{Logger: wc.logger()}); err != nil {
		t.Fatalf("soft failure must NOT abort the run: %v", err)
	}

	// The normal file after the soft failure must have applied.
	if !tableExists(t, pool, "soft_anchor") {
		t.Fatal("normal migration after a failed soft migration must still apply")
	}
	if n := countApplied(t, pool, "schema_migrations"); n != 1 {
		t.Fatalf("expected 1 tracking row (only the normal file), got %d", n)
	}

	// The soft file must NOT be recorded — so a later run retries it.
	var hasSoft bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM schema_migrations WHERE name = '0001_soft_missing_ext.sql'
		)`).Scan(&hasSoft); err != nil {
		t.Fatalf("check soft tracking row: %v", err)
	}
	if hasSoft {
		t.Fatal("failed soft migration must NOT be recorded — it must retry on the next run")
	}

	// A WARN must have been emitted naming the file.
	wc.mu.Lock()
	warns := wc.msgs
	wc.mu.Unlock()
	found := false
	for _, m := range warns {
		if strings.Contains(m, "0001_soft_missing_ext.sql") && strings.Contains(m, "soft") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a WARN log for the failed soft migration, got: %v", warns)
	}

	// Retry behaviour: a second RunMigrations re-attempts the soft file
	// (still absent extension → still fails softly, still no row, no abort).
	if err := RunMigrations(ctx, pool, fsys, MigrateOptions{Logger: wc.logger()}); err != nil {
		t.Fatalf("retry run must not abort: %v", err)
	}
	if n := countApplied(t, pool, "schema_migrations"); n != 1 {
		t.Fatalf("expected tracking row count to stay 1 after retry, got %d", n)
	}
}

// TestMigrate_SoftSucceedsRecorded — a "-- soft" migration whose SQL applies
// successfully is recorded in the tracking table exactly like any normal file.
func TestMigrate_SoftSucceedsRecorded(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	fsys := simpleFS(map[string]string{
		"0001_soft_ok.sql": "-- soft\nCREATE TABLE soft_ok_table (id SERIAL PRIMARY KEY);\n",
	})

	if err := RunMigrations(ctx, pool, fsys, MigrateOptions{}); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if !tableExists(t, pool, "soft_ok_table") {
		t.Fatal("soft migration body must execute when it succeeds")
	}
	if n := countApplied(t, pool, "schema_migrations"); n != 1 {
		t.Fatalf("successful soft migration must be recorded; expected 1 row, got %d", n)
	}
}

// TestMigrate_NonSoftFailsAborts — a non-soft migration that fails still
// aborts the run (regression guard for the soft-skip change).
func TestMigrate_NonSoftFailsAborts(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	fsys := simpleFS(map[string]string{
		"0001_ok.sql":    "CREATE TABLE nonsf_anchor (id SERIAL PRIMARY KEY);",
		"0002_bad.sql":   "THIS IS NOT VALID SQL;",
		"0003_after.sql": "CREATE TABLE nonsf_after (id SERIAL PRIMARY KEY);",
	})

	err := RunMigrations(ctx, pool, fsys, MigrateOptions{})
	if err == nil {
		t.Fatal("non-soft failure must abort the run with an error")
	}

	if n := countApplied(t, pool, "schema_migrations"); n != 1 {
		t.Fatalf("expected only 0001_ok.sql recorded (1 row), got %d", n)
	}
	if tableExists(t, pool, "nonsf_after") {
		t.Fatal("migration after a non-soft failure must NOT run")
	}
}

// TestMigrate_AtomicRollback — a migration whose SQL body fails mid-way:
// RunMigrations returns an error AND the tracking table has no row for it
// (no half-applied record, body + tracking row commit/rollback atomically).
func TestMigrate_AtomicRollback(t *testing.T) {
	pool := openPool(t, scratchDB(t))
	ctx := context.Background()

	fsys := simpleFS(map[string]string{
		"0001_ok.sql": `CREATE TABLE rollback_anchor (id SERIAL PRIMARY KEY);`,
		// This migration will fail: adding a NOT NULL column without a default
		// to a table that might have rows (or simply using invalid SQL).
		// We use explicitly invalid SQL to guarantee failure.
		"0002_bad.sql": `THIS IS NOT VALID SQL;`,
	})

	err := RunMigrations(ctx, pool, fsys, MigrateOptions{})
	if err == nil {
		t.Fatal("expected RunMigrations to return error for invalid SQL migration")
	}

	// 0001 should be applied (it ran before the failure).
	if n := countApplied(t, pool, "schema_migrations"); n != 1 {
		t.Fatalf("expected 1 tracking row (only 0001_ok.sql), got %d", n)
	}

	// 0002 must NOT appear in the tracking table — rollback was atomic.
	var hasBad bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM schema_migrations WHERE name = '0002_bad.sql'
		)`).Scan(&hasBad); err != nil {
		t.Fatalf("check 0002_bad.sql tracking: %v", err)
	}
	if hasBad {
		t.Fatal("failed migration must NOT have a tracking row — tx was not atomic")
	}

	// The anchor table from 0001 still exists (first migration succeeded).
	if !tableExists(t, pool, "rollback_anchor") {
		t.Fatal("rollback_anchor table from 0001_ok.sql should still exist")
	}
}
