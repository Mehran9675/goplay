package app

import (
	"path/filepath"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/giode/components/progress"
	"github.com/mehran9675/giode/components/spinner"
	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/goplay/internal/cast"
	"github.com/mehran9675/goplay/internal/library"
	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

// videoArea is the picture's rectangle: it takes clicks (pause,
// fullscreen, the context menu), the wheel and 360° dragging, and shows
// what goes over the video.
type videoArea struct {
	u     *ui
	press *elements.Pressable

	wheelTag struct{}
	vrDrag   gesture.Drag
	vrLast   f32.Point

	openBtn, urlBtn, folderBtn, netBtn *button.Button
	// againBtn and openNextBtn are the finished screen's: there the
	// replay is the main action and opening another file comes second.
	againBtn, openNextBtn *button.Button
}

func newVideoArea(u *ui) *videoArea {
	v := &videoArea{u: u}
	a := u.a
	v.press = elements.NewPressable(v.content).Label("Video").Focusable(false).
		Styles(styles.Styles{Width: -1, Height: -1, Cursor: properties.CursorDefault}).
		OnClick(func(e elements.ClickEvent) {
			st := &a.player.State
			// OnClick sees every button; the right one opens the menu.
			if e.Button != elements.MousePrimary || st.Idle || a.vrActive() || u.modalOpen() {
				return
			}
			switch e.Clicks {
			case 1:
				if a.settings.Bool(settings.KeyClickToPause) {
					a.togglePause()
				}
			case 2:
				// The first click of the pair paused; undo it.
				if a.settings.Bool(settings.KeyClickToPause) {
					a.togglePause()
					u.flash = ""
				}
				a.setFullscreen(!a.g.Fullscreen())
			}
		}).
		OnSecondaryClick(func(e elements.ClickEvent) {
			u.openMenuAt(u.contextMenu(), u.videoRect.Min.Add(e.Position))
		})

	primary := styles.Styles{
		Background: colHighlight, Color: colOnLight, FontSize: 14, FontWeight: properties.FontWeightSemiBold,
		Padding: properties.SymmetricInset(18, 11), BorderRadius: 22,
	}
	secondary := styles.Styles{
		Background: colSurface3, Color: colText, FontSize: 14, FontWeight: properties.FontWeightMedium,
		Padding: properties.SymmetricInset(18, 11), BorderRadius: 22,
	}
	v.openBtn = button.New("Open File", primary).Icon(ic("file", 18, colOnLight)).OnClick(a.openFileDialog)
	v.urlBtn = button.New("Open URL", secondary).Icon(ic("link", 18, colText)).OnClick(u.showOpenURL)
	v.folderBtn = button.New("Open Folder", secondary).Icon(ic("folder", 18, colText)).OnClick(a.openFolderDialog)
	v.netBtn = button.New("Network", secondary).Icon(ic("tv", 18, colText)).OnClick(func() { u.dialogs.net.show() })
	v.againBtn = button.New("Play Again", primary).Icon(ic("replay", 18, colOnLight)).OnClick(a.playAgain)
	v.openNextBtn = button.New("Open File", secondary).Icon(ic("file", 18, colText)).OnClick(a.openFileDialog)
	return v
}

func (v *videoArea) Layout(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	dims := v.press.Layout(gtx)
	// The wheel and 360° dragging sit above the click area and pass
	// events through to it.
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &v.wheelTag)
	v.handleWheel(gtx)
	if v.u.a.vrActive() {
		v.vrDrag.Add(gtx.Ops)
		pointer.CursorGrab.Add(gtx.Ops)
		v.handleVRDrag(gtx)
	}
	pass.Pop()
	area.Pop()
	return dims
}

func (v *videoArea) handleWheel(gtx layout.Context) {
	a := v.u.a
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &v.wheelTag, Kinds: pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6}})
		if !ok {
			return
		}
		e, ok := ev.(pointer.Event)
		if !ok || e.Scroll.Y == 0 || a.player.State.Idle {
			continue
		}
		steps := e.Scroll.Y / float32(gtx.Dp(48))
		switch {
		case a.vrActive():
			a.vrZoom(steps)
		case a.settings.Get(settings.KeyWheel) == "volume":
			a.addVolume(-float64(steps) * a.settings.Float(settings.KeyVolumeStep))
		case a.settings.Get(settings.KeyWheel) == "seek":
			a.seekBy(-float64(steps) * a.seekStep(false))
		}
	}
}

