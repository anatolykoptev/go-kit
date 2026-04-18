package fileopt

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/tabwriter"
	"time"
)

// TestBenchRatios runs each optimization stage on a diverse set of in-memory
// fixtures and prints a single summary table of ratio + duration per fixture.
// Not a Go `testing.B` benchmark — we care about ratio on varied content, not
// steady-state throughput. Skip with `-short`; auto-skip if required external
// binaries are missing (per-fixture, not suite-wide).
func TestBenchRatios(t *testing.T) {
	if testing.Short() {
		t.Skip("bench ratios skipped under -short")
	}

	ctx := context.Background()

	type row struct {
		fixture, stage string
		bytesIn        int
		bytesOut       int
		ratio          float64
		duration       time.Duration
		note           string
	}
	var rows []row

	hasGS := binaryAvailable("gs")
	hasQPDF := binaryAvailable("qpdf")
	hasOxi := binaryAvailable("oxipng")
	hasCwebp := binaryAvailable("cwebp")
	t.Logf("binaries: gs=%v qpdf=%v oxipng=%v cwebp=%v", hasGS, hasQPDF, hasOxi, hasCwebp)

	// PNG fixtures —————————————————————————————————————————————————————
	pngFixtures := []struct {
		name string
		data []byte
	}{
		{"png/solid_64x64", genSolidPNG(64, 64)},
		{"png/solid_500x500", genSolidPNG(500, 500)},
		{"png/gradient_64x64", genGradientPNG(64, 64)},
		{"png/gradient_500x500", genGradientPNG(500, 500)},
		{"png/noise_64x64", genNoisePNG(64, 64, 42)},
		{"png/noise_500x500", genNoisePNG(500, 500, 42)},
	}
	for _, f := range pngFixtures {
		if !hasOxi {
			rows = append(rows, row{f.name, "oxipng", len(f.data), 0, 0, 0, "SKIP (no binary)"})
			continue
		}
		start := time.Now()
		out, err := OptimizePNG(ctx, f.data)
		dur := time.Since(start)
		if err != nil {
			rows = append(rows, row{f.name, "oxipng", len(f.data), 0, 0, dur, "ERROR: " + err.Error()})
			continue
		}
		rows = append(rows, row{
			fixture: f.name, stage: "oxipng",
			bytesIn: len(f.data), bytesOut: len(out),
			ratio: float64(len(out)) / float64(len(f.data)), duration: dur,
		})
	}

	// WebP fixtures (cwebp accepts PNG input, re-encodes) ————————————————————
	for _, f := range pngFixtures {
		name := strings.Replace(f.name, "png/", "webp/", 1)
		if !hasCwebp {
			rows = append(rows, row{name, "cwebp", len(f.data), 0, 0, 0, "SKIP (no binary)"})
			continue
		}
		start := time.Now()
		out, err := OptimizeWebP(ctx, f.data, 80)
		dur := time.Since(start)
		if err != nil {
			rows = append(rows, row{name, "cwebp", len(f.data), 0, 0, dur, "ERROR: " + err.Error()})
			continue
		}
		rows = append(rows, row{
			fixture: name, stage: "cwebp",
			bytesIn: len(f.data), bytesOut: len(out),
			ratio: float64(len(out)) / float64(len(f.data)), duration: dur,
		})
	}

	// PDF fixtures ————————————————————————————————————————————————————————
	// Generated on the fly via gs. We run the full OptimizePDF (gs + qpdf)
	// but don't have a way to measure gs-only vs qpdf-only here without
	// re-implementing stages. Prometheus metrics already expose per-stage
	// timings in prod — this table is for ratio distribution.
	if hasGS {
		pdfFixtures := []struct {
			name   string
			gen    func(t *testing.T) []byte
			expect string // human hint on what we're testing
		}{
			{"pdf/text_short", func(t *testing.T) []byte { return genTextPDF(t, 5) }, "~1 line of text"},
			{"pdf/text_long", func(t *testing.T) []byte { return genTextPDF(t, 1000) }, "~20 pages text"},
			{"pdf/text_plus_image", func(t *testing.T) []byte { return genTextWithImagePDF(t) }, "text + embedded PNG"},
		}
		for _, f := range pdfFixtures {
			data := f.gen(t)
			start := time.Now()
			out, err := OptimizePDF(ctx, data, LevelEbook)
			dur := time.Since(start)
			stageName := "gs+qpdf"
			if !hasQPDF {
				stageName = "gs"
			}
			if err != nil {
				rows = append(rows, row{f.name, stageName, len(data), 0, 0, dur, "ERROR: " + err.Error()})
				continue
			}
			rows = append(rows, row{
				fixture: f.name, stage: stageName,
				bytesIn: len(data), bytesOut: len(out),
				ratio: float64(len(out)) / float64(len(data)), duration: dur,
				note: f.expect,
			})
		}
	}

	// Print summary table ————————————————————————————————————————————————
	var b strings.Builder
	b.WriteString("\n")
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FIXTURE\tSTAGE\tBYTES_IN\tBYTES_OUT\tRATIO\tDURATION\tNOTE")
	fmt.Fprintln(tw, "-------\t-----\t--------\t---------\t-----\t--------\t----")
	for _, r := range rows {
		if r.note != "" && strings.HasPrefix(r.note, "ERROR") || r.note == "SKIP (no binary)" {
			fmt.Fprintf(tw, "%s\t%s\t%d\t-\t-\t-\t%s\n", r.fixture, r.stage, r.bytesIn, r.note)
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%.4f\t%v\t%s\n",
			r.fixture, r.stage, r.bytesIn, r.bytesOut, r.ratio, r.duration.Round(time.Millisecond), r.note)
	}
	_ = tw.Flush()
	t.Log(b.String())
}

