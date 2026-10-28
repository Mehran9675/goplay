package app

import (
	"image"
	"runtime"
	"time"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/mehran9675/giode/components/drawer"
	"github.com/mehran9675/giode/components/menu"
	"github.com/mehran9675/giode/components/titlebar"
	"github.com/mehran9675/giode/components/toaster"
	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/goplay/internal/settings"
)

// ui is the view state: components created once and laid out every
// frame, and the transient state of the chrome (what's hovered, open,
// fading).
type ui struct {
	a *App

	fb        image.Point
	scale     float32
	videoRect image.Rectangle
	// marginTop and marginBottom are the video margins last sent to mpv.
	marginTop, marginBottom float64

	title *titlebar.TitleBar
	bar   *controlBar

	// The chrome in overlay mode (maximized, fullscreen, picture in
	// picture) shows while the pointer is over its strip.
	topHover, bottomHover gesture.Hover
	topShown, bottomShown bool
	lastMove              time.Time
	lastPointer           f32.Point
	cursorHidden          bool

	pip            bool
	pipRestore     image.Rectangle
	pipWasFloating bool

	video *videoArea

	menu *menu.Menu // the open context or popover menu

	panelOpen bool
	panelTab  int
	panel     *sidePanel
	drawer    *drawer.Drawer

	osdText  string
	osdLevel float64 // 0 to 1 for a level bar, -1 for none
	osdUntil time.Time
	flash    string
	flashAt  time.Time

	infoOpen bool
	infoRows [][2]string
	infoNext time.Time

	dialogs *dialogs

	toasts *toaster.Toaster

	appMenuAnchor *anchor

	// flipH and flipV mirror the picture (per file).
	flipH, flipV bool
}

func newUI(a *App) *ui {
	u := &ui{a: a, osdLevel: -1, lastMove: time.Now()}
	// The bar itself is transparent: titleBar puts it on a solid strip
	// or a fade, depending on the mode.
	u.title = titlebar.New(windowCtl{a}, "goplay", styles.Styles{
		Background: "transparent",
		Color:      colText,
		FontSize:   13,
		Height:     properties.Height(u.titleHeightDp()),
	})
	if runtime.GOOS == "darwin" {
		// macOS draws its own traffic lights (see the platform package);
		// the bar only leaves room for them.
		u.title.Variant(titlebar.VariantMac).Hide(titlebar.ButtonMinimize, titlebar.ButtonMaximize, titlebar.ButtonClose).
			Leading(elements.Box(styles.Styles{Width: 64}))
	}
	u.title.Trailing(u.titleButtons()...)
	u.bar = newControlBar(u)
	u.video = newVideoArea(u)
	u.panel = newSidePanel(u)
	u.drawer = drawer.New(&u.panelOpen, drawer.Right, panelWidth, u.panel.content, styles.Styles{
		Background: colSurface,
	})
	u.dialogs = newDialogs(u)
	u.toasts = toaster.New(styles.Styles{Background: colSurface2, BorderColor: colBorder, Color: colText, FontSize: 13}).
		Placement(toaster.TopCenter).Max(3)
	return u
}

// windowCtl lets the title bar drive the window.
type windowCtl struct{ a *App }

func (w windowCtl) Minimize() { w.a.win.Minimize() }
func (w windowCtl) ToggleMaximize() {
	if w.a.ui.pip {
		w.a.togglePiP()
	}
	if w.a.win.Fullscreen() {
		w.a.setFullscreen(false)
		return
	}
	w.a.win.ToggleMaximize()
}
func (w windowCtl) Close()          { w.a.Quit() }
func (w windowCtl) Maximized() bool { return w.a.win.Maximized() || w.a.win.Fullscreen() }
func (w windowCtl) Focused() bool   { return w.a.win.Focused() }

func (u *ui) titleHeightDp() int {
	if runtime.GOOS == "darwin" {
		return titleBarMacH
	}
	return titleBarH
}

func (u *ui) dp(v int) int { return int(float32(v)*u.scale + 0.5) }

func (u *ui) titleHeightPx() int { return u.dp(u.titleHeightDp()) }
func (u *ui) barHeightPx() int   { return u.dp(controlBarH) }

// overlay reports whether the chrome floats over the video (maximized,
// fullscreen, picture in picture) rather than sitting beside it.
func (u *ui) overlay() bool {
	w := u.a.win
	return w.Maximized() || w.Fullscreen() || u.pip
}

