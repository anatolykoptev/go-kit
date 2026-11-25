package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/anatolykoptev/go-kit/telegram/tgsafe"
)

const redactedForm = "files.Downloader{token: <redacted>}"

// state holds the secret. Downloader keeps it behind a pointer, and state
// prints redacted through every fmt verb too, so reflection-based printers that
// walk the Downloader's fields reach only a redacting Formatter.
type state struct {
	token   string
	base    string
	max     int64
	timeout time.Duration
	hc      Doer
	cfgErr  error
}

// Format implements fmt.Formatter: every verb prints the redacted form.
func (*state) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redactedForm) }

// String implements fmt.Stringer.
func (*state) String() string { return redactedForm }

// Format implements fmt.Formatter: every verb (%v %+v %#v %s %q %d %x ...)
// prints the redacted form, so no bad-verb fallback can reach the fields.
func (Downloader) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redactedForm) }

// String implements fmt.Stringer; the token is never printed.
func (Downloader) String() string { return redactedForm }

// GoString implements fmt.GoStringer; the token is never printed.
func (Downloader) GoString() string { return redactedForm }

// LogValue implements slog.LogValuer; the token is never logged.
func (Downloader) LogValue() slog.Value { return slog.StringValue(redactedForm) }

// scrub removes the token from s in every form it can take in network text: raw,
// query-escaped, percent-encoded (any hex case) and decoded from up to three
// layers of percent-encoding. It works on the whole text; callers truncate
// afterwards, never before, so a token straddling a cut cannot survive.
func (s *state) scrub(text string) string {
	text = tgsafe.Scrub(text, s.token)
	cur := text
	for range 3 {
		dec, err := url.PathUnescape(cur)
		if err != nil || dec == cur {
			break
		}
		if clean := tgsafe.Scrub(dec, s.token); clean != dec {
			return clean
		}
		cur = dec
	}
	return text
}

// truncate cuts s to at most n bytes without splitting a rune.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "..."
}

// walk visits err and everything it wraps, including errors.Join trees.
func walk(err error, visit func(error)) {
	for err != nil {
		visit(err)
		switch u := err.(type) { //nolint:errorlint // walking the chain by hand is the point
		case interface{ Unwrap() []error }:
			for _, e := range u.Unwrap() {
				walk(e, visit)
			}
			return
		case interface{ Unwrap() error }:
			err = u.Unwrap()
		default:
			return
		}
	}
}

// redact is the exit gate for every error the package returns. A chain whose
// every node prints clean is returned unchanged, so typed errors, redacted
// *url.Errors and sentinels keep working with errors.Is and errors.As. If any
// node in the chain prints the token (a raw *url.Error hiding behind a wrapper
// with clean text counts: its own text carries the URL), the result is a fresh
// error with the scrubbed message that keeps errors.Is and Timeout() but hands
// out nothing to errors.As.
func (s *state) redact(err error) error {
	if err == nil {
		return nil
	}
	dirty := false
	walk(err, func(e error) {
		m := e.Error()
		if s.scrub(m) != m {
			dirty = true
		}
	})
	if !dirty {
		return err
	}
	return &sanitizedError{msg: s.scrub(err.Error()), orig: err}
}

// sanitizedError is err's message with the token scrubbed and nothing else of
// its chain reachable by errors.As: errors.Is still answers for the original
// (context.Canceled, ErrTooLarge, syscall errors...) and Timeout reports the
// original's timeout.
type sanitizedError struct {
	msg  string
	orig error
}

func (e *sanitizedError) Error() string { return e.msg }

func (e *sanitizedError) Is(target error) bool { return errors.Is(e.orig, target) }

func (e *sanitizedError) Timeout() bool {
	var ne net.Error
	return errors.Is(e.orig, context.DeadlineExceeded) || (errors.As(e.orig, &ne) && ne.Timeout())
}
