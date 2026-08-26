package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// mockController is a minimal Controller for driving the UI in tests.
type mockController struct {
	hasMedia   bool
	msg        string
	openedFile bool
}

func (m *mockController) Pos() float64                 { return 30 }
func (m *mockController) Duration() float64            { return 100 }
func (m *mockController) Volume() float64              { return 1 }
func (m *mockController) Paused() bool                 { return false }
func (m *mockController) Finished() bool               { return false }
func (m *mockController) HasMedia() bool               { return m.hasMedia }
func (m *mockController) Message() string              { return m.msg }
func (m *mockController) TogglePause()                 {}
func (m *mockController) Stop()                        {}
func (m *mockController) Next()                        {}
func (m *mockController) OpenFile()                    { m.openedFile = true }
func (m *mockController) OpenFolder()                  {}
func (m *mockController) Seek(frac float64)            {}
func (m *mockController) SeekRelative(seconds float64) {}
func (m *mockController) SetVolume(v float64)          {}
func (m *mockController) ToggleFullscreen()            {}
func (m *mockController) Fullscreen() bool             { return false }
func (m *mockController) Quit()                        {}

// menuHarness drives the UI exactly like internal/app: pointer events are
// queued between frames and processed by router.Frame after Layout.
type menuHarness struct {
	u      *UI
	ctrl   *mockController
	router input.Router
	ops    op.Ops
	gtx    layout.Context
}

func newMenuHarness(t *testing.T, pxPerDp float32) *menuHarness {
	t.Helper()
	h := &menuHarness{
		u:    New(),
		ctrl: &mockController{hasMedia: true},
	}
	h.gtx = layout.Context{
		Ops:         &h.ops,
		Now:         time.Now(),
		Source:      h.router.Source(),
		Metric:      unit.Metric{PxPerDp: pxPerDp, PxPerSp: pxPerDp},
		Constraints: layout.Exact(image.Pt(1280, 720)),
	}
	return h
}

// frame renders one frame (Layout + router.Frame), matching the app loop.
func (h *menuHarness) frame() {
	h.ops.Reset()
	h.gtx.Ops = &h.ops
	h.gtx.Source = h.router.Source()
	h.u.Layout(h.gtx, h.ctrl)
	h.router.Frame(h.gtx.Ops)
}

// queue simulates a GLFW pointer event queued between frames, exactly like
// the app's mouse button callback.
func (h *menuHarness) queue(pos f32.Point, kind pointer.Kind, btns pointer.Buttons) {
	h.router.Queue(pointer.Event{
		Kind:     kind,
		Source:   pointer.Mouse,
		Time:     time.Duration(0),
		Position: pos,
		Buttons:  btns,
	})
}

// click simulates a full left-button click at pos.
func (h *menuHarness) click(pos f32.Point) {
	h.queue(pos, pointer.Press, pointer.ButtonPrimary)
	h.queue(pos, pointer.Release, 0)
}

// rightClick opens the menu the way the app does: ToggleMenuAt is called
// from the mouse button callback, and the press/release events are queued.
func (h *menuHarness) rightClick(pos f32.Point) {
	h.u.ToggleMenuAt(pos.X, pos.Y)
	h.queue(pos, pointer.Press, pointer.ButtonSecondary)
	h.queue(pos, pointer.Release, 0)
}

// openMenu right-clicks, then renders enough frames for the menu to be open
// and stable. It returns the menu's on-screen rectangle in dp.
func (h *menuHarness) openMenu(t *testing.T) image.Rectangle {
	t.Helper()
	pos := f32.Pt(200, 200)
	h.rightClick(pos)
	h.frame() // menu first appears
	h.frame() // let the right-click press/release drain
	if !h.u.menuOpen {
		t.Fatal("menu did not open")
	}
	r := h.u.menuRect
	if r.Empty() {
		t.Fatal("menuRect empty")
	}
	return r
}

func TestMenuDismissOnOutsideClick(t *testing.T) {
	h := newMenuHarness(t, 1)
	r := h.openMenu(t)

	// Click outside the menu panel.
	outside := f32.Pt(float32(r.Max.X+20), float32(r.Max.Y+20))
	h.click(outside)
	h.frame()
	h.frame()
	if h.u.menuOpen {
		t.Fatalf("menu still open after outside click at %v (rect %v)", outside, r)
	}
}

func TestMenuItemClickClosesAndFires(t *testing.T) {
	h := newMenuHarness(t, 1)
	r := h.openMenu(t)

	// Click inside the panel, on the first item row.
	item := f32.Pt(float32(r.Min.X+30), float32(r.Min.Y+15))
	h.click(item)
	h.frame()
	h.frame()
	if h.u.menuOpen {
		t.Fatalf("menu still open after item click at %v (rect %v)", item, r)
	}
	if !h.ctrl.openedFile {
		t.Fatalf("item action did not fire after click at %v", item)
	}
}

func TestMenuItemClickHighDPIScale(t *testing.T) {
	h := newMenuHarness(t, 1.5)
	r := h.openMenu(t)

	// The user clicks where the item is visually drawn (dp coordinates).
	item := f32.Pt(float32(r.Min.X+30), float32(r.Min.Y+15))
	h.click(item)
	h.frame()
	h.frame()
	if h.u.menuOpen {
		t.Fatalf("menu still open after item click at %v (rect %v) with scale 1.5", item, r)
	}
	if !h.ctrl.openedFile {
		t.Fatalf("item action did not fire at %v with scale 1.5", item)
	}
}
