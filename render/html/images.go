package html

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuin/goldmark/ast"

	"github.com/anatolykoptev/go-kit/httputil"
	"github.com/anatolykoptev/go-kit/render"
	"github.com/anatolykoptev/go-kit/tracing/httpmw"
)

// defaultImageMaxBytes is the fallback per-image size cap when
// ImageEmbedOptions.MaxBytes is zero.
const defaultImageMaxBytes int64 = 5 << 20 // 5 MB

// defaultImageTimeout is the fallback per-image fetch timeout when
// ImageEmbedOptions.Timeout is zero.
const defaultImageTimeout = 5 * time.Second

// newHTTPClientFn constructs the SSRF-guarded HTTP client used for image
// fetches. A package-level variable (rather than a direct call to
// newSafeHTTPClient) so tests can swap in an unguarded client factory when
// exercising httptest.NewServer, which binds to 127.0.0.1 and would
// otherwise be refused by the SSRF guard.
//
// Production code MUST NOT mutate this. Tests that override it are expected
// to restore the default via t.Cleanup.
var newHTTPClientFn = newSafeHTTPClient

// newSafeHTTPClient returns an *http.Client whose Transport is SSRF-guarded
// via the shared go-kit/httputil primitive (httputil.NewSSRFGuardedClient) —
// the single, framework-owned block-list also used by every other go-kit
// service that fetches a caller-supplied URL. render/html no longer defines
// its own private-IP predicate or dial wrapper; both now live in httputil,
// covering the wider CGNAT/NAT64/6to4/IPv4-compatible ranges too.
//
// The SSRF-safe Transport is wrapped with httpmw.WrapTransport so outbound
// image fetches emit OTel client spans and propagate W3C traceparent
// headers, joining the distributed trace for the enclosing render call. The
// guard is applied to the raw *http.Transport BEFORE this wrap (via
// NewSSRFGuardedClient), so tracing sits outermost without touching dial
// mechanics.
func newSafeHTTPClient(timeout time.Duration) *http.Client {
	base := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			ResponseHeaderTimeout: timeout,
			TLSHandshakeTimeout:   5 * time.Second,
		},
	}
	guarded := httputil.NewSSRFGuardedClient(base)
	guarded.Transport = httpmw.WrapTransport(guarded.Transport)
	return guarded
}

// embedImages walks the AST and replaces each *ast.Image node's Destination
// with a data: URL produced by fetchAndEncode. Graceful on per-image failures:
// individual fetch/resolve errors are logged and leave the destination
// untouched. Only a canceled context short-circuits the walk with its own
// error.
func embedImages(ctx context.Context, doc ast.Node, source []byte, opts render.ImageEmbedOptions) error {
	if doc == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if err := ctx.Err(); err != nil {
			return ast.WalkStop, err
		}
		img, ok := n.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}
		src := string(img.Destination)
		if src == "" {
			return ast.WalkContinue, nil
		}
		dataURL, err := fetchAndEncode(ctx, src, opts)
		if err != nil {
			slog.Warn("render/html: image embed failed", "src", src, "error", err)
			return ast.WalkContinue, nil
		}
		img.Destination = []byte(dataURL)
		return ast.WalkContinue, nil
	})
	return ctx.Err()
}

// fetchAndEncode resolves the given image reference and returns a
// data:<mime>;base64,... URL. Supports three schemes: existing data: URLs
// (pass-through), file:// or relative paths (resolved under opts.Workspace
// with the same path-safety as convert_document), and http(s):// with an
// SSRF blocklist (httputil.IsBlockedIP, enforced at connect time by the
// client newHTTPClientFn returns) plus AllowedHosts.
func fetchAndEncode(ctx context.Context, ref string, opts render.ImageEmbedOptions) (string, error) {
	if strings.HasPrefix(ref, "data:") {
		return ref, nil
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultImageMaxBytes
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultImageTimeout
	}

	parsed, parseErr := url.Parse(ref)
	isHTTP := parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https")
	isFile := parseErr == nil && parsed.Scheme == "file"
	isLocal := isFile || parseErr != nil || parsed.Scheme == ""

	switch {
	case isHTTP:
		return fetchHTTP(ctx, parsed, opts, maxBytes, timeout)
	case isLocal:
		var path string
		if isFile {
			path = parsed.Path
		} else {
			path = ref
		}
		return readWorkspaceFile(path, opts.Workspace, maxBytes)
	default:
		return "", fmt.Errorf("unsupported image scheme: %q", parsed.Scheme)
	}
}

