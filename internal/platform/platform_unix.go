//go:build !windows && !darwin

package platform

import (
	"os/exec"
	"sync"

	"gioui.org/gpu"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Frameless GLFW windows on X11 have no resize border.
const nativeResizeMissing = true

// contextHints asks for OpenGL ES 3.0 through EGL: Gio's GL backend on
// Linux uses OpenGL ES, and libmpv's render API supports it (including
// zero-copy VA-API decoding through EGL).
func contextHints() {
	glfw.WindowHint(glfw.ContextCreationAPI, glfw.EGLContextAPI)
	glfw.WindowHint(glfw.ClientAPI, glfw.OpenGLESAPI)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 0)
}

// GPU is the Gio GPU API matching the window's context.
func GPU() gpu.API { return gpu.OpenGL{ES: true, Shared: true} }

func platformInit(*Window) {}

var (
	inhibitMu  sync.Mutex
	inhibitCmd *exec.Cmd
)

// keepAwake stops the screen from blanking during playback, through
// systemd-logind's idle inhibitor where available.
func keepAwake(on bool) {
	inhibitMu.Lock()
	defer inhibitMu.Unlock()
	if on == (inhibitCmd != nil) {
		return
	}
	if !on {
		inhibitCmd.Process.Kill()
		inhibitCmd.Wait()
		inhibitCmd = nil
		return
	}
	path, err := exec.LookPath("systemd-inhibit")
	if err != nil {
		return
	}
	cmd := exec.Command(path, "--what=idle", "--who=goplay", "--why=Playing video", "--mode=block", "sleep", "infinity")
	if cmd.Start() == nil {
		inhibitCmd = cmd
	}
}
