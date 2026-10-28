// Package app is goplay itself: it wires the player (mpv), the window
// and the Giode UI together, and holds the views.
//
// Files: app.go (the window and frames), commands.go (every command),
// media.go (opening files and URLs), stereo.go (3D and 360°), casting.go
// (Chromecast and DLNA), theme.go and icons.go (look), and view_*.go
// (the UI).
//
// The window is a giode.App: mpv draws each video frame in its
// underlay, straight into the window's OpenGL framebuffer, and the UI is
// drawn over it.
//
// Threading: everything here runs on the UI goroutine, except the
// underlay (drawVideo), which runs on the window's render thread and
// only touches the render context. mpv's threads and background
// goroutines hand work to the UI goroutine with Post.
package app

import (
	"fmt"
	"image"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"

	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/elements"

	"github.com/mehran9675/goplay/internal/actions"
	"github.com/mehran9675/goplay/internal/library"
	"github.com/mehran9675/goplay/internal/mpv"
	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

// Version is goplay's version, set at build time with -ldflags.
var Version = "dev"

// App is the running player.
type App struct {
	g        *giode.App
	player   *player.Player
	settings *settings.Store
	history  *library.History
	actions  *actions.Registry
	ui       *ui

	configDir string

	// The render context lives on the render thread (see drawVideo).
	rend       *mpv.RenderContext
	rendFailed bool
	// showVideo tells the render thread whether to draw mpv's picture:
	// not while idle or casting, when the UI paints its own background.
	showVideo atomic.Bool

	// mpvPending is set while a Post to process mpv's events is queued.
	mpvPending atomic.Bool

	// dirty asks for another frame once this one is done.
	dirty bool

	// The window: its title as last set and its size in pixels.
	title   string
	winSize image.Point

	// starting runs once the video output is ready (or failed): the files
	// and commands from the command line. mpv turns video off for a file
	// that starts before it.
	starting []func()
	ready    bool

	// pausedHidden is set when minimizing paused playback, to resume it
	// on the way back.
	pausedHidden bool

	logMu sync.Mutex
	log   []string // mpv log lines, newest last

	stereo stereoState
	cast   castState

	// lastPath and lastPos remember the file being played, for the
	// recent list when it ends.
	lastPath  string
	lastTitle string

	// startupCommands run once the first file has loaded (--run).
	startupCommands []string
}

// New sets up the player and window. files are opened, then commands
// (action IDs) run, once the window shows. icon is the window icon, as
// PNG.
func New(files, commands []string, icon []byte) (*App, error) {
	// Most of a video player's memory is mpv's; keep the Go side lean.
	debug.SetGCPercent(50)

	a := &App{actions: actions.NewRegistry()}
	dir, err := settings.ConfigDir()
	if err == nil {
		a.configDir = dir
	}
	a.loadPrefs()

	a.g = giode.New(giode.Config{
		Title:       "GoPlay",
		Size:        image.Pt(960, 600),
		MinSize:     image.Pt(360, 240),
		Resizable:   true,
		Frameless:   true,
		Center:      true,
		AlwaysOnTop: a.settings.Bool(settings.KeyAlwaysOnTop),
		Background:  colBackground,
		Icon:        icon,
		Renderer:    giode.RendererOpenGL,
		Underlay:    a.drawVideo,
		// mpv's render context must be freed while the GL context is
		// current.
		UnderlayRelease: a.releaseVideo,
		OnEvent:         a.windowEvent,
		OnFileDrop:      a.dropped,
		OnOpenURL:       a.openedURL,
		OnContextAction: func(_ string, paths []string) {
			a.whenReady(func() { a.openItems(paths, false) })
		},
		OnStartup: func() {
			if runtime.GOOS == "linux" {
				// Linux file managers find "Play with GoPlay" in the
				// user's home; the other systems' installers add it.
				a.g.InstallContextActions()
			}
		},
		OnWindowStateChange: a.windowStateChanged,
		OnShutdown:          a.shutdown,
	})

	if err := a.startPlayer(); err != nil {
		return nil, err
	}

	setupTheme()
	a.registerCommands()
	a.applyKeyBindings()
	a.ui = newUI(a)

	if len(files) > 0 {
		a.starting = append(a.starting, func() { a.openItems(files, false) })
	}
	switch {
	case len(commands) > 0 && len(files) > 0:
		// Commands that act on the file wait for it to load.
		a.startupCommands = commands
	case len(commands) > 0:
		a.starting = append(a.starting, func() { a.runCommands(commands) })
	}
	return a, nil
}

func (a *App) loadPrefs() {
	path, histPath := "", ""
	if a.configDir != "" {
		path = filepath.Join(a.configDir, "settings.json")
		histPath = filepath.Join(a.configDir, "recent.json")
	}
	s, err := settings.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "GoPlay: settings:", err)
	}
	a.settings = s
	h, err := library.LoadHistory(histPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "GoPlay: recent files:", err)
	}
	a.history = h
}

