package typst

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
)

// snapshotThemes restores the registry after a test mutates it. The registry is
// process-global, so a test that registers without restoring changes what every
// later test in the package renders.
//
// Callers must NOT call t.Parallel(): the cleanup restores the whole map, which
// would clobber a sibling's registration mid-flight.
func snapshotThemes(t *testing.T) {
	t.Helper()
	themeMu.RLock()
	saved := make(map[string]Theme, len(themeReg))
	for k, v := range themeReg {
		saved[k] = v
	}
	themeMu.RUnlock()
	t.Cleanup(func() {
		themeMu.Lock()
		themeReg = saved
		themeMu.Unlock()
	})
}

// skipIfNoPandoc gates the source-assembly tests. They stop at the .typ string
// and never invoke typst, so requiring it too would silently drop the guard on a
// box that has pandoc only.
func skipIfNoPandoc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc binary not on PATH")
	}
}

const houseMarker = "PRODUCT_OWNED_PREAMBLE"

// A registered theme must be selectable by name like any built-in.
// Silent failure guarded: RegisterTheme accepts the theme and nothing reads it,
// so the document renders in a built-in look, still parses, still produces a
// plausible PDF, and no error is raised anywhere.
func TestRegisterTheme_ResolvesByName(t *testing.T) {
	snapshotThemes(t)

	RegisterTheme(Theme{Name: "house", Preamble: "#let x = 1 // " + houseMarker})

	got := resolveTypstTheme("house")
	if !strings.Contains(got.Preamble, houseMarker) {
		t.Errorf("registered preamble not returned; got %q", got.Preamble)
	}
}

// Registering over a built-in name replaces it, so a consumer can take over a
// look without go-kit shipping a variant.
func TestRegisterTheme_OverridesBuiltIn(t *testing.T) {
	snapshotThemes(t)

	RegisterTheme(Theme{Name: "resume", Preamble: "#let y = 2 // " + houseMarker})

	got := resolveTypstTheme("resume")
	if !strings.Contains(got.Preamble, houseMarker) {
		t.Error("built-in resume theme was not overridden by a later registration")
	}
	if strings.Contains(got.Preamble, `paper:  "us-letter"`) {
		t.Error("built-in resume preamble still present after override")
	}
}

// A blank name is unreachable by construction: the resolver treats a miss as
// "report", so such an entry could never be selected. Registering one is a
// programming error and must fail loudly at init rather than render as report
// forever.
func TestRegisterTheme_BlankNamePanics(t *testing.T) {
	snapshotThemes(t)

	defer func() {
		if recover() == nil {
			t.Error("RegisterTheme with a blank Name did not panic")
		}
	}()
	RegisterTheme(Theme{Preamble: "#let z = 3"})
}

// LookupTheme lets a consumer build on a built-in instead of copying its
// preamble out of this package and owning the drift.
func TestLookupTheme(t *testing.T) {
	if _, ok := LookupTheme("no-such-theme"); ok {
		t.Error("LookupTheme reported an unregistered name as found")
	}
	got, ok := LookupTheme("report")
	if !ok {
		t.Fatal("LookupTheme(report) not found")
	}
	if !strings.Contains(got.Preamble, `paper:  "a4"`) {
		t.Error("LookupTheme(report) returned something other than the report preamble")
	}
}

// Margin and title suppression travel with the theme entry, not with a switch
// over theme names. Before this, a caller could not get either without naming a
// built-in whose styling they did not want.
func TestRegisterTheme_CarriesMarginAndTitleSuppression(t *testing.T) {
	snapshotThemes(t)

	RegisterTheme(Theme{
		Name: "edge", Preamble: "#let z = 3", PageMarginPt: 7, OmitsTitleBlockOnImage: true,
	})

	got := resolveTypstTheme("edge")
	if got.PageMarginPt != 7 {
		t.Errorf("PageMarginPt = %v, want 7", got.PageMarginPt)
	}
	if !got.OmitsTitleBlockOnImage {
		t.Error("OmitsTitleBlockOnImage = false, want true for a theme that declares it")
	}
}

