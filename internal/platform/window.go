// Package platform is goplay's window: a GLFW window with an OpenGL
// context that libmpv renders video into and Gio draws the Giode UI over,
// in the same framebuffer, so video frames never leave the GPU.
//
// It feeds the window's input into a Gio input.Router, handles the
// clipboard and cursor requests that come back, and manages the window
// state a frameless player needs: maximize to the work area, borderless
// fullscreen on the window's own monitor, dragging and edge resizing.
//
// Everything here runs on the main OS thread (GLFW's rule); the one
// exception is Wake, which any goroutine may call.
package platform

import (
	"image"
	"runtime"
	"time"
	"unsafe"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"

	"github.com/go-gl/glfw/v3.3/glfw"
	"golang.org/x/image/draw"
)

func init() {
	// GLFW and the GL context must stay on the thread that created
	// them, the main thread.
	runtime.LockOSThread()
}

// Options configure a new window.
type Options struct {
	Title         string
	Width, Height int // in screen coordinates
	// MinWidth and MinHeight bound resizing (0 for none).
	MinWidth, MinHeight int
	Floating            bool // always on top
}

// Window is the player window.
type Window struct {
	win    *glfw.Window
	Router input.Router

	// Handlers, called on the main thread.
	OnDrop    func(paths []string)
	OnKey     func(e key.Event) bool // true when handled; unhandled keys go to the UI
	OnClose   func() bool            // false keeps the window open
	OnResize  func()
	OnFocus   func(focused bool)
	OnRefresh func()
	// CanDrag reports whether a press at p (pixels) on nothing
	// interactive moves the window: the video area, say. The title
	// bar's move area works without it.
	CanDrag func(p f32.Point) bool

	start      time.Time
	cursor     f32.Point // last position, window coordinates
	buttons    pointer.Buttons
	mods       key.Modifiers
	lastCursor pointer.Cursor
	// cursorStale makes the next UI cursor apply even if unchanged,
	// after the window showed a resize cursor of its own.
	cursorStale bool
	cursors     map[glfw.StandardCursor]*glfw.Cursor
	hidden      bool

	// maximized is goplay's own maximize: the window covers the work
	// area of its monitor. GLFW's would cover the taskbar for a
	// frameless window on Windows.
	maximized  bool
	fullscreen bool
	restore    image.Rectangle // window rect to restore, screen coordinates

	drag   dragState
	resize resizeState
	dirty  bool
}

// New creates the window with its GL context current.
func New(o Options) (*Window, error) {
	if err := glfw.Init(); err != nil {
		return nil, err
	}
	contextHints()
	glfw.WindowHint(glfw.Decorated, glfw.False)
	glfw.WindowHint(glfw.Resizable, glfw.True)
	glfw.WindowHint(glfw.ScaleToMonitor, glfw.True)
	glfw.WindowHint(glfw.Visible, glfw.False)
	if o.Floating {
		glfw.WindowHint(glfw.Floating, glfw.True)
	}
	win, err := glfw.CreateWindow(o.Width, o.Height, o.Title, nil, nil)
	if err != nil {
		glfw.Terminate()
		return nil, err
	}
	w := &Window{win: win, start: time.Now(), cursors: map[glfw.StandardCursor]*glfw.Cursor{}, dirty: true}
	win.MakeContextCurrent()
	glfw.SwapInterval(1)
	if o.MinWidth > 0 || o.MinHeight > 0 {
		win.SetSizeLimits(max(o.MinWidth, 1), max(o.MinHeight, 1), glfw.DontCare, glfw.DontCare)
	}
	w.center()
	w.install()
	platformInit(w)
	return w, nil
}

// Show makes the window visible (it is created hidden so it can be
// sized and placed first).
func (w *Window) Show() {
	w.win.Show()
	w.win.Focus()
}