// startPlayer creates mpv with the user's settings. Its render context
// is made on the render thread, on the first frame (drawVideo).
func (a *App) startPlayer() error {
	var opts []player.Option
	for _, o := range a.settings.MPVOptions() {
		opts = append(opts, player.Option{Name: o.Name, Value: o.Value})
	}
	if a.settings.Bool(settings.KeyRememberVolume) && a.settings.Get(settings.KeyVolume) != "" {
		opts = append(opts, player.Option{Name: "volume", Value: a.settings.Get(settings.KeyVolume)})
	}
	p, err := player.New(player.Config{
		Options:    opts,
		ConfigDir:  a.configDir,
		LoadConfig: a.settings.Bool(settings.KeyMPVConf),
		LogLevel:   a.settings.Get(settings.KeyLogLevel),
		Wakeup: func() {
			if !a.mpvPending.Swap(true) {
				a.g.Post(a.processPlayer)
			}
		},
	})
	var optErr *player.OptionError
	switch {
	case p == nil:
		return fmt.Errorf("mpv: %w", err)
	case err != nil && asOptionError(err, &optErr):
		a.starting = append(a.starting, func() { a.toastError(optErr.Error()) })
	}
	a.player = p
	p.OnLog = a.appendLog
	p.OnFileLoaded = a.fileLoaded
	p.OnEndFile = a.fileEnded
	return nil
}

func asOptionError(err error, target **player.OptionError) bool {
	oe, ok := err.(*player.OptionError)
	if ok {
		*target = oe
	}
	return ok
}

// Post runs fn on the UI goroutine before the next frame. It is safe
// from any goroutine and doesn't wait.
func (a *App) Post(fn func()) { a.g.Post(fn) }

// Run shows the window and runs until it closes; the process ends
// with it.
func (a *App) Run() error {
	return a.g.Run(func() giode.Element { return elements.Raw(a.frame) })
}

// processPlayer handles mpv's queued events.
func (a *App) processPlayer() {
	a.mpvPending.Store(false)
	if a.player.Process() {
		a.afterPlayerChange()
		a.dirty = true
	}
}

// windowEvent follows the window's size, for picture in picture and
// fitting the window to a video.
func (a *App) windowEvent(e event.Event) {
	if c, ok := e.(gioapp.ConfigEvent); ok {
		a.winSize = c.Config.Size
	}
}

// windowStateChanged pauses playback while the window is minimized, if
// the user asked for that, and resumes it after.
func (a *App) windowStateChanged(s giode.WindowState) {
	st := &a.player.State
	switch {
	case s == giode.WindowMinimized:
		if a.settings.Bool(settings.KeyPauseMinimized) && !st.Idle && !st.Paused && !a.casting() {
			a.player.SetPause(true)
			a.pausedHidden = true
		}
	case a.pausedHidden:
		a.pausedHidden = false
		a.player.SetPause(false)
	}
	a.dirty = true
}

// openedURL opens what macOS hands the app: files opened with it from
// Finder arrive as file:// URLs.
func (a *App) openedURL(u *url.URL) {
	item := u.String()
	if u.Scheme == "file" {
		item = u.Path
	}
	a.whenReady(func() { a.openItems([]string{item}, false) })
}

// whenReady runs fn now if the video output is ready, or once it is.
func (a *App) whenReady(fn func()) {
	if a.ready {
		fn()
		return
	}
	a.starting = append(a.starting, fn)
}

