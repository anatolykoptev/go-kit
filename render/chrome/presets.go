package chrome

import "fmt"

// ImagePreset bundles geometry + theme for a named image use case.
type ImagePreset struct {
	Name     string
	WidthPx  int
	HeightPx int
	PPI      int
	Theme    string
}

// imagePresets is the canonical lookup table; "custom" intentionally
// has zero geometry so the caller is forced to supply Width/Height.
var imagePresets = map[string]ImagePreset{
	"og-image":        {Name: "og-image", WidthPx: 1200, HeightPx: 630, PPI: 144, Theme: "card"},
	"twitter-card":    {Name: "twitter-card", WidthPx: 1200, HeightPx: 675, PPI: 144, Theme: "card"},
	"square-1080":     {Name: "square-1080", WidthPx: 1080, HeightPx: 1080, PPI: 144, Theme: "card"},
	"story-1080x1920": {Name: "story-1080x1920", WidthPx: 1080, HeightPx: 1920, PPI: 144, Theme: "dark"},
	"custom":          {Name: "custom", WidthPx: 0, HeightPx: 0, PPI: 144, Theme: "report"},
}

// ResolveImagePreset returns the preset for a given name. Unknown names
// produce an error so callers cannot silently fall back.
func ResolveImagePreset(name string) (ImagePreset, error) {
	p, ok := imagePresets[name]
	if !ok {
		return ImagePreset{}, fmt.Errorf("render: unknown image preset %q", name)
	}
	return p, nil
}
