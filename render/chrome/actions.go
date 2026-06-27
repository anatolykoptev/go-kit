package chrome

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	defaultHeaderText     = "" // empty = no header text; callers opt in via HeaderText
	defaultFooterTemplate = `<div style="font-size:8px;width:100%;text-align:center;color:#999;">Page <span class="pageNumber"></span> of <span class="totalPages"></span></div>`
	headerTemplatePrefix  = `<div style="font-size:8px;width:100%;text-align:right;padding-right:20px;color:#999;">`
	headerTemplateSuffix  = `</div>`

	letterWidthInches   = 8.5
	letterHeightInches  = 11.0
	defaultMarginInches = 0.7
	defaultTimeoutSecs  = 30

	cdpProbeTimeout = 3 * time.Second
)

const defaultPDFTimeout = defaultTimeoutSecs * time.Second

// resolvePDFDefaults fills in default header/footer/timeout from opts.
func resolvePDFDefaults(opts PDFOptions) (headerTemplate, footerTemplate string, timeout time.Duration) {
	timeout = opts.Timeout
	if timeout <= 0 {
		timeout = defaultPDFTimeout
	}

	headerText := opts.HeaderText
	if headerText == "" {
		headerText = defaultHeaderText
	}
	headerTemplate = headerTemplatePrefix + htmlAttrEscape(headerText) + headerTemplateSuffix

	footerTemplate = opts.FooterTemplate
	if footerTemplate == "" {
		footerTemplate = defaultFooterTemplate
	}
	return
}

// enableLifeCycleEvents enables Chrome page-lifecycle event emission so waits
// can hook into networkIdle/firstContentfulPaint/etc. Call BEFORE Navigate.
func enableLifeCycleEvents() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		if err := page.Enable().Do(ctx); err != nil {
			return fmt.Errorf("page.enable: %w", err)
		}
		if err := page.SetLifecycleEventsEnabled(true).Do(ctx); err != nil {
			return fmt.Errorf("page.setLifecycleEventsEnabled: %w", err)
		}
		return nil
	}
}

// waitForNetworkIdle blocks until Chrome reports the "networkIdle" lifecycle
// event, an internal 10s best-effort deadline fires, or the caller's ctx
// deadline fires. On the internal deadline, returns nil (DOM is already
// ready via WaitReady; external-resource loading is best-effort). Only the
// caller's ctx deadline propagates as an error.
func waitForNetworkIdle() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		innerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ch := make(chan struct{}, 1)
		chromedp.ListenTarget(innerCtx, func(ev interface{}) {
			if e, ok := ev.(*page.EventLifecycleEvent); ok && e.Name == "networkIdle" {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		})
		select {
		case <-ch:
			return nil
		case <-innerCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return nil
		}
	}
}

// pdfActions returns the chromedp action slice that loads HTML and prints it
// to PDF, writing the bytes into *bufOut. Shared by RenderPDF (one-shot) and
// Printer (reuse).
func pdfActions(html, headerTemplate, footerTemplate string, bufOut *[]byte) []chromedp.Action {
	return []chromedp.Action{
		enableLifeCycleEvents(),
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(tree.Frame.ID, html).Do(ctx)
		}),
		chromedp.WaitReady("body", chromedp.ByQuery),
		waitForNetworkIdle(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithDisplayHeaderFooter(true).
				WithPaperWidth(letterWidthInches).
				WithPaperHeight(letterHeightInches).
				WithMarginTop(defaultMarginInches).
				WithMarginBottom(defaultMarginInches).
				WithMarginLeft(defaultMarginInches).
				WithMarginRight(defaultMarginInches).
				WithHeaderTemplate(headerTemplate).
				WithFooterTemplate(footerTemplate).
				Do(ctx)
			if err != nil {
				return err
			}
			*bufOut = buf
			return nil
		}),
	}
}

// htmlAttrEscape replaces characters that would break an HTML attribute or
// template so user-supplied header text is safe to interpolate.
func htmlAttrEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}
