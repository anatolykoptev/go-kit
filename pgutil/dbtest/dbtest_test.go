package dbtest

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const testDSN = "postgres://u:pw@db.example:5432/app_test?sslmode=disable"

func fakeCatalog(names ...string) lister {
	return func(context.Context, *pgx.ConnConfig) ([]string, error) { return names, nil }
}

func neverListed(t *testing.T) lister {
	return func(context.Context, *pgx.ConnConfig) ([]string, error) {
		t.Helper()
		t.Fatal("catalog must not be read when the DSN already fails the name check")
		return nil, nil
	}
}

func TestCheck_RefusesServerWithExtraDatabase(t *testing.T) {
	err := check(context.Background(), testDSN, fakeCatalog("postgres", "prod_app", "other", "app_test"))
	if err == nil {
		t.Fatal("server holding non-test databases was accepted")
	}
	for _, want := range []string{"prod_app", "other", "not a scratch server"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "pw") {
		t.Errorf("error leaks the DSN password: %q", err)
	}
}

func TestCheck_AcceptsEphemeralServer(t *testing.T) {
	for name, dbs := range map[string][]string{
		"only maintenance":  {"postgres"},
		"test databases":    {"postgres", "app_test", "scratch_123_test"},
		"no postgres in db": {"app_test"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(context.Background(), testDSN, fakeCatalog(dbs...)); err != nil {
				t.Fatalf("ephemeral server refused: %v", err)
			}
		})
	}
}

func TestCheck_RefusesNonTestTargetWithoutListing(t *testing.T) {
	for _, dsn := range []string{
		"postgres://u:pw@db.example:5432/app?sslmode=disable",
		"postgres://u:pw@db.example:5432/postgres?sslmode=disable",
		"postgres://u:pw@db.example:5432/_test?sslmode=disable",
		"host=db.example user=u dbname=app",
	} {
		if err := check(context.Background(), dsn, neverListed(t)); err == nil {
			t.Errorf("DSN %q accepted", dsn)
		}
	}
}

func TestCheck_FailsClosedWhenServerCannotBeInspected(t *testing.T) {
	list := func(context.Context, *pgx.ConnConfig) ([]string, error) { return nil, errors.New("connection refused") }
	err := check(context.Background(), testDSN, list)
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("want fail-closed error wrapping the cause, got %v", err)
	}
}

func TestCheck_RejectsUnparsableDSN(t *testing.T) {
	err := check(context.Background(), "postgres://u:hunter2secret@h:notaport/x_test", neverListed(t))
	if err == nil {
		t.Fatal("unparsable DSN accepted")
	}
	if strings.Contains(err.Error(), "hunter2secret") {
		t.Errorf("parse error leaks the password: %q", err)
	}
}

func TestCheck_RejectsEmptyDSNEvenWithEnvDatabase(t *testing.T) {
	// pgx falls back to PG* env vars for an empty DSN; that must not pass.
	t.Setenv("PGDATABASE", "app_test")
	if err := check(context.Background(), "", fakeCatalog("postgres")); err == nil {
		t.Fatal("empty DSN accepted")
	}
}

func TestListSQL_IsSchemaQualified(t *testing.T) {
	if !strings.Contains(listSQL, "pg_catalog.pg_database") {
		t.Fatalf("catalog query must be schema-qualified against search_path shadowing: %s", listSQL)
	}
}

func TestCheck_AdmitsNoRoleEscape(t *testing.T) {
	// A superuser and a plain role must be judged identically: the policy
	// never looks at the role, only at the catalog.
	for _, user := range []string{"postgres", "app_ro", "app_rw"} {
		dsn := "postgres://" + user + ":pw@db.example:5432/app_test"
		if err := check(context.Background(), dsn, fakeCatalog("postgres", "real")); err == nil {
			t.Errorf("role %q opened the escape hatch", user)
		}
	}
}

// fakeTB records which terminal call the guard made. Fatalf panics like the
// real one stops the goroutine, so code after it must not run.
type fakeTB struct{ skipped, fatal string }

type stopped struct{}

func (f *fakeTB) Helper() {}
func (f *fakeTB) Skipf(format string, _ ...any) {
	f.skipped = format
}
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatal = format
	panic(stopped{})
}

func run(dsn string, list lister) (f *fakeTB) {
	f = &fakeTB{}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(stopped); !ok {
				panic(r)
			}
		}
	}()
	mustBeScratch(f, dsn, list)
	return f
}

func TestMustBeScratch_SkipsOnEmptyDSNFatalsOnRefusal(t *testing.T) {
	f := run("", neverListed(t))
	if f.skipped == "" || f.fatal != "" {
		t.Errorf("empty DSN: want skip only, got skip=%q fatal=%q", f.skipped, f.fatal)
	}
	f = run(testDSN, fakeCatalog("postgres", "real"))
	if f.fatal == "" || f.skipped != "" {
		t.Errorf("refusal: want fatal, never skip, got skip=%q fatal=%q", f.skipped, f.fatal)
	}
	f = run(testDSN, fakeCatalog("postgres", "x_test"))
	if f.fatal != "" || f.skipped != "" {
		t.Errorf("scratch server: want neither, got skip=%q fatal=%q", f.skipped, f.fatal)
	}
}

// TestListDatabases_Live proves the real catalog query works. It is read-only
// and runs only when TEST_DATABASE_URL is set; it does not call MustBeScratch
// because go-kit's own CI database is not named *_test.
func TestListDatabases_Live(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	dbs, err := listDatabases(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range dbs {
		if d == "postgres" {
			found = true
		}
		if strings.HasPrefix(d, "template") {
			t.Errorf("template database %q listed", d)
		}
	}
	if !found {
		t.Errorf("maintenance database missing from %v", dbs)
	}
}
