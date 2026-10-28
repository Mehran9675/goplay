package ui

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/mehran9675/giode/elements"

	"github.com/mehran9675/goplay-ebiten/internal/settings"
)

// fakeController is a Controller with fixed state.
type fakeController struct {
	pos, dur, vol    float64
	paused, finished bool
	media, full      bool
	store            *settings.Store
}

func (f *fakeController) Pos() float64         { return f.pos }
func (f *fakeController) Duration() float64    { return f.dur }
func (f *fakeController) Volume() float64      { return f.vol }
func (f *fakeController) Paused() bool         { return f.paused }
func (f *fakeController) Finished() bool       { return f.finished }
func (f *fakeController) HasMedia() bool       { return f.media }
func (f *fakeController) Message() string      { return "" }
func (f *fakeController) TogglePause()         {}
func (f *fakeController) Stop()                {}
func (f *fakeController) Next()                {}
func (f *fakeController) OpenFile()            {}
func (f *fakeController) OpenFolder()          {}
func (f *fakeController) Seek(float64)         {}
func (f *fakeController) SeekRelative(float64) {}
func (f *fakeController) SetVolume(v float64)  { f.vol = v }
func (f *fakeController) ToggleFullscreen()    {}
func (f *fakeController) Fullscreen() bool     { return f.full }
func (f *fakeController) Quit()                {}

func (f *fakeController) Tracks(kind TrackKind) []Track {
	if !f.media {
		return nil
	}
	return []Track{{ID: 1, Title: "English (eng)", Selected: true}, {ID: 2, Title: "Track 2"}}
}
func (f *fakeController) SelectTrack(TrackKind, int) {}
func (f *fakeController) LoadSubtitle()              {}
func (f *fakeController) AspectRatio() string        { return "" }
func (f *fakeController) SetAspectRatio(string)      {}
func (f *fakeController) Speed() float64             { return 1 }
func (f *fakeController) SetSpeed(float64)           {}
func (f *fakeController) Loop() bool                 { return false }
func (f *fakeController) ToggleLoop()                {}
func (f *fakeController) Settings() *settings.Store {
	if f.store == nil {
		f.store, _ = settings.Load("")
	}
	return f.store
}
func (f *fakeController) SetSetting(k, v string) { f.Settings().Set(k, v) }
func (f *fakeController) ResetSettings()         { f.Settings().Reset() }
func (f *fakeController) AudioDevices() []settings.Option {
	return []settings.Option{{Label: "Automatic", Value: "auto"}}
}
func (f *fakeController) EditMPVConf()    {}
func (f *fakeController) Title() string   { return "movie.mkv" }
func (f *fakeController) Minimize()       {}
func (f *fakeController) ToggleMaximize() {}
func (f *fakeController) Close()          {}
func (f *fakeController) Maximized() bool { return false }
func (f *fakeController) Focused() bool   { return true }

func context(scale float32, max image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Constraints{Max: max},
	}
}

func TestIconsRegistered(t *testing.T) {
	for _, name := range []string{
		"play", "pause", "stop", "skip-next", "check",
		"volume-up", "volume-down", "volume-off", "fullscreen", "fullscreen-exit",
	} {
		iconSrc(name) // panics if missing
	}
}

// TestButtonsSquare guards against icons and buttons taking a
// non-square box, at 1x and on HiDPI.
func TestButtonsSquare(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		u := New()
		c := &fakeController{dur: 100, vol: 0.7, media: true}
		u.updateIcons(c)
		for name, b := range map[string]struct {
			want int
			lay  func(layout.Context) layout.Dimensions
		}{
			"play": {playSize, u.playBtn.Layout},
			"stop": {btnSize, u.stopBtn.Layout},
			"mute": {btnSize, u.muteBtn.Layout},
		} {
			// A short row height must not squash the button.
			gtx := context(scale, image.Pt(1000, 1000))
			got := b.lay(gtx).Size
			want := gtx.Dp(unit.Dp(b.want))
			if got.X != want || got.Y != want {
				t.Errorf("scale %v: %s button is %v, want %dx%d", scale, name, got, want, want)
			}
		}
	}
}

func TestLayout(t *testing.T) {
	u := New()
	for _, c := range []*fakeController{
		{},
		{dur: 3700, pos: 12, vol: 0.3, media: true},
		{dur: 60, vol: 0, media: true, paused: true, full: true},
	} {
		gtx := context(1.5, image.Pt(1280, 720))
		gtx.Constraints.Min = gtx.Constraints.Max
		u.Layout(gtx, c)
	}
}

// TestMenu builds the context menu with and without media and lays it
// out open, submenus included.
func TestMenu(t *testing.T) {
	for _, c := range []*fakeController{{}, {media: true, dur: 10}} {
		u := New()
		u.c = c
		u.OpenMenuAt(10, 10)
		gtx := context(1.5, image.Pt(1280, 720))
		gtx.Constraints.Min = gtx.Constraints.Max
		u.Layout(gtx, c)
	}
}

