package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
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
	descLen        = 160     // cap on any server text placed in an error
	maxExtLen      = 12
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
	// ErrBaseURL is returned when the base URL is unusable: it must be https, or
	// http on a loopback host (a local Bot API server or a test), and carry no
	// credentials. The URL itself is never echoed.
	ErrBaseURL = errors.New("telegram files: base URL must be https, or http on a loopback host, without credentials")
)

// Error is an HTTP or Bot API failure. It never contains the token or a URL.
type Error struct {
	Op          string // "getFile" or "download"
	StatusCode  int    // HTTP status, or the Bot API error_code when the HTTP status was 200
	Description string // Bot API description or a short body snippet, scrubbed
	cause       error  // e.g. the context error that cut the body read short
}

func (e *Error) Error() string {
	s := fmt.Sprintf("telegram files: %s: HTTP %d", e.Op, e.StatusCode)
	if e.Description != "" {
		s += ": " + e.Description
	}
	return s
}

// Unwrap returns the underlying read failure, if any, so errors.Is still sees
// context.DeadlineExceeded when a timeout cut an error body short.
func (e *Error) Unwrap() error { return e.cause }

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

// Doer is the part of *http.Client the downloader needs.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Downloader fetches Telegram files by file_id. Build it with [New]; it is safe
// for concurrent use. The token lives behind an unexported pointer and every
// fmt verb prints a redacted form.
type Downloader struct{ s *state }

// Option configures a [Downloader].
type Option func(*config)

type config struct {
	base    string
	max     int64
	timeout time.Duration
	client  Doer
}

// WithMaxSize sets the size limit in bytes. A value <= 0 keeps [DefaultMaxSize]:
// the limit cannot be switched off by accident. Values above MaxInt64-1 are
// clamped.
func WithMaxSize(n int64) Option { return func(c *config) { c.max = n } }

// WithTimeout bounds getFile plus the whole download, body included, on top of
// the caller's ctx. A value <= 0 keeps [DefaultTimeout].
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithHTTPClient supplies the transport (proxy, TLS settings, a bot's own
// client). An *http.Client (or *tgsafe.HTTPClient) is copied with redirects
// refused; any other Doer is used as is, and a Doer that follows redirects or
// logs requests is outside this package's guarantees. Errors are scrubbed
// either way.
func WithHTTPClient(hc Doer) Option { return func(c *config) { c.client = hc } }

// WithBaseURL points the downloader at a self-hosted Bot API server (or a test
// server) instead of [DefaultBaseURL]. The bot<token> segments are appended.
// The URL must be https, or http on a loopback host; anything else makes every
// call return [ErrBaseURL].
func WithBaseURL(u string) Option { return func(c *config) { c.base = u } }

// New returns a Downloader for the bot token. An empty token is reported by the
// first call as [ErrNoToken], an unusable base URL as [ErrBaseURL].
func New(token string, opts ...Option) *Downloader {
	cfg := config{base: DefaultBaseURL, max: DefaultMaxSize, timeout: DefaultTimeout}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.max <= 0 {
		cfg.max = DefaultMaxSize
	}
	cfg.max = min(cfg.max, math.MaxInt64-1) // max+1 is the read bound: no overflow
	if cfg.timeout <= 0 {
		cfg.timeout = DefaultTimeout
	}
	if cfg.base == "" {
		cfg.base = DefaultBaseURL
	}
	s := &state{token: token, max: cfg.max, timeout: cfg.timeout}
	s.base, s.cfgErr = checkBase(cfg.base)
	s.hc = buildDoer(cfg.client, token)
	return &Downloader{s: s}
}

// checkBase accepts https URLs and http URLs on a loopback host, without
// credentials.
func checkBase(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return "", ErrBaseURL
	}
	switch u.Scheme {
	case "https":
	case "http":
		host := u.Hostname()
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", ErrBaseURL
		}
	default:
		return "", ErrBaseURL
	}
	return strings.TrimRight(raw, "/"), nil
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func buildDoer(c Doer, token string) Doer {
	var hc *http.Client
	switch v := c.(type) {
	case nil:
		hc = &http.Client{}
	case *http.Client:
		if v == nil {
			hc = &http.Client{}
			break
		}
		cp := *v
		hc = &cp
	case *tgsafe.HTTPClient:
		cp := *v.Client()
		hc = &cp
	default:
		return safeDoer{inner: c, token: token}
	}
	hc.CheckRedirect = noRedirect
	return tgsafe.NewHTTPClient(hc, token)
}

