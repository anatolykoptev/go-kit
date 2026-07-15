package chrome

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/chromedp/chromedp"
)

// defaultMaxUses is the number of successful Print calls before the browser
// context is rotated (reset+lazy re-init) to prevent Chrome memory creep under
// sustained load. Pattern borrowed from ChromicPDF (Elixir).
const defaultMaxUses = 500

// Printer maintains a long-lived Chrome DevTools Protocol connection and renders HTML
// documents to PDF. Tabs are created per-call but the underlying browser context is reused,
// avoiding the target-creation failures that occur when CDP state accumulates under load
// (pattern inspired by go-wowa's internal/report renderer).
//
// Zero-value is unusable; always construct via NewPrinter. Safe for concurrent use:
// state transitions (init/reset) are guarded by mu; the actual chromedp.Run on per-call
// tab contexts is safe under chromedp's own concurrency model.
type Printer struct {
	cdpURL string

	mu          sync.Mutex
	browserCtx  context.Context
	cancelAll   func() // tears down browser + allocator in order
	initialized bool

	maxUses    int // 0 means use defaultMaxUses
	useCount   int // guarded by mu
	rotations  int // guarded by mu; testing/observability counter
	probeCount int // guarded by mu; cumulative probeCDP successes (test hook)

	// inFlight counts goroutines currently executing printOnce or captureOnce.
	// We must not call cancelAll (or rotate browserCtx) while inFlight > 0 because
	// every tab context is a child of browserCtx: canceling the parent propagates
	// "context canceled" into chromedp.Run in all sibling goroutines, and
	// isStaleConnection deliberately excludes "context canceled" to avoid
	// false-positive retries — so those goroutines would surface a spurious error.
	inFlight int // guarded by mu

	// pendingCancel holds cancelAll functions that could not be invoked immediately
	// because inFlight > 0 at the time the rotation or Close was requested.
	// They are called in firePendingLocked once inFlight drops to zero.
	pendingCancel []func() // guarded by mu
}

// firePendingLocked calls and clears pendingCancel when inFlight has reached zero.
// Must be called after every inFlight decrement. Caller must hold p.mu.
func (p *Printer) firePendingLocked() {
	if p.inFlight > 0 || len(p.pendingCancel) == 0 {
		return
	}
	for _, c := range p.pendingCancel {
		c()
	}
	p.pendingCancel = p.pendingCancel[:0]
}

// probeCountForTests returns the number of times probeCDP has run successfully
// since this Printer was constructed (cumulative across rotations and
// stale-retry re-inits). Internal test hook only; not public API.
func (p *Printer) probeCountForTests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.probeCount
}

// WithMaxUses sets the number of successful Print calls before the browser
// context is reset (0 uses the default of 500). Prevents Chrome memory creep
// under sustained load. Must be called before the first Print; returns p for
// chaining.
func (p *Printer) WithMaxUses(n int) *Printer {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maxUses = n
	return p
}

// rotationCount returns the number of times the browser context has been
// reset due to max_uses (NOT stale-connection retries). For tests only.
func (p *Printer) rotationCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rotations
}

// NewPrinter returns a Printer for the given CDP endpoint.
// The browser connection is established lazily on first Print call.
func NewPrinter(cdpURL string) *Printer {
	return &Printer{cdpURL: cdpURL}
}

// Close releases the browser connection and resets to uninitialized state.
// Subsequent Print calls will re-initialize.
//
// If there are renders currently in progress (concurrent Print or CaptureImage
// calls), the browser context teardown is deferred until the last in-flight
// call completes, rather than immediately canceling their shared context.
// The printer is marked uninitialized immediately so new Print calls after
// Close will not attempt to use the old context.
func (p *Printer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inFlight == 0 {
		p.resetLocked()
		return
	}
	// Defer teardown: stash the old cancelAll and mark as uninitialized.
	// firePendingLocked will fire the cancel when inFlight reaches zero.
	if p.cancelAll != nil {
		p.pendingCancel = append(p.pendingCancel, p.cancelAll)
		p.cancelAll = nil
	}
	p.browserCtx = nil
	p.initialized = false
}

