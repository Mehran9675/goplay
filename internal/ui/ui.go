// Package ui renders the player controls (transport, seek bar, volume) and
// the right-click context menu with Gio.
//
// File layout:
//   - ui.go            — UI struct, Layout entry, keyboard handling, message
//   - controller.go    — Controller interface (the app methods the UI drives)
//   - controls.go      — bottom control bar
//   - slider.go        — seek + volume sliders
//   - buttons.go       — icon-button helper
//   - menu.go          — right-click context menu
//   - icons.go         — Material Design icon set
package ui

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	// controlH is the height of the bottom control bar in dp.
	controlH = 72
	// pad is the margin used by the status message in dp.
	pad      = 10
	seekStep = 5.0
	volStep  = 0.05
)

// Key identifies an app-level shortcut. The app translates platform key
// events into these and feeds them to KeyPress before each frame.
type Key uint8

const (
	KeyEscape Key = iota
	KeyF
	KeyO
	KeyD
	KeySpace
	KeyS
	KeyN
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
)

// UI owns the widget state and theme.
type UI struct {
	th *material.Theme

	playBtn, stopBtn, nextBtn, openBtn, folderBtn, fullscreenBtn widget.Clickable
	seekFloat, volFloat                                          widget.Float

	icons iconSet

	menuOpen       bool
	menuPos        f32.Point
	menuDismiss    widget.Clickable
	menuOpenFile   widget.Clickable
	menuOpenFolder widget.Clickable

	controlRect image.Rectangle
	menuRect    image.Rectangle
}

// New creates the UI: a dark theme with the Go fonts and the Material icon
// set. widget.Icon rasterizes IconVG directly, so no icon font is loaded.
func New() *UI {
	u := &UI{}
	u.th = material.NewTheme()
	u.th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	u.th.Palette = material.Palette{
		Bg:         rgb(0x0b0e14),
		Fg:         rgb(0xdfe3ea),
		ContrastBg: rgb(0x1e8ae5),
		ContrastFg: rgb(0xffffff),
	}
	u.th.TextSize = 14
	u.icons = newIcons()
	return u
}

// Layout draws the UI into gtx. c drives the player.
func (u *UI) Layout(gtx layout.Context, c Controller) {
	u.controlRect = image.Rectangle{}
	u.menuRect = image.Rectangle{}
	layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return u.messageLayout(gtx, c)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return u.menuLayout(gtx, c)
		}))
	layout.Stack{Alignment: layout.S}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return u.controlsLayout(gtx, c)
		}),
	)
}

// KeyPress handles an app-level shortcut.
func (u *UI) KeyPress(c Controller, k Key) {
	switch k {
	case KeyEscape:
		if u.menuOpen {
			u.menuOpen = false
		} else {
			c.Quit()
		}
	case KeyF:
		u.menuOpen = false
		c.ToggleFullscreen()
	case KeyO:
		u.menuOpen = false
		c.OpenFile()
	case KeyD:
		u.menuOpen = false
		c.OpenFolder()
	case KeySpace:
		u.menuOpen = false
		c.TogglePause()
	case KeyS:
		u.menuOpen = false
		c.Stop()
	case KeyN:
		u.menuOpen = false
		c.Next()
	case KeyLeft:
		if c.HasMedia() {
			u.menuOpen = false
			c.SeekRelative(-seekStep)
		}
	case KeyRight:
		if c.HasMedia() {
			u.menuOpen = false
			c.SeekRelative(seekStep)
		}
	case KeyUp:
		if c.HasMedia() {
			c.SetVolume(c.Volume() + volStep)
		}
	case KeyDown:
		if c.HasMedia() {
			c.SetVolume(c.Volume() - volStep)
		}
	}
}

// ToggleMenuAt opens/closes the context menu at dp coordinates.
func (u *UI) ToggleMenuAt(x, y float32) {
	if !u.menuOpen {
		u.menuOpen = true
	}
	u.menuPos = f32.Pt(x, y)
}

// RectContains reports whether the point (dp) is over the control bar or the
// open context menu. The app uses it to gate frameless window dragging.
func (u *UI) RectContains(x, y float32) bool {
	p := image.Pt(int(x), int(y))
	if !u.controlRect.Empty() && p.In(u.controlRect) {
		return true
	}
	if !u.menuRect.Empty() && p.In(u.menuRect) {
		return true
	}
	return false
}

// messageLayout draws the top-left status text.
func (u *UI) messageLayout(gtx layout.Context, c Controller) layout.Dimensions {
	msg := c.Message()
	if msg == "" && !c.HasMedia() {
		msg = "Open a file (O) or a folder (D) to start playing."
	}
	if msg == "" {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: pad, Left: pad}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Label(u.th, u.th.TextSize, msg)
		label.Color = color.NRGBA{A: 0xff, R: 0xff, G: 0xff, B: 0xff}
		macro := op.Record(gtx.Ops)
		dims := label.Layout(gtx)
		call := macro.Stop()
		bg := clip.UniformRRect(image.Rectangle{Max: dims.Size.Add(image.Pt(16, 10))}, gtx.Dp(6))
		paint.FillShape(gtx.Ops, rgba(0x0b0e14, 0x99), bg.Op(gtx.Ops))
		call.Add(gtx.Ops)
		return dims
	})
}

func formatTime(secs float64) string {
	if secs < 0 {
		secs = 0
	}
	s := int(secs)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{A: 0xff, R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c)}
}

func rgba(c uint32, a uint8) color.NRGBA {
	return color.NRGBA{A: a, R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c)}
}
