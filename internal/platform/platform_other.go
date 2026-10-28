//go:build !darwin

package platform

// Outside macOS, fullscreen covers the monitor with the frameless window
// itself, and there are no native window buttons or open-file events.

func nativeFullscreen() bool { return false }

func (w *Window) setNativeFullscreen(bool) {}

// SetTrafficLights is macOS only.
func (w *Window) SetTrafficLights(bool) {}

// TakeOpenedFiles is macOS only: elsewhere files to open arrive as
// command-line arguments.
func TakeOpenedFiles() []string { return nil }
