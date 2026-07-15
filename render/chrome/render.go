// Package chrome provides a headless-Chrome PDF and image rendering adapter
// built on chromedp. It composes render/html.RenderHTML for markdown→HTML
// and then uses the Chrome DevTools Protocol for HTML→PDF/image conversion.
package chrome

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	render "github.com/anatolykoptev/go-kit/render"
	htmlpkg "github.com/anatolykoptev/go-kit/render/html"
)

// PDFOptions configures headless-Chrome PDF rendering.
type PDFOptions struct {
	// CDPURL is the Chrome DevTools Protocol endpoint (e.g. "http://172.19.0.5:9222").
	// Required; no local Chrome is spawned.
	CDPURL string
	// HeaderText appears in the top-right of each page. Defaults to empty
	// (no header text); callers opt in by setting a non-empty value.
	HeaderText string
	// FooterTemplate is the full Chrome footer HTML. When empty, a default
	// "Page X of Y" footer is used.
	//
	// Rendered as raw HTML by Chrome; do not interpolate untrusted input.
	// Use the default or build a static template.
	FooterTemplate string
	// Timeout bounds the whole render. Defaults to 30s.
	Timeout time.Duration
}

// CombinedOptions composes markdown→HTML and HTML→PDF options.
// Fields are named (not embedded) so that future additions to Options or
// PDFOptions cannot silently collide.
type CombinedOptions struct {
	HTML render.Options
	PDF  PDFOptions
}

// emptyFooterTemplate is a minimal HTML fragment Chrome accepts as a footer
// template. DisplayHeaderFooter=true with an empty string still reserves
// margin and falls back to Chrome's default; this fragment renders nothing.
const emptyFooterTemplate = `<div></div>`

// PDFOption configures PDF printing. Use the With* constructors; fields on
// PDFOptions remain exported for backward compat but new code should prefer
// these functional options.
type PDFOption func(*PDFOptions)

// WithHeaderText sets the top-right header text (e.g. "CONFIDENTIAL").
// Empty string disables the header. Default: empty.
func WithHeaderText(s string) PDFOption {
	return func(o *PDFOptions) { o.HeaderText = s }
}

// WithoutHeader is a convenience for WithHeaderText("").
func WithoutHeader() PDFOption {
	return func(o *PDFOptions) { o.HeaderText = "" }
}

// WithFooterTemplate sets the full footer HTML (Chrome accepts <span class="pageNumber"></span>
// and <span class="totalPages"></span> placeholders).
func WithFooterTemplate(s string) PDFOption {
	return func(o *PDFOptions) { o.FooterTemplate = s }
}

// WithoutFooter disables the footer entirely. Internally sets FooterTemplate
// to a single-space <div> which Chrome accepts and renders as empty.
func WithoutFooter() PDFOption {
	return func(o *PDFOptions) {
		o.FooterTemplate = emptyFooterTemplate
	}
}

// WithPageNumbers toggles the default "Page X of Y" footer. true restores the
// default footer template; false disables it (equivalent to WithoutFooter).
func WithPageNumbers(enabled bool) PDFOption {
	return func(o *PDFOptions) {
		if enabled {
			o.FooterTemplate = "" // empty triggers default in resolvePDFDefaults
		} else {
			o.FooterTemplate = emptyFooterTemplate
		}
	}
}

// ApplyPDFOptions builds a PDFOptions from a variadic slice, layered over an
// existing base.
func ApplyPDFOptions(base PDFOptions, opts ...PDFOption) PDFOptions {
	for _, o := range opts {
		o(&base)
	}
	return base
}

// probeCDP does a GET {cdpURL}/json/version with a short timeout to confirm
// the endpoint is reachable before we hand off to chromedp. Without this,
// chromedp's lazy connect would surface any downstream error (navigation,
// print) as "unreachable" — this separates connection errors from render
// errors cleanly.
func probeCDP(ctx context.Context, cdpURL string) error {
	if cdpURL == "" {
		return errors.New("PDFOptions.CDPURL is required")
	}
	probeCtx, cancel := context.WithTimeout(ctx, cdpProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, cdpURL+"/json/version", nil)
	if err != nil {
		return fmt.Errorf("invalid CDPURL %q: %w", cdpURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("CDP endpoint %s unreachable: %w", cdpURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CDP endpoint %s returned %d", cdpURL, resp.StatusCode)
	}
	return nil
}

// RenderPDF prints the given complete HTML document to a PDF via a remote
// Chrome DevTools Protocol endpoint. Returns PDF bytes (starts with "%PDF-").
//
// This is a one-shot helper that creates and tears down a browser context per
// call. For repeated renders, use Printer instead — it keeps the browser
// context alive across calls and auto-retries on stale-connection errors.
func RenderPDF(ctx context.Context, html string, opts PDFOptions) ([]byte, error) {
	if opts.CDPURL == "" {
		return nil, errors.New("PDFOptions.CDPURL is required")
	}
	p := NewPrinter(opts.CDPURL)
	defer p.Close()
	return p.Print(ctx, html, opts)
}

// RenderMarkdownToPDF composes RenderHTML + RenderPDF.
func RenderMarkdownToPDF(ctx context.Context, markdown string, opts CombinedOptions) ([]byte, error) {
	html, err := htmlpkg.RenderHTML(ctx, markdown, opts.HTML)
	if err != nil {
		return nil, err
	}
	return RenderPDF(ctx, html, opts.PDF)
}