// beginFrame records the frame's size and refreshes what depends on
// time.
func (u *ui) beginFrame(fb image.Point, scale float32) {
	u.fb, u.scale = fb, scale
	if u.overlay() {
		u.videoRect = image.Rectangle{Max: fb}
	} else {
		u.videoRect = image.Rect(0, u.titleHeightPx(), fb.X, fb.Y-u.barHeightPx())
	}
	if u.infoOpen && !time.Now().Before(u.infoNext) {
		u.infoRows = u.a.player.MediaInfo()
		u.infoNext = time.Now().Add(time.Second)
	}
	u.a.vrTick()
	u.updateCursor()
}

// nextTimer is the next moment the UI changes on its own.
func (u *ui) nextTimer() time.Time {
	var next time.Time
	consider := func(t time.Time) {
		if !t.IsZero() && t.After(time.Now().Add(-time.Second)) && (next.IsZero() || t.Before(next)) {
			next = t
		}
	}
	if u.osdText != "" {
		consider(u.osdUntil)
	}
	if u.flash != "" {
		consider(u.flashAt.Add(flashDuration))
	}
	if u.infoOpen {
		consider(u.infoNext)
	}
	if u.overlay() && !u.cursorHidden {
		consider(u.lastMove.Add(u.hideDelay()))
	}
	consider(u.a.vrTimer())
	return next
}

func (u *ui) hideDelay() time.Duration {
	s := u.a.settings.Float(settings.KeyHideDelay)
	if s <= 0 {
		s = 2
	}
	return time.Duration(s * float64(time.Second))
}

// updateCursor hides the pointer over playing video after it rests for a
// moment in overlay mode, like every player does.
func (u *ui) updateCursor() {
	p := u.a.win.CursorPx()
	if p != u.lastPointer {
		u.lastPointer = p
		u.lastMove = time.Now()
	}
	st := &u.a.player.State
	hide := u.overlay() && !st.Idle && !st.Paused && !u.topShown && !u.bottomShown && !u.modalOpen() &&
		time.Since(u.lastMove) >= u.hideDelay()
	if hide != u.cursorHidden {
		u.cursorHidden = hide
		u.a.win.HideCursor(hide)
	}
}

// layout draws the whole UI.
func (u *ui) layout(gtx layout.Context) {
	fb := gtx.Constraints.Max
	overlay := u.overlay()
	titleH, barH := u.titleHeightPx(), u.barHeightPx()
	st := &u.a.player.State

	// Overlay mode: hovering a strip shows its bar. Menus and drags
	// keep the bottom bar up; with nothing loaded both bars stay.
	topZone := image.Rect(0, 0, fb.X, titleH+u.dp(16))
	bottomZone := image.Rect(0, fb.Y-barH-u.dp(8), fb.X, fb.Y)
	topHovered := u.topHover.Update(gtx.Source)
	bottomHovered := u.bottomHover.Update(gtx.Source)
	u.topShown = !overlay || st.Idle || topHovered
	u.bottomShown = !overlay || st.Idle || bottomHovered || u.bar.busy() || (u.menu != nil && u.menu.Opened())
	if u.a.win.Fullscreen() && !st.Idle {
		u.topShown = u.topShown && topHovered
	}
	u.a.win.SetTrafficLights(u.topShown)

	// The video area, with everything drawn over the picture.
	place(gtx, u.videoRect, u.video)

	// The bars.
	if u.topShown {
		u.title.Title(u.windowTitle())
		place(gtx, image.Rect(0, 0, fb.X, titleH), u.titleBar(overlay))
	}
	if u.bottomShown {
		place(gtx, image.Rect(0, fb.Y-barH, fb.X, fb.Y), u.bar.element(overlay))
	}
	if overlay {
		// Hover zones go on top and pass events through, so moving onto
		// a bar's buttons still counts as hovering its strip.
		zone(gtx, topZone, &u.topHover)
		zone(gtx, bottomZone, &u.bottomHover)
	}

	u.drawer.Layout(gtx)
	u.dialogs.layout(gtx)
	if u.menu != nil {
		u.menu.Layout(gtx)
	}
	u.toasts.Layout(gtx)
}

// titleBar puts the title bar on a solid strip beside the video, or on a
// soft fade from the top edge over it.
func (u *ui) titleBar(overlay bool) elements.Element {
	st := styles.Styles{Width: -1, Height: -1, Background: colSurface}
	if overlay {
		st.Background = ""
		st.BackgroundGradient = properties.Linear(180, "#000000b3", "#00000000")
	}
	return elements.Box(st, u.title)
}

func (u *ui) windowTitle() string {
	st := &u.a.player.State
	switch {
	case u.a.casting():
		return st.Title + " — casting to " + u.a.cast.device
	case !st.Idle && st.Title != "":
		return st.Title
	}
	return "goplay"
}

