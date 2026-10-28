// Package app is goplay itself: it wires the player (mpv), the window
// and the Giode UI together, runs the event loop, and holds the views.
//
// Files: app.go (loop and rendering), commands.go (every command),
// media.go (opening files and URLs), stereo.go (3D and 360°), casting.go
// (Chromecast and DLNA), theme.go and icons.go (look), and view_*.go
// (the UI).
//
// Threading: everything here runs on the main thread, the UI goroutine.
// mpv's threads and background goroutines only wake it (platform.Wake)
// or hand it work with Post.
package app

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/gpu"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/state"

	"github.com/mehran9675/goplay/internal/actions"
	"github.com/mehran9675/goplay/internal/library"
	"github.com/mehran9675/goplay/internal/mpv"
	"github.com/mehran9675/goplay/internal/platform"
	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

// Version is goplay's version, set at build time with -ldflags.
var Version = "dev"

// App is the running player.
type App struct {
	win      *platform.Window
	player   *player.Player
	rend     *mpv.RenderContext
	gpu      gpu.GPU
	ops      op.Ops
	settings *settings.Store
	history  *library.History
	actions  *actions.Registry
	ui       *ui

	configDir string

	// Wakeups from other threads.
	mpvEvents  atomic.Bool // mpv has events waiting
	newFrame   atomic.Bool // mpv's render context wants a redraw
	invalidate atomic.Bool // a Giode State or a background task changed something

	postMu sync.Mutex
	posted []func()

	// Things that need a redraw on the UI goroutine.
	dirty bool

	logMu sync.Mutex
	log   []string // mpv log lines, newest last

	stereo stereoState
	cast   castState

	// lastPath and lastPos remember the file being played, for the
	// recent list when it ends.
	lastPath  string
	lastTitle string

	quitting bool

	// startupCommands run once the first file has loaded (--run).
	startupCommands []string
}

// New sets up the player and window. files are opened, then commands
// (action IDs) run, once the window shows.
func New(files, commands []string) (*App, error) {
	// Most of a video player's memory is mpv's; keep the Go side lean.
	debug.SetGCPercent(50)

	a := &App{actions: actions.NewRegistry()}
	dir, err := settings.ConfigDir()
	if err == nil {
		a.configDir = dir
	}
	a.loadPrefs()

	win, err := platform.New(platform.Options{
		Title:     "goplay",
		Width:     960,
		Height:    600,
		MinWidth:  360,
		MinHeight: 240,
		Floating:  a.settings.Bool(settings.KeyAlwaysOnTop),
	})
	if err != nil {
		return nil, fmt.Errorf("window: %w", err)
	}
	a.win = win
	state.SetInvalidator(func() {
		a.invalidate.Store(true)
		platform.Wake()
	})

	if err := a.startPlayer(); err != nil {
		win.Destroy()
		return nil, err
	}
	g, err := gpu.New(platform.GPU())
	if err != nil {
		a.shutdownPlayer()
		win.Destroy()
		return nil, fmt.Errorf("gpu: %w", err)
	}
	a.gpu = g

	setupTheme()
	a.registerCommands()
	a.applyKeyBindings()
	a.ui = newUI(a)
	a.installHandlers()

	if len(files) > 0 {
		a.Post(func() { a.openItems(files, false) })
	}
	switch {
	case len(commands) > 0 && len(files) > 0:
		// Commands that act on the file wait for it to load.
		a.startupCommands = commands
	case len(commands) > 0:
		a.Post(func() { a.runCommands(commands) })
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
		fmt.Fprintln(os.Stderr, "goplay: settings:", err)
	}
	a.settings = s
	h, err := library.LoadHistory(histPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goplay: recent files:", err)
	}
	a.history = h
}

// startPlayer creates mpv with the user's settings and its OpenGL render
// context in the window.
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
			a.mpvEvents.Store(true)
			platform.Wake()
		},
	})
	var optErr *player.OptionError
	switch {
	case p == nil:
		return fmt.Errorf("mpv: %w", err)
	case err != nil && asOptionError(err, &optErr):
		a.Post(func() { a.toastError(optErr.Error()) })
	}
	a.player = p
	p.OnLog = a.appendLog
	p.OnFileLoaded = a.fileLoaded
	p.OnEndFile = a.fileEnded

	rend, err := p.Handle().NewRenderContext(platform.GetProcAddress)
	if err != nil {
		p.Destroy()
		return fmt.Errorf("mpv render: %w", err)
	}
	rend.SetUpdateCallback(func() {
		a.newFrame.Store(true)
		platform.Wake()
	})
	a.rend = rend
	return nil
}

func asOptionError(err error, target **player.OptionError) bool {
	oe, ok := err.(*player.OptionError)
	if ok {
		*target = oe
	}
	return ok
}

func (a *App) shutdownPlayer() {
	if a.rend != nil {
		a.rend.Free()
		a.rend = nil
	}
	if a.player != nil {
		a.player.Destroy()
	}
}

