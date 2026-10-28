//go:build windows

package platform

import (
	"syscall"
	"unsafe"

	"gioui.org/gpu"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Frameless GLFW windows on Windows have no resize border.
const nativeResizeMissing = true

// contextHints asks for OpenGL ES 3.0 through EGL (ANGLE): Gio's GL
// backend on Windows loads ANGLE's libGLESv2.dll, so the window's
// context must come from the same stack. libmpv renders with it too,
// and decodes into D3D11 textures it shares with ANGLE without copies.
func contextHints() {
	glfw.WindowHint(glfw.ContextCreationAPI, glfw.EGLContextAPI)
	glfw.WindowHint(glfw.ClientAPI, glfw.OpenGLESAPI)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 0)
}

// GPU is the Gio GPU API matching the window's context.
func GPU() gpu.API { return gpu.OpenGL{ES: true, Shared: true} }

var (
	dwmapi                = syscall.NewLazyDLL("dwmapi.dll")
	dwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	setThreadExecState    = kernel32.NewProc("SetThreadExecutionState")
)

// platformInit gives the frameless window Windows 11's rounded corners
// and dark frame. Older Windows ignores both.
func platformInit(w *Window) {
	hwnd := uintptr(unsafe.Pointer(w.win.GetWin32Window()))
	if hwnd == 0 || dwmSetWindowAttribute.Find() != nil {
		return
	}
	const (
		dwmwaUseImmersiveDarkMode   = 20
		dwmwaWindowCornerPreference = 33
		dwmwcpRound                 = 2
	)
	round := int32(dwmwcpRound)
	dwmSetWindowAttribute.Call(hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&round)), 4)
	dark := int32(1)
	dwmSetWindowAttribute.Call(hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&dark)), 4)
}

// keepAwake stops the display from sleeping while on, as a video player
// must during playback.
func keepAwake(on bool) {
	const (
		esContinuous      = 0x80000000
		esSystemRequired  = 0x00000001
		esDisplayRequired = 0x00000002
	)
	flags := uintptr(esContinuous)
	if on {
		flags |= esSystemRequired | esDisplayRequired
	}
	setThreadExecState.Call(flags)
}