func (v *videoArea) handleVRDrag(gtx layout.Context) {
	a := v.u.a
	for {
		e, ok := v.vrDrag.Update(gtx.Metric, gtx.Source, gesture.Both)
		if !ok {
			return
		}
		switch e.Kind {
		case pointer.Press:
			v.vrLast = e.Position
			a.stereo.dragging = true
		case pointer.Drag:
			d := e.Position.Sub(v.vrLast)
			v.vrLast = e.Position
			a.vrDrag(d.X, d.Y)
		case pointer.Release, pointer.Cancel:
			a.vrDragEnd()
		}
	}
}

// content is everything drawn over the picture.
func (v *videoArea) content(elements.PressState) elements.Element {
	u, a := v.u, v.u.a
	st := &a.player.State
	var layers []elements.Element

	switch {
	case a.casting():
		layers = append(layers, v.castScreen())
	case st.Idle:
		layers = append(layers, v.welcome(false))
	case a.finished():
		layers = append(layers, v.welcome(true))
	case !st.HasVideo() && !st.Loading:
		layers = append(layers, v.audioOnly())
	}
	if (st.Loading || st.Buffering) && !st.Idle && !a.casting() {
		layers = append(layers, centered(elements.Box(styles.Styles{
			Background: "#000000a0", BorderRadius: 28, Padding: properties.UniformInset(14),
		}, spinner.Spinner(styles.Styles{Width: 28, Height: 28, Color: colText}))))
	}
	if d, ok := a.g.FileDrag(); ok && d.Kind != giode.FileDragLeave {
		layers = append(layers, v.dropTarget(!st.Idle))
	}
	if u.flash != "" {
		if age := time.Since(u.flashAt); age < flashDuration {
			f := float32(age) / float32(flashDuration)
			layers = append(layers, centered(elements.Box(styles.Styles{
				Background: "#00000080", BorderRadius: 40, Padding: properties.UniformInset(18),
				Opacity:   properties.Opacity(max(0.01, 1-f)),
				Transform: properties.Transform{ScaleX: 1 + f*0.3, ScaleY: 1 + f*0.3},
			}, ic(u.flash, 40, colHighlight))))
		} else {
			u.flash = ""
		}
	}
	if u.osdText != "" {
		if time.Now().Before(u.osdUntil) {
			layers = append(layers, v.osd())
		} else {
			u.osdText = ""
		}
	}
	if u.infoOpen && !st.Idle {
		layers = append(layers, v.info())
	}
	return elements.Stack(styles.Styles{Width: -1, Height: -1}, layers...)
}

// centered places el in the middle of the video area.
func centered(el elements.Element) elements.Element {
	return elements.Box(styles.Styles{Position: properties.PositionAbsolute, Top: 0, Left: 0, Right: 0, Bottom: 0,
		Justify: properties.JustifyCenter, Align: properties.AlignCenter, Width: -1, Height: -1}, el)
}

// dropTarget shows that files dragged over the window can be dropped.
func (v *videoArea) dropTarget(playing bool) elements.Element {
	hint := "Drop to play"
	if playing {
		hint = "Drop to play, or to add subtitles"
	}
	return elements.Box(styles.Styles{Position: properties.PositionAbsolute, Top: 0, Left: 0, Right: 0, Bottom: 0,
		Padding: properties.UniformInset(12)},
		elements.Box(styles.Styles{Width: -1, Height: -1, Justify: properties.JustifyCenter, Align: properties.AlignCenter,
			Background: "#000000a6", BorderRadius: 16, BorderWidth: 2, BorderColor: colHighlight},
			elements.Box(styles.Styles{FlexDirection: properties.FlexDirectionColumn, Align: properties.AlignCenter, Gap: 12},
				ic("add", 40, colHighlight),
				elements.Text(hint, styles.Styles{Color: colText, FontSize: 16, FontWeight: properties.FontWeightMedium}))))
}

// osd is the transient message at the top of the picture.
func (v *videoArea) osd() elements.Element {
	u := v.u
	top := 20
	if u.overlay() {
		top += u.titleHeightDp()
	}
	children := []elements.Element{elements.Text(u.osdText, styles.Styles{Color: colText, FontSize: 15, FontWeight: properties.FontWeightMedium})}
	if u.osdLevel >= 0 {
		children = append(children, progress.Progress(float32(u.osdLevel), styles.Styles{
			Width: 160, Height: 4, Color: colHighlight, Background: colTrack, BorderRadius: 2,
		}))
	}
	return elements.Box(styles.Styles{Position: properties.PositionAbsolute, Top: properties.Top(top), Left: 0, Right: 0,
		Width: -1, Justify: properties.JustifyCenter},
		elements.Box(styles.Styles{
			Background: "#121214d9", BorderRadius: 12, Padding: properties.SymmetricInset(16, 10),
			FlexDirection: properties.FlexDirectionColumn, Align: properties.AlignCenter, Gap: 8,
			BoxShadow: properties.BoxShadow{Y: 4, Blur: 16, Color: "#00000080"},
		}, children...))
}

