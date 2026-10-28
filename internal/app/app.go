package app

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/ncruces/zenity"

	"gioui.org/f32"
	"gioui.org/gpu"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/mehran9675/giode/elements"

	"github.com/mehran9675/goplay-ebiten/internal/glfwutil"
	"github.com/mehran9675/goplay-ebiten/internal/libmpv"
	"github.com/mehran9675/goplay-ebiten/internal/settings"
	"github.com/mehran9675/goplay-ebiten/internal/ui"
)

var mediaExts = map[string]bool{
	".mp4": true, ".mkv": true, ".mov": true, ".avi": true, ".webm": true,
	".m4v": true, ".ts": true, ".m2ts": true, ".mts": true, ".flv": true,
	".wmv": true, ".mpg": true, ".mpeg": true, ".ogv": true,
	".mp3": true, ".flac": true, ".wav": true, ".ogg": true, ".oga": true,
	".m4a": true, ".aac": true, ".opus": true, ".mka": true, ".wma": true,
}

var dialogPatterns = []string{
	"*.mp4", "*.mkv", "*.mov", "*.avi", "*.webm", "*.m4v", "*.ts", "*.m2ts",
	"*.mts", "*.flv", "*.wmv", "*.mpg", "*.mpeg", "*.ogv", "*.mp3", "*.flac",
	"*.wav", "*.ogg", "*.oga", "*.m4a", "*.aac", "*.opus", "*.mka", "*.wma",
}

type dialogResult struct {
	files []string
	err   error
	// subtitle marks files[0] as a subtitle to add to the current
	// file rather than media to play.
	subtitle bool
}

// App owns the libmpv instance, the playlist, and the GLFW window.
type App struct {
	mpv  *libmpv.Handle
	rend *libmpv.RenderContext

	playlist []string
	idx      int

	pos, duration float64
	paused        bool
	finished      bool
	hasMedia      bool
	hasVideo      bool
	videoProbed   bool
	volume        float64

	msg string

	aspect string // forced aspect ratio, "" for the file's own
	title  string // playing file's name, "" for none

	settings  *settings.Store
	lastFlush time.Time

	// videoMargins are the shares of the window height, at the top and
	// bottom, that mpv keeps the video out of (the title bar's and
	// control bar's strips outside fullscreen). -1 until first set.
	videoMargins [2]float64

	// loading is set from loadfile until mpv reports the file loaded;
	// eof-reached still describes the previous file meanwhile.
	loading bool

	dialogBusy bool
	dialogs    chan dialogResult

	window *glfw.Window
	gpuCtx gpu.GPU
	router input.Router
	ops    op.Ops
	ui     *ui.UI

	// keys holds key presses queued by the GLFW key callback since the last
	// frame. It is drained before each UI layout.
	keys []ui.Key

	lastX, lastY float32

	restoreX, restoreY int
	restoreW, restoreH int

	dragging                 bool
	dragCursorX, dragCursorY float64
	dragWinX, dragWinY       int

	beginning time.Time
}

// New creates the mpv core and configures it. initial may be empty.
func New(initial []string) (*App, error) {
	mpv, err := libmpv.Create()
	if err != nil {
		return nil, err
	}
	a := &App{
		mpv:          mpv,
		volume:       1,
		videoMargins: [2]float64{-1, -1},
		playlist:     initial,
		dialogs:      make(chan dialogResult, 1),
		ui:           ui.New(),
	}
	a.loadSettings()
	if err := a.configure(); err != nil {
		mpv.TerminateDestroy()
		return nil, err
	}
	return a, nil
}

