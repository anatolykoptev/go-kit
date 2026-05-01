package pgxotel_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/tracing/pgxotel"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestAttachTracer_NilConfigSafe — defensive: mis-init must not panic.
func TestAttachTracer_NilConfigSafe(t *testing.T) {
	if got := pgxotel.AttachTracer(nil); got != nil {
		t.Errorf("nil cfg should return nil, got %v", got)
	}
}

// TestAttachTracer_SetsTracer — happy path: tracer installed on the
// underlying ConnConfig where pgx looks it up.
func TestAttachTracer_SetsTracer(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := pgxotel.AttachTracer(cfg)
	if out == nil {
		t.Fatal("nil result on valid config")
	}
	if out.ConnConfig.Tracer == nil {
		t.Errorf("tracer not attached to ConnConfig")
	}
}

// TestAttachTracer_Idempotent — second call replaces, doesn't append.
func TestAttachTracer_Idempotent(t *testing.T) {
	cfg, _ := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db?sslmode=disable")
	pgxotel.AttachTracer(cfg)
	first := cfg.ConnConfig.Tracer
	pgxotel.AttachTracer(cfg)
	second := cfg.ConnConfig.Tracer
	if first == second {
		t.Error("second attach should produce a new tracer instance (replace, not stack)")
	}
}