// titleButtons are the title bar's extra controls: the app menu.
func (u *ui) titleButtons() []elements.Element {
	more := ghostButton("menu", 18, "Menu", func() { u.openAppMenu() })
	u.appMenuAnchor = &anchor{el: more}
	return []elements.Element{u.appMenuAnchor}
}

// place lays el out exactly filling r, with AbsolutePosition kept in
// step so popups inside it open in the right spot.
func place(gtx layout.Context, r image.Rectangle, el elements.Element) layout.Dimensions {
	if r.Empty() {
		return layout.Dimensions{}
	}
	t := op.Offset(r.Min).Push(gtx.Ops)
	pop := elements.PushOffset(r.Min)
	cgtx := gtx
	cgtx.Constraints = layout.Exact(r.Size())
	dims := el.Layout(cgtx)
	pop()
	t.Pop()
	return dims
}

// zone registers a pass-through hover area.
func zone(gtx layout.Context, r image.Rectangle, h *gesture.Hover) {
	area := clip.Rect(r).Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	h.Add(gtx.Ops)
	pass.Pop()
	area.Pop()
}

// anchor records where an element was laid out, to open popovers from
// it.
type anchor struct {
	el   elements.Element
	pos  image.Point
	size image.Point
}

func (an *anchor) Layout(gtx layout.Context) layout.Dimensions {
	an.pos = elements.AbsolutePosition()
	d := an.el.Layout(gtx)
	an.size = d.Size
	return d
}

// openMenuAt shows m with its top-left at p (pixels); it flips above
// when there's no room below.
func (u *ui) openMenuAt(m *menu.Menu, p image.Point) {
	if u.menu != nil {
		u.menu.Close()
	}
	u.menu = m
	m.OpenAt(f32.Pt(float32(p.X), float32(p.Y)))
	u.a.dirty = true
}

// openMenuOver opens m from a control: above it when the control is in
// the bottom half of the window, below otherwise.
func (u *ui) openMenuOver(m *menu.Menu, an *anchor) {
	p := an.pos
	if p.Y > u.fb.Y/2 {
		u.openMenuAt(m, image.Pt(p.X, p.Y-u.dp(6)))
		return
	}
	u.openMenuAt(m, image.Pt(p.X, p.Y+an.size.Y+u.dp(6)))
}

// modalOpen reports whether a dialog takes the keyboard.
func (u *ui) modalOpen() bool { return u.dialogs.anyOpen() }

// closeTopmost closes the innermost open thing; false if none was.
func (u *ui) closeTopmost() bool {
	switch {
	case u.menu != nil && u.menu.Opened():
		u.menu.Close()
	case u.dialogs.closeTop():
	case u.panelOpen:
		u.panelOpen = false
	case u.infoOpen:
		u.infoOpen = false
	default:
		return false
	}
	u.a.dirty = true
	return true
}

func (u *ui) togglePanel(tab int) {
	if u.panelOpen && u.panelTab == tab {
		u.panelOpen = false
		return
	}
	u.panelTab = tab
	u.panelOpen = true
	u.panel.opened()
}

func (u *ui) toggleInfo() {
	u.infoOpen = !u.infoOpen
	u.infoNext = time.Time{}
}

// fileLoaded resets per-file view state.
func (u *ui) fileLoaded() {
	u.a.Post(u.a.sizeToVideo)
	u.panel.scrollToCurrent()
}

// On-screen display.

const (
	osdDuration   = 1400 * time.Millisecond
	flashDuration = 500 * time.Millisecond
)

// osd briefly shows a message over the video.
func (a *App) osd(text string) { a.osdLevel(text, -1) }

// osdLevel shows a message with a level bar (volume), level from 0 to 1.
func (a *App) osdLevel(text string, level float64) {
	if !a.settings.Bool(settings.KeyOSD) {
		return
	}
	u := a.ui
	u.osdText, u.osdLevel, u.osdUntil = text, level, time.Now().Add(osdDuration)
	a.dirty = true
}

// flashPlayPause shows a large play or pause symbol fading out in the
// middle of the picture.
func (u *ui) flashPlayPause(playing bool) {
	u.flash = "pause"
	if playing {
		u.flash = "play"
	}
	u.flashAt = time.Now()
}

func (a *App) toast(msg string)      { a.ui.toasts.Show(toaster.Info, msg) }
func (a *App) toastError(msg string) { a.ui.toasts.Show(toaster.Error, msg) }

// canDragAt reports whether a press at p (pixels) should move the
// window: on the video in a normal window, with nothing open over it.
func (a *App) canDragAt(p f32.Point) bool {
	u := a.ui
	if u.overlay() || u.modalOpen() || u.panelOpen || (u.menu != nil && u.menu.Opened()) || a.vrActive() {
		return false
	}
	return image.Pt(int(p.X), int(p.Y)).In(u.videoRect)
}
