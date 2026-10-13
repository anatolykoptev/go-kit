package wowa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anatolykoptev/go-kit/breaker"
)

// defaultTimeout bounds a call when neither the caller's ctx nor a wire
// timeout field sets an earlier deadline. Sized to clear go-wowa's own
// server-side defaults (render 20s, interact 30s, extract's internal LLM
// budget) plus travel — it is a ceiling, not a latency target.
const defaultTimeout = 90 * time.Second

// timeoutHeadroom is added on top of a request's wire timeout field so the
// server hits its own deadline first and flushes a structured error body —
// the client then reads a parseable RemoteError instead of a connection-level
// abort. Codifies the go-search invariant: HTTP deadline = timeout_secs + 4s.
const timeoutHeadroom = 4 * time.Second

// maxBodyBytes caps a response body read. Rendered pages regularly exceed
// 2 MiB; 5 MiB leaves headroom while bounding a runaway response.
const maxBodyBytes = 5 << 20

// doer is the minimal HTTP client interface (*http.Client, breaker.HTTPDoer).
type doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is a go-wowa REST client. Construct via NewClient.
type Client struct {
	url         string
	secret      string
	timeout     time.Duration
	http        *http.Client
	breaker     *breaker.Breaker
	requireAuth bool
}

// Opt configures a Client.
type Opt func(*Client)

// WithToken sets the internal shared secret sent as X-Internal-Secret.
// Empty means "resolve from INTERNAL_SERVICE_SECRET".
func WithToken(token string) Opt {
	return func(c *Client) { c.secret = token }
}

// WithTimeout sets the per-call cap applied via http.Client.Timeout
// (default 90s). A stricter caller ctx still wins; a request's TimeoutSecs
// field overrides it with timeout_secs+4s headroom for that call.
func WithTimeout(d time.Duration) Opt {
	return func(c *Client) { c.timeout = d }
}

// WithHTTPClient replaces the HTTP client entirely (custom transports,
// instrumentation). WithTimeout is ignored when this is set — the provided
// client's own timeout applies to calls that carry no TimeoutSecs.
func WithHTTPClient(h *http.Client) Opt {
	return func(c *Client) { c.http = h }
}

// WithBreaker runs every call through b (breaker.NewHTTPDoer semantics:
// transport errors and 5xx count as failures; ErrOpen short-circuits the
// call). Note the wrapper discards 5xx response bodies — under a breaker a
// 502 surfaces as a generic error, not the remote's error message.
func WithBreaker(b *breaker.Breaker) Opt {
	return func(c *Client) { c.breaker = b }
}

// WithRequireAuth fails NewClient with ErrNoToken when no secret resolves.
// Without it a missing secret logs once and proceeds — matching go-wowa's
// SOFT auth mode (absent header allowed, present-but-wrong is 401).
func WithRequireAuth() Opt {
	return func(c *Client) { c.requireAuth = true }
}

// NewClient builds a go-wowa client for baseURL (e.g. "http://wowa:8906").
func NewClient(baseURL string, opts ...Opt) (*Client, error) {
	c := &Client{
		url:     strings.TrimRight(baseURL, "/"),
		timeout: defaultTimeout,
	}
	for _, o := range opts {
		o(c)
	}
	if c.secret == "" {
		c.secret = os.Getenv("INTERNAL_SERVICE_SECRET")
	}
	if strings.TrimSpace(c.secret) == "" {
		if c.requireAuth {
			return nil, ErrNoToken
		}
		slog.Info("wowa: no X-Internal-Secret configured — assuming SOFT-auth deployment")
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: c.timeout}
	}
	return c, nil
}

// Fetch posts to /api/v1/fetch (ox-browser stealth HTTP fetch). The
// response's Status is the upstream page status — a 404 is data, not an
// error. Only transport failure, deadlines, and the remote Error field
// produce an error.
func (c *Client) Fetch(ctx context.Context, req FetchRequest) (*FetchResponse, error) {
	var r FetchResponse
	if err := c.do(ctx, "fetch", "/api/v1/fetch", req, &r, req.TimeoutSecs); err != nil {
		return nil, err
	}
	if err := remoteFieldError("fetch", r.Error); err != nil {
		return nil, err
	}
	wowaRequestsTotal.WithLabelValues("fetch", "ok").Inc()
	return &r, nil
}

// Read posts to /api/v1/read (ox-browser readability extraction).
func (c *Client) Read(ctx context.Context, req ReadRequest) (*ReadResponse, error) {
	var r ReadResponse
	if err := c.do(ctx, "read", "/api/v1/read", req, &r, req.TimeoutSecs); err != nil {
		return nil, err
	}
	if err := remoteFieldError("read", r.Error); err != nil {
		return nil, err
	}
	wowaRequestsTotal.WithLabelValues("read", "ok").Inc()
	return &r, nil
}

// Render posts to /api/v1/render (stealth Chrome). go-wowa answers HTTP 200
// even on render failure, so both the Error field and an empty HTML payload
// are treated as failures — the go-search convention.
func (c *Client) Render(ctx context.Context, req RenderRequest) (*RenderResponse, error) {
	var r RenderResponse
	if err := c.do(ctx, "render", "/api/v1/render", req, &r, req.TimeoutSecs); err != nil {
		return nil, err
	}
	msg := r.Error
	if msg == "" && r.HTML == "" {
		msg = "empty html"
	}
	if err := remoteFieldError("render", msg); err != nil {
		return nil, err
	}
	wowaRequestsTotal.WithLabelValues("render", "ok").Inc()
	return &r, nil
}

