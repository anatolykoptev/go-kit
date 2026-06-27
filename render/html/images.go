package html

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuin/goldmark/ast"

	"github.com/anatolykoptev/go-kit/render"
)

// defaultImageMaxBytes is the fallback per-image size cap when
// ImageEmbedOptions.MaxBytes is zero.
const defaultImageMaxBytes int64 = 5 << 20 // 5 MB

// defaultImageTimeout is the fallback per-image fetch timeout when
// ImageEmbedOptions.Timeout is zero.
const defaultImageTimeout = 5 * time.Second

// privateIPCheck is the SSRF blocklist predicate used by fetchAndEncode. It is
// a package-level variable (rather than a direct call to isPrivateIP) so tests
// can swap it out when using httptest.NewServer, which binds to 127.0.0.1 and
// would otherwise be rejected by the guard.
//
// Production code MUST NOT mutate this. Tests that override it are expected to
// restore the default via defer.
var privateIPCheck = isPrivateIP

// newSafeHTTPClient returns an *http.Client whose Transport re-resolves the
// host at dial time and refuses any IP that the privateIPCheck predicate
// rejects. This is defense-in-depth against DNS rebinding: fetchHTTP also
// checks IPs at resolve-time (fast-fail), but an attacker-controlled DNS
// server could return a public IP during resolve and a private IP at dial.
// The dial-time check closes that gap by pinning the dial to an IP we have
// just verified.
func newSafeHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           safeDial,
			ResponseHeaderTimeout: timeout,
			TLSHandshakeTimeout:   5 * time.Second,
		},
	}
}

// safeDial resolves host -> IP, runs every returned IP through privateIPCheck,
// and then dials the first IP directly (pinning). Any private/loopback IP in
// the result aborts the dial with an error — we refuse to connect even if
// one of several returned IPs is public, to keep behavior strict.
func safeDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("split host/port: %w", err)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}
	if len(addrs) == 0 {
		return nil, errors.New("no IP addresses for host")
	}
	for _, a := range addrs {
		if privateIPCheck(a.IP) {
			return nil, fmt.Errorf("dial to private IP refused: %s", a.IP)
		}
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addrs[0].IP.String(), port))
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
// SSRF blocklist based on privateIPCheck plus AllowedHosts.
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

// fetchHTTP downloads an image over http(s) with SSRF and size/timeout guards.
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

	resolveCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		return "", fmt.Errorf("resolve host: %w", err)
	}
	if len(addrs) == 0 {
		return "", errors.New("no IP addresses for host")
	}
	for _, a := range addrs {
		if privateIPCheck(a.IP) {
			return "", fmt.Errorf("refusing private/loopback IP for %s: %s", host, a.IP)
		}
	}

	client := newSafeHTTPClient(timeout)
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

// isPrivateIP reports whether the given IP should be refused as an image
// source. It covers loopback, RFC1918/ULA private ranges, link-local unicast
// and multicast, generic multicast, and unspecified. The well-known cloud
// metadata address 169.254.169.254 is also refused explicitly; link-local
// catches it already, but the explicit check makes intent clear.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if ip.Equal(net.IPv4(169, 254, 169, 254)) {
		return true
	}
	return false
}
