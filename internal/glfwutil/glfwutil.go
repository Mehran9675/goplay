// Package glfwutil exposes a few GLFW functions that cimgui-go's glfwbackend
// does not wrap: fullscreen toggling and monitor work-area queries. The GLFW
// symbols are statically linked by glfwbackend, so they are only referenced
// here (no extra -l flags needed).
package glfwutil

/*
extern void* glfwGetCurrentContext(void);
extern void* glfwGetPrimaryMonitor(void);
extern void glfwGetMonitorWorkarea(void* monitor, int* xpos, int* ypos, int* width, int* height);
extern void glfwSetWindowPos(void* window, int xpos, int ypos);
extern void glfwSetWindowSize(void* window, int width, int height);
extern void glfwGetCursorPos(void* window, double* xpos, double* ypos);
extern int  glfwGetMouseButton(void* window, int button);

static void goplay_get_workarea(int* x, int* y, int* w, int* h) {
	glfwGetMonitorWorkarea(glfwGetPrimaryMonitor(), x, y, w, h);
}

// Borderless fullscreen: resize/position the window over the monitor's work
// area instead of attaching it to the monitor. Attaching (glfwSetWindowMonitor)
// switches the display mode, which makes the monitor flicker/black out.
static void goplay_set_fullscreen(int on, int x, int y, int w, int h) {
	void* win = glfwGetCurrentContext();
	if (on) {
		int mx = 0, my = 0, mw = 0, mh = 0;
		glfwGetMonitorWorkarea(glfwGetPrimaryMonitor(), &mx, &my, &mw, &mh);
		glfwSetWindowPos(win, mx, my);
		glfwSetWindowSize(win, mw, mh);
	} else {
		glfwSetWindowPos(win, x, y);
		glfwSetWindowSize(win, w, h);
	}
}

static void goplay_get_cursor_pos(double* x, double* y) {
	glfwGetCursorPos(glfwGetCurrentContext(), x, y);
}

static int goplay_mouse_down(int button) {
	return glfwGetMouseButton(glfwGetCurrentContext(), button) == 1;
}
*/
import "C"

// WorkArea returns the primary monitor's work area in screen coordinates.
func WorkArea() (x, y, w, h int) {
	var cx, cy, cw, ch C.int
	C.goplay_get_workarea(&cx, &cy, &cw, &ch)
	return int(cx), int(cy), int(cw), int(ch)
}

// SetFullscreen switches the current window between borderless fullscreen and
// windowed mode. x/y/w/h are the windowed-mode restore position and size (only
// used when leaving fullscreen). The display mode is never changed, so the
// monitor does not flicker.
func SetFullscreen(on bool, x, y, w, h int) {
	onI := 0
	if on {
		onI = 1
	}
	C.goplay_set_fullscreen(C.int(onI), C.int(x), C.int(y), C.int(w), C.int(h))
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
