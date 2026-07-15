// image.go provides raster-image capture via CDP, mirroring the
// PDF rendering design (one-shot RenderImage + persistent Printer.CaptureImage).
package chrome

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	render "github.com/anatolykoptev/go-kit/render"
	htmlpkg "github.com/anatolykoptev/go-kit/render/html"
)

// ImageFormat identifies the raster encoding for CaptureImage output.
type ImageFormat string

const (
	ImageFormatPNG  ImageFormat = "png"
	ImageFormatJPEG ImageFormat = "jpeg"
	ImageFormatWebP ImageFormat = "webp"
)

// ImageOptions configures raster-image capture via CDP.
// CDPURL is ignored on Printer.CaptureImage (URL set at NewPrinter); included
// for the one-shot RenderImage helper.
type ImageOptions struct {
	CDPURL            string
	Format            ImageFormat   // default PNG
	Quality           int           // 1-100 for JPEG/WebP; ignored for PNG
	Width             int           // viewport CSS pixels; default 1024
	Height            int           // viewport CSS pixels; default 768 (ignored when FullPage)
	FullPage          bool          // default false; true = entire scrollable content
	DeviceScaleFactor float64       // default 2 (retina-crisp text)
	Timeout           time.Duration // default 30s
	// SettleDelay is a fixed sleep after networkIdle before the screenshot.
	// CSS paint and layout commit happen after networkIdle fires, so without
	// a settle delay the capture can race ahead of the first visible frame.
	// Default 300ms; set to 0 to disable.
	SettleDelay time.Duration
}

const (
	defaultImageWidth       = 1024
	defaultImageHeight      = 768
	defaultImageScaleFactor = 2.0
	defaultImageTimeoutSecs = 30
	defaultImageQuality     = 90
	defaultImageSettleDelay = 300 * time.Millisecond
)

// RenderImage is a one-shot convenience: construct a Printer, capture, close.
// Prefer Printer.CaptureImage for multiple calls (reuses browser context).
func RenderImage(ctx context.Context, html string, opts ImageOptions) ([]byte, error) {
	if opts.CDPURL == "" {
		return nil, errors.New("ImageOptions.CDPURL is required")
	}
	p := NewPrinter(opts.CDPURL)
	defer p.Close()
	return p.CaptureImage(ctx, html, opts)
}

// CombinedImageOptions pairs HTML render options with image capture options.
type CombinedImageOptions struct {
	HTML  render.Options
	Image ImageOptions
}

// RenderMarkdownToImage composes RenderHTML + RenderImage.
func RenderMarkdownToImage(ctx context.Context, markdown string, opts CombinedImageOptions) ([]byte, error) {
	html, err := htmlpkg.RenderHTML(ctx, markdown, opts.HTML)
	if err != nil {
		return nil, fmt.Errorf("markdown to html: %w", err)
	}
	return RenderImage(ctx, html, opts.Image)
}

// resolveImageDefaults fills zero fields with sensible defaults.
func resolveImageDefaults(opts ImageOptions) ImageOptions {
	if opts.Format == "" {
		opts.Format = ImageFormatPNG
	}
	if opts.Width == 0 {
		opts.Width = defaultImageWidth
	}
	if opts.Height == 0 {
		opts.Height = defaultImageHeight
	}
	if opts.DeviceScaleFactor == 0 {
		opts.DeviceScaleFactor = defaultImageScaleFactor
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaultImageTimeoutSecs * time.Second
	}
	if opts.Quality == 0 {
		opts.Quality = defaultImageQuality
	}
	if opts.SettleDelay == 0 {
		opts.SettleDelay = defaultImageSettleDelay
	}
	return opts
}

// imageActions builds the chromedp action list for capturing an image after
// loading HTML. Mirrors pdfActions in actions.go.
//
// Note: chromedp.FullScreenshot always returns PNG when quality<=0 or JPEG
// respects the opts.Format choice; FullPage uses GetLayoutMetrics + SetDeviceMetricsOverride
// to size the viewport to the full document height, then CaptureScreenshot
// with the user's chosen format. This bypasses chromedp.FullScreenshot's
// JPEG-only limitation.
func imageActions(html string, opts ImageOptions, out *[]byte) []chromedp.Action {
	return []chromedp.Action{
		enableLifeCycleEvents(),
		emulation.SetDeviceMetricsOverride(
			int64(opts.Width),
			int64(opts.Height),
			opts.DeviceScaleFactor,
			false,
		),
		// Disable forced-colors (high-contrast accessibility mode) and lock
		// color scheme to dark. Without this, Chrome profiles that have
		// forced-colors:active strip all background fills — dark rects render
		// transparent on white, completely breaking dark-theme HTML renders.
		emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{
			{Name: "forced-colors", Value: "none"},
			{Name: "prefers-color-scheme", Value: "dark"},
		}),
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
		// Fixed settle: CSS paint/layout commit happens after networkIdle.
		// Without this sleep the screenshot races ahead of the first frame.
		chromedp.Sleep(opts.SettleDelay),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// If FullPage, resize viewport to content dimensions before capture.
			if opts.FullPage {
				_, _, _, _, _, contentSize, err := page.GetLayoutMetrics().Do(ctx)
				if err != nil {
					return fmt.Errorf("get layout metrics: %w", err)
				}
				w := int64(contentSize.Width)
				if minW := int64(opts.Width); minW > w {
					w = minW
				}
				if err := emulation.SetDeviceMetricsOverride(
					w,
					int64(contentSize.Height),
					opts.DeviceScaleFactor,
					false,
				).Do(ctx); err != nil {
					return fmt.Errorf("resize viewport for full page: %w", err)
				}
			}

			fmtType := page.CaptureScreenshotFormatPng
			switch opts.Format {
			case ImageFormatJPEG:
				fmtType = page.CaptureScreenshotFormatJpeg
			case ImageFormatWebP:
				fmtType = page.CaptureScreenshotFormatWebp
			}
			params := page.CaptureScreenshot().WithFormat(fmtType).WithCaptureBeyondViewport(opts.FullPage)
			// Quality only applies to JPEG/WebP.
			if fmtType != page.CaptureScreenshotFormatPng {
				params = params.WithQuality(int64(opts.Quality))
			}
			data, err := params.Do(ctx)
			if err != nil {
				return err
			}
			*out = data
			return nil
		}),
	}
}
