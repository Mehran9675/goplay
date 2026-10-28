//go:build darwin

package platform

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework IOKit
#include <stdint.h>

void goplay_style_window(void* nswindow);
void goplay_set_traffic_lights(void* nswindow, int visible);
void goplay_toggle_fullscreen(void* nswindow);
int goplay_is_fullscreen(void* nswindow);
void goplay_install_open_handler(void);
void goplay_keep_awake(int on);
*/
import "C"

import (
	"sync"

	"gioui.org/gpu"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// macOS resizes the window from its edges itself (the window keeps a
// resizable style mask; see goplay_style_window).
const nativeResizeMissing = false

// contextHints asks for a core OpenGL 3.2+ context: macOS's OpenGL,
// which both Gio's GL backend and libmpv's render API use.
func contextHints() {
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 2)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.CocoaRetinaFramebuffer, glfw.True)
}

// GPU is the Gio GPU API matching the window's context.
func GPU() gpu.API { return gpu.OpenGL{ES: false, Shared: true} }

// platformInit turns GLFW's borderless window into a full-size-content
// window with a transparent title bar: native rounded corners, shadow,
// edge resizing and traffic-light buttons, with goplay drawing
// everything else. It also starts listening for files opened from
// Finder.
func platformInit(w *Window) {
	C.goplay_style_window(w.win.GetCocoaWindow())
	C.goplay_install_open_handler()
}

// SetTrafficLights shows or hides the native window buttons (hidden with
// the title bar while the controls are tucked away).
func (w *Window) SetTrafficLights(visible bool) {
	v := C.int(0)
	if visible {
		v = 1
	}
	C.goplay_set_traffic_lights(w.win.GetCocoaWindow(), v)
}

// nativeFullscreen reports that fullscreen uses macOS's own, which
// moves the window to its own Space.
func nativeFullscreen() bool { return true }

func (w *Window) setNativeFullscreen(on bool) {
	isOn := C.goplay_is_fullscreen(w.win.GetCocoaWindow()) != 0
	if isOn != on {
		C.goplay_toggle_fullscreen(w.win.GetCocoaWindow())
	}
}

func keepAwake(on bool) {
	v := C.int(0)
	if on {
		v = 1
	}
	C.goplay_keep_awake(v)
}

var (
	openedMu sync.Mutex
	opened   []string
)

//export goplayOpenedFile
func goplayOpenedFile(path *C.char) {
	openedMu.Lock()
	opened = append(opened, C.GoString(path))
	openedMu.Unlock()
	Wake()
}

// TakeOpenedFiles returns files the OS asked the app to open (Finder's
// Open With, dropping on the Dock icon) since the last call.
func TakeOpenedFiles() []string {
	openedMu.Lock()
	defer openedMu.Unlock()
	out := opened
	opened = nil
	return out
}
