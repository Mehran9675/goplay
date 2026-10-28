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

// check appends a row with a checkmark when on; unchecked rows keep the
// column so labels line up.
func check(m *menu.Menu, label string, on bool, fn func()) {
	icon := "blank"
	if on {
		icon = "check"
	}
	m.IconItem(src(icon), label, fn)
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
		m.DisabledItem(act.Title)
	case act.Checked != nil:
		check(m, act.Title, act.IsChecked(), act.Run)
	default:
		m.Item(act.Title, act.Run)
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
		u.items(m, "open", "open_folder", "open_url", "paste")
		m.SubMenu("Open Recent", u.recentMenu())
		m.Separator()
		u.items(m, "settings", "shortcuts", "palette", "-", "about", "quit")
		return m
	}
	u.items(m, "play_pause", "stop")
	m.Separator()
	m.SubMenu("Playback", u.playbackMenu())
	m.SubMenu("Video", u.videoMenu())
	m.SubMenu("Audio", u.audioMenu())
	m.SubMenu("Subtitles", u.subtitleMenu())
	m.SubMenu("3D & 360°", u.stereoMenu())
	m.Separator()
	m.SubMenu("Open", u.openMenu())
	u.items(m, "playlist", "cast", "-", "fullscreen", "pip", "always_on_top", "-", "settings", "palette")
	return m
}

func (u *ui) openMenu() *menu.Menu {
	m := newMenu()
	u.items(m, "open", "open_folder", "open_url", "paste", "-", "add_files", "load_subtitle", "load_audio")
	m.Separator()
	m.SubMenu("Recent", u.recentMenu())
	u.items(m, "-", "reveal", "copy_path", "close_media")
	return m
}

func (u *ui) recentMenu() *menu.Menu {
	a := u.a
	m := newMenu()
	items := a.history.Items()
	if len(items) == 0 {
		m.DisabledItem("Nothing yet")
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
	m.Item("Clear Recent", func() { a.history.Clear(); a.history.Save() })
	return m
}

func (u *ui) playbackMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	m.SubMenu("Speed", u.speedMenu())
	if len(st.Chapters) > 0 {
		m.SubMenu("Chapters", u.chapterMenu())
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
			title = fmt.Sprintf("Chapter %d", i+1)
		}
		check(m, player.FormatTime(c.Time)+"   "+ellipsize(title, 40), i == st.Chapter, func() { a.player.SetChapter(i) })
	}
	return m
}

func (u *ui) speedMenu() *menu.Menu {
	a := u.a
	cur := a.player.State.Speed
	m := newMenu()
	for _, s := range speedSteps {
		label := speedLabel(s)
		if s == 1 {
			label = "Normal"
		}
		check(m, label, math.Abs(s-cur) < 1e-6, func() { a.setSpeed(s) })
	}
	return m
}

// trackMenu lists one kind's tracks with "Off" for subtitles.
func (u *ui) trackMenu(m *menu.Menu, k player.TrackKind) {
	a := u.a
	st := &a.player.State
	tracks := st.TracksOf(k)
	sel, hasSel := st.SelectedTrack(k)
	if k == player.Subtitle {
		check(m, "Off", !hasSel, func() { a.player.SelectTrack(k, 0) })
	}
	if len(tracks) == 0 && k != player.Subtitle {
		m.DisabledItem("No tracks")
		return
	}
	for _, t := range tracks {
		id := t.ID
		check(m, ellipsize(t.Name(), 52), hasSel && t.ID == sel.ID, func() { a.player.SelectTrack(k, id) })
	}
}

func (u *ui) subtitleMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	u.trackMenu(m, player.Subtitle)
	if subs := st.TracksOf(player.Subtitle); len(subs) > 1 {
		sec := newMenu()
		cur, has := st.SecondarySubtitle()
		check(sec, "Off", !has, func() { a.player.SelectSecondarySubtitle(0) })
		for _, t := range subs {
			id := t.ID
			check(sec, ellipsize(t.Name(), 52), has && t.ID == cur.ID, func() { a.player.SelectSecondarySubtitle(id) })
		}
		m.Separator()
		m.SubMenu("Second Subtitle", sec)
	}
	m.Separator()
	u.items(m, "load_subtitle", "-", "sub_delay_down", "sub_delay_up", "sub_smaller", "sub_bigger")
	if st.SubDelay != 0 {
		m.Item(fmt.Sprintf("Reset Delay (%+.1f s)", st.SubDelay), func() { a.player.SetSubDelay(0); a.osd("Subtitle delay 0") })
	}
	m.Separator()
	m.Item("Subtitle Settings…", func() { u.showSettings("Subtitles") })
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
		m.Item(fmt.Sprintf("Reset Delay (%+.1f s)", st.AudioDelay), func() { a.player.SetAudioDelay(0); a.osd("Audio delay 0") })
	}
	m.Separator()
	eq := newMenu()
	d := settings.Lookup("equalizer")
	for _, o := range d.Options {
		v := o.Value
		check(eq, o.Label, a.settings.Get("equalizer") == v, func() { a.setSetting("equalizer", v) })
	}
	m.SubMenu("Equalizer", eq)
	check(m, "Even Out Loudness", a.settings.Bool("normalize"), func() { a.toggleSetting("normalize", "Even out loudness") })
	check(m, "Night Mode", a.settings.Bool("night_mode"), func() { a.toggleSetting("night_mode", "Night mode") })
	m.Separator()
	m.Item("Audio Settings…", func() { u.showSettings("Audio") })
	return m
}