// safeDoer scrubs the errors of a caller-supplied Doer.
type safeDoer struct {
	inner Doer
	token string
}

func (d safeDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.inner.Do(req)
	return resp, tgsafe.Error(err, d.token)
}

// Download resolves fileID with getFile, fetches it and returns the body. The
// caller closes it; the reader enforces the size limit and the timeout while it
// is read. Every error is free of the token and of the URL.
func (d *Downloader) Download(ctx context.Context, fileID string) (io.ReadCloser, Info, error) {
	rc, info, _, err := d.s.open(ctx, fileID)
	if err != nil {
		return nil, Info{}, d.s.redact(err)
	}
	return rc, info, nil
}

// DownloadToTemp downloads fileID into a new file in dir ("" means os.TempDir())
// and returns its path. cleanup removes the file; it is never nil, idempotent,
// and removes only the file this call created (a replacement placed at the same
// path is left alone). The file is also removed on every failure path, panic
// included. The name keeps the extension Telegram reported (".oga", ".jpg"),
// which ffmpeg and STT servers use to sniff the format.
func (d *Downloader) DownloadToTemp(ctx context.Context, fileID, dir string) (string, func() error, error) {
	noop := func() error { return nil }
	rc, _, ext, err := d.s.open(ctx, fileID)
	if err != nil {
		return "", noop, d.s.redact(err)
	}
	defer rc.Close()
	f, err := os.CreateTemp(dir, tempPattern+ext)
	if err != nil {
		return "", noop, d.s.redact(fmt.Errorf("telegram files: create temp file: %w", err))
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", noop, d.s.redact(fmt.Errorf("telegram files: stat temp file: %w", err))
	}
	name := f.Name()
	remove := newRemover(name, fi)
	cleanup := func() error { return d.s.redact(remove()) }
	done := false
	defer func() {
		if !done { // error return or panic
			_ = f.Close()
			_ = remove()
		}
	}()
	_, err = io.Copy(f, rc)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", noop, d.s.redact(fmt.Errorf("telegram files: write temp file: %w", err))
	}
	done = true
	return name, cleanup, nil
}

// newRemover returns an idempotent remover for the file name that was created as
// fi: it unlinks only if the path still holds that very file.
func newRemover(name string, fi fs.FileInfo) func() error {
	var (
		once sync.Once
		rerr error
	)
	return func() error {
		once.Do(func() {
			cur, err := os.Lstat(name)
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				rerr = err
			case !os.SameFile(fi, cur):
			default:
				if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
					rerr = err
				}
			}
		})
		return rerr
	}
}

