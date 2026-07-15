package html

import (
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/anatolykoptev/go-kit/render"
)

// allowedLogoSchemes restricts Cover.Logo URLs to http/https only.
//
// Removed from earlier revisions:
//   - "file": Chrome would load and embed the local file (e.g. file:///etc/passwd).
//     This is an SSRF / local-file-read vector. File-based logos must be embedded
//     as data:image/* by the caller before passing to CoverPage.Logo.
//   - bare "data:": allows data:text/html, data:application/javascript, etc.
//     Only data:image/* is accepted; checked explicitly in validLogoURL below.
var allowedLogoSchemes = map[string]bool{
	"http":  true,
	"https": true,
}

// validLogoURL returns true when u is safe to render as an <img src=...>.
// Empty strings are considered valid (caller skips rendering). Strings without
// a ":" are treated as relative paths and allowed.
//
// data:image/* is the only accepted data: sub-type — it allows callers to embed
// logos as base64 without an HTTP round-trip while blocking data:text/html and
// similar executable payloads. file: is explicitly rejected (local-file SSRF).
func validLogoURL(u string) bool {
	if u == "" {
		return true
	}
	if !strings.Contains(u, ":") {
		return true // relative path, no scheme
	}
	lower := strings.ToLower(u)
	// data:image/* is safe to embed; all other data: subtypes are not.
	if strings.HasPrefix(lower, "data:") {
		return strings.HasPrefix(lower, "data:image/")
	}
	scheme := strings.SplitN(lower, ":", 2)[0]
	return allowedLogoSchemes[scheme]
}

// renderCoverPage returns HTML for the cover page. Returns "" when cp is nil.
// All user-supplied fields are HTML-escaped. Date empty -> today (ISO YYYY-MM-DD).
// Logo is rendered as <img> with the URL passed through html.EscapeString, and
// the URL scheme is whitelisted (http/https/data/file or a relative path).
// Disallowed schemes (javascript:, etc.) cause the Logo to be omitted silently
// after a warning log — rendering continues with the rest of the cover.
func renderCoverPage(cp *render.CoverPage) string {
	if cp == nil {
		return ""
	}
	date := cp.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	var b strings.Builder
	b.WriteString(`<div class="cover-page">`)

	if cp.Logo != "" {
		if validLogoURL(cp.Logo) {
			fmt.Fprintf(&b, `<img class="cover-logo" src="%s" alt="logo">`, html.EscapeString(cp.Logo))
		} else {
			slog.Warn("cover logo URL scheme not allowed; skipping", "logo", cp.Logo)
		}
	}
	if cp.Title != "" {
		fmt.Fprintf(&b, `<h1 class="cover-title">%s</h1>`, html.EscapeString(cp.Title))
	}
	if cp.Subtitle != "" {
		fmt.Fprintf(&b, `<h2 class="cover-subtitle">%s</h2>`, html.EscapeString(cp.Subtitle))
	}

	b.WriteString(`<div class="cover-meta">`)
	if cp.Author != "" {
		fmt.Fprintf(&b, `<div class="cover-author">%s</div>`, html.EscapeString(cp.Author))
	}
	fmt.Fprintf(&b, `<div class="cover-date">%s</div>`, html.EscapeString(date))
	b.WriteString(`</div></div>`)

	return b.String()
}