// Destroy closes the window and shuts GLFW down.
func (w *Window) Destroy() {
	for _, c := range w.cursors {
		c.Destroy()
	}
	w.win.Destroy()
	glfw.Terminate()
}

// GetProcAddress resolves OpenGL functions of the window's context.
func GetProcAddress(name string) unsafe.Pointer { return glfw.GetProcAddress(name) }

// Wake interrupts Wait from any goroutine, e.g. when mpv has events or a
// new video frame.
func Wake() { glfw.PostEmptyEvent() }

// Wait blocks until input arrives, Wake is called, or the deadline
// passes (zero waits indefinitely), then processes window events.
func (w *Window) Wait(deadline time.Time) {
	switch {
	case deadline.IsZero():
		glfw.WaitEvents()
	default:
		d := time.Until(deadline)
		if d <= 0 {
			glfw.PollEvents()
		} else {
			glfw.WaitEventsTimeout(d.Seconds())
		}
	}
}

// Poll processes waiting window events without blocking.
func (w *Window) Poll() { glfw.PollEvents() }

// ShouldClose reports whether the window was asked to close.
func (w *Window) ShouldClose() bool { return w.win.ShouldClose() }

// Close asks the window to close.
func (w *Window) Close() { w.win.SetShouldClose(true) }

// Swap presents the frame.
func (w *Window) Swap() { w.win.SwapBuffers() }

// TakeDirty reports whether input or a window change since the last call
// means the UI should be redrawn.
func (w *Window) TakeDirty() bool {
	d := w.dirty
	w.dirty = false
	return d
}

// MarkDirty requests a redraw.
func (w *Window) MarkDirty() { w.dirty = true }

// FramebufferSize is the drawable size in pixels.
func (w *Window) FramebufferSize() image.Point {
	fw, fh := w.win.GetFramebufferSize()
	return image.Pt(fw, fh)
}

// Scale is pixels per dp: the monitor's content scale.
func (w *Window) Scale() float32 {
	sx, _ := w.win.GetContentScale()
	if sx <= 0 {
		return 1
	}
	return sx
}

// pxPerScreen is how many framebuffer pixels one screen coordinate spans:
// 2 on a Retina Mac, where screen coordinates are points; 1 on Windows,
// whatever the content scale.
func (w *Window) pxPerScreen() float32 {
	ww, _ := w.win.GetSize()
	fw, _ := w.win.GetFramebufferSize()
	if ww <= 0 || fw <= 0 {
		return 1
	}
	return float32(fw) / float32(ww)
}

// ToPx converts window screen coordinates to framebuffer pixels.
func (w *Window) ToPx(p f32.Point) f32.Point {
	r := w.pxPerScreen()
	return f32.Pt(p.X*r, p.Y*r)
}

// ScreenToPx converts a length in screen coordinates to pixels.
func (w *Window) ScreenToPx(v float32) float32 { return v * w.pxPerScreen() }

// Cursor position in framebuffer pixels.
func (w *Window) CursorPx() f32.Point { return w.ToPx(w.cursor) }

// SetTitle sets the title the OS shows (taskbar, Mission Control).
func (w *Window) SetTitle(t string) { w.win.SetTitle(t) }

// Focused reports whether the window has keyboard focus.
func (w *Window) Focused() bool { return w.win.GetAttrib(glfw.Focused) == glfw.True }

// SetFloating keeps the window above others.
func (w *Window) SetFloating(on bool) { w.win.SetAttrib(glfw.Floating, glfwBool(on)) }

// Floating reports whether the window stays on top.
func (w *Window) Floating() bool { return w.win.GetAttrib(glfw.Floating) == glfw.True }

// Minimize iconifies the window.
func (w *Window) Minimize() { w.win.Iconify() }

// Minimized reports whether the window is iconified.
func (w *Window) Minimized() bool { return w.win.GetAttrib(glfw.Iconified) == glfw.True }

