package html

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/anatolykoptev/go-kit/httputil"
	"github.com/anatolykoptev/go-kit/render"
)

// parseMarkdownWithImage parses md into a goldmark AST and returns the root
// document plus the source bytes. The doc is guaranteed to have at least one
// *ast.Image descendant when md contains `![...](...)`.
func parseMarkdownWithImage(t *testing.T, md string) (ast.Node, []byte) {
	t.Helper()
	src := []byte(md)
	gm := goldmark.New()
	doc := gm.Parser().Parse(text.NewReader(src))
	return doc, src
}

// firstImage returns the first *ast.Image node found under doc, or nil.
func firstImage(doc ast.Node) *ast.Image {
	var found *ast.Image
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if img, ok := n.(*ast.Image); ok {
			found = img
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

// pngBytes returns a minimal PNG-signature byte slice sufficient for MIME
// sniffing (not a valid PNG image beyond the first 4 bytes, but enough for
// our tests which only encode+decode).
func pngBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
}

// allowAllIPs is a helper to bypass the SSRF guard for httptest servers. It
// swaps the whole client-construction seam (newHTTPClientFn) for an
// unguarded *http.Client, mirroring go-enriche's WithClient escape hatch —
// httputil.NewSSRFGuardedClient's own predicate is not swappable by design
// (a framework-owned block-list must not be weakenable by an arbitrary
// caller), so tests that need a real fetch against a local httptest server
// (bound to 127.0.0.1) opt out of the guard entirely at this seam instead.
func allowAllIPs(t *testing.T) {
	t.Helper()
	orig := newHTTPClientFn
	newHTTPClientFn = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout}
	}
	t.Cleanup(func() { newHTTPClientFn = orig })
}

func TestEmbedImages_DataURLPassthrough(t *testing.T) {
	const url = "data:image/png;base64,aGVsbG8="
	doc, src := parseMarkdownWithImage(t, "![x]("+url+")")
	if err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{}); err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if img == nil {
		t.Fatal("no image node found")
	}
	if string(img.Destination) != url {
		t.Fatalf("data URL mutated: %q", img.Destination)
	}
}

func TestEmbedImages_RelativePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	if err := os.WriteFile(path, pngBytes(), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	doc, src := parseMarkdownWithImage(t, "![x](./test.png)")
	err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{Workspace: dir})
	if err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	dest := string(img.Destination)
	if !strings.HasPrefix(dest, "data:image/png;base64,") {
		t.Fatalf("destination not replaced: %q", dest)
	}
	enc := strings.TrimPrefix(dest, "data:image/png;base64,")
	dec, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(dec) != string(pngBytes()) {
		t.Fatalf("payload mismatch: %x", dec)
	}
}

func TestEmbedImages_FileURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	if err := os.WriteFile(path, pngBytes(), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	doc, src := parseMarkdownWithImage(t, "![x](file://"+path+")")
	err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{Workspace: dir})
	if err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if !strings.HasPrefix(string(img.Destination), "data:image/png;base64,") {
		t.Fatalf("destination not replaced: %q", img.Destination)
	}
}

func TestEmbedImages_OutsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	doc, src := parseMarkdownWithImage(t, "![x](/etc/passwd)")
	if err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{Workspace: dir}); err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if string(img.Destination) != "/etc/passwd" {
		t.Fatalf("destination unexpectedly mutated: %q", img.Destination)
	}
}

func TestEmbedImages_SSRFLoopback(t *testing.T) {
	const url = "http://127.0.0.1/z.png"
	doc, src := parseMarkdownWithImage(t, "![x]("+url+")")
	if err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{}); err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if string(img.Destination) != url {
		t.Fatalf("SSRF bypass — destination mutated: %q", img.Destination)
	}
}

func TestEmbedImages_SSRFPrivateIP(t *testing.T) {
	const url = "http://10.0.0.1/z.png"
	doc, src := parseMarkdownWithImage(t, "![x]("+url+")")
	if err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{Timeout: 100 * time.Millisecond}); err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if string(img.Destination) != url {
		t.Fatalf("SSRF bypass — destination mutated: %q", img.Destination)
	}
}

func TestEmbedImages_SSRFMetadataIP(t *testing.T) {
	const url = "http://169.254.169.254/latest/"
	doc, src := parseMarkdownWithImage(t, "![x]("+url+")")
	if err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{Timeout: 100 * time.Millisecond}); err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if string(img.Destination) != url {
		t.Fatalf("SSRF bypass — destination mutated: %q", img.Destination)
	}
}

func TestEmbedImages_HTTPSuccess(t *testing.T) {
	allowAllIPs(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes())
	}))
	defer srv.Close()

	doc, src := parseMarkdownWithImage(t, "![x]("+srv.URL+"/img.png)")
	if err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{}); err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if !strings.HasPrefix(string(img.Destination), "data:image/png;base64,") {
		t.Fatalf("destination not replaced: %q", img.Destination)
	}
}

func TestEmbedImages_SizeCap(t *testing.T) {
	allowAllIPs(t)
	big := make([]byte, 6<<20)
	copy(big, pngBytes())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	url := srv.URL + "/big.png"
	doc, src := parseMarkdownWithImage(t, "![x]("+url+")")
	err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{MaxBytes: 1 << 20})
	if err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if string(img.Destination) != url {
		t.Fatalf("size cap bypassed — destination: %q", img.Destination)
	}
}

