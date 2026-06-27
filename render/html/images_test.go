package html

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

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

// allowAllIPs is a helper to bypass the SSRF guard for httptest servers.
func allowAllIPs(t *testing.T) {
	t.Helper()
	orig := privateIPCheck
	privateIPCheck = func(ip net.IP) bool { return false }
	t.Cleanup(func() { privateIPCheck = orig })
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

// TestSafeDial_RejectsPrivateIP verifies the dial-time DNS rebinding guard.
// "localhost" resolves to 127.0.0.1 (or ::1), which the default privateIPCheck
// rejects. safeDial must return an error before any TCP connect happens.
func TestSafeDial_RejectsPrivateIP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := safeDial(ctx, "tcp", "localhost:1")
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("safeDial accepted loopback host; expected private-IP refusal")
	}
	if !strings.Contains(err.Error(), "private IP") && !strings.Contains(err.Error(), "resolve") {
		t.Fatalf("expected private-IP or resolve error, got: %v", err)
	}
}

func TestIsPrivateIP_Table(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.0.1", true},
		{"169.254.1.1", true},
		{"169.254.169.254", true},
		{"fe80::1", true},
		{"224.0.0.1", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("parse %q", c.ip)
		}
		if got := isPrivateIP(ip); got != c.want {
			t.Errorf("isPrivateIP(%s)=%v, want %v", c.ip, got, c.want)
		}
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
