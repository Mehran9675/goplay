package ui

import (
	"strconv"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/icon"
	"github.com/mehran9675/giode/components/menu"
)

// Choices offered by the Video > Aspect Ratio and Playback > Speed
// submenus. An empty ratio means the file's own.
var (
	aspectRatios = []struct{ label, ratio string }{
		{"Auto", ""}, {"16:9", "16:9"}, {"4:3", "4:3"}, {"21:9", "2.33:1"}, {"1:1", "1:1"},
	}
	speeds = []float64{0.5, 0.75, 1, 1.25, 1.5, 2}
)

// noIcon fills the icon column of unchecked rows so their labels line up
// with checked ones.
var noIcon = icon.Path{Data: "M0 0"}

// newMenu returns a menu (or submenu) in the player's style: a dark,
// raised panel with a soft drop shadow instead of a border.
func newMenu() *menu.Menu {
	return giode.Menu(giode.Styles{
		Background:   menuBg,
		Color:        menuFg,
		BorderRadius: 8,
		Padding:      giode.UniformInset(6),
		BoxShadow:    giode.BoxShadow{Y: 8, Blur: 24, Color: "#000000d9"},
	}).
		ItemPadding(giode.SymmetricInset(18, 9)).
		HoverColor(menuHover).
		SeparatorColor(menuSeparator).
		DisabledColor(menuDisabled)
}

// checkItem appends a row showing a checkmark when on. Checkmarks are
// the menu's only icons: they mark the current track or setting.
func checkItem(m *menu.Menu, label string, on bool, action func()) {
	var ic icon.Source = noIcon
	if on {
		ic = iconSrc("check")
	}
	m.IconItem(ic, label, action)
}

// buildMenu creates the right-click context menu. Entries that depend on
// the current state (tracks, fullscreen, settings) are chosen here, so
// OpenMenuAt rebuilds the menu every time it opens.
func (u *UI) buildMenu() *menu.Menu {
	m := newMenu().
		Item("Open File…", func() { u.act(Controller.OpenFile) }).
		Item("Open Folder…", func() { u.act(Controller.OpenFolder) }).
		Separator().
		SubMenu("Subtitles", u.subtitleMenu()).
		SubMenu("Video", u.videoMenu()).
		SubMenu("Audio", u.audioMenu()).
		SubMenu("Playback", u.playbackMenu()).
		Separator().
		Item("Settings…", u.OpenSettings).
		Separator()
	if u.Fullscreen {
		m.Item("Exit Fullscreen", func() { u.act(Controller.ToggleFullscreen) })
	} else {
		m.Item("Fullscreen", func() { u.act(Controller.ToggleFullscreen) })
	}
	return m.Separator().
		Item("Quit", func() { u.act(Controller.Quit) })
}

// tracks lists the current tracks of kind, or none before a controller
// is attached.
func (u *UI) tracks(kind TrackKind) []Track {
	if u.c == nil {
		return nil
	}
	return u.c.Tracks(kind)
}

// trackItems appends one checkable row per track of kind, or a disabled
// placeholder when there are none.
func (u *UI) trackItems(m *menu.Menu, kind TrackKind, none string) {
	tracks := u.tracks(kind)
	if len(tracks) == 0 {
		m.DisabledItem(none)
		return
	}
	for _, t := range tracks {
		id := t.ID
		checkItem(m, t.Title, t.Selected, func() {
			u.act(func(c Controller) { c.SelectTrack(kind, id) })
		})
	}
}

func (u *UI) subtitleMenu() *menu.Menu {
	m := newMenu().
		IconItem(noIcon, "Load Subtitle File…", func() { u.act(Controller.LoadSubtitle) }).
		Separator()
	tracks := u.tracks(TrackSubtitle)
	off := true
	for _, t := range tracks {
		off = off && !t.Selected
	}
	checkItem(m, "Off", off, func() {
		u.act(func(c Controller) { c.SelectTrack(TrackSubtitle, 0) })
	})
	for _, t := range tracks {
		id := t.ID
		checkItem(m, t.Title, t.Selected, func() {
			u.act(func(c Controller) { c.SelectTrack(TrackSubtitle, id) })
		})
	}
	return m
}

func (u *UI) videoMenu() *menu.Menu {
	m := newMenu()
	u.trackItems(m, TrackVideo, "No video tracks")
	aspect := newMenu()
	current := ""
	if u.c != nil {
		current = u.c.AspectRatio()
	}
	for _, a := range aspectRatios {
		ratio := a.ratio
		checkItem(aspect, a.label, ratio == current, func() {
			u.act(func(c Controller) { c.SetAspectRatio(ratio) })
		})
	}
	m.Separator()
	// Line up with the track rows' checkmark column; with no tracks the
	// placeholder above has none.
	if len(u.tracks(TrackVideo)) == 0 {
		return m.SubMenu("Aspect Ratio", aspect)
	}
	return m.IconSubMenu(noIcon, "Aspect Ratio", aspect)
}

func (u *UI) audioMenu() *menu.Menu {
	m := newMenu()
	u.trackItems(m, TrackAudio, "No audio tracks")
	return m
}

func (u *UI) playbackMenu() *menu.Menu {
	speed, loop := 1.0, false
	if u.c != nil {
		speed, loop = u.c.Speed(), u.c.Loop()
	}
	speedMenu := newMenu()
	for _, s := range speeds {
		label := strconv.FormatFloat(s, 'f', -1, 64) + "×"
		if s == 1 {
			label = "Normal"
		}
		checkItem(speedMenu, label, s == speed, func() {
			u.act(func(c Controller) { c.SetSpeed(s) })
		})
	}
	m := newMenu().IconSubMenu(noIcon, "Speed", speedMenu)
	checkItem(m, "Loop", loop, func() { u.act(Controller.ToggleLoop) })
	return m
}