func TestToggleMute(t *testing.T) {
	u := New()
	c := &fakeController{vol: 0.4}
	u.toggleMute(c)
	if c.vol != 0 {
		t.Fatalf("muted volume = %v, want 0", c.vol)
	}
	u.toggleMute(c)
	if c.vol != 0.4 {
		t.Fatalf("restored volume = %v, want 0.4", c.vol)
	}
}

func TestVolumeIcon(t *testing.T) {
	for v, want := range map[float64]string{0: "volume-off", 0.3: "volume-down", 0.5: "volume-up", 1: "volume-up"} {
		if got := volumeIcon(v); got != want {
			t.Errorf("volumeIcon(%v) = %q, want %q", v, got, want)
		}
	}
}

// TestFullscreenBarShowsOnHover checks the bar always shows in a window,
// and in fullscreen only while the pointer is over the bottom strip.
func TestFullscreenBarShowsOnHover(t *testing.T) {
	var r input.Router
	u := New()
	c := &fakeController{media: true, dur: 100, vol: 1}
	win := image.Pt(1280, 720)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{Ops: ops, Source: r.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(win)}
		u.Layout(gtx, c)
		r.Frame(ops)
	}
	moveTo := func(x, y float32) {
		r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(x, y)})
		frame()
	}
	move := func(y float32) { moveTo(640, y) }

	frame()
	if !u.RectContains(640, 700) {
		t.Fatal("windowed: control bar not shown")
	}

	u.Fullscreen = true
	move(100)
	if u.RectContains(640, 700) {
		t.Fatal("fullscreen: bar shown without hovering")
	}
	move(700)
	if !u.RectContains(640, 700) {
		t.Fatal("fullscreen: bar hidden while hovering the bottom strip")
	}
	// Over the play button, which sits on the bar: still hovering.
	moveTo(16+playSize/2, 720-10-playSize/2)
	if !u.RectContains(640, 700) {
		t.Fatal("fullscreen: bar hidden while hovering one of its buttons")
	}
	move(100)
	if u.RectContains(640, 700) {
		t.Fatal("fullscreen: bar still shown after the pointer left")
	}
}

// TestSettingsDialog lays the settings dialog out on every section.
func TestSettingsDialog(t *testing.T) {
	var r input.Router
	u := New()
	c := &fakeController{media: true, dur: 100, vol: 1}
	u.c = c
	u.OpenSettings()
	for i := range settings.Sections {
		u.prefs.section = i
		for range 2 {
			ops := new(op.Ops)
			gtx := layout.Context{Ops: ops, Source: r.Source(), Metric: unit.Metric{PxPerDp: 1.5, PxPerSp: 1.5}, Constraints: layout.Exact(image.Pt(1280, 900))}
			u.Layout(gtx, c)
			elements.DrainPortals(gtx)
			r.Frame(ops)
		}
	}
	if !u.RectContains(5, 5) {
		t.Error("open dialog does not block window dragging")
	}
}

func TestRangeValues(t *testing.T) {
	d := settings.Lookup("sub_scale") // 0.5..2.5 step 0.1
	for v, want := range map[float32]string{0: "0.5", 1: "2.5", 0.25: "1.0", 0.26: "1.0", 0.3: "1.1"} {
		if got := formatRange(d, rangeValueAt(d, v)); got != want {
			t.Errorf("sub_scale at %v = %s, want %s", v, got, want)
		}
	}
	d = settings.Lookup("brightness") // -100..100 step 1
	if got := formatRange(d, rangeValueAt(d, 0.5)); got != "0" {
		t.Errorf("brightness at 0.5 = %s, want 0", got)
	}
}

// TestEscapeOrder checks Escape closes the settings dialog, then leaves
// fullscreen, before it quits.
type quitCounter struct {
	*fakeController
	quits, fullscreens int
}

func (q *quitCounter) Quit()             { q.quits++ }
func (q *quitCounter) ToggleFullscreen() { q.fullscreens++; q.full = !q.full }

func TestEscapeOrder(t *testing.T) {
	u := New()
	c := &quitCounter{fakeController: &fakeController{full: true}}
	u.c = c
	u.OpenSettings()
	u.KeyPress(c, KeyEscape)
	if u.settingsOpen || c.fullscreens != 0 || c.quits != 0 {
		t.Fatalf("first Escape: dialog open %v, fullscreen toggles %d, quits %d", u.settingsOpen, c.fullscreens, c.quits)
	}
	u.KeyPress(c, KeyEscape)
	if c.fullscreens != 1 || c.quits != 0 {
		t.Fatalf("second Escape: fullscreen toggles %d, quits %d", c.fullscreens, c.quits)
	}
	u.KeyPress(c, KeyEscape)
	if c.quits != 1 {
		t.Fatalf("third Escape: quits %d", c.quits)
	}
}
