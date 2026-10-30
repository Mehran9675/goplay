package app

import (
	"fmt"
	"math"

	"github.com/mehran9675/giode/components/menu"

	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/giode/styles/properties"
	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

// Menus are rebuilt each time they open, so they show the current
// tracks, chapters and settings.

func newMenu() *menu.Menu {
	return menu.New(styles.Styles{
		Background:   colSurface,
		Color:        colText,
		BorderRadius: 10,
		Padding:      properties.UniformInset(6),
		BoxShadow:    properties.BoxShadow{Y: 10, Blur: 28, Color: "#000000d0"},
	}).
		ItemPadding(properties.SymmetricInset(14, 8)).
		HoverColor(colSurface3).
		SeparatorColor(colBorder).
		DisabledColor(colFaint)
}

// check appends an on/off row with a checkmark while on reports true,
// and radio one of a group of choices with a dot while it's the one
// chosen. Both are read every frame, so an open menu follows changes.
func check(m *menu.Menu, label string, on func() bool, fn func()) {
	m.CheckItem(label, on, fn)
}

func radio(m *menu.Menu, label string, on func() bool, fn func()) {
	m.RadioItem(label, on, fn)
}

// item appends a command by its ID: its title, greyed out when it can't
// run, checked when it's an active toggle.
func (u *ui) item(m *menu.Menu, id string) {
	act := u.a.actions.Get(id)
	if act == nil {
		return
	}
	switch {
	case !act.IsEnabled():
		m.DisabledItem(tr(act.Title))
	case act.Checked != nil:
		check(m, tr(act.Title), act.IsChecked, act.Run)
	default:
		m.Item(tr(act.Title), act.Run)
	}
}

func (u *ui) items(m *menu.Menu, ids ...string) {
	for _, id := range ids {
		if id == "-" {
			m.Separator()
			continue
		}
		u.item(m, id)
	}
}

// contextMenu is the right-click menu: everything, grouped.
func (u *ui) contextMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	if st.Idle {
		u.items(m, "open", "open_folder", "open_url", "network_media", "paste")
		m.SubMenu(tr("Open Recent"), u.recentMenu())
		m.Separator()
		u.items(m, "settings", "shortcuts", "palette", "-", "about", "quit")
		return m
	}
	u.items(m, "play_pause", "stop")
	m.Separator()
	m.SubMenu(tr("Playback"), u.playbackMenu())
	m.SubMenu(tr("Video"), u.videoMenu())
	m.SubMenu(tr("Audio"), u.audioMenu())
	m.SubMenu(tr("Subtitles"), u.subtitleMenu())
	m.SubMenu(tr("3D & 360°"), u.stereoMenu())
	m.Separator()
	m.SubMenu(tr("Open"), u.openMenu())
	u.items(m, "playlist", "cast", "-", "fullscreen", "pip", "always_on_top", "-", "settings", "palette")
	return m
}

func (u *ui) openMenu() *menu.Menu {
	m := newMenu()
	u.items(m, "open", "open_folder", "open_url", "network_media", "paste", "-", "add_files", "load_subtitle", "load_audio")
	m.Separator()
	m.SubMenu(tr("Recent"), u.recentMenu())
	u.items(m, "-", "reveal", "copy_path", "close_media")
	return m
}

func (u *ui) recentMenu() *menu.Menu {
	a := u.a
	m := newMenu()
	items := a.history.Items()
	if len(items) == 0 {
		m.DisabledItem(tr("Nothing yet"))
		return m
	}
	for i, r := range items {
		if i == 12 {
			break
		}
		path := r.Path
		m.Item(ellipsize(r.Name(), 48), func() { a.openItems([]string{path}, false) })
	}
	m.Separator()
	m.Item(tr("Clear Recent"), func() { a.history.Clear(); a.history.Save() })
	return m
}

func (u *ui) playbackMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	m.SubMenu(tr("Speed"), u.speedMenu())
	if len(st.Chapters) > 0 {
		m.SubMenu(tr("Chapters"), u.chapterMenu())
	}
	u.items(m, "-", "previous", "next", "chapter_previous", "chapter_next", "-",
		"jump_to", "restart", "frame_back", "frame_forward", "-",
		"ab_loop", "loop_file", "loop_playlist", "shuffle")
	return m
}

func (u *ui) chapterMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	for i, c := range st.Chapters {
		i := i
		title := c.Title
		if title == "" {
			title = tr("Chapter {number}", "number", i+1)
		}
		radio(m, player.FormatTime(c.Time)+"   "+ellipsize(title, 40), func() bool { return i == st.Chapter }, func() { a.player.SetChapter(i) })
	}
	return m
}

func (u *ui) speedMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	for _, s := range speedSteps {
		label := speedLabel(s)
		if s == 1 {
			label = tr("Normal")
		}
		radio(m, label, func() bool { return math.Abs(s-st.Speed) < 1e-6 }, func() { a.setSpeed(s) })
	}
	return m
}