// drawVideo is the window's underlay: mpv draws the current video frame
// into the window's framebuffer, before the UI is drawn over it. It runs
// on the render thread with the window's OpenGL context current, where
// the render context is made and used.
func (a *App) drawVideo(t giode.GPUTarget) {
	if a.rend == nil {
		if a.rendFailed {
			return
		}
		rend, err := a.player.Handle().NewRenderContext(t.GetProcAddress)
		if err != nil {
			a.rendFailed = true
			a.appendLog("error", "render", err.Error())
			a.Post(func() {
				a.toastError("Couldn't start video output: " + err.Error())
				a.runStarting()
			})
			return
		}
		rend.SetUpdateCallback(a.g.Invalidate)
		a.rend = rend
		a.Post(a.runStarting)
	}
	a.rend.Update()
	if !a.showVideo.Load() || t.Size.X <= 0 || t.Size.Y <= 0 {
		return
	}
	// GL's origin is the bottom left, the window's the top left.
	if err := a.rend.Render(int(t.Framebuffer), t.Size.X, t.Size.Y, true); err != nil {
		a.appendLog("error", "render", err.Error())
	}
}

// releaseVideo frees mpv's render context on the render thread, before
// the window's GL context goes away. If the context comes back,
// drawVideo makes a new one.
func (a *App) releaseVideo() {
	if a.rend != nil {
		a.rend.Free()
		a.rend = nil
	}
}

// runStarting runs what waited for the video output.
func (a *App) runStarting() {
	a.ready = true
	fns := a.starting
	a.starting = nil
	for _, fn := range fns {
		fn()
	}
	a.dirty = true
}

// frame lays out one frame of the UI.
func (a *App) frame(gtx layout.Context) layout.Dimensions {
	fb := gtx.Constraints.Max
	a.ui.beginFrame(fb, gtx.Metric.PxPerDp)
	a.updateVideoMargins(fb)
	st := &a.player.State
	a.showVideo.Store(!st.Idle && !a.casting())
	a.handleKeys(gtx)
	a.ui.layout(gtx)

	if a.dirty {
		a.dirty = false
		gtx.Execute(op.InvalidateCmd{})
	} else if t := a.ui.nextTimer(); !t.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: t})
	}
	return layout.Dimensions{Size: fb}
}

// updateVideoMargins keeps mpv's picture out of the title bar's and
// control bar's strips when they're beside the video (a normal window);
// maximized and fullscreen, the bars float over it.
func (a *App) updateVideoMargins(fb image.Point) {
	top, bottom := 0.0, 0.0
	if !a.ui.overlay() {
		top = float64(a.ui.titleHeightPx()) / float64(fb.Y)
		bottom = float64(a.ui.barHeightPx()) / float64(fb.Y)
		if top+bottom > 0.8 {
			top, bottom = 0, 0
		}
	}
	if top != a.ui.marginTop {
		a.player.Handle().SetDouble("video-margin-ratio-top", top)
		a.ui.marginTop = top
	}
	if bottom != a.ui.marginBottom {
		a.player.Handle().SetDouble("video-margin-ratio-bottom", bottom)
		a.ui.marginBottom = bottom
		// Subtitles go in the picture's black bars when there are any,
		// but the margins goplay adds are covered by its own bars.
		a.player.Handle().SetFlag("sub-use-margins", bottom == 0)
	}
}

// handleKeys takes the keys nothing in the UI took: shortcuts, and
// otherwise mpv's own bindings.
func (a *App) handleKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(key.Filter{Optional: key.ModCtrl | key.ModShift | key.ModAlt | key.ModSuper | key.ModCommand})
		if !ok {
			return
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press && a.handleKey(e) {
			a.dirty = true
		}
	}
}

// handleKey runs the shortcut for a key press. It reports whether the
// key was used.
func (a *App) handleKey(e key.Event) bool {
	if a.ui.capturingKey() {
		a.ui.captureKey(e)
		return true
	}
	if a.ui.modalOpen() && e.Name != key.NameEscape {
		return false
	}
	if e.Name == key.NameEscape {
		return a.escape()
	}
	if act := a.actions.Lookup(e); act != nil {
		a.actions.Run(act.ID)
		return true
	}
	if a.settings.Bool(settings.KeyPassKeys) && !a.player.State.Idle && isPlainKey(e) {
		a.player.Keypress(actions.MPVKey(e))
		return true
	}
	return false
}

// isPlainKey reports whether a key is worth forwarding to mpv: not a
// lone modifier, Tab or Enter (which move focus and activate UI).
func isPlainKey(e key.Event) bool {
	switch e.Name {
	case key.NameTab, key.NameReturn, key.NameEnter, key.NameCtrl, key.NameShift, key.NameAlt, key.NameSuper, key.NameCommand:
		return false
	}
	return true
}

// escape closes the innermost thing open: a menu, dialog or panel, then
// fullscreen.
func (a *App) escape() bool {
	switch {
	case a.ui.closeTopmost():
		return true
	case a.g.Fullscreen():
		a.setFullscreen(false)
		return true
	case a.ui.pip:
		a.togglePiP()
		return true
	}
	return false
}

