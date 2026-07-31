package typst

import (
	"context"
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
		Name: "edge", Preamble: "#let z = 3", PageMarginPt: 7, OmitsTitleBlock: true,
	})

	got := resolveTypstTheme("edge")
	if got.PageMarginPt != 7 {
		t.Errorf("PageMarginPt = %v, want 7", got.PageMarginPt)
	}
	if !got.OmitsTitleBlock {
		t.Error("OmitsTitleBlock = false, want true for a theme that declares it")
	}
}

// Built-in geometry must survive the move from switch statements to registry
// entries: card is edge-to-edge and self-titled, dark is roomier.
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
		if got.OmitsTitleBlock != tc.omitTitle {
			t.Errorf("%s: OmitsTitleBlock = %v, want %v", tc.name, got.OmitsTitleBlock, tc.omitTitle)
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

// A theme that declares it renders its own heading must suppress the injected
// title block on the PDF path too, not only on the image path. A consumer theme
// with its own masthead otherwise gets a duplicate H1 on every PDF, silently.
func TestBuildTypstSource_OmitsTitleBlockOnPDFPath(t *testing.T) {
	skipIfNoTypstPandoc(t)
	snapshotThemes(t)

	RegisterTheme(Theme{Name: "selftitled", Preamble: "#let s = 1", OmitsTitleBlock: true})

	r := NewTypstRenderer()
	th := resolveTypstTheme("selftitled")
	src, err := r.buildTypstSource(
		context.Background(), "Body text.\n", "markdown",
		render.Options{Title: "Quarterly Review", Theme: "selftitled"}, th, "", th.OmitsTitleBlock,
	)
	if err != nil {
		t.Fatalf("buildTypstSource: %v", err)
	}
	if strings.Contains(src, "= Quarterly Review") {
		t.Errorf("title block injected for a theme declaring OmitsTitleBlock:\n%s", src)
	}
}

// End to end: a registered theme reaches the assembled document, and the
// built-in it displaced does not.
func TestBuildTypstSource_UsesRegisteredTheme(t *testing.T) {
	skipIfNoTypstPandoc(t)
	snapshotThemes(t)

	RegisterTheme(Theme{
		Name:     "house",
		Preamble: "#let marker = \"" + houseMarker + "\"\n#set page(paper: \"us-legal\")\n",
	})

	r := NewTypstRenderer()
	th := resolveTypstTheme("house")
	src, err := r.buildTypstSource(
		context.Background(), "# Heading\n\nBody text.\n", "markdown",
		render.Options{Theme: "house"}, th, "", th.OmitsTitleBlock,
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
