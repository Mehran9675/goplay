package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"

	"github.com/mehran9675/goplay-ebiten/internal/settings"
)

// flushInterval is how often changed settings are written to disk.
const flushInterval = time.Second

// mpvConfTemplate seeds mpv.conf the first time the user edits it.
const mpvConfTemplate = `# mpv options for goplay, one per line: name=value
# Every option is listed at https://mpv.io/manual/stable/#options
# Changes take effect the next time goplay starts. Options set here
# override goplay's own settings.
#
# Examples:
# sub-font="Segoe UI"
# screenshot-format=png
# deband-iterations=2
`

// loadSettings reads the saved settings; on any error the defaults are
// used and the problem is shown.
func (a *App) loadSettings() {
	path := ""
	if dir, err := settings.ConfigDir(); err == nil {
		path = filepath.Join(dir, "settings.json")
	}
	s, err := settings.Load(path)
	if err != nil {
		a.msg = "Could not read settings, using defaults: " + err.Error()
	}
	a.settings = s
	if s.Bool(settings.KeyRememberVolume) && s.Get(settings.KeyVolume) != "" {
		a.volume = min(1, max(0, s.Float(settings.KeyVolume)))
	}
}

// flushSettings saves changed settings, at most once per flushInterval
// unless final (on exit, when the volume is saved too).
func (a *App) flushSettings(final bool) {
	if final {
		if a.settings.Bool(settings.KeyRememberVolume) {
			a.settings.Set(settings.KeyVolume, strconv.FormatFloat(a.volume, 'f', 3, 64))
		}
	} else if time.Since(a.lastFlush) < flushInterval {
		return
	}
	a.lastFlush = time.Now()
	if err := a.settings.Flush(); err != nil {
		a.msg = "Could not save settings: " + err.Error()
	}
}

// saveResumePosition stores the current file's position for the resume
// setting.
func (a *App) saveResumePosition() {
	if a.mpv != nil && a.hasMedia && !a.finished && a.settings.Bool(settings.KeyResume) {
		a.mpv.Command("write-watch-later-config")
	}
}

func (a *App) Settings() *settings.Store { return a.settings }

// SetSetting stores a setting and applies it right away.
func (a *App) SetSetting(key, value string) {
	if a.settings.Get(key) == value {
		return
	}
	a.settings.Set(key, value)
	a.applySetting(key)
}

// ResetSettings restores every default and applies them.
func (a *App) ResetSettings() {
	a.settings.Reset()
	for i := range settings.Defs {
		a.applySetting(settings.Defs[i].Key)
	}
	a.flushSettings(true)
}

// applySetting puts the stored value of key into effect: the mpv options
// it maps to, or the app state it controls. Settings mpv reads only at
// startup say so instead.
func (a *App) applySetting(key string) {
	d := settings.Lookup(key)
	if d == nil {
		return
	}
	v := a.settings.Get(key)
	if key == settings.KeyAlwaysOnTop && a.window != nil {
		a.window.SetAttrib(glfw.Floating, glfwBool(v == "yes"))
	}
	if d.Restart {
		a.msg = d.Label + ": restart goplay to apply."
		return
	}
	if a.mpv == nil {
		return
	}
	for _, o := range d.MPVOptions(v) {
		if err := a.mpv.SetString(o.Name, o.Value); err != nil {
			a.msg = fmt.Sprintf("%s: mpv rejected %s=%s (%v)", d.Label, o.Name, o.Value, err)
		}
	}
}

func glfwBool(b bool) int {
	if b {
		return glfw.True
	}
	return glfw.False
}

// AudioDevices lists mpv's audio outputs, "auto" first.
func (a *App) AudioDevices() []settings.Option {
	devices := []settings.Option{{Label: "Automatic", Value: "auto"}}
	if a.mpv == nil {
		return devices
	}
	n, _ := a.mpv.GetInt("audio-device-list/count")
	for i := range int(n) {
		p := fmt.Sprintf("audio-device-list/%d/", i)
		name, _ := a.mpv.GetString(p + "name")
		if name == "" || name == "auto" {
			continue
		}
		desc, _ := a.mpv.GetString(p + "description")
		if desc == "" {
			desc = name
		}
		devices = append(devices, settings.Option{Label: desc, Value: name})
	}
	return devices
}

// EditMPVConf opens mpv.conf in a text editor, creating it with a short
// explanation first if it doesn't exist.
func (a *App) EditMPVConf() {
	dir, err := settings.ConfigDir()
	if err != nil {
		a.msg = err.Error()
		return
	}
	path := filepath.Join(dir, "mpv.conf")
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err == nil {
			err = os.WriteFile(path, []byte(mpvConfTemplate), 0o644)
		}
		if err != nil {
			a.msg = "Could not create mpv.conf: " + err.Error()
			return
		}
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad.exe", path)
	case "darwin":
		cmd = exec.Command("open", "-t", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		a.msg = "Could not open " + path + ": " + err.Error()
		return
	}
	go cmd.Wait()
}

// Title is the playing file's name, or "goplay" with nothing loaded.
func (a *App) Title() string {
	if a.title == "" {
		return "goplay"
	}
	return a.title
}

// Window controls, for the UI's title bar.

func (a *App) Minimize() {
	if a.window != nil {
		a.window.Iconify()
	}
}

func (a *App) ToggleMaximize() {
	if a.window == nil || a.ui.Fullscreen {
		return
	}
	if a.Maximized() {
		a.window.Restore()
	} else {
		a.window.Maximize()
	}
}

func (a *App) Close() { a.Quit() }

func (a *App) Maximized() bool {
	return a.window != nil && a.window.GetAttrib(glfw.Maximized) == glfw.True
}

func (a *App) Focused() bool {
	return a.window != nil && a.window.GetAttrib(glfw.Focused) == glfw.True
}
