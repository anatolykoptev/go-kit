package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/anatolykoptev/go-kit/telegram/tgsafe"
)

const (
	// DefaultBaseURL is the public Bot API server.
	DefaultBaseURL = "https://api.telegram.org"
	// DefaultMaxSize is the largest file the public Bot API will serve (20 MiB).
	// A self-hosted Bot API server serves up to 2 GB: raise it with [WithMaxSize].
	DefaultMaxSize int64 = 20 << 20
	// DefaultTimeout bounds getFile plus the whole download, body included.
	DefaultTimeout = 60 * time.Second

	maxAPIResponse = 1 << 20 // getFile answers are a few hundred bytes
	snippetLen     = 160
	tempPattern    = "tgfile-*"
)

var (
	// ErrTooLarge means the file is bigger than the configured maximum, either
	// as declared (before any byte is read) or while streaming.
	ErrTooLarge = errors.New("telegram files: file exceeds the size limit")
	// ErrNoToken is returned when the Downloader was built with an empty token.
	ErrNoToken = errors.New("telegram files: empty bot token")
	// ErrEmptyFileID is returned for an empty file_id.
	ErrEmptyFileID = errors.New("telegram files: empty file_id")
)

// Error is an HTTP or Bot API failure. It never contains the token or a URL.
type Error struct {
	Op          string // "getFile" or "download"
	StatusCode  int    // HTTP status, or the Bot API error_code when the HTTP status was 200
	Description string // Bot API description or a short body snippet, scrubbed
}

func (e *Error) Error() string {
	s := fmt.Sprintf("telegram files: %s: HTTP %d", e.Op, e.StatusCode)
	if e.Description != "" {
		s += ": " + e.Description
	}
	return s
}

// Info describes a downloaded file. It deliberately has no path or URL field.
type Info struct {
	FileID       string
	FileUniqueID string
	// Size is the size getFile declared, 0 when Telegram did not say.
	Size int64
	// MimeType is the Content-Type of the download, or a guess from the file
	// extension when the server sent none or a generic one. Empty if unknown.
	MimeType string
}

// Downloader fetches Telegram files by file_id. Build it with [New]; it is safe
// for concurrent use.
type Downloader struct {
	token   string
	base    string
	max     int64
	timeout time.Duration
	hc      *tgsafe.HTTPClient
}

// Option configures a [Downloader].
type Option func(*config)

type config struct {
	base    string
	max     int64
	timeout time.Duration
	client  *http.Client
}

// WithMaxSize sets the size limit in bytes. A value <= 0 keeps [DefaultMaxSize]:
// the limit cannot be switched off by accident.
func WithMaxSize(n int64) Option { return func(c *config) { c.max = n } }

// WithTimeout bounds getFile plus the whole download, body included, on top of
// the caller's ctx. A value <= 0 keeps [DefaultTimeout].
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithHTTPClient supplies the transport (proxy, TLS settings). The client is
// copied and wrapped, so its errors are scrubbed and redirects are refused; its
// own Timeout still applies.
func WithHTTPClient(hc *http.Client) Option { return func(c *config) { c.client = hc } }

// WithBaseURL points the downloader at a self-hosted Bot API server (or a test
// server) instead of [DefaultBaseURL]. The bot<token> segments are appended.
func WithBaseURL(u string) Option { return func(c *config) { c.base = u } }

// New returns a Downloader for the bot token. An empty token is reported by the
// first call as [ErrNoToken].
func New(token string, opts ...Option) *Downloader {
	cfg := config{base: DefaultBaseURL, max: DefaultMaxSize, timeout: DefaultTimeout}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.max <= 0 {
		cfg.max = DefaultMaxSize
	}
	if cfg.timeout <= 0 {
		cfg.timeout = DefaultTimeout
	}
	if cfg.base == "" {
		cfg.base = DefaultBaseURL
	}
	base := http.Client{}
	if cfg.client != nil {
		base = *cfg.client
	}
	base.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Downloader{
		token:   token,
		base:    strings.TrimRight(cfg.base, "/"),
		max:     cfg.max,
		timeout: cfg.timeout,
		hc:      tgsafe.NewHTTPClient(&base, token),
	}
}

