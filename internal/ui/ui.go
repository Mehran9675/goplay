// Package ui renders the player's chrome with Giode: the title bar, the
// control bar (transport, seek bar, volume), the right-click context menu
// and the settings dialog.
//
// File layout:
//   - ui.go        — UI struct, Layout entry, keyboard handling, message
//   - controller.go — Controller interface (the app methods the UI drives)
//   - controls.go  — bottom control bar
//   - icons.go     — embedded SVG icons
//   - menu.go      — right-click context menu
//   - prefs.go     — settings dialog
//   - stack.go     — layout helpers that keep portals anchored correctly
//   - slider.go    — seek + volume slider sync
package ui

import (
	"fmt"
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/dialog"
	"github.com/mehran9675/giode/components/iconbutton"
	"github.com/mehran9675/giode/components/menu"
	"github.com/mehran9675/giode/components/slider"
	"github.com/mehran9675/giode/components/titlebar"
	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/goplay-ebiten/internal/settings"
)

const (
	// controlH and titleH are the heights of the bottom control bar
	// and the title bar in dp. Giode styles are in dp and scaled to
	// pixels by the elements themselves.
	controlH = 84
	titleH   = 32
	// pad is the margin used by the status message in dp.
	pad = 16
	// seekHoldTime is how long the seek slider ignores the player's
	// position after a click, while mpv performs the seek.
	seekHoldTime = 400 * time.Millisecond
)

// Palette. The chrome is neutral: light glyphs and text on near-black,
// with white as the only highlight.
const (
	fgColor       = "#e6e9ef"
	mutedColor    = "#9aa0aa"
	iconColor     = "#e6e9ef"
	highlight     = "#f2f3f5" // slider fill, play button
	onHighlight   = "#0e0f12" // glyph on the play button
	trackColor    = "#ffffff30"
	thumbColor    = "#ffffff"
	barColor      = "#0e0f12"
	menuBg        = "#121316"
	menuFg        = "#d9dce1"
	menuHover     = "#24262b"
	menuSeparator = "#26282d"
	menuDisabled  = "#8b909a"
)