// open does getFile and starts the download. Errors are raw: the exported
// callers redact them.
func (s *state) open(ctx context.Context, fileID string) (io.ReadCloser, Info, string, error) {
	switch {
	case s.cfgErr != nil:
		return nil, Info{}, "", s.cfgErr
	case s.token == "":
		return nil, Info{}, "", ErrNoToken
	case fileID == "":
		return nil, Info{}, "", ErrEmptyFileID
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	info, filePath, err := s.getFile(ctx, fileID)
	if err != nil {
		cancel()
		return nil, Info{}, "", err
	}
	if info.Size > s.max {
		cancel()
		return nil, Info{}, "", fmt.Errorf("%w: declared %d bytes, limit %d", ErrTooLarge, info.Size, s.max)
	}
	body, contentType, err := s.fetch(ctx, filePath)
	if err != nil {
		cancel()
		return nil, Info{}, "", err
	}
	ext := safeExt(path.Ext(filePath))
	info.MimeType = s.scrub(resolveMime(contentType, ext))
	return &limitedBody{rc: body, lr: io.LimitReader(body, s.max+1), max: s.max, cancel: cancel, s: s}, info, ext, nil
}

// safeExt keeps a file extension usable in a temp-file pattern: short, with
// letters, digits and dots only.
func safeExt(ext string) string {
	if len(ext) > maxExtLen {
		return ""
	}
	for _, r := range ext {
		if r != '.' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return ext
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

// apiError builds a typed error whose text is scrubbed in full and only then
// cut to descLen. Scrubbing first matters: a token straddling the cut would
// otherwise survive as an unrecognisable fragment.
func (s *state) apiError(op string, code int, desc string) *Error {
	return &Error{Op: op, StatusCode: code, Description: truncate(s.scrub(desc), descLen)}
}

func collapse(raw []byte) string { return strings.Join(strings.Fields(string(raw)), " ") }

func (s *state) getFile(ctx context.Context, fileID string) (Info, string, error) {
	body, err := json.Marshal(map[string]string{"file_id": fileID})
	if err != nil {
		return Info{}, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/bot"+s.token+"/getFile", bytes.NewReader(body))
	if err != nil {
		return Info{}, "", fmt.Errorf("telegram files: getFile: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.hc.Do(req)
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
		return Info{}, "", s.apiError("getFile", resp.StatusCode, "unreadable response: "+collapse(raw))
	case resp.StatusCode != http.StatusOK || !r.OK:
		code := resp.StatusCode
		if code == http.StatusOK {
			code = r.ErrorCode
		}
		return Info{}, "", s.apiError("getFile", code, r.Description)
	case r.Result.FilePath == "":
		return Info{}, "", s.apiError("getFile", resp.StatusCode, "response has no file_path: "+collapse(raw))
	}
	return Info{
		FileID:       truncate(s.scrub(r.Result.FileID), descLen),
		FileUniqueID: truncate(s.scrub(r.Result.FileUniqueID), descLen),
		Size:         r.Result.FileSize,
	}, r.Result.FilePath, nil
}

func (s *state) fetch(ctx context.Context, filePath string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/file/bot"+s.token+"/"+strings.TrimLeft(filePath, "/"), nil)
	if err != nil {
		return nil, "", fmt.Errorf("telegram files: download: build request: %w", err)
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("telegram files: download: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponse))
		e := s.apiError("download", resp.StatusCode, describe(raw))
		e.cause = rerr // keeps errors.Is(ctx errors) when the read was cut short
		return nil, "", e
	}
	if resp.ContentLength > s.max {
		resp.Body.Close()
		return nil, "", fmt.Errorf("%w: Content-Length %d, limit %d", ErrTooLarge, resp.ContentLength, s.max)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

// describe returns the Bot API description of an error body, else the body.
func describe(raw []byte) string {
	var r getFileResponse
	if json.Unmarshal(raw, &r) == nil && r.Description != "" {
		return r.Description
	}
	return collapse(raw)
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
// detected and cut off with ErrTooLarge instead of being delivered. Every error
// it returns, Close included, goes through the redaction gate.
type limitedBody struct {
	rc     io.ReadCloser
	lr     io.Reader
	max    int64
	n      int64
	cancel context.CancelFunc
	s      *state
}

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.lr.Read(p)
	b.n += int64(n)
	if b.n > b.max {
		n -= int(b.n - b.max) // the one byte past the limit is not delivered
		return max(n, 0), b.s.redact(fmt.Errorf("%w: stream exceeded %d bytes", ErrTooLarge, b.max))
	}
	if err != nil && err != io.EOF { //nolint:errorlint // only a bare io.EOF is the clean end; anything else is redacted
		err = b.s.redact(fmt.Errorf("telegram files: download: read: %w", err))
	}
	return n, err
}

func (b *limitedBody) Close() error {
	defer b.cancel()
	if err := b.rc.Close(); err != nil {
		return b.s.redact(fmt.Errorf("telegram files: download: close: %w", err))
	}
	return nil
}