func (u *ui) videoMenu() *menu.Menu {
	a := u.a
	st := &a.player.State
	m := newMenu()
	if len(st.TracksOf(player.Video)) > 1 {
		tm := newMenu()
		u.trackMenu(tm, player.Video)
		m.SubMenu("Video Track", tm)
	}
	fit := newMenu()
	for _, o := range settings.Lookup("fit").Options {
		v := o.Value
		check(fit, o.Label, a.settings.Get("fit") == v, func() { a.setSetting("fit", v) })
	}
	m.SubMenu("Picture Size", fit)
	aspect := newMenu()
	cur, _ := a.player.Get("video-aspect-override")
	for _, r := range []struct{ label, value string }{
		{"Original", ""}, {"16:9", "16:9"}, {"4:3", "4:3"}, {"21:9", "2.35:1"}, {"1.85:1", "1.85:1"}, {"1:1", "1:1"},
	} {
		v := r.value
		on := (v == "" && (cur == "no" || cur == "-1" || cur == "")) || aspectMatches(cur, v)
		check(aspect, r.label, on, func() { a.player.SetAspect(v) })
	}
	m.SubMenu("Aspect Ratio", aspect)
	u.items(m, "-", "zoom_in", "zoom_out", "zoom_reset", "rotate")
	flipH, flipV := u.flipH, u.flipV
	check(m, "Mirror", flipH, func() { u.flipH = !u.flipH; a.player.SetFlip(u.flipH, u.flipV) })
	check(m, "Flip Upside Down", flipV, func() { u.flipV = !u.flipV; a.player.SetFlip(u.flipH, u.flipV) })
	u.items(m, "-", "deinterlace", "-", "screenshot", "screenshot_clean", "-", "info")
	m.Separator()
	m.Item("Video Settings…", func() { u.showSettings("Video") })
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
	check(m, "Normal (2D)", s.layout == player.Layout2D && s.projection == player.ProjectionFlat, func() {
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
		check(in, l.label, s.layout == l.layout, func() { a.setStereoLayout(l.layout) })
	}
	m.SubMenu("3D Video Format", in)

	out := newMenu()
	for _, o := range settings.StereoViews {
		v := player.StereoView(o.Value)
		check(out, o.Label, s.layout != player.Layout2D && s.view == v, func() { a.setStereoView(v) })
	}
	m.SubMenu("Show 3D As", out)
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
		check(vr, p.label, s.projection == p.proj && s.vrStereo == p.stereo, func() { a.setProjection(p.proj, p.stereo) })
	}
	m.SubMenu("360° and VR", vr)
	u.items(m, "vr_reset")
	m.Separator()
	m.Item("3D Settings…", func() { u.showSettings("3D & VR") })
	return m
}

// moreMenu is the bar's overflow: the less frequent controls.
func (u *ui) moreMenu() *menu.Menu {
	m := newMenu()
	m.SubMenu("Video", u.videoMenu())
	m.SubMenu("3D & 360°", u.stereoMenu())
	m.SubMenu("Playback", u.playbackMenu())
	m.Separator()
	u.items(m, "screenshot", "info", "pip", "always_on_top", "-", "settings")
	return m
}

// openAppMenu is the title bar's menu: the whole app, for anyone who
// doesn't know about right-clicking.
func (u *ui) openAppMenu() {
	m := newMenu()
	m.SubMenu("Open", u.openMenu())
	if !u.a.player.State.Idle {
		m.SubMenu("Playback", u.playbackMenu())
		m.SubMenu("Video", u.videoMenu())
		m.SubMenu("Audio", u.audioMenu())
		m.SubMenu("Subtitles", u.subtitleMenu())
		m.SubMenu("3D & 360°", u.stereoMenu())
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