// readWorkspaceFile reads a local file after validating it sits inside the
// configured workspace (or /tmp/). Returns the encoded data: URL.
func readWorkspaceFile(path, workspace string, maxBytes int64) (string, error) {
	if workspace == "" {
		return "", errors.New("no workspace configured for local image")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	// Resolve symlinks to block symlink-escape attacks: a symlink inside the
	// workspace pointing at /etc/passwd would pass the HasPrefix check below
	// without this. For read paths the file must exist, so EvalSymlinks
	// should succeed — on failure we fall back to the unresolved path so a
	// clear "not found" error surfaces below at Stat.
	if real, errResolve := filepath.EvalSymlinks(resolved); errResolve == nil {
		resolved = real
	}
	base, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	if real, errResolve := filepath.EvalSymlinks(base); errResolve == nil {
		base = real
	}
	inWorkspace := strings.HasPrefix(resolved, base+string(filepath.Separator)) || resolved == base
	inTmp := strings.HasPrefix(resolved, "/tmp/") || strings.HasPrefix(resolved, "/private/tmp/")
	if !inWorkspace && !inTmp {
		return "", errors.New("image path outside workspace")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat image: %w", err)
	}
	if info.Size() > maxBytes {
		return "", fmt.Errorf("image exceeds %d bytes", maxBytes)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("image exceeds %d bytes", maxBytes)
	}
	return encodeDataURL(data, sniffImageMIME(data, "")), nil
}

// fetchHTTP downloads an image over http(s) with SSRF and size/timeout
// guards. The SSRF guard is enforced entirely inside the client
// newHTTPClientFn returns (httputil.NewSSRFGuardedClient's connect-time
// DialContext hook, defeating DNS-rebinding) — no separate resolve-time
// check is duplicated here.
func fetchHTTP(ctx context.Context, u *url.URL, opts render.ImageEmbedOptions, maxBytes int64, timeout time.Duration) (string, error) {
	host := u.Hostname()
	if host == "" {
		return "", errors.New("empty host")
	}

	if len(opts.AllowedHosts) > 0 {
		allowed := false
		for _, h := range opts.AllowedHosts {
			if h == u.Host || h == host {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("host %q not in allow-list", u.Host)
		}
	}

	client := newHTTPClientFn(timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("image exceeds %d bytes", maxBytes)
	}
	mime := sniffImageMIME(body, resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(mime, "image/") {
		return "", fmt.Errorf("not an image: mime=%q", mime)
	}
	return encodeDataURL(body, mime), nil
}

// encodeDataURL produces "data:<mime>;base64,<encoded>".
func encodeDataURL(data []byte, mime string) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// sniffImageMIME picks a MIME type for the payload. Prefers a trusted
// Content-Type header when it starts with "image/"; otherwise falls back to
// a first-4-bytes magic check. Defaults to "image/png" for unrecognized
// payloads to keep embedding best-effort.
func sniffImageMIME(data []byte, contentType string) string {
	if contentType != "" {
		ct := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
		if strings.HasPrefix(ct, "image/") {
			return ct
		}
	}
	if len(data) >= 4 {
		switch {
		case data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
			return "image/png"
		case data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
			return "image/jpeg"
		case data[0] == 'G' && data[1] == 'I' && data[2] == 'F' && data[3] == '8':
			return "image/gif"
		case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
			return "image/webp"
		case data[0] == '<' && (data[1] == '?' || data[1] == 's'):
			return "image/svg+xml"
		}
	}
	return "image/png"
}