func (a *App) configure() error {
	opts := [][2]string{
		{"vo", "libmpv"},
		{"osd-level", "0"},
		{"osc", "no"},
		{"video-timing-offset", "0"},
		// Stay on the last frame at the end instead of unloading the
		// file: eof-reached, which drives auto-advance and the replay
		// on Play, is only set while the file stays open.
		{"keep-open", "yes"},
		// Bound the seek-back cache so a large file (e.g. 4K) doesn't
		// balloon memory; the read-ahead size is a setting.
		{"demuxer-max-back-bytes", "32MiB"},
	}
	// mpv.conf and resume data live in goplay's config folder.
	if dir, err := settings.ConfigDir(); err == nil {
		opts = append(opts, [2]string{"config-dir", dir}, [2]string{"watch-later-dir", filepath.Join(dir, "watch_later")})
	}
	for _, o := range opts {
		if err := a.mpv.SetOption(o[0], o[1]); err != nil {
			return fmt.Errorf("setting option %s: %w", o[0], err)
		}
	}
	// A bad saved value must not stop the player from starting: report
	// it and carry on with mpv's default.
	for _, o := range a.settings.MPVOptions() {
		if err := a.mpv.SetOption(o.Name, o.Value); err != nil {
			fmt.Fprintf(os.Stderr, "goplay: setting %s=%s: %v\n", o.Name, o.Value, err)
		}
	}
	if err := a.mpv.Initialize(); err != nil {
		return err
	}
	if err := a.mpv.RequestLog("v"); err != nil {
		return err
	}
	return a.mpv.SetDouble("volume", a.volume*100)
}

// Run creates the window and blocks in the render loop until it closes.
func (a *App) Run() error {
	// GLFW and OpenGL callbacks must all run on the thread that created the
	// window.
	runtime.LockOSThread()
	if err := glfw.Init(); err != nil {
		return err
	}
	defer glfw.Terminate()
	defer a.cleanup()

	// OpenGL ES 3.0 via ANGLE (EGL), which is what Gio's Windows backend
	// requires: gioui.org loads libGLESv2.dll unconditionally on Windows, so
	// the window context must come from the same ANGLE stack. libmpv's
	// render API supports OpenGL ES 2.0+ and shares this context. The
	// framebuffer stays in the default (linear) colorspace: Gio emulates
	// sRGB with an internal FBO and blends the UI over the video.
	glfw.WindowHint(glfw.ContextCreationAPI, glfw.EGLContextAPI)
	glfw.WindowHint(glfw.ClientAPI, glfw.OpenGLESAPI)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 0)
	glfw.WindowHint(glfw.ScaleToMonitor, glfw.True)

	// Frameless window: the UI draws its own title bar. It is moved by
	// dragging the title bar or the empty video area.
	glfw.WindowHint(glfw.Decorated, glfw.False)

	win, err := glfw.CreateWindow(1280, 720, "goplay", nil, nil)
	if err != nil {
		return err
	}
	a.window = win
	if a.settings.Bool(settings.KeyAlwaysOnTop) {
		win.SetAttrib(glfw.Floating, glfw.True)
	}
	win.MakeContextCurrent()

	// OpenGL ES 3.0 has vertex array objects as a core feature and does not
	// need the default VAO that desktop core profiles require. Gio's GLES
	// backend manages the rest of the GL state.
	glfw.SwapInterval(1)
	a.beginning = time.Now()
	a.registerCallbacks(win)

	// The libmpv render context must be created with the GL context current.
	if a.mpv != nil {
		if rend, err := a.mpv.NewRenderContext(); err != nil {
			a.msg = err.Error()
			fmt.Fprintf(os.Stderr, "goplay: render context: %v\n", err)
		} else {
			a.rend = rend
		}
	}

	// Load a file given on the command line only now: loaded before the
	// render context exists, mpv finds no video output, drops the video
	// track and ends playback.
	if a.idx < len(a.playlist) {
		a.load(a.playlist[a.idx])
	}

	gpuCtx, err := gpu.New(gpu.OpenGL{Shared: true, ES: true})
	if err != nil {
		return err
	}
	a.gpuCtx = gpuCtx

	for !win.ShouldClose() {
		glfw.PollEvents()
		a.drainDialogs()
		a.update()

		for _, k := range a.keys {
			a.ui.KeyPress(a, k)
		}
		a.keys = a.keys[:0]

		scale, _ := win.GetContentScale()
		if fbw, fbh := win.GetFramebufferSize(); fbw > 0 && fbh > 0 {
			a.renderFrame(scale, fbw, fbh)
		}
		a.handleDrag()
		a.autoAdvance()
		a.flushSettings(false)
	}
	return nil
}

