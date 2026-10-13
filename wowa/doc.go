// Package wowa provides a client for go-wowa — the fleet's browser/automation
// edge service (REST on :8906). It replaces the per-repo bespoke copies with
// one codified contract.
//
// Covered endpoints:
//
//   - POST /api/v1/fetch           — proxied ox-browser stealth HTTP fetch
//   - POST /api/v1/read            — proxied ox-browser content extraction
//   - POST /api/v1/render          — stealth Chrome render (full HTML)
//   - POST /api/v1/chrome/interact — Chrome action chains (evaluate/click/...)
//   - POST /api/v1/extract         — LLM structured extraction from a URL
//
// Construction:
//
//	c, err := wowa.NewClient("http://wowa:8906",
//	    wowa.WithTimeout(60*time.Second))
//	html, err := c.RenderHTML(ctx, "https://example.com", 15, "networkidle")
//
// Auth: the internal shared secret resolves from WithToken or the
// INTERNAL_SERVICE_SECRET env var and is sent as X-Internal-Secret.
// With WithRequireAuth a missing secret fails construction with ErrNoToken;
// without it the client proceeds unauthenticated, matching go-wowa's SOFT
// mode (absent header allowed, present-but-wrong is 401).
//
// Error taxonomy: TimeoutError (deadline hit — caller ctx, client timeout, or
// the timeout_secs+4s bound), RemoteError (go-wowa reported the failure —
// both the 200+error-field convention of render/interact and the
// {"error": ...} body on non-200 from extract/proxied ox endpoints),
// StatusError (non-200 without an error envelope), ErrBodyTruncated
// (response exceeded the 5 MiB read cap). Anything else is a transport
// error wrapped with %w.
//
// Timeouts: a request's TimeoutSecs field becomes the server-side bound AND
// tightens the HTTP deadline to timeout_secs+4s, so go-wowa flushes its
// structured error body before the connection dies. Without it the client's
// timeout (default 90s) or the caller's ctx bounds the call.
//
// SSRF: this package deliberately adds no egress guard — it forwards URLs to
// go-wowa verbatim, and go-wowa guards its own egress. Do not point the
// client at an untrusted wowa endpoint.
//
// Retries: none built in — interact action chains (click, type_text) are
// non-idempotent, so replay policy belongs to the caller. Idempotent calls
// (fetch, read) can be wrapped with go-kit/retry.Do by the caller.
package wowa
