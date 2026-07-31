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
func snapshotThemes(t *testing.T) {
	t.Helper()
	themeMu.RLock()
	saved := make(map[string]TypstTheme, len(themeReg))
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
// Silent failure guarded: RegisterTypstTheme accepts the theme and nothing reads
// it, so the document renders in a built-in look, still parses, still produces a
// plausible PDF, and no error is raised anywhere.
func TestRegisterTypstTheme_ResolvesByName(t *testing.T) {
	snapshotThemes(t)

	RegisterTypstTheme(TypstTheme{Name: "house", Preamble: "#let x = 1 // " + houseMarker})

	got := resolveTypstTheme("house")
	if !strings.Contains(got.Preamble, houseMarker) {
		t.Errorf("registered preamble not returned; got %q", got.Preamble)
	}
}

// Registering over a built-in name replaces it, so a consumer can take over a
// look without go-kit shipping a variant.
func TestRegisterTypstTheme_OverridesBuiltIn(t *testing.T) {
	snapshotThemes(t)

	RegisterTypstTheme(TypstTheme{Name: "resume", Preamble: "#let y = 2 // " + houseMarker})

	got := resolveTypstTheme("resume")
	if !strings.Contains(got.Preamble, houseMarker) {
		t.Error("built-in resume theme was not overridden by a later registration")
	}
	if strings.Contains(got.Preamble, `paper:  "us-letter"`) {
		t.Error("built-in resume preamble still present after override")
	}
}

// Margin and title suppression travel with the theme entry, not with a switch
// over theme names. Before this, a caller could not get either without naming a
// built-in whose look they did not want.
func TestRegisterTypstTheme_CarriesMarginAndTitleSuppression(t *testing.T) {
	snapshotThemes(t)

	RegisterTypstTheme(TypstTheme{
		Name: "edge", Preamble: "#let z = 3", PageMarginPt: 7, OmitsTitleBlock: true,
	})

	if got := themePageMarginPt("edge"); got != 7 {
		t.Errorf("themePageMarginPt = %v, want 7", got)
	}
	if !themeOmitsTitleBlock("edge") {
		t.Error("themeOmitsTitleBlock = false, want true for a theme that declares it")
	}
}

// Built-in geometry must survive the move from switch statements to registry
// entries: card is edge-to-edge and suppresses its own title, dark is roomier.
func TestBuiltInThemes_GeometryPreserved(t *testing.T) {
	for _, tc := range []struct {
		name      string
		marginPt  float64
		omitTitle bool
	}{
		{"report", 24, false},
		{"minimal", 24, false},
		{"corporate", 24, false},
		{"card", 0, true},
		{"dark", 32, true},
		{"resume", 24, false},
	} {
		if got := themePageMarginPt(tc.name); got != tc.marginPt {
			t.Errorf("%s: margin = %v, want %v", tc.name, got, tc.marginPt)
		}
		if got := themeOmitsTitleBlock(tc.name); got != tc.omitTitle {
			t.Errorf("%s: omitsTitleBlock = %v, want %v", tc.name, got, tc.omitTitle)
		}
	}
}

// Unknown and empty names fall back to report, matching render/html.LookupTheme.
func TestResolveTypstTheme_FallsBackToReport(t *testing.T) {
	want := resolveTypstTheme("report").Preamble
	for _, name := range []string{"", "no-such-theme"} {
		if got := resolveTypstTheme(name).Preamble; got != want {
			t.Errorf("resolveTypstTheme(%q) did not fall back to report", name)
		}
	}
}

// End to end: a registered theme reaches the assembled document, and the
// built-in it displaced does not.
func TestBuildTypstSource_UsesRegisteredTheme(t *testing.T) {
	skipIfNoTypstPandoc(t)
	snapshotThemes(t)

	RegisterTypstTheme(TypstTheme{
		Name:     "house",
		Preamble: "#let marker = \"" + houseMarker + "\"\n#set page(paper: \"us-legal\")\n",
	})

	r := NewTypstRenderer()
	src, err := r.buildTypstSource(
		context.Background(), "# Heading\n\nBody text.\n", "markdown",
		render.Options{Theme: "house"}, "", false,
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
