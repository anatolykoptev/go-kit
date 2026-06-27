package html

import (
	"bytes"
	_ "embed"
	"sync"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/anatolykoptev/go-kit/render/html/directives"
)

//go:embed templates/shell.html
var shellHTML string

//go:embed templates/styles/report.css
var reportCSSBase string

//go:embed templates/styles/minimal.css
var minimalStyleCSS string

//go:embed templates/styles/corporate.css
var corporateStyleCSS string

//go:embed templates/styles/academic.css
var academicStyleCSS string

//go:embed templates/styles/dark.css
var darkStyleCSS string

//go:embed templates/styles/directives.css
var directivesBaseCSS string

// Theme is a named CSS bundle appended to the rendered HTML document.
type Theme struct {
	Name string
	CSS  string
}

var (
	themeMu  sync.RWMutex
	themeReg = map[string]Theme{}
)

// RegisterTheme adds a theme to the registry. Safe to call from init functions.
// A later call with the same name overwrites the earlier registration.
func RegisterTheme(t Theme) {
	themeMu.Lock()
	themeReg[t.Name] = t
	themeMu.Unlock()
}

// LookupTheme returns the theme CSS for the given name, falling back to "report"
// if the name is unknown or empty. Returns an empty string if "report" is also
// somehow unregistered (should not happen in practice).
func LookupTheme(name string) string {
	themeMu.RLock()
	defer themeMu.RUnlock()
	if name == "" {
		name = "report"
	}
	if t, ok := themeReg[name]; ok {
		return t.CSS
	}
	if t, ok := themeReg["report"]; ok {
		return t.CSS
	}
	return ""
}

// appendDirectiveCSS concatenates the directive-block base CSS and every
// registered Handler's CSS fragment. Returns "" when no fragments exist
// beyond the base placeholder.
func appendDirectiveCSS(base string) string {
	extra := directives.AllCSS()
	if len(extra) == 0 && directivesBaseCSS == "" {
		return base
	}
	out := base + "\n" + directivesBaseCSS
	if len(extra) > 0 {
		out = out + "\n" + string(extra)
	}
	return out
}

// chromaStyleName selects which chroma style generates the highlighting CSS.
// Must match the style passed to highlighting.WithStyle in html.go.
const chromaStyleName = "github"

// loadReportCSS concatenates the embedded base stylesheet with a chroma
// stylesheet rendered from chromaStyleName. Called once at package init to
// register the "report" theme.
func loadReportCSS() string {
	style := styles.Get(chromaStyleName)
	if style == nil {
		style = styles.Fallback
	}
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	var buf bytes.Buffer
	if err := formatter.WriteCSS(&buf, style); err != nil {
		return reportCSSBase
	}
	return appendDirectiveCSS(reportCSSBase + "\n/* chroma " + chromaStyleName + " */\n" + buf.String())
}

// loadMinimalCSS returns the minimal-theme stylesheet.
func loadMinimalCSS() string {
	return appendDirectiveCSS(minimalStyleCSS)
}

// loadCorporateCSS returns the corporate-theme stylesheet.
func loadCorporateCSS() string {
	return appendDirectiveCSS(corporateStyleCSS)
}

// loadAcademicCSS returns the academic-theme stylesheet.
func loadAcademicCSS() string {
	return appendDirectiveCSS(academicStyleCSS)
}

// loadDarkCSS returns the dark-theme stylesheet.
func loadDarkCSS() string {
	return appendDirectiveCSS(darkStyleCSS)
}

func init() {
	RegisterTheme(Theme{Name: "report", CSS: loadReportCSS()})
	RegisterTheme(Theme{Name: "minimal", CSS: loadMinimalCSS()})
	RegisterTheme(Theme{Name: "corporate", CSS: loadCorporateCSS()})
	RegisterTheme(Theme{Name: "academic", CSS: loadAcademicCSS()})
	RegisterTheme(Theme{Name: "dark", CSS: loadDarkCSS()})
}