// Print renders HTML to PDF bytes using a fresh tab inside the shared browser context.
// Honors PDFOptions.Timeout per-call; the browser context itself has no timeout.
// On "stale connection" errors (e.g. browser restarted underneath us), Print automatically
// resets the context and retries once before surfacing the failure. When the retry itself
// fails, the returned error is wrapped with "print to pdf after reset+retry" so ops logs
// can distinguish a single transient failure from a genuinely dead browser.
//
// The CDPURL field on opts is ignored — the endpoint is determined at NewPrinter time.
func (p *Printer) Print(ctx context.Context, html string, opts PDFOptions) ([]byte, error) {
	p.mu.Lock()
	if !p.initialized {
		if err := p.initLocked(ctx); err != nil {
			p.mu.Unlock()
			return nil, err
		}
	}
	browserCtx := p.browserCtx
	p.inFlight++
	p.mu.Unlock()

	pdfBytes, err := p.printOnce(ctx, browserCtx, html, opts)

	if err == nil {
		// Successful print: bump use count and possibly rotate the browser
		// context to avoid Chrome memory creep. Rotation tears down the
		// current browser/allocator; next Print will lazily re-init.
		//
		// If other goroutines are still using the old browserCtx (inFlight > 0
		// after our decrement), we defer the cancelAll so we don't propagate
		// "context canceled" into their in-flight chromedp.Run calls.
		p.mu.Lock()
		p.inFlight--
		p.firePendingLocked()
		p.useCount++
		limit := p.maxUses
		if limit <= 0 {
			limit = defaultMaxUses
		}
		if p.useCount >= limit {
			oldCancel := p.cancelAll
			p.cancelAll = nil
			p.browserCtx = nil
			p.initialized = false
			p.rotations++
			p.useCount = 0
			if p.inFlight == 0 {
				// No other goroutines are using the old context; safe to cancel now.
				if oldCancel != nil {
					oldCancel()
				}
			} else {
				// Other goroutines are still in printOnce on this browserCtx.
				// Queue the cancel for when they finish.
				if oldCancel != nil {
					p.pendingCancel = append(p.pendingCancel, oldCancel)
				}
			}
		}
		p.mu.Unlock()
		return pdfBytes, nil
	}

	// Error path: decrement before the stale check.
	p.mu.Lock()
	p.inFlight--
	p.firePendingLocked()
	p.mu.Unlock()

	if !isStaleConnection(err) {
		return nil, err
	}

	// Stale connection: the browser is dead — cancel immediately (no point deferring
	// on a dead connection), reset, re-init, retry once. If re-init fails, surface
	// the original error (don't mask it with an init error).
	p.mu.Lock()
	p.resetLocked()
	if initErr := p.initLocked(ctx); initErr != nil {
		p.mu.Unlock()
		return nil, err
	}
	browserCtx = p.browserCtx
	p.inFlight++
	p.mu.Unlock()

	retried, retryErr := p.printOnce(ctx, browserCtx, html, opts)

	p.mu.Lock()
	p.inFlight--
	p.firePendingLocked()
	p.mu.Unlock()

	if retryErr != nil {
		return nil, fmt.Errorf("print to pdf after reset+retry: %w", retryErr)
	}
	return retried, nil
}

// initLocked establishes the remote allocator and browser contexts. Caller must hold p.mu.
// The probe uses the caller's ctx so that caller deadlines propagate; the browser
// contexts themselves use context.Background so they outlive a single Print call.
func (p *Printer) initLocked(ctx context.Context) error {
	if err := probeCDP(ctx, p.cdpURL); err != nil {
		return err
	}
	p.probeCount++
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), p.cdpURL)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	// Force first connect so we fail fast if the browser attach doesn't work,
	// rather than on the first Print.
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocCancel()
		return fmt.Errorf("chromedp attach: %w", err)
	}

	p.browserCtx = browserCtx
	p.cancelAll = func() {
		browserCancel()
		allocCancel()
	}
	p.initialized = true
	return nil
}

// resetLocked tears down the current browser context and marks the printer
// uninitialized. Idempotent. Caller must hold p.mu.
//
// This cancels the browser context immediately. Use only when the browser is
// known to be dead (stale connection) or when inFlight == 0. For graceful
// rotation with concurrent renders, see the rotation logic in Print.
func (p *Printer) resetLocked() {
	if p.cancelAll != nil {
		p.cancelAll()
	}
	p.browserCtx = nil
	p.cancelAll = nil
	p.initialized = false
}

// printOnce runs the chromedp actions on a fresh tab derived from browserCtx,
// bounded by opts.Timeout (or default). The tab is a child of the shared
// browser context (for reuse); a timeout-derived child of the tab is used as
// the run context so both the user's cancellation (via userCtx-rooted deadlines
// upstream) and our per-call timeout apply.
func (p *Printer) printOnce(userCtx, browserCtx context.Context, html string, opts PDFOptions) ([]byte, error) {
	headerTemplate, footerTemplate, timeout := resolvePDFDefaults(opts)

	tabCtx, cancelTab := chromedp.NewContext(browserCtx)
	defer cancelTab()

	// Apply the user's ctx deadline (if any) and the per-call timeout by
	// chaining: tab -> userCtx-influenced timeout. We pick the tighter of
	// userCtx's deadline and our per-call timeout.
	runCtx, cancelRun := context.WithTimeout(tabCtx, timeout)
	defer cancelRun()
	// If userCtx is canceled, propagate that to runCtx.
	stopProp := make(chan struct{})
	defer close(stopProp)
	// Watcher goroutine: exits via either branch. On the happy path, the deferred
	// close(stopProp) drains it; on userCtx cancellation, it cancels runCtx and
	// then returns. Note defer LIFO: stopProp closes before cancelRun, which is
	// harmless because both branches handle cancellation idempotently.
	go func() {
		select {
		case <-userCtx.Done():
			cancelRun()
		case <-stopProp:
		}
	}()

	var buf []byte
	actions := pdfActions(html, headerTemplate, footerTemplate, &buf)
	if err := chromedp.Run(runCtx, actions...); err != nil {
		return nil, fmt.Errorf("print to pdf: %w", err)
	}
	return buf, nil
}