// ——— fixture generators ————————————————————————————————————————————————

func genSolidPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func genGradientPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 255 / max1(w-1)),
				G: uint8(y * 255 / max1(h-1)),
				B: 128,
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func genNoisePNG(w, h int, seed int64) []byte {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic fixture
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(r.Intn(256)),
				G: uint8(r.Intn(256)),
				B: uint8(r.Intn(256)),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func genTextPDF(t *testing.T, numLines int) []byte {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "text.pdf")
	scriptPath := filepath.Join(dir, "gen.ps")

	var ps bytes.Buffer
	ps.WriteString("/Helvetica findfont 10 scalefont setfont\n")
	line := "lorem ipsum dolor sit amet consectetur adipiscing elit"
	for i := 0; i < numLines; i++ {
		lineOnPage := i % 65
		y := 792 - (lineOnPage+1)*12
		if lineOnPage == 0 && i > 0 {
			ps.WriteString("showpage\n")
		}
		fmt.Fprintf(&ps, "72 %d moveto (L%d %s) show\n", y, i, line)
	}
	ps.WriteString("showpage\n")
	if err := os.WriteFile(scriptPath, ps.Bytes(), 0600); err != nil {
		t.Fatalf("write PS script: %v", err)
	}

	cmd := exec.Command("gs", //nolint:gosec // test-only PDF generation via canonical gs CLI
		"-sDEVICE=pdfwrite", "-dNOPAUSE", "-dBATCH", "-dQUIET",
		"-sOutputFile="+out, scriptPath)
	if outErr, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gs text PDF gen: %v\n%s", err, outErr)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated PDF: %v", err)
	}
	return data
}

func genTextWithImagePDF(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "noise.png")
	if err := os.WriteFile(pngPath, genNoisePNG(300, 300, 7), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "mixed.pdf")

	// Embed the raster via gs viewJPEG-style Postscript would be complex; simpler:
	// convert the PNG to PDF via gs's image device indirectly — use pdfwrite with
	// the PNG as a concatenated PDF source via the img2pdf-like trick is not
	// stdlib. Shortcut: embed the image binary as a dummy stream is not valid.
	//
	// Pragmatic alternative: use gs to convert PNG→PDF via `-sDEVICE=pdfwrite` with
	// the `viewpng` operator from GhostPDL. Most distros ship this. If it fails,
	// fall back to text-only so the suite still runs.
	ps := fmt.Sprintf(`
/Helvetica findfont 14 scalefont setfont
72 750 moveto (Mixed content: text above, noise raster below.) show
(%s) (r) file /ReusableStreamDecode filter /img exch def
gsave
72 72 translate
468 468 scale
<< /ImageType 1 /Width 300 /Height 300 /BitsPerComponent 8 /Decode [0 1 0 1 0 1]
   /ImageMatrix [300 0 0 -300 0 300] /DataSource img >> image
grestore
showpage
`, pngPath)
	cmd := exec.Command("gs", //nolint:gosec // test-only
		"-sDEVICE=pdfwrite", "-dNOPAUSE", "-dBATCH", "-dQUIET",
		"-sOutputFile="+out, "-c", ps)
	if outErr, err := cmd.CombinedOutput(); err != nil {
		t.Logf("mixed-PDF gen failed, falling back to text-only: %v\n%s", err, outErr)
		return genTextPDF(t, 50)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read mixed PDF: %v", err)
	}
	return data
}

func binaryAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}
