// Package all imports every built-in directive subpackage so its init()
// self-registers with the directives registry. Import this package (blank)
// from render/html/html.go to opt into all directives at once.
package all

import (
	_ "github.com/anatolykoptev/go-kit/render/html/directives/callout"
	_ "github.com/anatolykoptev/go-kit/render/html/directives/grid"
	_ "github.com/anatolykoptev/go-kit/render/html/directives/math"
	_ "github.com/anatolykoptev/go-kit/render/html/directives/stats"
	_ "github.com/anatolykoptev/go-kit/render/html/directives/timeline"
)
