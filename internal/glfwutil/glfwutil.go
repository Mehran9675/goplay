// Package glfwutil exposes a few GLFW helpers: monitor selection for
// fullscreen and work-area queries (on top of the go-gl/glfw bindings), and
// cursor/mouse polling through the current context.
package glfwutil

/*
extern void* glfwGetCurrentContext(void);
extern void glfwGetCursorPos(void* window, double* xpos, double* ypos);
extern int  glfwGetMouseButton(void* window, int button);

static void goplay_get_cursor_pos(double* x, double* y) {
	glfwGetCursorPos(glfwGetCurrentContext(), x, y);
}

static int goplay_mouse_down(int button) {
	return glfwGetMouseButton(glfwGetCurrentContext(), button) == 1;
}
*/
import "C"

import (
	"image"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// MonitorOf returns the monitor showing most of win, falling back to the
// primary monitor when the window is off-screen.
func MonitorOf(win *glfw.Window) *glfw.Monitor {
	wx, wy := win.GetPos()
	ww, wh := win.GetSize()
	r := image.Rect(wx, wy, wx+ww, wy+wh)
	best, bestArea := glfw.GetPrimaryMonitor(), 0
	for _, m := range glfw.GetMonitors() {
		ov := r.Intersect(monitorBounds(m))
		if a := ov.Dx() * ov.Dy(); a > bestArea {
			best, bestArea = m, a
		}
	}
	return best
}

// monitorBounds is m's full area in screen coordinates.
func monitorBounds(m *glfw.Monitor) image.Rectangle {
	x, y := m.GetPos()
	mode := m.GetVideoMode()
	if mode == nil {
		return image.Rectangle{}
	}
	return image.Rect(x, y, x+mode.Width, y+mode.Height)
}

// WorkArea returns the work area (the monitor minus taskbars and docks) of
// the monitor showing win, in screen coordinates.
func WorkArea(win *glfw.Window) (x, y, w, h int) {
	return MonitorOf(win).GetWorkarea()
}

// SetFullscreen switches win between borderless fullscreen and windowed
// mode. Fullscreen covers the whole monitor the window is on. x/y/w/h are
// the windowed-mode restore position and size (only used when leaving
// fullscreen). The window is resized rather than attached to the monitor
// (SetMonitor), which would switch the display mode and make the monitor
// flicker/black out.
func SetFullscreen(win *glfw.Window, on bool, x, y, w, h int) {
	if on {
		b := monitorBounds(MonitorOf(win))
		if b.Empty() {
			return
		}
		win.SetPos(b.Min.X, b.Min.Y)
		win.SetSize(b.Dx(), b.Dy())
		return
	}
	win.SetPos(x, y)
	win.SetSize(w, h)
}

// GLFW mouse button ids.
const (
	MouseButtonLeft = 0
)

// CursorPos returns the cursor position in window coordinates.
func CursorPos() (x, y float64) {
	var cx, cy C.double
	C.goplay_get_cursor_pos(&cx, &cy)
	return float64(cx), float64(cy)
}

// MouseButtonDown reports whether the given GLFW mouse button is held.
func MouseButtonDown(button int) bool {
	return C.goplay_mouse_down(C.int(button)) != 0
}