// HideCursor hides the mouse pointer over the window (shown again by
// any movement in the app's handling).
func (w *Window) HideCursor(hide bool) {
	if hide == w.hidden {
		return
	}
	w.hidden = hide
	mode := glfw.CursorNormal
	if hide {
		mode = glfw.CursorHidden
	}
	w.win.SetInputMode(glfw.CursorMode, mode)
}

// Clipboard returns the system clipboard's text.
func (w *Window) Clipboard() string { return glfw.GetClipboardString() }

// SetClipboard puts text on the system clipboard.
func (w *Window) SetClipboard(s string) { glfw.SetClipboardString(s) }

// Bounds is the window rectangle in screen coordinates.
func (w *Window) Bounds() image.Rectangle {
	x, y := w.win.GetPos()
	ww, wh := w.win.GetSize()
	return image.Rect(x, y, x+ww, y+wh)
}

// SetBounds moves and resizes the window, in screen coordinates.
func (w *Window) SetBounds(r image.Rectangle) {
	w.win.SetPos(r.Min.X, r.Min.Y)
	w.win.SetSize(r.Dx(), r.Dy())
}

// SetSize resizes the window around its center, keeping it on its
// monitor's work area.
func (w *Window) SetSize(width, height int) {
	if w.maximized || w.fullscreen {
		w.restore = centered(w.restore, width, height)
		return
	}
	work := workArea(monitorOf(w.Bounds()))
	r := centered(w.Bounds(), width, height)
	w.SetBounds(fitInto(r, work))
}

// Maximized reports goplay's maximized state.
func (w *Window) Maximized() bool { return w.maximized }

// Fullscreen reports whether the window covers its monitor.
func (w *Window) Fullscreen() bool { return w.fullscreen }

// ToggleMaximize fills the monitor's work area, or restores.
func (w *Window) ToggleMaximize() { w.SetMaximized(!w.maximized) }

// SetMaximized maximizes to the work area of the window's monitor, or
// restores the previous size.
func (w *Window) SetMaximized(on bool) {
	if w.fullscreen || on == w.maximized {
		return
	}
	if on {
		w.restore = w.Bounds()
		w.SetBounds(workArea(monitorOf(w.restore)))
	} else {
		w.SetBounds(w.restore)
	}
	w.maximized = on
	w.dirty = true
}

// SetFullscreen covers the window's whole monitor without changing its
// display mode, or restores. On macOS it uses the system's fullscreen.
func (w *Window) SetFullscreen(on bool) {
	if on == w.fullscreen {
		return
	}
	if nativeFullscreen() {
		w.setNativeFullscreen(on)
		w.fullscreen = on
		w.dirty = true
		return
	}
	if on {
		if !w.maximized {
			w.restore = w.Bounds()
		}
		w.SetBounds(monitorBounds(monitorOf(w.Bounds())))
	} else if w.maximized {
		w.SetBounds(workArea(monitorOf(w.Bounds())))
	} else {
		w.SetBounds(w.restore)
	}
	w.fullscreen = on
	w.dirty = true
}

// center places the window in the middle of the primary monitor's work
// area.
func (w *Window) center() {
	m := glfw.GetPrimaryMonitor()
	if m == nil {
		return
	}
	work := workArea(m)
	w.SetBounds(fitInto(centered(work, w.Bounds().Dx(), w.Bounds().Dy()), work))
}