// Built-in geometry must survive the move from switch statements to registry
// entries: card is edge-to-edge and image-self-titled, dark is roomier.
func TestBuiltInThemes_GeometryPreserved(t *testing.T) {
	for _, tc := range []struct {
		name      string
		marginPt  float64
		omitTitle bool
	}{
		{"report", 24, false},
		{"minimal", 24, false},
		{"corporate", 24, false},
		{themeCard, 0, true},
		{themeDark, 32, true},
		{"resume", 24, false},
	} {
		got := resolveTypstTheme(tc.name)
		if got.PageMarginPt != tc.marginPt {
			t.Errorf("%s: margin = %v, want %v", tc.name, got.PageMarginPt, tc.marginPt)
		}
		if got.OmitsTitleBlockOnImage != tc.omitTitle {
			t.Errorf("%s: OmitsTitleBlockOnImage = %v, want %v", tc.name, got.OmitsTitleBlockOnImage, tc.omitTitle)
		}
	}
}

// Unknown and empty names fall back to report, matching render/html.LookupTheme.
func TestResolveTypstTheme_FallsBackToReport(t *testing.T) {
	want := resolveTypstTheme(themeDefault).Preamble
	for _, name := range []string{"", "no-such-theme"} {
		if got := resolveTypstTheme(name).Preamble; got != want {
			t.Errorf("resolveTypstTheme(%q) did not fall back to report", name)
		}
	}
}

// No built-in preamble emits a title of its own — card and dark only STYLE a
// level-1 heading the body is expected to supply. This pins the theme DATA,
// which is where the hazard lives: honoring the image-path flag on the PDF path
// would drop the title with nothing replacing it, no error, and output that
// still looks like a document.
func TestBuiltInThemes_DoNotSelfTitle(t *testing.T) {
	for _, name := range []string{"report", "minimal", "corporate", themeCard, themeDark, "resume"} {
		th := resolveTypstTheme(name)
		emitsTitle := strings.Contains(th.Preamble, "{{.Title}}")
		if emitsTitle && th.OmitsTitleBlockOnImage {
			t.Errorf("%s: suppresses the injected title AND emits its own — one of the two is wrong", name)
		}
		if !emitsTitle && !strings.Contains(th.Preamble, "heading.where(level: 1)") {
			t.Errorf("%s: neither emits {{.Title}} nor styles a level-1 heading — "+
				"a document rendered with it has no title path at all", name)
		}
	}
}

// The image path suppresses the injected title only for themes declaring it; the
// PDF path never does.
//
// Drives pdfSource/imageSource — the functions Render and RenderImage actually
// call — rather than restating their arguments. An earlier version of this test
// re-typed them, which made it a mirror: reintroducing the regression left the
// whole suite green because the test asserted on arguments it supplied itself.
func TestSource_TitleBlockPerPath(t *testing.T) {
	skipIfNoPandoc(t)

	r := NewTypstRenderer()
	const title = "Quarterly Review"
	opts := render.Options{Title: title, Theme: themeCard}

	pdfSrc, err := r.pdfSource(context.Background(), "Body text.\n", "markdown", opts)
	if err != nil {
		t.Fatalf("pdfSource: %v", err)
	}
	if !strings.Contains(pdfSrc, "= "+title) {
		t.Errorf("card: title missing on the PDF path, and the preamble emits none either:\n%s", pdfSrc)
	}

	imgSrc, err := r.imageSource(context.Background(), "Body text.\n", "markdown", opts)
	if err != nil {
		t.Fatalf("imageSource: %v", err)
	}
	if strings.Contains(imgSrc, "= "+title) {
		t.Errorf("card: title injected on the image path despite OmitsTitleBlockOnImage:\n%s", imgSrc)
	}
}

// End to end: a registered theme reaches the assembled document, and the
// built-in it displaced does not.
func TestBuildTypstSource_UsesRegisteredTheme(t *testing.T) {
	skipIfNoPandoc(t)
	snapshotThemes(t)

	RegisterTheme(Theme{
		Name:     "house",
		Preamble: "#let marker = \"" + houseMarker + "\"\n#set page(paper: \"us-legal\")\n",
	})

	r := NewTypstRenderer()
	th := resolveTypstTheme("house")
	src, err := r.buildTypstSource(
		context.Background(), "# Heading\n\nBody text.\n", "markdown",
		render.Options{Theme: "house"}, th, "", th.OmitsTitleBlockOnImage,
	)
	if err != nil {
		t.Fatalf("buildTypstSource: %v", err)
	}
	if !strings.Contains(src, houseMarker) {
		t.Errorf("registered preamble missing from assembled source:\n%s", src)
	}
	if strings.Contains(src, `paper:  "a4"`) {
		t.Errorf("report theme preamble present; registration did not take effect:\n%s", src)
	}
	if !strings.Contains(src, "Body text.") {
		t.Errorf("converted body missing from assembled source:\n%s", src)
	}
}