// autoAdvance moves to the next file when the current one ends, if the
// settings ask for it, wrapping around when the folder repeats.
func (a *App) autoAdvance() {
	if !a.finished || !a.settings.Bool(settings.KeyAutoplayNext) {
		return
	}
	switch {
	case a.idx+1 < len(a.playlist):
		a.next()
	case len(a.playlist) > 1 && a.settings.Bool(settings.KeyRepeatFolder):
		a.idx = 0
		a.load(a.playlist[0])
	}
}

// toPx converts a GLFW cursor position (window screen coordinates) to the
// framebuffer pixel space that gtx.Constraints and all Ops are laid out
// in (see renderFrame: Constraints is Exact(fbw, fbh)). Gio's pointer
// events are in that space too. The ratio comes from the framebuffer
// and window sizes, not the content scale: on macOS screen coordinates
// are points (ratio 2 on Retina), while on Windows they are already
// pixels (ratio 1) whatever the content scale.
func (a *App) toPx(p f32.Point) f32.Point {
	r := a.pxPerScreen()
	return f32.Pt(p.X*r, p.Y*r)
}

// pxPerScreen is how many framebuffer pixels one screen coordinate
// spans.
func (a *App) pxPerScreen() float32 {
	if a.window == nil {
		return 1
	}
	ww, _ := a.window.GetSize()
	fbw, _ := a.window.GetFramebufferSize()
	if ww <= 0 || fbw <= 0 {
		return 1
	}
	return float32(fbw) / float32(ww)
}

// chromeHeight is the combined height of the title bar and control bar,
// in window screen coordinates.
func (a *App) chromeHeight() int {
	if a.window == nil {
		return 0
	}
	scale, _ := a.window.GetContentScale()
	return int(float32(ui.TitleBarHeight+ui.ControlHeight)*scale/a.pxPerScreen() + 0.5)
}

// updateVideoMargins keeps mpv's video out of the title bar's and control
// bar's strips outside fullscreen, so they sit above and below the
// picture instead of over it. In fullscreen the bars overlay the video
// and the margins are zero.
func (a *App) updateVideoMargins(scale float32, fbh int) {
	if a.mpv == nil || fbh <= 0 {
		return
	}
	var m [2]float64
	if !a.ui.Fullscreen {
		m[0] = float64(float32(ui.TitleBarHeight)*scale) / float64(fbh)
		m[1] = float64(float32(ui.ControlHeight)*scale) / float64(fbh)
		if m[0]+m[1] > 0.9 {
			m = [2]float64{}
		}
	}
	for i, prop := range []string{"video-margin-ratio-top", "video-margin-ratio-bottom"} {
		if m[i] == a.videoMargins[i] {
			continue
		}
		if err := a.mpv.SetDouble(prop, m[i]); err == nil {
			a.videoMargins[i] = m[i]
		}
	}
}

func (a *App) cleanup() {
	a.saveResumePosition()
	a.flushSettings(true)
	if a.rend != nil {
		a.rend.Free()
		a.rend = nil
	}
	if a.gpuCtx != nil {
		a.gpuCtx.Release()
		a.gpuCtx = nil
	}
	if a.mpv != nil {
		a.mpv.TerminateDestroy()
		a.mpv = nil
	}
}

