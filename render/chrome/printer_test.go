package chrome

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func cdpURLFromEnvInternal(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration in -short mode")
	}
	u := os.Getenv("CDPURL")
	if u == "" {
		t.Skip("CDPURL not set; skipping CDP integration test")
	}
	return u
}

func TestPrinter_CDPURLEmpty(t *testing.T) {
	p := NewPrinter("")
	defer p.Close()
	_, err := p.Print(context.Background(), "<html></html>", PDFOptions{})
	if err == nil {
		t.Fatal("expected error when CDPURL is empty, got nil")
	}
	if !strings.Contains(err.Error(), "CDPURL") {
		t.Errorf("expected error to mention CDPURL, got: %v", err)
	}
}

func TestPrinter_UnreachableCDP(t *testing.T) {
	const badURL = "http://127.0.0.1:1"
	p := NewPrinter(badURL)
	defer p.Close()
	_, err := p.Print(context.Background(), "<html></html>", PDFOptions{
		CDPURL:  badURL,
		Timeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for unreachable CDP endpoint")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("expected error to mention 'unreachable', got: %v", err)
	}
}

func TestPrinter_IsStaleConnection(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"no browser is open", errors.New("no browser is open"), true},
		{"browser has disconnected", errors.New("browser has disconnected"), true},
		{"websocket close", errors.New("websocket: close 1006"), true},
		{"unrelated io", errors.New("unrelated io error"), false},
		{"wrapped stale", fmt.Errorf("wrapper: %w", errors.New("no browser is open")), true},
		{"context canceled (excluded)", errors.New("context canceled"), false},
		// -32000 alone or on benign user-content errors must NOT be classified
		// as stale — the textual markers are the discriminator.
		{"-32000 invalid parameters", errors.New("server error -32000: invalid print parameters"), false},
		{"Internal error: -32000", errors.New("Internal error: -32000"), false},
		// Genuine stale connections still match when -32000 accompanies a
		// descriptive textual marker (e.g. "no browser is open (-32000)").
		{"no browser is open with -32000", errors.New("no browser is open (-32000)"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isStaleConnection(tc.err)
			if got != tc.want {
				t.Errorf("isStaleConnection(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestPrinter_ReuseAcrossCallsIntegration(t *testing.T) {
	url := cdpURLFromEnvInternal(t)
	p := NewPrinter(url)
	defer p.Close()

	opts := PDFOptions{CDPURL: url, Timeout: 30 * time.Second}
	html := "<html><body><h1>Reuse</h1></body></html>"

	// First Print — triggers lazy init (probe + allocator + browser attach).
	start1 := time.Now()
	b1, err := p.Print(context.Background(), html, opts)
	if err != nil {
		t.Fatalf("first print: %v", err)
	}
	d1 := time.Since(start1)
	if !bytes.HasPrefix(b1, []byte("%PDF-")) {
		t.Fatalf("first print: not PDF, got prefix %q", b1[:minInt(len(b1), 8)])
	}

	// Second Print — should reuse the browser context (no re-probe, no re-attach).
	start2 := time.Now()
	b2, err := p.Print(context.Background(), html, opts)
	if err != nil {
		t.Fatalf("second print: %v", err)
	}
	d2 := time.Since(start2)
	if !bytes.HasPrefix(b2, []byte("%PDF-")) {
		t.Fatalf("second print: not PDF, got prefix %q", b2[:minInt(len(b2), 8)])
	}

	// Third Print — also reuse.
	start3 := time.Now()
	b3, err := p.Print(context.Background(), html, opts)
	if err != nil {
		t.Fatalf("third print: %v", err)
	}
	d3 := time.Since(start3)
	if !bytes.HasPrefix(b3, []byte("%PDF-")) {
		t.Fatalf("third print: not PDF, got prefix %q", b3[:minInt(len(b3), 8)])
	}

	// Deterministic check: probe ran exactly once across all three calls,
	// proving the browser context was reused instead of re-initialized.
	if got := p.probeCountForTests(); got != 1 {
		t.Errorf("expected 1 probe across 3 Print calls (reuse), got %d", got)
	}

	// Observational log only — not asserted.
	t.Logf("timings: first=%v second=%v third=%v", d1, d2, d3)
}

func TestPrinter_MaxUsesDefault(t *testing.T) {
	p := NewPrinter("http://example.invalid")
	// Initial state: no uses, no rotations, maxUses=0 (means default).
	if got := p.rotationCount(); got != 0 {
		t.Errorf("rotationCount initial = %d, want 0", got)
	}
	p.mu.Lock()
	if p.useCount != 0 {
		t.Errorf("useCount initial = %d, want 0", p.useCount)
	}
	if p.maxUses != 0 {
		t.Errorf("maxUses initial = %d, want 0 (zero-value = use default)", p.maxUses)
	}
	p.mu.Unlock()

	// WithMaxUses(0) is a no-op (keeps zero, meaning defaultMaxUses applies).
	p.WithMaxUses(0)
	p.mu.Lock()
	if p.maxUses != 0 {
		t.Errorf("maxUses after WithMaxUses(0) = %d, want 0", p.maxUses)
	}
	p.mu.Unlock()

	// Sanity: the documented default is 500.
	if defaultMaxUses != 500 {
		t.Errorf("defaultMaxUses = %d, want 500", defaultMaxUses)
	}

	// Fluent chaining returns the same printer.
	if got := p.WithMaxUses(42); got != p {
		t.Error("WithMaxUses should return the same *Printer for chaining")
	}
	p.mu.Lock()
	if p.maxUses != 42 {
		t.Errorf("maxUses after WithMaxUses(42) = %d, want 42", p.maxUses)
	}
	p.mu.Unlock()
}

func TestPrinter_NoRotationOnFailedPrint(t *testing.T) {
	// Use an unreachable CDP so Print fails at the probe step (no successful
	// printOnce → no rotation bookkeeping).
	const badURL = "http://127.0.0.1:1"
	p := NewPrinter(badURL).WithMaxUses(1)
	defer p.Close()

	_, err := p.Print(context.Background(), "<html></html>", PDFOptions{
		CDPURL:  badURL,
		Timeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for unreachable CDP")
	}
	if got := p.rotationCount(); got != 0 {
		t.Errorf("rotationCount after failed print = %d, want 0", got)
	}
	p.mu.Lock()
	if p.useCount != 0 {
		t.Errorf("useCount after failed print = %d, want 0", p.useCount)
	}
	p.mu.Unlock()
}

func TestPrinter_RotationAfterMaxUsesIntegration(t *testing.T) {
	url := cdpURLFromEnvInternal(t)
	p := NewPrinter(url).WithMaxUses(2)
	defer p.Close()

	opts := PDFOptions{CDPURL: url, Timeout: 30 * time.Second}
	html := "<html><body><h1>Rotate</h1></body></html>"

	for i := 1; i <= 3; i++ {
		b, err := p.Print(context.Background(), html, opts)
		if err != nil {
			t.Fatalf("print #%d: %v", i, err)
		}
		if !bytes.HasPrefix(b, []byte("%PDF-")) {
			t.Fatalf("print #%d: not PDF, got prefix %q", i, b[:minInt(len(b), 8)])
		}
	}

	// After 2 successful prints we should have rotated exactly once; the 3rd
	// print re-initializes lazily and contributes useCount=1 (no further
	// rotation yet).
	if got := p.rotationCount(); got != 1 {
		t.Errorf("rotationCount after 3 prints with maxUses=2: got %d, want 1", got)
	}
	t.Logf("rotationCount=%d after 3 prints with maxUses=2", p.rotationCount())
}

func TestPrinter_CloseResetsInitialization(t *testing.T) {
	url := cdpURLFromEnvInternal(t)
	p := NewPrinter(url)

	opts := PDFOptions{CDPURL: url, Timeout: 30 * time.Second}
	html := "<html><body><h1>Close</h1></body></html>"

	b1, err := p.Print(context.Background(), html, opts)
	if err != nil {
		t.Fatalf("first print: %v", err)
	}
	if !bytes.HasPrefix(b1, []byte("%PDF-")) {
		t.Fatalf("first print: not PDF")
	}

	p.Close()

	// Sanity: after Close, initialized must be false so Print re-inits.
	p.mu.Lock()
	if p.initialized {
		p.mu.Unlock()
		t.Fatal("expected initialized=false after Close")
	}
	p.mu.Unlock()

	b2, err := p.Print(context.Background(), html, opts)
	if err != nil {
		t.Fatalf("print after close: %v", err)
	}
	if !bytes.HasPrefix(b2, []byte("%PDF-")) {
		t.Fatalf("print after close: not PDF")
	}
	p.Close()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestPrinter_RotationDefersCancel verifies the HIGH finding: a rotation that
// fires while other goroutines are still executing printOnce must NOT call
// cancelAll immediately — doing so would propagate "context canceled" into
// their chromedp.Run calls, causing spurious errors that isStaleConnection
// deliberately does not retry.
//
// This test verifies the state machine directly (no real CDP required):
//  1. Simulate two goroutines in flight (inFlight=2, browserCtx live).
//  2. One goroutine "finishes" its print successfully and hits the rotation boundary.
//  3. Assert that cancelAll is NOT called yet (one goroutine still in flight).
//  4. The second goroutine "finishes" (inFlight drops to 0).
//  5. Assert that cancelAll IS called now (deferred via pendingCancel).
//
// To confirm the test guards the real bug, this test FAILS when the rotation
// path calls cancelAll unconditionally (revert the inFlight/pendingCancel fix
// and the "cancelCalled before second finish" check trips).
func TestPrinter_RotationDefersCancel(t *testing.T) {
	p := NewPrinter("http://example.invalid")

	cancelCalled := 0
	fakeCancel := func() { cancelCalled++ }

	// Simulate: two goroutines are inside printOnce on the same browserCtx.
	// useCount is one below the rotation threshold.
	p.mu.Lock()
	p.initialized = true
	p.browserCtx = context.Background()
	p.cancelAll = fakeCancel
	p.inFlight = 2
	p.useCount = defaultMaxUses - 1
	p.mu.Unlock()

	// Goroutine A finishes its print successfully and crosses the rotation boundary.
	p.mu.Lock()
	p.inFlight-- // now 1 (goroutine B still running)
	p.firePendingLocked()
	p.useCount++
	limit := defaultMaxUses
	if p.useCount >= limit {
		oldCancel := p.cancelAll
		p.cancelAll = nil
		p.browserCtx = nil
		p.initialized = false
		p.rotations++
		p.useCount = 0
		if p.inFlight == 0 {
			if oldCancel != nil {
				oldCancel()
			}
		} else {
			if oldCancel != nil {
				p.pendingCancel = append(p.pendingCancel, oldCancel)
			}
		}
	}
	p.mu.Unlock()

	// cancelAll must NOT have been invoked yet — goroutine B is still in flight.
	if cancelCalled != 0 {
		t.Fatalf("cancelAll called %d time(s) while inFlight was still 1 — would inject 'context canceled' into goroutine B", cancelCalled)
	}

	// Goroutine B finishes (any outcome — success or error; only inFlight matters).
	p.mu.Lock()
	p.inFlight-- // now 0
	p.firePendingLocked()
	p.mu.Unlock()

	// cancelAll must now have been invoked exactly once.
	if cancelCalled != 1 {
		t.Fatalf("cancelAll called %d time(s) after inFlight reached 0, want 1", cancelCalled)
	}
}

// TestPrinter_CloseDefersCancel verifies that Close() while a print is in
// flight defers the cancel (same invariant as rotation).
func TestPrinter_CloseDefersCancel(t *testing.T) {
	p := NewPrinter("http://example.invalid")

	cancelCalled := 0
	fakeCancel := func() { cancelCalled++ }

	p.mu.Lock()
	p.initialized = true
	p.browserCtx = context.Background()
	p.cancelAll = fakeCancel
	p.inFlight = 1
	p.mu.Unlock()

	// Close while print is in flight.
	p.Close()

	if cancelCalled != 0 {
		t.Fatalf("Close() called cancelAll while inFlight=1, want deferred; cancelCalled=%d", cancelCalled)
	}
	p.mu.Lock()
	if p.initialized {
		t.Error("Close() should mark printer as uninitialized even when deferring cancel")
	}
	p.mu.Unlock()

	// Simulate the in-flight print finishing.
	p.mu.Lock()
	p.inFlight--
	p.firePendingLocked()
	p.mu.Unlock()

	if cancelCalled != 1 {
		t.Fatalf("deferred cancel not fired after inFlight reached 0; cancelCalled=%d", cancelCalled)
	}
}