// trackMenu lists one kind's tracks with "Off" for subtitles.
func (u *ui) trackMenu(m *menu.Menu, k player.TrackKind) {
	a := u.a
	st := &a.player.State
	tracks := st.TracksOf(k)
	selected := func(id int) func() bool {
		return func() bool {
			sel, ok := st.SelectedTrack(k)
			return ok == (id != 0) && (!ok || sel.ID == id)
		}
	}
	if k == player.Subtitle {
		radio(m, tr("Off"), selected(0), func() { a.player.SelectTrack(k, 0) })
	}
	if len(tracks) == 0 && k != player.Subtitle {
		m.DisabledItem(tr("No tracks"))
		return
	}
	for _, t := range tracks {
		id := t.ID
		radio(m, ellipsize(t.Name(), 52), selected(id), func() { a.player.SelectTrack(k, id) })
	}
}

func (u *ui) subtitleMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	u.trackMenu(m, player.Subtitle)
	if subs := st.TracksOf(player.Subtitle); len(subs) > 1 {
		sec := newMenu()
		second := func(id int) func() bool {
			return func() bool {
				cur, ok := st.SecondarySubtitle()
				return ok == (id != 0) && (!ok || cur.ID == id)
			}
		}
		radio(sec, tr("Off"), second(0), func() { a.player.SelectSecondarySubtitle(0) })
		for _, t := range subs {
			id := t.ID
			radio(sec, ellipsize(t.Name(), 52), second(id), func() { a.player.SelectSecondarySubtitle(id) })
		}
		m.Separator()
		m.SubMenu(tr("Second Subtitle"), sec)
	}
	m.Separator()
	u.items(m, "load_subtitle", "-", "sub_delay_down", "sub_delay_up", "sub_smaller", "sub_bigger")
	if st.SubDelay != 0 {
		m.Item(tr("Reset Delay ({seconds} s)", "seconds", fmt.Sprintf("%+.1f", st.SubDelay)), func() { a.player.SetSubDelay(0); a.osd(tr("Subtitle delay 0")) })
	}
	m.Separator()
	m.Item(tr("Subtitle Settings…"), func() { u.showSettings("Subtitles") })
	return m
}

func (u *ui) audioMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	u.trackMenu(m, player.Audio)
	m.Separator()
	u.items(m, "load_audio", "-", "audio_delay_down", "audio_delay_up")
	if st.AudioDelay != 0 {
		m.Item(tr("Reset Delay ({seconds} s)", "seconds", fmt.Sprintf("%+.1f", st.AudioDelay)), func() { a.player.SetAudioDelay(0); a.osd(tr("Audio delay 0")) })
	}
	m.Separator()
	eq := newMenu()
	d := settings.Lookup("equalizer")
	for _, o := range d.Options {
		v := o.Value
		radio(eq, tr(o.Label), func() bool { return a.settings.Get("equalizer") == v }, func() { a.setSetting("equalizer", v) })
	}
	m.SubMenu(tr("Equalizer"), eq)
	check(m, tr("Even Out Loudness"), func() bool { return a.settings.Bool("normalize") }, func() { a.toggleSetting("normalize", "Even out loudness") })
	check(m, tr("Night Mode"), func() bool { return a.settings.Bool("night_mode") }, func() { a.toggleSetting("night_mode", "Night mode") })
	m.Separator()
	m.Item(tr("Audio Settings…"), func() { u.showSettings("Audio") })
	return m
}

func (u *ui) videoMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	if len(st.TracksOf(player.Video)) > 1 {
		tm := newMenu()
		u.trackMenu(tm, player.Video)
		m.SubMenu(tr("Video Track"), tm)
	}
	fit := newMenu()
	for _, o := range settings.Lookup("fit").Options {
		v := o.Value
		radio(fit, tr(o.Label), func() bool { return a.settings.Get("fit") == v }, func() { a.setSetting("fit", v) })
	}
	m.SubMenu(tr("Picture Size"), fit)
	aspect := newMenu()
	aspectOn := func(v string) func() bool {
		return func() bool {
			cur, _ := a.player.Get("video-aspect-override")
			return (v == "" && (cur == "no" || cur == "-1" || cur == "")) || aspectMatches(cur, v)
		}
	}
	for _, r := range []struct{ label, value string }{
		{"Original", ""}, {"16:9", "16:9"}, {"4:3", "4:3"}, {"21:9", "2.35:1"}, {"1.85:1", "1.85:1"}, {"1:1", "1:1"},
	} {
		v := r.value
		radio(aspect, tr(r.label), aspectOn(v), func() { a.player.SetAspect(v) })
	}
	m.SubMenu(tr("Aspect Ratio"), aspect)
	u.items(m, "-", "zoom_in", "zoom_out", "zoom_reset", "rotate")
	check(m, tr("Mirror"), func() bool { return u.flipH }, func() { u.flipH = !u.flipH; a.player.SetFlip(u.flipH, u.flipV) })
	check(m, tr("Flip Upside Down"), func() bool { return u.flipV }, func() { u.flipV = !u.flipV; a.player.SetFlip(u.flipH, u.flipV) })
	u.items(m, "-", "deinterlace", "-", "screenshot", "screenshot_clean", "-", "info")
	m.Separator()
	m.Item(tr("Video Settings…"), func() { u.showSettings("Video") })
	return m
}

