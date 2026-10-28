package ui

import (
	"embed"
	"fmt"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/icon"
	"github.com/mehran9675/giode/styles/properties"
)

// Giode ships no icon set, so the player embeds the icons it uses. They
// are Material Design glyphs (Apache 2.0) on a 24x24 viewBox, registered
// by file name: "icons/skip-next.svg" becomes "skip-next".
//
//go:embed icons/*.svg
var iconFiles embed.FS

func init() {
	if err := giode.RegisterIconsFS(iconFiles, "icons"); err != nil {
		panic(err)
	}
}

// iconSrc returns the registered icon name, for APIs that take an
// icon.Source (the context menu). The icons are embedded, so a missing
// one is a programming error.
func iconSrc(name string) icon.Source {
	src, ok := giode.LookupIcon(name)
	if !ok {
		panic(fmt.Sprintf("ui: icon %q is not registered", name))
	}
	return src
}

// glyph renders a registered icon as a square of size dp. Width and
// Height are both set so the icon never takes a non-square box.
func glyph(name string, size int, color string) giode.Element {
	return giode.Icon(name, giode.Styles{
		Width:  properties.Width(size),
		Height: properties.Height(size),
		Color:  color,
	})
}