// staleConnectionMarkers is the narrow list of error substrings we treat as
// "browser went away" signals, triggering a single reset+retry in Print.
//
//   - "no browser is open": the raw CDP failure from Target.createTarget when
//     cloakbrowser loses its root target.
//   - "browser has disconnected": chromedp's own message when the websocket to
//     the browser is closed unexpectedly.
//   - "websocket: close": underlying gorilla-websocket close error that often
//     surfaces before chromedp wraps it.
//
// "-32000" is deliberately NOT used as a marker: it's the CDP JSON-RPC generic
// server error code and fires on benign user-content issues (malformed HTML,
// invalid print parameters) as well as protocol failures. The markers above
// already catch real stale-connection cases because Chrome's full error
// message includes both the code AND descriptive text ("no browser is open
// (-32000)", "browser has disconnected: websocket: close ..."), so narrowing
// to the textual markers prevents false-positive retries on user-input errors.
//
// "context canceled" is deliberately excluded: it's ambiguous (user cancels
// trigger the same string) and would cause inappropriate retries.
var staleConnectionMarkers = []string{
	"no browser is open",
	"browser has disconnected",
	"websocket: close",
}

// isStaleConnection reports whether err looks like a dropped CDP connection
// that a fresh browser context would fix.
func isStaleConnection(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, marker := range staleConnectionMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// CaptureImage renders the given HTML to a raster image (PNG/JPEG/WebP) using
// the printer's persistent browser context. Mirrors Print's reuse + stale-retry
// semantics.
//
// The opts.CDPURL field is ignored — the endpoint is determined at NewPrinter time.
func (p *Printer) CaptureImage(ctx context.Context, html string, opts ImageOptions) ([]byte, error) {
	opts = resolveImageDefaults(opts)

	p.mu.Lock()
	if !p.initialized {
		if err := p.initLocked(ctx); err != nil {
			p.mu.Unlock()
			return nil, err
		}
	}
	browserCtx := p.browserCtx
	p.inFlight++
	p.mu.Unlock()

	out, err := p.captureOnce(ctx, browserCtx, html, opts)

	p.mu.Lock()
	p.inFlight--
	p.firePendingLocked()
	p.mu.Unlock()

	if err == nil {
		return out, nil
	}
	if !isStaleConnection(err) {
		return nil, err
	}

	// Stale connection: reset immediately (browser is dead), re-init, retry once.
	p.mu.Lock()
	p.resetLocked()
	if initErr := p.initLocked(ctx); initErr != nil {
		p.mu.Unlock()
		return nil, err
	}
	browserCtx = p.browserCtx
	p.inFlight++
	p.mu.Unlock()

	retried, retryErr := p.captureOnce(ctx, browserCtx, html, opts)

	p.mu.Lock()
	p.inFlight--
	p.firePendingLocked()
	p.mu.Unlock()

	if retryErr != nil {
		return nil, fmt.Errorf("capture image after reset+retry: %w", retryErr)
	}
	return retried, nil
}

// captureOnce runs the chromedp action list for image capture on a fresh tab.
// Mirrors printOnce.
func (p *Printer) captureOnce(userCtx, browserCtx context.Context, html string, opts ImageOptions) ([]byte, error) {
	tabCtx, cancelTab := chromedp.NewContext(browserCtx)
	defer cancelTab()

	runCtx, cancelRun := context.WithTimeout(tabCtx, opts.Timeout)
	defer cancelRun()

	// Propagate userCtx cancellation to runCtx.
	stopProp := make(chan struct{})
	defer close(stopProp)
	go func() {
		select {
		case <-userCtx.Done():
			cancelRun()
		case <-stopProp:
		}
	}()

	var buf []byte
	if err := chromedp.Run(runCtx, imageActions(html, opts, &buf)...); err != nil {
		return nil, fmt.Errorf("capture to image: %w", err)
	}
	return buf, nil
}