// registerCallbacks wires GLFW input into the Gio input router. Keyboard
// shortcuts are queued as ui.Key values and handled by the UI; the right
// click opens the context menu.
func (a *App) registerCallbacks(win *glfw.Window) {
	var btns pointer.Buttons
	lastPos := f32.Point{}

	toPx := func(p f32.Point) f32.Point { return a.toPx(p) }

	win.SetCursorPosCallback(func(w *glfw.Window, xpos, ypos float64) {
		p := f32.Pt(float32(xpos), float32(ypos))
		lastPos = p
		a.lastX, a.lastY = p.X, p.Y
		a.router.Queue(pointer.Event{
			Kind:     pointer.Move,
			Source:   pointer.Mouse,
			Time:     time.Since(a.beginning),
			Position: toPx(p),
			Buttons:  btns,
		})
	})

	win.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		var btn pointer.Buttons
		switch button {
		case glfw.MouseButton1:
			btn = pointer.ButtonPrimary
		case glfw.MouseButton2:
			btn = pointer.ButtonSecondary
		case glfw.MouseButton3:
			btn = pointer.ButtonTertiary
		}
		var typ pointer.Kind
		switch action {
		case glfw.Release:
			typ = pointer.Release
			btns &^= btn
		case glfw.Press:
			typ = pointer.Press
			btns |= btn
		}
		a.router.Queue(pointer.Event{
			Kind:     typ,
			Source:   pointer.Mouse,
			Time:     time.Since(a.beginning),
			Position: toPx(lastPos),
			Buttons:  btns,
		})
		if button == glfw.MouseButton2 && action == glfw.Press {
			p := toPx(lastPos)
			a.ui.OpenMenuAt(p.X, p.Y)
		}
	})

	win.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action != glfw.Press {
			return
		}
		if k, ok := uiKey(key); ok {
			a.keys = append(a.keys, k)
		}
	})

	win.SetDropCallback(func(w *glfw.Window, paths []string) {
		if len(paths) > 0 {
			a.loadPlaylist(paths)
		}
	})
}

// uiKey maps a GLFW key to the app shortcut set. Returns false for keys that
// do not map to a shortcut.
func uiKey(key glfw.Key) (ui.Key, bool) {
	switch key {
	case glfw.KeyEscape:
		return ui.KeyEscape, true
	case glfw.KeyF:
		return ui.KeyF, true
	case glfw.KeyO:
		return ui.KeyO, true
	case glfw.KeyD:
		return ui.KeyD, true
	case glfw.KeySpace:
		return ui.KeySpace, true
	case glfw.KeyS:
		return ui.KeyS, true
	case glfw.KeyN:
		return ui.KeyN, true
	case glfw.KeyLeft:
		return ui.KeyLeft, true
	case glfw.KeyRight:
		return ui.KeyRight, true
	case glfw.KeyUp:
		return ui.KeyUp, true
	case glfw.KeyDown:
		return ui.KeyDown, true
	}
	return 0, false
}

// renderFrame draws the current video frame, lays out the Gio UI on top,
// and presents. When media is playing the surface is left un-cleared on
// purpose: libmpv fills the whole target with the video frame (or black
// letterbox bars), and Gio's GLES backend blends the UI over it (gpu.Clear
// would wipe the video). Without media, nothing else paints the background,
// so Gio must clear it itself each frame - otherwise every frame's UI (e.g.
// each new context-menu position) just accumulates on top of the last one.
func (a *App) renderFrame(scale float32, fbw, fbh int) {
	a.updateVideoMargins(scale, fbh)
	if a.rend != nil && a.hasMedia {
		if err := a.rend.Render(fbw, fbh); err != nil {
			fmt.Fprintf(os.Stderr, "goplay: render: %v\n", err)
		}
	} else {
		a.gpuCtx.Clear(color.NRGBA{A: 0xff, R: 0x0b, G: 0x0e, B: 0x14})
	}

	sz := image.Pt(fbw, fbh)
	a.ops.Reset()
	gtx := layout.Context{
		Ops:         &a.ops,
		Now:         time.Now(),
		Source:      a.router.Source(),
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(sz),
	}
	a.ui.Layout(gtx, a)
	elements.DrainPortals(gtx)
	_ = a.gpuCtx.Frame(gtx.Ops, gpu.OpenGLRenderTarget{}, sz)
	a.router.Frame(gtx.Ops)
	a.window.SwapBuffers()
}