// ControlHeight and TitleBarHeight are the bars' heights in dp. Outside
// fullscreen the bars sit above and below the video, so the app keeps
// the video out of those strips and makes the window that much taller.
const (
	ControlHeight  = controlH
	TitleBarHeight = titleH
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

// UI owns the Giode elements and components that make up the player
// chrome.
type UI struct {
	Fullscreen bool

	// c is the controller the UI drives. Button and menu callbacks are
	// installed once, before a controller exists, and go through act,
	// which reads c; Layout refreshes it every frame.
	c Controller

	title     *titlebar.TitleBar
	menu      *menu.Menu
	seek, vol *slider.Slider

	playBtn, stopBtn, nextBtn, muteBtn, settingsBtn, fullscreenBtn *iconbutton.Button

	settingsOpen bool
	settings     *dialog.Dialog
	prefs        *prefsView

	// barHover tracks the pointer over the bottom strip, which shows the
	// otherwise hidden control bar in fullscreen.
	barHover gesture.Hover

	// unmuteVol is the volume the mute button restores.
	unmuteVol float64

	// seekHold keeps the seek slider at a clicked position until mpv
	// has had time to report it (see syncSliders).
	seekHold time.Time

	controlRect image.Rectangle
}

// New creates the UI: the bars' components, the two sliders, the context
// menu and the settings dialog. UseGofont embeds the Go fonts for
// Giode's text shaper.
func New() *UI {
	giode.UseGofont()

	u := &UI{unmuteVol: 1}
	sliderSt := giode.Styles{Color: highlight, Background: trackColor, BorderColor: thumbColor}
	u.seek = giode.Input.Slider(sliderSt).OnChange(func(v float32) {
		if u.c != nil {
			u.c.Seek(float64(v))
			u.seekHold = time.Now().Add(seekHoldTime)
		}
	})
	u.vol = giode.Input.Slider(sliderSt).OnChange(func(v float32) {
		if u.c != nil {
			u.c.SetVolume(float64(v))
		}
	})

	ghost := func(label string, fn func(Controller)) *iconbutton.Button {
		return iconbutton.New(nil, giode.Styles{
			Color:        iconColor,
			Width:        btnSize,
			Height:       btnSize,
			Padding:      giode.UniformInset((btnSize - btnIcon) / 2),
			BorderRadius: 8,
		}).Variant(giode.IconButtonGhost).Label(label).
			OnClick(func() { u.act(fn) })
	}
	u.stopBtn = ghost("Stop", Controller.Stop)
	u.nextBtn = ghost("Next", Controller.Next)
	u.muteBtn = ghost("Mute", u.toggleMute)
	u.settingsBtn = ghost("Settings", func(Controller) { u.OpenSettings() })
	u.fullscreenBtn = ghost("Fullscreen", Controller.ToggleFullscreen)
	u.playBtn = iconbutton.New(nil, giode.Styles{
		Background:   highlight,
		Width:        playSize,
		Height:       playSize,
		Padding:      giode.UniformInset((playSize - playIcon) / 2),
		BorderRadius: playSize / 2,
	}).Label("Play/Pause").
		OnClick(func() { u.act(Controller.TogglePause) })

	u.title = giode.TitleBar(windowControls{u}, "goplay", giode.Styles{
		Background: barColor,
		Color:      "#c9ccd2",
		Height:     titleH,
		FontSize:   13,
	})

	u.prefs = newPrefsView(u)
	u.settings = giode.Dialog(&u.settingsOpen, u.prefs.content, giode.Styles{
		Background:   "#16171b",
		BorderRadius: 12,
		Padding:      giode.UniformInset(0),
		BoxShadow:    giode.BoxShadow{Y: 12, Blur: 32, Color: "#000000e6"},
	})

	u.menu = u.buildMenu()
	return u
}

// windowControls adapts the current controller to the title bar, which
// is created before a controller exists.
type windowControls struct{ u *UI }

func (w windowControls) Minimize()       { w.u.act(Controller.Minimize) }
func (w windowControls) ToggleMaximize() { w.u.act(Controller.ToggleMaximize) }
func (w windowControls) Close()          { w.u.act(Controller.Close) }
func (w windowControls) Maximized() bool { return w.u.c != nil && w.u.c.Maximized() }
func (w windowControls) Focused() bool   { return w.u.c == nil || w.u.c.Focused() }

// Layout draws the UI into gtx. c drives the player.
//
// Outside fullscreen the title bar and control bar always show, in
// their own strips above and below the video (see TitleBarHeight and
// ControlHeight). In fullscreen there is no title bar, and the control
// bar overlays the video and shows only while the pointer is over the
// bottom strip, or while a slider is being dragged.
func (u *UI) Layout(gtx layout.Context, c Controller) {
	u.c = c
	elements.SetWindowSize(gtx.Constraints.Max)
	u.syncSliders(c)
	u.updateIcons(c)

	win := gtx.Constraints.Max
	barArea := image.Rect(0, win.Y-gtx.Dp(unit.Dp(controlH)), win.X, win.Y)
	hovered := u.barHover.Update(gtx.Source)
	shown := !u.Fullscreen || hovered || u.seek.Dragging() || u.vol.Dragging()
	u.controlRect = image.Rectangle{}
	if shown {
		u.controlRect = barArea
	}

	var children []giode.Element
	if !u.Fullscreen {
		children = append(children, u.title.Title(c.Title()))
	}
	// Spacer: pushes the control bar to the bottom edge.
	children = append(children, giode.Box(giode.Styles{Height: -1}))
	if shown {
		children = append(children, u.bar(c))
	}
	if msg := messageText(c); msg != "" {
		children = append(children, u.message(msg))
	}
	giode.Box(giode.Styles{
		Width:         -1,
		Height:        -1,
		FlexDirection: giode.FlexDirectionColumn,
	}, children...).Layout(gtx)

	if u.Fullscreen {
		// The hover area goes on top of the bar and passes events
		// through to it, so moving onto the bar's buttons still counts
		// as hovering the strip.
		area := clip.Rect(barArea).Push(gtx.Ops)
		pass := pointer.PassOp{}.Push(gtx.Ops)
		u.barHover.Add(gtx.Ops)
		pass.Pop()
		area.Pop()
	}

	u.settings.Layout(gtx)
	u.menu.Layout(gtx)
}

// OpenSettings shows the settings dialog.
func (u *UI) OpenSettings() {
	u.menu.Close()
	u.prefs.refresh()
	u.settingsOpen = true
}

// KeyPress handles an app-level shortcut. Escape closes, in order, the
// menu, the settings dialog and fullscreen, and only then quits.
func (u *UI) KeyPress(c Controller, k Key) {
	seekStep := c.Settings().Float(settings.KeySeekStep)
	volStep := c.Settings().Float(settings.KeyVolumeStep) / 100
	switch k {
	case KeyEscape:
		switch {
		case u.menu.Opened():
			u.menu.Close()
		case u.settingsOpen:
			u.settingsOpen = false
		case c.Fullscreen():
			c.ToggleFullscreen()
		default:
			c.Quit()
		}
	case KeyF:
		u.menu.Close()
		c.ToggleFullscreen()
	case KeyO:
		u.menu.Close()
		c.OpenFile()
	case KeyD:
		u.menu.Close()
		c.OpenFolder()
	case KeySpace:
		u.menu.Close()
		c.TogglePause()
	case KeyS:
		u.menu.Close()
		c.Stop()
	case KeyN:
		u.menu.Close()
		c.Next()
	case KeyLeft:
		if c.HasMedia() {
			u.menu.Close()
			c.SeekRelative(-seekStep)
		}
	case KeyRight:
		if c.HasMedia() {
			u.menu.Close()
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

// OpenMenuAt opens the context menu at x, y, in window pixel
// coordinates (the space pointer events and the layout constraints are
// in).
func (u *UI) OpenMenuAt(x, y float32) {
	if u.settingsOpen {
		return
	}
	// Rebuild the menu so entries that depend on the current state
	// (fullscreen, say) are up to date.
	u.menu = u.buildMenu()
	u.menu.OpenAt(f32.Pt(x, y))
}

// RectContains reports whether the point, in frame pixels (the space
// pointer events are in), is over the shown control bar, the open
// context menu or the settings dialog. The app uses it to gate frameless
// window dragging.
func (u *UI) RectContains(x, y float32) bool {
	if u.menu.Opened() || u.settingsOpen {
		return true
	}
	p := image.Pt(int(x), int(y))
	return !u.controlRect.Empty() && p.In(u.controlRect)
}

// toggleMute silences the player, or restores the volume it had before
// it was muted.
func (u *UI) toggleMute(c Controller) {
	if v := c.Volume(); v > 0 {
		u.unmuteVol = v
		c.SetVolume(0)
		return
	}
	if u.unmuteVol <= 0 {
		u.unmuteVol = 1
	}
	c.SetVolume(u.unmuteVol)
}

// act closes the menu and runs fn against the current controller, if
// any.
func (u *UI) act(fn func(Controller)) {
	if u.c == nil {
		return
	}
	u.menu.Close()
	fn(u.c)
}

// messageText is the status message to show, or "" for none.
func messageText(c Controller) string {
	msg := c.Message()
	if msg == "" && !c.HasMedia() {
		msg = "Open a file (O) or a folder (D) to start playing."
	}
	return msg
}

// message draws the top-left status pill over the video.
func (u *UI) message(msg string) giode.Element {
	top := pad
	if !u.Fullscreen {
		top += titleH
	}
	return giode.Box(giode.Styles{
		Position:     giode.PositionAbsolute,
		Top:          properties.Top(top),
		Left:         pad,
		MaxWidth:     properties.MaxWidth(560),
		Background:   "#14171ed9",
		BorderWidth:  1,
		BorderColor:  "#ffffff1f",
		Padding:      giode.SymmetricInset(14, 8),
		BorderRadius: 10,
	}, giode.Text(msg, giode.Styles{Color: fgColor, FontSize: 14}))
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