// info is the media information panel.
func (v *videoArea) info() elements.Element {
	u := v.u
	top := 16
	if u.overlay() {
		top += u.titleHeightDp()
	}
	rows := []elements.Element{elements.Text("Media information", styles.Styles{Color: colText, FontSize: 13, FontWeight: properties.FontWeightSemiBold})}
	for _, r := range u.infoRows {
		rows = append(rows, elements.Row(styles.Styles{Gap: 12, Wrap: properties.WrapNoWrap},
			elements.Box(styles.Styles{Width: 120}, elements.Text(r[0], styles.Styles{Color: colMuted, FontSize: 12})),
			elements.Box(styles.Styles{MaxWidth: 300}, elements.Text(r[1], styles.Styles{Color: colText, FontSize: 12, MaxLines: 2})),
		))
	}
	return elements.Box(styles.Styles{
		Position: properties.PositionAbsolute, Top: properties.Top(top), Left: 16,
		Background: "#121214e6", BorderRadius: 12, Padding: properties.UniformInset(14),
		FlexDirection: properties.FlexDirectionColumn, Gap: 6,
	}, rows...)
}

// welcome is the empty state: what to open, and what was played lately.
// finished shows it over the last frame once everything has played,
// saying so and offering to play it again.
func (v *videoArea) welcome(finished bool) elements.Element {
	a := v.u.a
	head := elements.Box(styles.Styles{FlexDirection: properties.FlexDirectionColumn, Align: properties.AlignCenter, Gap: 10},
		ic("logo", 56, colHighlight),
		elements.Text("Drop a video here, or", styles.Styles{Color: colMuted, FontSize: 15}),
		elements.Row(styles.Styles{Gap: 10, Wrap: properties.WrapNoWrap, Margin: properties.Inset{Top: 6}}, v.openBtn, v.urlBtn, v.folderBtn, v.netBtn),
	)
	if finished {
		st := &a.player.State
		note := displayTitle(st)
		if n := len(st.Playlist); n > 1 {
			note = "All " + plural(n, "file", "files") + " in the playlist have played."
		}
		head = elements.Box(styles.Styles{FlexDirection: properties.FlexDirectionColumn, Align: properties.AlignCenter, Gap: 10},
			elements.Text("Finished playing", styles.Styles{Color: colText, FontSize: 20, FontWeight: properties.FontWeightSemiBold}),
			elements.Box(styles.Styles{MaxWidth: 520},
				elements.Text(note, styles.Styles{Color: colMuted, FontSize: 14, MaxLines: 2, TextAlign: properties.TextAlignCenter})),
			elements.Box(styles.Styles{Margin: properties.Inset{Top: 6, Bottom: 10}}, v.againBtn),
			elements.Text("Or open something else", styles.Styles{Color: colMuted, FontSize: 13}),
			elements.Row(styles.Styles{Gap: 10, Wrap: properties.WrapNoWrap}, v.openNextBtn, v.urlBtn, v.folderBtn, v.netBtn),
		)
	}
	col := []elements.Element{head}
	if recent := a.history.Items(); len(recent) > 0 {
		// The finished screen's heading is taller; it lists fewer.
		limit := 6
		if finished {
			limit = 3
		}
		if len(recent) > limit {
			recent = recent[:limit]
		}
		rows := []elements.Element{elements.Row(styles.Styles{Width: -1, Align: properties.AlignCenter, Wrap: properties.WrapNoWrap},
			elements.Box(styles.Styles{FlexGrow: 1, Width: -1}, elements.Text("Recent", styles.Styles{Color: colMuted, FontSize: 12, FontWeight: properties.FontWeightSemiBold})),
			v.clearRecent(),
		)}
		for _, r := range recent {
			rows = append(rows, v.recentRow(r))
		}
		col = append(col, elements.Box(styles.Styles{Width: 460, FlexDirection: properties.FlexDirectionColumn, Gap: 2, Margin: properties.Inset{Top: 28}}, rows...))
	}
	screen := centered(elements.Box(styles.Styles{FlexDirection: properties.FlexDirectionColumn, Align: properties.AlignCenter}, col...))
	if finished {
		// The last frame stays behind, dimmed.
		return elements.Stack(styles.Styles{Width: -1, Height: -1},
			elements.Box(styles.Styles{Width: -1, Height: -1, Background: "#000000d9"}),
			screen)
	}
	return screen
}

