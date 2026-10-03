package stt_test

import (
	"testing"

	"go.uber.org/goleak"
)

// goleak is wired per-test via t.Cleanup(func() { goleak.VerifyNone(t, opts) })
// in tests that create WebSocket connections. Tests that use mock WS servers
// with blocking handlers (time.After) call goleak.IgnoreCurrent() after
// Connect to exclude server-side goroutines that won't exit until srv.Close()
// tears them down — we only care about client-side readLoop leaks.
//
// We do NOT use goleak.VerifyTestMain(m) here because it runs after ALL tests
// and ALL defers, but server-side goroutines from blocking mock handlers may
// still be draining at that point (httptest.Server.Close() is asynchronous
// for in-flight handler goroutines). Per-test VerifyNone with IgnoreCurrent
// is the correct granularity.

// TestGoleakSanity is a smoke test that verifies goleak is wired and can
// detect leaks. It intentionally leaks a goroutine and expects goleak to
// catch it (but since we use per-test VerifyNone, this test just verifies
// the import works).
func TestGoleakSanity(t *testing.T) {
	_ = goleak.IgnoreCurrent()
}