// afterPlayerChange reacts to state changes: keeping the display awake
// while playing, the window title, remembering progress.
func (a *App) afterPlayerChange() {
	st := &a.player.State
	playing := !st.Idle && !st.Paused && !a.casting()
	a.g.KeepScreenOn(playing && st.HasVideo())
	title := "GoPlay"
	if !st.Idle && st.Title != "" {
		title = st.Title + " — GoPlay"
	}
	if title != a.title {
		a.title = title
		a.g.SetTitle(title)
	}
	if !st.Idle && st.Path != "" {
		a.lastPath, a.lastTitle = st.Path, st.Title
	}
}

// fileLoaded runs when mpv starts playing a file.
func (a *App) fileLoaded() {
	st := &a.player.State
	a.history.Played(library.Recent{Path: st.Path, Title: titleFor(st)})
	a.history.Save()
	a.stereoFileLoaded()
	a.ui.fileLoaded()
	if cmds := a.startupCommands; cmds != nil {
		a.startupCommands = nil
		a.Post(func() { a.runCommands(cmds) })
	}
}

// fileEnded records how far the file got, and reports failures.
func (a *App) fileEnded(reason int, err error) {
	st := &a.player.State
	if a.lastPath != "" && st.Duration > 0 {
		pos := st.Position
		if reason == mpv.EndEOF {
			pos = st.Duration
		}
		a.history.Update(a.lastPath, a.lastTitle, pos, st.Duration)
		a.history.Save()
	}
	if cmds := a.startupCommands; cmds != nil && reason == mpv.EndError {
		a.startupCommands = nil
		a.Post(func() { a.runCommands(cmds) })
	}
	if reason == mpv.EndError {
		name := filepath.Base(a.player.State.Path)
		if name == "." || name == "" {
			name = "the file"
		}
		msg := "Couldn't play " + name
		if err != nil {
			msg += ": " + err.Error()
		}
		a.toastError(msg)
	}
}

func titleFor(st *player.State) string {
	if st.Title != "" && st.Title != filepath.Base(st.Path) {
		return st.Title
	}
	return ""
}

// appendLog keeps the last mpv log lines for the log window.
func (a *App) appendLog(level, prefix, text string) {
	line := "[" + prefix + "] " + level + ": " + trimNewline(text)
	a.logMu.Lock()
	a.log = append(a.log, line)
	if len(a.log) > 2000 {
		a.log = append(a.log[:0], a.log[len(a.log)-1500:]...)
	}
	a.logMu.Unlock()
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func (a *App) logLines() []string {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	return append([]string(nil), a.log...)
}

// shutdown saves state once the window has closed. mpv is left to the
// process's exit: its render context belonged to the window's GL
// context, which is gone, and the core mustn't outlive it.
func (a *App) shutdown() {
	st := &a.player.State
	if a.lastPath != "" && st.Duration > 0 {
		a.history.Update(a.lastPath, a.lastTitle, st.Position, st.Duration)
	}
	a.history.Save()
	if a.settings.Bool(settings.KeyRememberVolume) && st.Volume > 0 {
		a.settings.Set(settings.KeyVolume, strconv.FormatFloat(st.Volume, 'f', 0, 64))
	}
	a.settings.Flush()
	a.stopCasting()
	a.player.SaveResumePosition()
}

// runCommands runs commands by ID (from --run).
func (a *App) runCommands(ids []string) {
	for _, id := range ids {
		if a.actions.Get(id) == nil {
			a.toastError("Unknown command: " + id)
			continue
		}
		a.actions.Run(id)
	}
	a.dirty = true
}

// Quit closes the window, which ends the app.
func (a *App) Quit() { a.g.Close() }

// windowCtl lets the title bar drive the window.
type windowCtl struct{ a *App }

func (w windowCtl) Minimize() { w.a.g.Minimize() }
func (w windowCtl) ToggleMaximize() {
	if w.a.ui.pip {
		w.a.togglePiP()
	}
	if w.a.g.Fullscreen() {
		w.a.setFullscreen(false)
		return
	}
	w.a.g.ToggleMaximize()
}
func (w windowCtl) Close()          { w.a.Quit() }
func (w windowCtl) Maximized() bool { return w.a.g.Maximized() || w.a.g.Fullscreen() }
func (w windowCtl) Focused() bool   { return w.a.g.Focused() }