func aspectMatches(cur, want string) bool {
	if want == "" {
		return false
	}
	var cw, ch, ww, wh float64
	if _, err := fmt.Sscanf(want, "%g:%g", &ww, &wh); err != nil || wh == 0 {
		return false
	}
	if _, err := fmt.Sscanf(cur, "%g:%g", &cw, &ch); err == nil && ch != 0 {
		return math.Abs(cw/ch-ww/wh) < 0.01
	}
	var f float64
	if _, err := fmt.Sscanf(cur, "%g", &f); err == nil {
		return math.Abs(f-ww/wh) < 0.01
	}
	return false
}

// stereoMenu chooses how 3D and 360° video is treated.
func (u *ui) stereoMenu() *menu.Menu {
	a := u.a
	s := &a.stereo
	m := newMenu()
	radio(m, tr("Normal (2D)"), func() bool { return s.layout == player.Layout2D && s.projection == player.ProjectionFlat }, func() {
		a.setProjection(player.ProjectionFlat, "")
		a.setStereoLayout(player.Layout2D)
	})

	in := newMenu()
	for _, l := range []struct {
		label  string
		layout player.StereoLayout
	}{
		{"Side by side (half width)", player.LayoutSBSHalf},
		{"Side by side (full width)", player.LayoutSBSFull},
		{"Side by side, right eye first", player.LayoutSBSHalfRight},
		{"Top and bottom (half height)", player.LayoutTBHalf},
		{"Top and bottom (full height)", player.LayoutTBFull},
		{"Top and bottom, right eye first", player.LayoutTBHalfRight},
	} {
		l := l
		radio(in, tr(l.label), func() bool { return s.layout == l.layout }, func() { a.setStereoLayout(l.layout) })
	}
	m.SubMenu(tr("3D Video Format"), in)

	out := newMenu()
	for _, o := range settings.StereoViews {
		v := player.StereoView(o.Value)
		radio(out, tr(o.Label), func() bool { return s.layout != player.Layout2D && s.view == v }, func() { a.setStereoView(v) })
	}
	m.SubMenu(tr("Show 3D As"), out)
	m.Separator()

	vr := newMenu()
	for _, p := range []struct {
		label  string
		proj   player.Projection
		stereo player.VRStereo
	}{
		{"360° video", player.ProjectionEquirect, player.VRMono},
		{"360° 3D, top and bottom", player.ProjectionEquirect, player.VRTopBottom},
		{"360° 3D, side by side", player.ProjectionEquirect, player.VRSideBySide},
		{"180° 3D (VR180)", player.ProjectionHalf, player.VRSideBySide},
		{"180° video", player.ProjectionHalf, player.VRMono},
		{"Fisheye 180° 3D", player.ProjectionFisheye, player.VRSideBySide},
		{"YouTube cubemap (EAC)", player.ProjectionEAC, player.VRMono},
	} {
		p := p
		radio(vr, tr(p.label), func() bool { return s.projection == p.proj && s.vrStereo == p.stereo }, func() { a.setProjection(p.proj, p.stereo) })
	}
	m.SubMenu(tr("360° and VR"), vr)
	u.items(m, "vr_reset")
	m.Separator()
	m.Item(tr("3D Settings…"), func() { u.showSettings("3D & VR") })
	return m
}

// moreMenu is the bar's overflow: the less frequent controls.
func (u *ui) moreMenu() *menu.Menu {
	m := newMenu()
	m.SubMenu(tr("Video"), u.videoMenu())
	m.SubMenu(tr("3D & 360°"), u.stereoMenu())
	m.SubMenu(tr("Playback"), u.playbackMenu())
	m.Separator()
	u.items(m, "screenshot", "info", "pip", "always_on_top", "-", "settings")
	return m
}

// openAppMenu is the title bar's menu: the whole app, for anyone who
// doesn't know about right-clicking.
func (u *ui) openAppMenu() {
	m := newMenu()
	m.SubMenu(tr("Open"), u.openMenu())
	if !u.a.player.State.Idle {
		m.SubMenu(tr("Playback"), u.playbackMenu())
		m.SubMenu(tr("Video"), u.videoMenu())
		m.SubMenu(tr("Audio"), u.audioMenu())
		m.SubMenu(tr("Subtitles"), u.subtitleMenu())
		m.SubMenu(tr("3D & 360°"), u.stereoMenu())
	}
	m.Separator()
	u.items(m, "playlist", "cast", "-", "settings", "shortcuts", "palette", "-", "log", "config_folder", "about", "-", "quit")
	u.openMenuOver(m, u.appMenuAnchor)
}

func ellipsize(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