// RenderHTML is the common-case convenience: render url and return the HTML.
// wait selects the page-ready strategy ("load", "domcontentloaded",
// "networkidle"); pass "" for the server default.
func (c *Client) RenderHTML(ctx context.Context, url string, timeoutSecs int, wait string) (string, error) {
	r, err := c.Render(ctx, RenderRequest{URL: url, TimeoutSecs: timeoutSecs, Wait: wait})
	if err != nil {
		return "", err
	}
	return r.HTML, nil
}

// Interact posts to /api/v1/chrome/interact — a chain of Chrome actions on
// a named session's tab. Status != "ok" surfaces as RemoteError; per-action
// results stay in Actions for the caller to inspect (a failed action with
// skip_on_error set does not flip the overall status, by design).
func (c *Client) Interact(ctx context.Context, req InteractRequest) (*InteractResponse, error) {
	var r InteractResponse
	if err := c.do(ctx, "interact", "/api/v1/chrome/interact", req, &r, req.TimeoutSecs); err != nil {
		return nil, err
	}
	if r.Status != "ok" {
		msg := r.Error
		if msg == "" {
			for _, a := range r.Actions {
				if !a.Ok {
					msg = fmt.Sprintf("action %q failed: %s", a.Action, a.Error)
					break
				}
			}
		}
		if msg == "" {
			msg = "status " + r.Status
		}
		if err := remoteFieldError("interact", msg); err != nil {
			return nil, err
		}
	}
	wowaRequestsTotal.WithLabelValues("interact", "ok").Inc()
	return &r, nil
}

// Extract posts to /api/v1/extract — LLM structured extraction over the
// page at req.URL. The endpoint answers non-200 with an {"error": ...} body
// on invalid input or pipeline failure; do() surfaces it as RemoteError.
func (c *Client) Extract(ctx context.Context, req ExtractRequest) (*ExtractResponse, error) {
	var r ExtractResponse
	if err := c.do(ctx, "extract", "/api/v1/extract", req, &r, 0); err != nil {
		return nil, err
	}
	wowaRequestsTotal.WithLabelValues("extract", "ok").Inc()
	return &r, nil
}

// Ping issues one HEAD against the base URL — the startup reachability
// probe convention. Any HTTP response (even 404/405) counts as reachable;
// only a transport failure or timeout is an error. Callers decide whether
// to hard-fail.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.url+"/", nil)
	if err != nil {
		return fmt.Errorf("wowa: ping: create request: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("X-Internal-Secret", c.secret)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wowa: ping: %w", err)
	}
	_ = resp.Body.Close()
	return nil
}

// do posts body to path and decodes the response envelope into out.
// timeoutSecs>0 tightens this call's HTTP deadline to timeoutSecs+headroom,
// matching the wire timeout field the request carries (see timeoutHeadroom).
// Metric increments happen here for every outcome except "ok"/"remote_error"
// on a decoded 200 — those belong to the caller, which sees the payload.
func (c *Client) do(ctx context.Context, endpoint, path string, body, out any, timeoutSecs int) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("wowa: %s: marshal request: %w", endpoint, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("wowa: %s: create request: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Internal-Secret", c.secret)
	}

	hc := c.http
	if timeoutSecs > 0 {
		cc := *hc
		cc.Timeout = time.Duration(timeoutSecs)*time.Second + timeoutHeadroom
		hc = &cc
	}
	var d doer = hc
	if c.breaker != nil {
		d = breaker.NewHTTPDoer(hc, c.breaker)
	}

	resp, err := d.Do(req)
	if err != nil {
		if isTimeout(err) {
			wowaRequestsTotal.WithLabelValues(endpoint, "timeout").Inc()
			return &TimeoutError{Endpoint: endpoint, Err: err}
		}
		wowaRequestsTotal.WithLabelValues(endpoint, "transport_error").Inc()
		return fmt.Errorf("wowa: %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		wowaRequestsTotal.WithLabelValues(endpoint, "transport_error").Inc()
		return fmt.Errorf("wowa: %s: read body: %w", endpoint, err)
	}
	if len(rawBody) > maxBodyBytes {
		wowaRequestsTotal.WithLabelValues(endpoint, "truncated").Inc()
		return fmt.Errorf("wowa: %s: %w", endpoint, ErrBodyTruncated)
	}

	if resp.StatusCode != http.StatusOK {
		// go-wowa error bodies are {"error": "..."} — surface the remote
		// message when present (extract 400/500, proxied ox 502/504, the
		// 401/429/503 guards on the chrome routes).
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(rawBody, &e) == nil && e.Error != "" {
			wowaRequestsTotal.WithLabelValues(endpoint, "remote_error").Inc()
			return &RemoteError{Endpoint: endpoint, StatusCode: resp.StatusCode, Message: e.Error}
		}
		wowaRequestsTotal.WithLabelValues(endpoint, "http_error").Inc()
		return &StatusError{Endpoint: endpoint, StatusCode: resp.StatusCode, Body: truncate(string(rawBody), 200)}
	}

	if err := json.Unmarshal(rawBody, out); err != nil {
		wowaRequestsTotal.WithLabelValues(endpoint, "decode_error").Inc()
		return fmt.Errorf("wowa: %s: decode response: %w", endpoint, err)
	}
	return nil
}

// remoteFieldError converts a response envelope's error string into a
// RemoteError (HTTP 200 + error field convention). nil msg → nil error.
func remoteFieldError(endpoint, msg string) error {
	if msg == "" {
		return nil
	}
	wowaRequestsTotal.WithLabelValues(endpoint, "remote_error").Inc()
	return &RemoteError{Endpoint: endpoint, StatusCode: http.StatusOK, Message: msg}
}

// isTimeout reports whether err is a deadline failure — caller ctx, the
// http.Client timeout, or the per-call timeout_secs bound. net.Error covers
// *url.Error (Client.Timeout) and os.ErrDeadlineExceeded; context deadline
// is checked explicitly for the ctx-done path.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