// String, GoString and LogValue keep the token out of every formatting path
// (%v, %+v, %#v, slog).
func (d Downloader) String() string { return "files.Downloader{token: <redacted>}" }

// GoString implements fmt.GoStringer; see [Downloader.String].
func (d Downloader) GoString() string { return d.String() }

// LogValue implements slog.LogValuer; see [Downloader.String].
func (d Downloader) LogValue() slog.Value { return slog.StringValue(d.String()) }

// redact is the single exit gate for every error the package returns.
func (d *Downloader) redact(err error) error { return tgsafe.Error(err, d.token) }

// scrub cleans text that came from the network. It runs where an *Error is
// built, so the typed error survives (tgsafe.Error would replace a typed error
// whose text holds the token with a message-only one and break errors.As); the
// exit gate in redact is the second line.
func (d *Downloader) scrub(s string) string { return tgsafe.Scrub(s, d.token) }

func (d *Downloader) apiError(op string, code int, desc string) *Error {
	return &Error{Op: op, StatusCode: code, Description: d.scrub(desc)}
}

// Download resolves fileID with getFile, fetches it and returns the body. The
// caller closes it; the reader enforces the size limit and the timeout while it
// is read. Every error is free of the token and of the URL.
func (d *Downloader) Download(ctx context.Context, fileID string) (io.ReadCloser, Info, error) {
	rc, info, _, err := d.open(ctx, fileID)
	if err != nil {
		return nil, Info{}, d.redact(err)
	}
	return rc, info, nil
}

// DownloadToTemp downloads fileID into a new file in dir ("" means os.TempDir())
// and returns its path. cleanup removes the file; it is never nil and safe to
// call more than once, including after an error. The file name keeps the
// extension Telegram reported (".oga", ".jpg"), which ffmpeg and STT servers
// use to sniff the format.
func (d *Downloader) DownloadToTemp(ctx context.Context, fileID, dir string) (string, func(), error) {
	noop := func() {}
	rc, _, ext, err := d.open(ctx, fileID)
	if err != nil {
		return "", noop, d.redact(err)
	}
	defer rc.Close()
	f, err := os.CreateTemp(dir, tempPattern+ext)
	if err != nil {
		return "", noop, d.redact(fmt.Errorf("telegram files: create temp file: %w", err))
	}
	name := f.Name()
	cleanup := func() { _ = os.Remove(name) }
	_, err = io.Copy(f, rc)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cleanup()
		return "", noop, d.redact(fmt.Errorf("telegram files: write temp file: %w", err))
	}
	return name, cleanup, nil
}

// open does getFile and starts the download. Errors are raw: the exported
// callers redact them.
func (d *Downloader) open(ctx context.Context, fileID string) (io.ReadCloser, Info, string, error) {
	if d.token == "" {
		return nil, Info{}, "", ErrNoToken
	}
	if fileID == "" {
		return nil, Info{}, "", ErrEmptyFileID
	}
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	info, filePath, err := d.getFile(ctx, fileID)
	if err != nil {
		cancel()
		return nil, Info{}, "", err
	}
	if info.Size > d.max {
		cancel()
		return nil, Info{}, "", fmt.Errorf("%w: declared %d bytes, limit %d", ErrTooLarge, info.Size, d.max)
	}
	body, contentType, err := d.fetch(ctx, filePath)
	if err != nil {
		cancel()
		return nil, Info{}, "", err
	}
	ext := path.Ext(filePath)
	info.MimeType = resolveMime(contentType, ext)
	return &limitedBody{rc: body, lr: io.LimitReader(body, d.max+1), max: d.max, cancel: cancel, d: d}, info, ext, nil
}

type getFileResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Result      struct {
		FileID       string `json:"file_id"`
		FileUniqueID string `json:"file_unique_id"`
		FileSize     int64  `json:"file_size"`
		FilePath     string `json:"file_path"`
	} `json:"result"`
}

func (d *Downloader) getFile(ctx context.Context, fileID string) (Info, string, error) {
	body, err := json.Marshal(map[string]string{"file_id": fileID})
	if err != nil {
		return Info{}, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+"/bot"+d.token+"/getFile", bytes.NewReader(body))
	if err != nil {
		return Info{}, "", fmt.Errorf("telegram files: getFile: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.hc.Do(req)
	if err != nil {
		return Info{}, "", fmt.Errorf("telegram files: getFile: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponse))
	if err != nil {
		return Info{}, "", fmt.Errorf("telegram files: getFile: read response: %w", err)
	}
	var r getFileResponse
	jerr := json.Unmarshal(raw, &r)
	switch {
	case jerr != nil:
		return Info{}, "", d.apiError("getFile", resp.StatusCode, "unreadable response: "+snippet(raw))
	case resp.StatusCode != http.StatusOK || !r.OK:
		code := resp.StatusCode
		if code == http.StatusOK {
			code = r.ErrorCode
		}
		return Info{}, "", d.apiError("getFile", code, r.Description)
	case r.Result.FilePath == "":
		return Info{}, "", d.apiError("getFile", resp.StatusCode, "response has no file_path: "+snippet(raw))
	}
	return Info{FileID: r.Result.FileID, FileUniqueID: r.Result.FileUniqueID, Size: r.Result.FileSize}, r.Result.FilePath, nil
}

func (d *Downloader) fetch(ctx context.Context, filePath string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+"/file/bot"+d.token+"/"+strings.TrimLeft(filePath, "/"), nil)
	if err != nil {
		return nil, "", fmt.Errorf("telegram files: download: build request: %w", err)
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("telegram files: download: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponse))
		return nil, "", d.apiError("download", resp.StatusCode, describe(raw))
	}
	if resp.ContentLength > d.max {
		resp.Body.Close()
		return nil, "", fmt.Errorf("%w: Content-Length %d, limit %d", ErrTooLarge, resp.ContentLength, d.max)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

// describe returns the Bot API description of an error body, else a snippet.
func describe(raw []byte) string {
	var r getFileResponse
	if json.Unmarshal(raw, &r) == nil && r.Description != "" {
		return r.Description
	}
	return snippet(raw)
}

func snippet(raw []byte) string {
	s := strings.Join(strings.Fields(string(raw)), " ")
	if len(s) > snippetLen {
		s = s[:snippetLen] + "..."
	}
	return s
}

func resolveMime(contentType, ext string) string {
	if mt, _, err := mime.ParseMediaType(contentType); err == nil && mt != "application/octet-stream" {
		return mt
	}
	if mt, _, err := mime.ParseMediaType(mime.TypeByExtension(ext)); err == nil {
		return mt
	}
	return ""
}

// limitedBody enforces the size limit while reading and releases the timeout
// context on Close. It reads max+1 bytes at most, so a stream that overshoots is
// detected and cut off with ErrTooLarge instead of being delivered.
type limitedBody struct {
	rc     io.ReadCloser
	lr     io.Reader
	max    int64
	n      int64
	cancel context.CancelFunc
	d      *Downloader
}

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.lr.Read(p)
	b.n += int64(n)
	if b.n > b.max {
		n -= int(b.n - b.max) // the one byte past the limit is not delivered
		return max(n, 0), b.d.redact(fmt.Errorf("%w: stream exceeded %d bytes", ErrTooLarge, b.max))
	}
	if err != nil && !errors.Is(err, io.EOF) {
		err = b.d.redact(fmt.Errorf("telegram files: download: read: %w", err))
	}
	return n, err
}

func (b *limitedBody) Close() error {
	defer b.cancel()
	return b.rc.Close()
}
