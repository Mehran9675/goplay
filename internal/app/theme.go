package app

import (
	"os"
	"runtime"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"github.com/mehran9675/giode/fonts"
	"github.com/mehran9675/giode/styles/properties"
)

// The look: near-black surfaces, white for what matters, no color but
// for status. The video is the color.
const (
	colBackground = "#09090b"
	colSurface    = "#151518"
	colSurface2   = "#1d1d21"
	colSurface3   = "#26262b"
	colBorder     = "#2c2c32"
	colText       = "#ececef"
	colMuted      = "#9a9aa3"
	colFaint      = "#6b6b74"
	colHighlight  = "#ffffff"
	colOnLight    = "#0b0b0d"
	colDanger     = "#f05252"
	colScrim      = "#000000"
	colTrack      = "#ffffff33"
	colSwitchOn   = "#30d158"
)

// Sizes in dp.
const (
	titleBarH    = 36
	titleBarMacH = 30
	controlBarH  = 92
	panelWidth   = 360
	buttonSize   = 40
	iconSize     = 22
	playSize     = 46
	playIconSize = 26
	radius       = 10
)

// setupTheme installs the palette Giode components draw with, and the
// platform's own UI font.
func setupTheme() {
	properties.SetMode(properties.ModeDark)
	properties.SetPalette(properties.ModeDark, properties.Palette{
		Primary:    colHighlight,
		Secondary:  colMuted,
		Background: colBackground,
		Surface:    colSurface,
		Text:       colText,
		Border:     colBorder,
		OnPrimary:  colOnLight,
		Muted:      colMuted,
		Accent:     colHighlight,
		Success:    "#3ecf8e",
		Warning:    "#f5a524",
		Danger:     colDanger,
		Info:       "#c9c9d1",
	})
	fonts.Register(uiFonts())
	registerIcons()
}

// uiFonts loads the platform's interface font - Segoe UI on Windows,
// Helvetica Neue on macOS, Cantarell or DejaVu Sans on Linux - with the
// Go fonts as a fallback for anything they lack.
func uiFonts() []font.FontFace {
	var candidates []struct {
		path   string
		weight font.Weight
	}
	add := func(path string, w font.Weight) {
		candidates = append(candidates, struct {
			path   string
			weight font.Weight
		}{path, w})
	}
	switch runtime.GOOS {
	case "windows":
		dir := os.Getenv("WINDIR") + `\Fonts\`
		add(dir+"segoeui.ttf", font.Normal)
		add(dir+"seguisb.ttf", font.SemiBold)
		add(dir+"segoeuib.ttf", font.Bold)
	case "darwin":
		add("/System/Library/Fonts/HelveticaNeue.ttc", font.Normal)
	default:
		add("/usr/share/fonts/cantarell/Cantarell-VF.otf", font.Normal)
		add("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", font.Normal)
		add("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", font.Bold)
		add("/usr/share/fonts/TTF/DejaVuSans.ttf", font.Normal)
	}
	var faces []font.FontFace
	for _, c := range candidates {
		data, err := os.ReadFile(c.path)
		if err != nil {
			continue
		}
		fs, err := opentype.ParseCollection(data)
		if err != nil {
			continue
		}
		for _, f := range fs {
			// A collection (.ttc) carries its own weights; a single
			// file is the weight it was listed as.
			if len(fs) == 1 {
				f.Font.Weight = c.weight
			}
			f.Font.Typeface = ""
			faces = append(faces, f)
		}
	}
	return append(faces, gofont.Collection()...)
}