func TestEmbedImages_Timeout(t *testing.T) {
	allowAllIPs(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer srv.Close()

	url := srv.URL + "/slow.png"
	doc, src := parseMarkdownWithImage(t, "![x]("+url+")")
	err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if string(img.Destination) != url {
		t.Fatalf("timeout not enforced — destination: %q", img.Destination)
	}
}

func TestEmbedImages_HTTPNon200(t *testing.T) {
	allowAllIPs(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	url := srv.URL + "/nope.png"
	doc, src := parseMarkdownWithImage(t, "![x]("+url+")")
	if err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{}); err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if string(img.Destination) != url {
		t.Fatalf("500 response was embedded — destination: %q", img.Destination)
	}
}

// TestEmbedImages_SSRFLocalhostHostname verifies the dial-time DNS-rebind
// guard through the REAL entrypoint (embedImages), rather than a deleted
// low-level safeDial helper: "localhost" resolves to 127.0.0.1 (or ::1) only
// at dial time, after fetchHTTP has already handed the request to the
// httputil-guarded client — proving the guard fires on the resolved address,
// not a hostname string, exactly as GuardedDialContext is documented to.
// isPrivateIP/safeDial no longer exist locally; the block-list and dial
// wrapper are now go-kit/httputil's (see httputil.TestIsBlockedIP /
// TestGuardedDialContext_BlocksResolvedAddress for the predicate-level
// table).
func TestEmbedImages_SSRFLocalhostHostname(t *testing.T) {
	const url = "http://localhost:1/z.png"
	doc, src := parseMarkdownWithImage(t, "![x]("+url+")")
	err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if string(img.Destination) != url {
		t.Fatalf("SSRF bypass — destination mutated: %q", img.Destination)
	}
}

// TestFetchHTTP_SSRFBlocked is the network-noise-immune sibling of the
// TestEmbedImages_SSRF* tests above. embedImages swallows every fetch
// failure into a log line and leaves the AST destination untouched
// REGARDLESS of the failure reason — on a shared host that happens to run
// other services bound to loopback/private addresses, a coincidental
// non-SSRF failure (connection refused, TLS error, unrelated 404) would
// make embedImages "pass" even if the SSRF guard were silently removed.
// This test calls fetchHTTP directly and asserts the error specifically
// wraps httputil.ErrSSRFBlocked, so it cannot be satisfied by an unrelated
// network failure.
func TestFetchHTTP_SSRFBlocked(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
	}{
		{"loopback literal", "http://127.0.0.1/z.png"},
		{"private literal", "http://10.0.0.1/z.png"},
		{"cloud metadata literal", "http://169.254.169.254/latest/"},
		{"localhost hostname", "http://localhost:1/z.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, parseErr := url.Parse(tt.rawURL)
			if parseErr != nil {
				t.Fatalf("parse %q: %v", tt.rawURL, parseErr)
			}
			_, fetchErr := fetchHTTP(context.Background(), u, render.ImageEmbedOptions{}, defaultImageMaxBytes, 500*time.Millisecond)
			if fetchErr == nil {
				t.Fatalf("fetchHTTP(%q) succeeded, want SSRF refusal", tt.rawURL)
			}
			if !errors.Is(fetchErr, httputil.ErrSSRFBlocked) {
				t.Errorf("fetchHTTP(%q) error %v does not wrap httputil.ErrSSRFBlocked", tt.rawURL, fetchErr)
			}
		})
	}
}

// TestEmbedImages_SymlinkEscape verifies that a symlink inside the workspace
// pointing at a file outside the workspace is rejected. filepath.Abs alone
// does NOT follow symlinks, so before the EvalSymlinks fix a symlink escape
// would happily leak /etc/passwd-shaped files via the PDF.
func TestEmbedImages_SymlinkEscape(t *testing.T) {
	// readWorkspaceFile has a legacy /tmp/ allow-list bypass, so we must
	// place BOTH the workspace and the outside target in a non-/tmp
	// location for the symlink-escape check to actually gate the read.
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skipf("no home dir for non-/tmp workspace: %v", err)
	}
	root, err := os.MkdirTemp(home, "gokit-symlink-test-")
	if err != nil {
		t.Skipf("mkdir under home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	workspace := filepath.Join(root, "ws")
	outsideDir := filepath.Join(root, "outside")
	for _, d := range []string{workspace, outsideDir} {
		if mkErr := os.Mkdir(d, 0o700); mkErr != nil {
			t.Fatalf("mkdir %s: %v", d, mkErr)
		}
	}
	outside := filepath.Join(outsideDir, "secret.png")
	if err := os.WriteFile(outside, pngBytes(), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	// Symlink inside workspace -> outside file.
	linkPath := filepath.Join(workspace, "link.png")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	doc, src := parseMarkdownWithImage(t, "![x](./link.png)")
	if err := embedImages(context.Background(), doc, src, render.ImageEmbedOptions{Workspace: workspace}); err != nil {
		t.Fatalf("embedImages: %v", err)
	}
	img := firstImage(doc)
	if img == nil {
		t.Fatal("no image node")
	}
	// Destination must be unchanged (escape rejected). If the fix regresses,
	// it would be replaced with data:image/png;base64,...
	if string(img.Destination) != "./link.png" {
		t.Errorf("symlink escape succeeded — destination mutated to %q", img.Destination)
	}
}