// install wires the GLFW callbacks.
func (w *Window) install() {
	w.win.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		w.cursor = f32.Pt(float32(x), float32(y))
		w.dirty = true
		if w.trackDragResize() {
			return
		}
		w.updateResizeCursor()
		w.Router.Queue(pointer.Event{
			Kind:      w.moveKind(),
			Source:    pointer.Mouse,
			Time:      w.now(),
			Position:  w.ToPx(w.cursor),
			Buttons:   w.buttons,
			Modifiers: w.mods,
		})
	})
	w.win.SetCursorEnterCallback(func(_ *glfw.Window, entered bool) {
		w.dirty = true
		if !entered {
			w.Router.Queue(pointer.Event{
				Kind:     pointer.Leave,
				Source:   pointer.Mouse,
				Time:     w.now(),
				Position: w.ToPx(w.cursor),
			})
		}
	})
	w.win.SetMouseButtonCallback(func(_ *glfw.Window, b glfw.MouseButton, a glfw.Action, m glfw.ModifierKey) {
		w.dirty = true
		w.mods = modifiers(m)
		var btn pointer.Buttons
		switch b {
		case glfw.MouseButtonLeft:
			btn = pointer.ButtonPrimary
		case glfw.MouseButtonRight:
			btn = pointer.ButtonSecondary
		case glfw.MouseButtonMiddle:
			btn = pointer.ButtonTertiary
		default:
			return
		}
		if a == glfw.Press && btn == pointer.ButtonPrimary && w.beginResizeOrDrag() {
			return
		}
		if a == glfw.Release && btn == pointer.ButtonPrimary {
			taken := w.drag.active || w.resize.active
			w.drag, w.resize = dragState{}, resizeState{}
			if taken {
				return
			}
		}
		kind := pointer.Press
		if a == glfw.Release {
			kind = pointer.Release
			w.buttons &^= btn
		} else {
			w.buttons |= btn
		}
		w.Router.Queue(pointer.Event{
			Kind:      kind,
			Source:    pointer.Mouse,
			Time:      w.now(),
			Position:  w.ToPx(w.cursor),
			Buttons:   w.buttons,
			Modifiers: w.mods,
		})
	})
	w.win.SetScrollCallback(func(_ *glfw.Window, dx, dy float64) {
		w.dirty = true
		// GLFW reports wheel notches (or trackpad deltas) up-positive;
		// Gio scrolls in pixels, down-positive.
		step := w.ScreenToPx(48)
		w.Router.Queue(pointer.Event{
			Kind:      pointer.Scroll,
			Source:    pointer.Mouse,
			Time:      w.now(),
			Position:  w.ToPx(w.cursor),
			Buttons:   w.buttons,
			Scroll:    f32.Pt(float32(-dx)*step, float32(-dy)*step),
			Modifiers: w.mods,
		})
	})
	w.win.SetKeyCallback(func(_ *glfw.Window, k glfw.Key, _ int, a glfw.Action, m glfw.ModifierKey) {
		w.dirty = true
		w.mods = modifiers(m)
		name, ok := keyName(k)
		if !ok {
			return
		}
		state := key.Press
		if a == glfw.Release {
			state = key.Release
		}
		e := key.Event{Name: name, Modifiers: w.mods, State: state}
		// The UI gets first refusal while a text field has focus, so
		// typing a letter doesn't also trigger its shortcut.
		if w.textFocused() || w.OnKey == nil || state == key.Release || !w.OnKey(e) {
			w.Router.Queue(e)
		}
	})
	w.win.SetCharCallback(func(_ *glfw.Window, r rune) {
		w.dirty = true
		if !w.textFocused() {
			return
		}
		st := w.Router.EditorState()
		w.Router.Queue(key.EditEvent{Range: st.Selection.Range, Text: string(r)})
	})
	w.win.SetDropCallback(func(_ *glfw.Window, paths []string) {
		w.dirty = true
		if w.OnDrop != nil {
			w.OnDrop(paths)
		}
	})
	w.win.SetCloseCallback(func(win *glfw.Window) {
		if w.OnClose != nil && !w.OnClose() {
			win.SetShouldClose(false)
		}
	})
	w.win.SetFramebufferSizeCallback(func(*glfw.Window, int, int) {
		w.dirty = true
		if w.OnResize != nil {
			w.OnResize()
		}
	})
	w.win.SetContentScaleCallback(func(*glfw.Window, float32, float32) { w.dirty = true })
	w.win.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		w.dirty = true
		if !focused {
			// Releases happen elsewhere: forget held buttons and keys.
			w.buttons, w.mods = 0, 0
		}
		if w.OnFocus != nil {
			w.OnFocus(focused)
		}
	})
	w.win.SetRefreshCallback(func(*glfw.Window) {
		w.dirty = true
		if w.OnRefresh != nil {
			w.OnRefresh()
		}
	})
}