// handleDrag implements frameless drag-to-move: holding the left mouse
// button on the title bar's empty space, or on empty video (not over the
// control bar, menu or a dialog), moves the window.
func (a *App) handleDrag() {
	if a.ui.Fullscreen {
		a.dragging = false
		return
	}
	if a.dragging {
		if !glfwutil.MouseButtonDown(glfwutil.MouseButtonLeft) {
			a.dragging = false
			return
		}
		cx, cy := glfwutil.CursorPos()
		a.window.SetPos(a.dragWinX+int(cx-a.dragCursorX), a.dragWinY+int(cy-a.dragCursorY))
		return
	}
	if glfwutil.MouseButtonDown(glfwutil.MouseButtonLeft) {
		cx, cy := glfwutil.CursorPos()
		p := a.toPx(f32.Pt(float32(cx), float32(cy)))
		// The title bar marks its empty space as a move area and its
		// buttons as not draggable (Gio system actions); elsewhere,
		// anything that isn't UI is video.
		action, ok := a.router.ActionAt(p)
		if (ok && action == system.ActionMove) || (!ok && !a.ui.RectContains(p.X, p.Y)) {
			a.dragCursorX, a.dragCursorY = cx, cy
			a.dragWinX, a.dragWinY = a.window.GetPos()
			a.dragging = true
		}
	}
}

// drainDialogs consumes the result of a completed file dialog, if any.
func (a *App) drainDialogs() {
	select {
	case r := <-a.dialogs:
		a.dialogBusy = false
		switch {
		case r.err == zenity.ErrCanceled:
		case r.err != nil:
			a.msg = r.err.Error()
		case r.subtitle && len(r.files) > 0:
			a.addSubtitle(r.files[0])
		case len(r.files) > 0:
			a.loadPlaylist(r.files)
		}
	default:
	}
}

func (a *App) update() {
	if a.mpv == nil {
		return
	}
	for {
		name, text := a.mpv.WaitEvent(0)
		if name == "none" && text == "" {
			break
		}
		if text != "" {
			fmt.Fprint(os.Stderr, text)
		}
		if name == "file-loaded" {
			a.loading = false
		}
	}
	a.pos, _ = a.mpv.GetDouble("time-pos")
	a.duration, _ = a.mpv.GetDouble("duration")
	a.paused, _ = a.mpv.GetBool("pause")
	eof, _ := a.mpv.GetBool("eof-reached")
	a.finished = eof && !a.loading

	if a.hasMedia && !a.videoProbed {
		if w, ok := a.mpv.GetDouble("width"); ok && w > 0 {
			if h, ok2 := a.mpv.GetDouble("height"); ok2 && h > 0 {
				a.hasVideo = true
				a.videoProbed = true
				if a.settings.Bool(settings.KeyResizeToVideo) && !a.ui.Fullscreen {
					a.resizeToVideo(int(w), int(h))
				}
			}
		}
	}
}

func (a *App) load(path string) {
	if a.mpv == nil {
		return
	}
	// mpv only saves the position by itself on quit; save the file being
	// replaced too, so switching files also resumes.
	a.saveResumePosition()
	if err := a.mpv.Command("loadfile", path, "replace"); err != nil {
		a.msg = err.Error()
		return
	}
	a.msg = ""
	a.loading = true
	a.title = filepath.Base(path)
	a.hasMedia = true
	a.hasVideo = false
	a.videoProbed = false
	a.finished = false
	a.pos = 0
	a.duration = 0
	if a.window != nil {
		a.window.SetTitle(filepath.Base(path) + " - goplay")
	}
}

func (a *App) loadPlaylist(files []string) {
	if len(files) == 0 {
		return
	}
	a.playlist = files
	a.idx = 0
	a.load(files[0])
}