func (v *videoArea) clearRecent() elements.Element {
	a := v.u.a
	return elements.Keyed("welcome", "clear", func() *elements.Pressable {
		return elements.NewPressable(func(s elements.PressState) elements.Element {
			c := colFaint
			if s.Hovered {
				c = colText
			}
			return elements.Text("Clear", styles.Styles{Color: c, FontSize: 12})
		}).Label("Clear recent").Focusable(false).OnClick(func(elements.ClickEvent) {
			a.history.Clear()
			a.history.Save()
		})
	})
}

// recentRow is one recently played file: name, where it's from, and how
// far it got.
func (v *videoArea) recentRow(r library.Recent) elements.Element {
	a := v.u.a
	return elements.Keyed("recent", r.Path, func() *elements.Pressable {
		return elements.NewPressable(func(s elements.PressState) elements.Element {
			bg := ""
			if s.Hovered {
				bg = colSurface2
			}
			icon := "file"
			where := filepath.Dir(r.Path)
			if library.IsURL(r.Path) {
				icon, where = "link", hostOf(r.Path)
			}
			meta := where
			if p := r.Progress(); p > 0.02 && p < 0.98 {
				meta = player.FormatTime(r.Duration-r.Position) + " left · " + where
			}
			body := []elements.Element{
				elements.Text(r.Name(), styles.Styles{Color: colText, FontSize: 13, MaxLines: 1, TextOverflow: properties.TextOverflowEllipsis}),
				elements.Text(meta, styles.Styles{Color: colFaint, FontSize: 11, MaxLines: 1, TextOverflow: properties.TextOverflowEllipsis}),
			}
			if p := r.Progress(); p > 0.02 && p < 0.98 {
				body = append(body, progress.Progress(float32(p), styles.Styles{Width: -1, Height: 2, Color: colMuted, Background: colSurface3}))
			}
			return elements.Row(styles.Styles{Width: -1, Align: properties.AlignCenter, Gap: 12, Padding: properties.SymmetricInset(10, 8),
				Background: bg, BorderRadius: 8, Wrap: properties.WrapNoWrap},
				ic(icon, 18, colMuted),
				elements.Box(styles.Styles{FlexGrow: 1, Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 3}, body...),
			)
		}).Label(r.Name()).OnClick(func(elements.ClickEvent) { a.openItems([]string{r.Path}, false) })
	})
}

func hostOf(u string) string {
	s := u
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimPrefix(s, "www.")
}

// audioOnly is shown for files without video: a large note and the
// title.
func (v *videoArea) audioOnly() elements.Element {
	st := &v.u.a.player.State
	return centered(elements.Box(styles.Styles{FlexDirection: properties.FlexDirectionColumn, Align: properties.AlignCenter, Gap: 14},
		elements.Box(styles.Styles{Background: colSurface2, BorderRadius: 24, Padding: properties.UniformInset(28)}, ic("music-large", 72, colMuted)),
		elements.Text(displayTitle(st), styles.Styles{Color: colText, FontSize: 18, FontWeight: properties.FontWeightSemiBold, MaxLines: 2, TextAlign: properties.TextAlignCenter}),
	))
}

// castScreen replaces the picture while the video plays on a TV.
func (v *videoArea) castScreen() elements.Element {
	a := v.u.a
	c := &a.cast
	state := map[cast.State]string{
		cast.StateLoading: "Loading on", cast.StatePlaying: "Playing on", cast.StatePaused: "Paused on",
		cast.StateBuffering: "Buffering on", cast.StateIdle: "Connected to", cast.StateStopped: "Finished on",
	}[c.status.State]
	if state == "" {
		state = "Casting to"
	}
	stop := elements.Keyed("cast", "stop", func() *button.Button {
		return button.New("Stop Casting", styles.Styles{
			Background: colSurface3, Color: colText, FontSize: 14, Padding: properties.SymmetricInset(18, 10), BorderRadius: 20,
		}).OnClick(a.stopCasting)
	})
	return centered(elements.Box(styles.Styles{FlexDirection: properties.FlexDirectionColumn, Align: properties.AlignCenter, Gap: 12},
		ic("cast-on", 64, colText),
		elements.Text(state, styles.Styles{Color: colMuted, FontSize: 14}),
		elements.Text(c.device, styles.Styles{Color: colText, FontSize: 22, FontWeight: properties.FontWeightSemiBold}),
		elements.Text(displayTitle(&a.player.State), styles.Styles{Color: colMuted, FontSize: 13, MaxLines: 1}),
		elements.Box(styles.Styles{Margin: properties.Inset{Top: 8}}, stop),
	))
}

func displayTitle(st *player.State) string {
	if st.Title != "" {
		return st.Title
	}
	return filepath.Base(st.Path)
}