func (w *Window) now() time.Duration { return time.Since(w.start) }

func (w *Window) moveKind() pointer.Kind {
	if w.buttons != 0 {
		return pointer.Drag
	}
	return pointer.Move
}

// textFocused reports whether a text editor has keyboard focus.
func (w *Window) textFocused() bool {
	_, ok := w.Router.TextInputHint()
	return ok
}

// AfterFrame services the router's requests after a frame: clipboard
// reads and writes, and the cursor shape.
func (w *Window) AfterFrame() {
	if mime, content, ok := w.Router.WriteClipboard(); ok && (mime == "application/text" || mime == "text/plain") {
		w.SetClipboard(string(content))
	}
	if w.Router.ClipboardRequested() {
		text := w.Clipboard()
		w.Router.Queue(clipboardEvent(text))
		w.dirty = true
	}
	if !w.drag.active && !w.resize.active && w.resizeEdge(w.cursor) == 0 {
		w.setCursor(w.Router.Cursor())
	}
}

// Wakeup returns when the UI next wants a frame on its own (an
// animation, a timed refresh), if it does.
func (w *Window) Wakeup() (time.Time, bool) { return w.Router.WakeupTime() }

func (w *Window) setCursor(c pointer.Cursor) {
	if c == w.lastCursor && !w.cursorStale {
		return
	}
	w.lastCursor, w.cursorStale = c, false
	shape := glfw.ArrowCursor
	switch c {
	case pointer.CursorPointer, pointer.CursorGrab, pointer.CursorGrabbing:
		shape = glfw.HandCursor
	case pointer.CursorText:
		shape = glfw.IBeamCursor
	case pointer.CursorCrosshair:
		shape = glfw.CrosshairCursor
	case pointer.CursorColResize, pointer.CursorEastResize, pointer.CursorWestResize, pointer.CursorEastWestResize:
		shape = glfw.HResizeCursor
	case pointer.CursorRowResize, pointer.CursorNorthResize, pointer.CursorSouthResize, pointer.CursorNorthSouthResize:
		shape = glfw.VResizeCursor
	case pointer.CursorNone:
		w.HideCursor(true)
		return
	}
	w.applyShape(shape)
}

func (w *Window) applyShape(shape glfw.StandardCursor) {
	gc, ok := w.cursors[shape]
	if !ok {
		gc = glfw.CreateStandardCursor(shape)
		w.cursors[shape] = gc
	}
	w.win.SetCursor(gc)
}

// ActionAt reports the window action the UI marked under p (pixels): a
// title bar's move area, or a widget that blocks dragging.
func (w *Window) ActionAt(p f32.Point) (system.Action, bool) { return w.Router.ActionAt(p) }

func glfwBool(b bool) int {
	if b {
		return glfw.True
	}
	return glfw.False
}

// KeepAwake stops the display from sleeping while on (during playback).
func KeepAwake(on bool) { keepAwake(on) }

// SetIcon sets the window's icon (taskbar, Alt+Tab, window lists) from
// a large square image, scaled to the sizes systems ask for. macOS takes
// the icon from the app bundle instead.
func (w *Window) SetIcon(img image.Image) {
	if img == nil || runtime.GOOS == "darwin" {
		return
	}
	var imgs []image.Image
	for _, s := range []int{256, 64, 48, 32, 16} {
		dst := image.NewRGBA(image.Rect(0, 0, s, s))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
		imgs = append(imgs, dst)
	}
	w.win.SetIcon(imgs)
}