// resizeToVideo sizes the window to the video, plus the title bar's and
// control bar's strips, shrinking it to fit the monitor if needed.
func (a *App) resizeToVideo(vw, vh int) {
	if vw <= 0 || vh <= 0 {
		return
	}
	bar := a.chromeHeight()
	_, _, mw, mh := glfwutil.WorkArea(a.window)
	mh -= bar
	if mw > 0 && mh > 0 {
		scale := min(1.0, float64(mw)*0.9/float64(vw), float64(mh)*0.9/float64(vh))
		vw = int(float64(vw) * scale)
		vh = int(float64(vh) * scale)
	}
	if vw < 320 {
		vw = 320
	}
	if vh < 200 {
		vh = 200
	}
	if a.window != nil {
		a.window.SetSize(vw, vh+bar)
	}
}

// Controller methods consumed by internal/ui.

func (a *App) Pos() float64      { return a.pos }
func (a *App) Duration() float64 { return a.duration }
func (a *App) Volume() float64   { return a.volume }
func (a *App) Paused() bool      { return a.paused }
func (a *App) Finished() bool    { return a.finished }
func (a *App) HasMedia() bool    { return a.hasMedia }
func (a *App) Message() string   { return a.msg }

func (a *App) TogglePause() {
	if a.mpv == nil || !a.hasMedia {
		return
	}
	if a.finished {
		a.mpv.Command("seek", "0", "absolute")
		a.mpv.Command("set", "pause", "no")
		a.finished = false
		return
	}
	a.mpv.Command("cycle", "pause")
}

func (a *App) Stop() {
	if a.mpv == nil || !a.hasMedia {
		return
	}
	a.mpv.Command("seek", "0", "absolute")
	a.mpv.Command("set", "pause", "yes")
}

func (a *App) next() {
	if a.idx+1 < len(a.playlist) {
		a.idx++
		a.load(a.playlist[a.idx])
	}
}

func (a *App) Next() { a.next() }

func (a *App) Seek(frac float64) {
	if a.mpv == nil || !a.hasMedia || a.duration <= 0 {
		return
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	a.mpv.Command("seek", fmt.Sprintf("%.3f", frac*a.duration), "absolute")
}

func (a *App) SeekRelative(seconds float64) {
	if a.mpv == nil || !a.hasMedia {
		return
	}
	a.mpv.Command("seek", fmt.Sprintf("%.3f", seconds), "relative")
}

func (a *App) SetVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	a.volume = v
	if a.mpv != nil {
		a.mpv.SetDouble("volume", v*100)
	}
}

func (a *App) Fullscreen() bool { return a.ui.Fullscreen }

func (a *App) ToggleFullscreen() {
	if a.window == nil {
		return
	}
	if a.ui.Fullscreen {
		glfwutil.SetFullscreen(a.window, false, a.restoreX, a.restoreY, a.restoreW, a.restoreH)
		a.ui.Fullscreen = false
	} else {
		a.restoreX, a.restoreY = a.window.GetPos()
		a.restoreW, a.restoreH = a.window.GetSize()
		glfwutil.SetFullscreen(a.window, true, 0, 0, 0, 0)
		a.ui.Fullscreen = true
	}
}

func (a *App) Quit() {
	if a.window != nil {
		a.window.SetShouldClose(true)
	}
}

func (a *App) OpenFile() {
	a.openDialog(func() dialogResult {
		f, err := zenity.SelectFile(
			zenity.Title("Open media file"),
			zenity.FileFilters{{Name: "Media files", Patterns: dialogPatterns, CaseFold: true}},
		)
		if err != nil {
			return dialogResult{err: err}
		}
		return dialogResult{files: []string{f}}
	})
}

func (a *App) OpenFolder() {
	a.openDialog(func() dialogResult {
		dir, err := zenity.SelectFile(zenity.Title("Open folder"), zenity.Directory())
		if err != nil {
			return dialogResult{err: err}
		}
		files, err := scanDir(dir)
		return dialogResult{files: files, err: err}
	})
}

func (a *App) openDialog(show func() dialogResult) {
	if a.dialogBusy {
		return
	}
	a.dialogBusy = true
	go func() { a.dialogs <- show() }()
}

func scanDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && mediaExts[strings.ToLower(filepath.Ext(e.Name()))] {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no media files in %s", dir)
	}
	sort.Strings(files)
	return files, nil
}