// Post runs fn on the UI goroutine before the next frame. It is safe
// from any goroutine and doesn't wait.
func (a *App) Post(fn func()) {
	a.postMu.Lock()
	a.posted = append(a.posted, fn)
	a.postMu.Unlock()
	platform.Wake()
}

func (a *App) runPosted() bool {
	a.postMu.Lock()
	fns := a.posted
	a.posted = nil
	a.postMu.Unlock()
	for _, fn := range fns {
		fn()
	}
	return len(fns) > 0
}

// installHandlers connects the window's events.
func (a *App) installHandlers() {
	w := a.win
	w.OnKey = a.handleKey
	w.OnDrop = func(paths []string) { a.Post(func() { a.dropped(paths) }) }
	w.OnClose = func() bool {
		a.quitting = true
		return true
	}
	w.OnFocus = func(bool) { a.dirty = true }
	w.OnRefresh = func() { a.render() }
	w.CanDrag = a.canDragAt
}

// Run shows the window and runs until it closes.
func (a *App) Run() error {
	a.win.Show()
	defer a.shutdown()
	for !a.win.ShouldClose() {
		a.win.Wait(a.nextWakeup())
		a.step()
	}
	return nil
}

// step handles everything that happened since the last one, and draws a
// frame if anything changed.
func (a *App) step() {
	changed := a.runPosted()
	for _, f := range platform.TakeOpenedFiles() {
		a.openItems([]string{f}, false)
		changed = true
	}
	if a.mpvEvents.Swap(false) && a.player.Process() {
		changed = true
		a.afterPlayerChange()
	}
	if a.invalidate.Swap(false) {
		changed = true
	}
	frame := false
	if a.newFrame.Swap(false) && a.rend != nil {
		frame = a.rend.Update()
	}
	if a.win.TakeDirty() || a.dirty || a.timersDue() {
		changed = true
	}
	a.dirty = false
	if changed || frame {
		a.render()
	}
}

// nextWakeup is when the loop must run even without input: an
// animation's next frame, the controls hiding, the OSD fading.
func (a *App) nextWakeup() time.Time {
	var next time.Time
	consider := func(t time.Time) {
		if !t.IsZero() && (next.IsZero() || t.Before(next)) {
			next = t
		}
	}
	if t, ok := a.win.Wakeup(); ok {
		consider(t)
	}
	consider(a.ui.nextTimer())
	if !next.IsZero() && time.Until(next) < 0 {
		return time.Now()
	}
	return next
}

func (a *App) timersDue() bool {
	t := a.ui.nextTimer()
	return !t.IsZero() && !time.Now().Before(t)
}

// render draws the video frame, then the UI over it, and presents.
func (a *App) render() {
	fb := a.win.FramebufferSize()
	if fb.X <= 0 || fb.Y <= 0 || a.win.Minimized() {
		return
	}
	scale := a.win.Scale()
	a.ui.beginFrame(fb, scale)
	a.updateVideoMargins(fb)

	st := &a.player.State
	if a.rend != nil && !st.Idle && !a.casting() {
		if err := a.rend.Render(0, fb.X, fb.Y, true); err != nil {
			a.appendLog("error", "render", err.Error())
		}
	} else {
		a.gpu.Clear(color.NRGBA{R: 0x09, G: 0x09, B: 0x0b, A: 0xff})
	}

	a.ops.Reset()
	gtx := layout.Context{
		Ops:         &a.ops,
		Now:         time.Now(),
		Source:      a.win.Router.Source(),
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(fb),
	}
	elements.BeginFrame()
	elements.SetWindowSize(fb)
	elements.SetMetric(gtx.Metric)
	a.ui.layout(gtx)
	elements.DrainPortals(gtx)
	elements.DispatchBack(gtx)
	if elements.RunAfterLayout() {
		a.dirty = true
	}
	if err := a.gpu.Frame(gtx.Ops, gpu.OpenGLRenderTarget{}, fb); err != nil {
		a.appendLog("error", "gpu", err.Error())
	}
	a.win.Router.Frame(gtx.Ops)
	elements.EndFrame()
	a.win.Swap()
	if a.rend != nil {
		a.rend.ReportSwap()
	}
	a.win.AfterFrame()
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

// handleKey runs the shortcut for a key press. It reports whether the
// key was used; unused keys go to the UI (and from there, if nothing
// takes them, to mpv's own bindings).
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
		a.dirty = true
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
	case a.win.Fullscreen():
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
	platform.KeepAwake(playing && st.HasVideo())
	title := "goplay"
	if !st.Idle && st.Title != "" {
		title = st.Title + " — goplay"
	}
	a.win.SetTitle(title)
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

// shutdown saves state and releases everything, in the order mpv
// requires (render context before the core).
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
	platform.KeepAwake(false)
	a.player.SaveResumePosition()
	a.shutdownPlayer()
	a.gpu.Release()
	a.win.Destroy()
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

// SetIcon sets the window icon.
func (a *App) SetIcon(img image.Image) { a.win.SetIcon(img) }

// Quit closes the window, which ends Run.
func (a *App) Quit() { a.win.Close() }
